package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/parser"
)

func (r *cliRenderer) buildTree(wf *parser.Workflow) error {
	dag, err := engine.BuildDAG(wf.Steps)
	if err != nil {
		return err
	}

	stepIndex := make(map[string]parser.Step, len(wf.Steps))

	for _, s := range wf.Steps {
		stepIndex[s.ID] = s
	}

	r.steps = make(map[string]*stepTracker, len(wf.Steps))
	r.order = nil

	visited := make(map[string]bool, len(wf.Steps))

	var dfs func(node *engine.DAGNode, depth int, parentID string, isLast bool)
	dfs = func(node *engine.DAGNode, depth int, parentID string, isLast bool) {
		if visited[node.Step.ID] {
			return
		}

		visited[node.Step.ID] = true

		r.addStepTracker(stepIndex[node.Step.ID], depth, parentID, isLast)

		for i, child := range node.Children {
			last := i == len(node.Children)-1
			dfs(child, depth+1, node.Step.ID, last)
		}
	}

	for i, root := range dag.Roots {
		last := i == len(dag.Roots)-1
		dfs(root, 0, "", last)
	}

	r.resolveConvergentNodes(dag)
	r.buildLoopDisplays()

	return nil
}

func (r *cliRenderer) addStepTracker(s parser.Step, depth int, parentID string, isLast bool) {
	title := s.Title
	if title == "" {
		title = s.ID
	}

	st := &stepTracker{
		id:       s.ID,
		title:    title,
		action:   s.Action,
		depth:    depth,
		isLast:   isLast,
		parentID: parentID,
	}

	if s.Goto != nil {
		st.gotoTarget = s.Goto.Target
		st.gotoMaxIter = s.Goto.MaxIterations
	}

	if s.Action == "loop" {
		st.pipelineActions = parsePipelineActions(s.Config)
	}

	r.steps[s.ID] = st
	r.order = append(r.order, s.ID)
}

func (r *cliRenderer) resolveConvergentNodes(dag *engine.DAG) {
	processed := make(map[string]bool)

	for {
		found := false

		for _, id := range r.order {
			if processed[id] {
				continue
			}

			node := dag.Nodes[id]
			if len(node.Parents) <= 1 {
				continue
			}

			parentIDs := make([]string, 0, len(node.Parents))
			sharedParent := ""
			allSiblings := true

			for i, p := range node.Parents {
				pid := p.Step.ID
				st := r.steps[pid]

				if st == nil {
					allSiblings = false
					break
				}

				if i == 0 {
					sharedParent = st.parentID
				} else if st.parentID != sharedParent {
					allSiblings = false
					break
				}

				parentIDs = append(parentIDs, pid)
			}

			if !allSiblings {
				processed[id] = true
				continue
			}

			orderIdx := make(map[string]int, len(r.order))
			for i, oid := range r.order {
				orderIdx[oid] = i
			}

			sort.Slice(parentIDs, func(a, b int) bool {
				return orderIdx[parentIDs[a]] < orderIdx[parentIDs[b]]
			})

			descSet := map[string]bool{id: true}

			var collect func(string)
			collect = func(nid string) {
				for _, c := range dag.Nodes[nid].Children {
					if !descSet[c.Step.ID] {
						descSet[c.Step.ID] = true
						collect(c.Step.ID)
					}
				}
			}

			collect(id)

			var subtree, remaining []string

			for _, oid := range r.order {
				if descSet[oid] {
					subtree = append(subtree, oid)
				} else {
					remaining = append(remaining, oid)
				}
			}

			lastParentID := parentIDs[len(parentIDs)-1]
			lastParentIdx := -1

			for i, oid := range remaining {
				if oid == lastParentID {
					lastParentIdx = i
					break
				}
			}

			if lastParentIdx == -1 {
				processed[id] = true
				continue
			}

			lastParentDepth := r.steps[lastParentID].depth
			insertIdx := lastParentIdx + 1

			for insertIdx < len(remaining) {
				if r.steps[remaining[insertIdx]].depth <= lastParentDepth {
					break
				}

				insertIdx++
			}

			for i := insertIdx - 1; i > lastParentIdx; i-- {
				if r.steps[remaining[i]].parentID == lastParentID {
					r.steps[remaining[i]].isLast = false
					break
				}
			}

			newOrder := make([]string, 0, len(r.order))
			newOrder = append(newOrder, remaining[:insertIdx]...)
			newOrder = append(newOrder, subtree...)
			newOrder = append(newOrder, remaining[insertIdx:]...)
			r.order = newOrder

			childSt := r.steps[id]
			childSt.parentID = lastParentID
			childSt.isLast = true

			for i, pid := range parentIDs {
				pst := r.steps[pid]

				switch {
				case i == 0:
					pst.mergeMarker = "┐"
				case i == len(parentIDs)-1:
					pst.mergeMarker = "┘"
				default:
					pst.mergeMarker = "┤"
				}
			}

			processed[id] = true
			found = true

			break
		}

		if !found {
			break
		}
	}
}

func (r *cliRenderer) buildLoopDisplays() {
	orderIdx := make(map[string]int, len(r.order))

	for i, id := range r.order {
		orderIdx[id] = i
	}

	for _, id := range r.order {
		st := r.steps[id]
		if st.gotoTarget == "" {
			continue
		}

		ti, ok := orderIdx[st.gotoTarget]
		if !ok {
			continue
		}

		gi := orderIdx[id]
		r.loopDisplays = append(r.loopDisplays, loopDisplay{startIdx: ti, endIdx: gi})
	}

	r.hasLoops = len(r.loopDisplays) > 0
}

func (r *cliRenderer) treePrefix(stepID string) string {
	st := r.steps[stepID]
	if st.depth == 0 {
		return ""
	}

	var own string

	if st.isLast {
		own = "└── "
	} else {
		own = "├── "
	}

	var parts []string

	cur := st.parentID

	for d := st.depth - 1; d > 0; d-- {
		parent := r.steps[cur]

		if parent.isLast {
			parts = append(parts, "    ")
		} else {
			parts = append(parts, "│   ")
		}

		cur = parent.parentID
	}

	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}

	return strings.Join(parts, "") + own
}

func (r *cliRenderer) treeContinuation(stepID string) string {
	st := r.steps[stepID]

	var own string
	if st.isLast {
		own = "    "
	} else {
		own = "│   "
	}

	if st.depth == 0 {
		return own
	}

	var parts []string

	cur := st.parentID

	for d := st.depth - 1; d > 0; d-- {
		parent := r.steps[cur]

		if parent.isLast {
			parts = append(parts, "    ")
		} else {
			parts = append(parts, "│   ")
		}

		cur = parent.parentID
	}

	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}

	return strings.Join(parts, "") + own
}

func (r *cliRenderer) bracketChar(displayIdx int, isAnnotation bool) string {
	if !r.hasLoops {
		return ""
	}

	for _, ld := range r.loopDisplays {
		if displayIdx == ld.startIdx {
			return "╭ "
		}

		if displayIdx == ld.endIdx {
			if isAnnotation {
				return "╰ "
			}

			return "│ "
		}

		if displayIdx > ld.startIdx && displayIdx < ld.endIdx {
			return "│ "
		}
	}

	return "  "
}

func (r *cliRenderer) printTree() {
	fmt.Println()
	fmt.Printf("  %s\n", r.c("1", "TailFlow ─ "+r.wfName))
	fmt.Println()

	var bracketWidth int
	if r.hasLoops {
		bracketWidth = 2
	}

	for idx, id := range r.order {
		r.printTreeStep(idx, id, bracketWidth)
	}

	fmt.Println()
}

func (r *cliRenderer) printTreeStep(idx int, id string, bracketWidth int) {
	st := r.steps[id]
	bracket := r.bracketChar(idx, false)
	prefix := r.treePrefix(id)

	label := fmt.Sprintf("%s [%s]", st.title, st.action)
	idPart := st.id + " "

	const totalWidth = 45

	usedWidth := bracketWidth + len(prefix) + len(idPart) + len(label)
	dots := max(totalWidth-usedWidth, 2)
	dotStr := strings.Repeat("·", dots) + " "

	var mergeSuffix string
	if st.mergeMarker != "" {
		mergeSuffix = " ──" + st.mergeMarker
	}

	fmt.Printf("  %s%s%s%s%s%s\n",
		r.c("33", bracket),
		r.c("90", prefix),
		r.c("1", idPart),
		r.c("90", dotStr),
		r.c("90", label),
		r.c("36", mergeSuffix),
	)

	if len(st.pipelineActions) > 0 {
		r.printTreePipelineActions(idx, id, st.pipelineActions)
	}

	if st.gotoTarget != "" {
		r.printTreeGoto(idx, st)
	}
}

func (r *cliRenderer) printTreePipelineActions(idx int, id string, actions []pipelineAction) {
	contPrefix := r.treeContinuation(id)
	midBracket := r.bracketChar(idx, false)

	for i, pa := range actions {
		var connector string
		if i == len(actions)-1 {
			connector = "└─ "
		} else {
			connector = "├─ "
		}

		paLabel := pa.action
		if pa.title != "" {
			paLabel = pa.title + " [" + pa.action + "]"
		}

		fmt.Printf("  %s%s%s%s\n",
			r.c("33", midBracket),
			r.c("90", contPrefix+connector),
			r.c("33", fmt.Sprintf("%d. ", i+1)),
			r.c("90", paLabel),
		)
	}
}

func (r *cliRenderer) printTreeGoto(idx int, st *stepTracker) {
	closeBracket := r.bracketChar(idx, true)

	gotoLabel := "↻ goto " + st.gotoTarget
	if st.gotoMaxIter > 0 {
		gotoLabel += fmt.Sprintf(" (max %d)", st.gotoMaxIter)
	}

	fmt.Printf("  %s%s\n",
		r.c("33", closeBracket),
		r.c("33", gotoLabel),
	)
}

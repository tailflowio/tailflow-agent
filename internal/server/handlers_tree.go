package server

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/pkg/workflow"
)

func buildTreeLines(wf *parser.Workflow, dag *engine.DAG) []workflow.TreeLine {
	r := &treeBuilder{
		steps: make(map[string]*treeStep, len(wf.Steps)),
	}

	stepIndex := make(map[string]parser.Step, len(wf.Steps))
	for _, s := range wf.Steps {
		stepIndex[s.ID] = s
	}

	visited := make(map[string]bool)
	var dfs func(node *engine.DAGNode, depth int, parentID string, isLast bool)
	dfs = func(node *engine.DAGNode, depth int, parentID string, isLast bool) {
		if visited[node.Step.ID] {
			return
		}

		visited[node.Step.ID] = true

		s := stepIndex[node.Step.ID]

		title := s.Title
		if title == "" {
			title = s.ID
		}

		ts := &treeStep{
			id: s.ID, title: title, action: s.Action,
			depth: depth, parentID: parentID, isLast: isLast,
			when: s.When,
		}
		if s.Goto != nil {
			ts.gotoTarget = s.Goto.Target
			ts.gotoMax = s.Goto.MaxIterations
		}

		if s.Action == "loop" {
			ts.pipeline = extractLoopPipeline(s.Config)
		}

		r.steps[s.ID] = ts
		r.order = append(r.order, s.ID)

		for i, child := range node.Children {
			dfs(child, depth+1, node.Step.ID, i == len(node.Children)-1)
		}
	}

	for i, root := range dag.Roots {
		dfs(root, 0, "", i == len(dag.Roots)-1)
	}

	r.resolveConvergent(dag)
	r.buildLoops()
	r.markLoopBodies()

	return r.render()
}

type treeStep struct {
	id, title, action, parentID string
	depth                       int
	isLast                      bool
	gotoTarget                  string
	gotoMax                     int
	pipeline                    []workflow.PipelineAction
	mergeMarker                 string
	when                        string
	inLoop                      bool
}

type loopDisplay struct {
	startIdx, endIdx int
}

type treeBuilder struct {
	steps map[string]*treeStep
	order []string
	loops []loopDisplay
}

func (r *treeBuilder) resolveConvergent(dag *engine.DAG) {
	processed := make(map[string]bool)

	for changed := true; changed; {
		changed = false

		for _, id := range r.order {
			if processed[id] {
				continue
			}

			node := dag.Nodes[id]
			if node == nil || len(node.Parents) <= 1 {
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
				dn := dag.Nodes[nid]
				if dn == nil {
					return
				}

				for _, c := range dn.Children {
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
			for insertIdx < len(remaining) && r.steps[remaining[insertIdx]].depth > lastParentDepth {
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
			changed = true

			break
		}
	}
}

func (r *treeBuilder) buildLoops() {
	r.loops = nil
	for i, id := range r.order {
		st := r.steps[id]
		if st.gotoTarget == "" {
			continue
		}

		startIdx := -1

		for j, oid := range r.order {
			if oid == st.gotoTarget {
				startIdx = j
				break
			}
		}

		if startIdx >= 0 {
			r.loops = append(r.loops, loopDisplay{startIdx: startIdx, endIdx: i})
		}
	}
}

func (r *treeBuilder) markLoopBodies() {
	for _, ld := range r.loops {
		for i := ld.startIdx; i <= ld.endIdx; i++ {
			r.steps[r.order[i]].inLoop = true
		}
	}
}

func (r *treeBuilder) bracketChar(idx int, isAnnotation bool) string {
	if len(r.loops) == 0 {
		return ""
	}

	for _, ld := range r.loops {
		if idx == ld.startIdx {
			return "╭ "
		}

		if idx == ld.endIdx {
			if isAnnotation {
				return "╰ "
			}

			return "│ "
		}

		if idx > ld.startIdx && idx < ld.endIdx {
			return "│ "
		}
	}

	return "  "
}

func (r *treeBuilder) treePrefix(id string) string {
	st := r.steps[id]
	if st.depth == 0 {
		return ""
	}

	own := "├── "
	if st.isLast {
		own = "└── "
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

func (r *treeBuilder) render() []workflow.TreeLine {
	var lines []workflow.TreeLine

	for idx, id := range r.order {
		st := r.steps[id]
		bracket := r.bracketChar(idx, false)
		prefix := r.treePrefix(id)

		merge := ""
		if st.mergeMarker != "" {
			merge = " ──" + st.mergeMarker
		}

		lines = append(lines, workflow.TreeLine{
			StepID:     st.id,
			Prefix:     bracket + prefix,
			Name:       st.id,
			Label:      st.title,
			Action:     st.action,
			Merge:      merge,
			Type:       "step",
			Depth:      st.depth,
			InLoop:     st.inLoop,
			When:       st.when,
			GotoTarget: st.gotoTarget,
			GotoMax:    st.gotoMax,
		})

		for i, pa := range st.pipeline {
			connector := "├─ "
			if i == len(st.pipeline)-1 {
				connector = "└─ "
			}

			cont := r.treeContinuation(id)
			paBracket := r.bracketChar(idx, false)

			paLabel := pa.Action
			if pa.Title != "" {
				paLabel = pa.Title
			}

			lines = append(lines, workflow.TreeLine{
				StepID: st.id,
				Prefix: paBracket + cont + connector,
				Name:   fmt.Sprintf("%d. %s", i+1, pa.Action),
				Label:  paLabel,
				Type:   "pipeline",
			})
		}

		if st.gotoTarget != "" {
			closeBracket := r.bracketChar(idx, true)

			gotoLabel := "↻ goto " + st.gotoTarget
			if st.gotoMax > 0 {
				gotoLabel += fmt.Sprintf(" (max %d)", st.gotoMax)
			}

			lines = append(lines, workflow.TreeLine{
				Prefix: closeBracket,
				Name:   gotoLabel,
				Type:   "goto",
			})
		}
	}

	return lines
}

func (r *treeBuilder) treeContinuation(id string) string {
	st := r.steps[id]

	own := "│   "
	if st.isLast {
		own = "    "
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

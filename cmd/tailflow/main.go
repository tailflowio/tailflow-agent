package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/export/saas"
	agentfx "github.com/tailflow/tailflow/internal/fx"
	tfotel "github.com/tailflow/tailflow/internal/otel"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

var version = "dev"

type stepStatus int

const (
	statusPending stepStatus = iota
	statusRunning
	statusCompleted
	statusFailed
	statusSkipped
)

const (
	eventBusBuffer   = 1000
	logDisplayMaxLen = 40
	logTruncatedLen  = 37
)

type pipelineAction struct {
	action string
	title  string
}

type stepTracker struct {
	id              string
	title           string
	action          string
	depth           int
	isLast          bool
	parentID        string
	status          stepStatus
	start           time.Time
	duration        time.Duration
	errMsg          string
	lastLog         string // latest streaming output line (exec actions)
	pipelineActions []pipelineAction
	gotoTarget      string // target step ID for goto, empty if none
	gotoMaxIter     int    // max_iterations for goto
	mergeMarker     string // "┐", "┤", "┘" — merge connector for convergent nodes
}

type loopDisplay struct {
	startIdx int // index in display order of the target step
	endIdx   int // index in display order of the goto step
}

type cliRenderer struct {
	mu          sync.Mutex
	steps       map[string]*stepTracker
	order       []string // DFS display order
	noColor     bool
	isTTY       bool
	wfName      string
	activeSteps []string // step IDs currently running (display order)
	activeLines int      // number of active-area lines on screen

	loopDisplays []loopDisplay
	hasLoops     bool

	loopBody      map[string]bool // step IDs in the current loop body
	loopIteration int             // current iteration (0 = no loop active)
	loopMaxIter   int             // max_iterations from the goto event
	loopStepID    string          // step ID that carries the goto
	streamLog     string          // last stream log message (persists across steps)
}

func (r *cliRenderer) c(code, text string) string {
	if r.noColor {
		return text
	}

	return fmt.Sprintf("\033[%sm%s\033[0m", code, text)
}

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

func parsePipelineActions(config map[string]any) []pipelineAction {
	rawActions, ok := config["actions"]
	if !ok {
		return nil
	}

	arr, ok := rawActions.([]any)
	if !ok {
		return nil
	}

	actions := make([]pipelineAction, 0, len(arr))

	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		actName, _ := m["action"].(string)
		actTitle, _ := m["title"].(string)

		if actName != "" {
			actions = append(actions, pipelineAction{action: actName, title: actTitle})
		}
	}

	return actions
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

func (r *cliRenderer) eraseActive() {
	if !r.isTTY || r.activeLines == 0 {
		return
	}

	for i := 0; i < r.activeLines; i++ {
		fmt.Print("\033[A\033[2K")
	}

	r.activeLines = 0
}

func (r *cliRenderer) redrawActive() {
	if !r.isTTY {
		r.activeLines = 0

		return
	}

	lines := 0

	if r.loopIteration > 1 {
		r.printLoopIterationLine()

		lines++
	}

	activeHasStreamLog := false

	for _, id := range r.activeSteps {
		st := r.steps[id]
		if st != nil && st.lastLog == r.streamLog {
			activeHasStreamLog = true
			break
		}
	}

	if r.streamLog != "" && !activeHasStreamLog {
		display := r.streamLog
		if len(display) > 120 {
			display = display[:117] + "..."
		}

		fmt.Printf("  %s   %s\n", r.c("36", "│"), r.c("90", display))

		lines++
	}

	now := time.Now()

	for _, id := range r.activeSteps {
		r.printActiveStepLine(r.steps[id], now)

		lines++
	}

	r.activeLines = lines
}

func (r *cliRenderer) printLoopIterationLine() {
	var iterLabel string
	if r.loopMaxIter > 0 {
		iterLabel = fmt.Sprintf("iteration %d/%d", r.loopIteration, r.loopMaxIter)
	} else {
		iterLabel = fmt.Sprintf("iteration %d", r.loopIteration)
	}

	fmt.Printf("  %s  %s  %s\n",
		r.c("33", "↻"),
		r.c("1", iterLabel),
		r.c("90", r.stepTitle(r.loopStepID)),
	)
}

func (r *cliRenderer) printActiveStepLine(st *stepTracker, now time.Time) {
	elapsed := now.Sub(st.start).Truncate(100 * time.Millisecond)

	if st.lastLog == "" {
		fmt.Printf("  %s  %s %s\n", r.c("36", "⟳"), st.title, r.c("90", elapsed.String()))

		return
	}

	log := st.lastLog
	if len(log) > logDisplayMaxLen {
		log = log[:logTruncatedLen] + "..."
	}

	fmt.Printf("  %s  %s %s %s\n",
		r.c("36", "⟳"), st.title,
		r.c("90", log),
		r.c("90", "── "+elapsed.String()),
	)
}

func (r *cliRenderer) printLinef(format string, args ...any) {
	r.eraseActive()
	fmt.Printf(format, args...)
	r.redrawActive()
}

func (r *cliRenderer) tick() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.activeSteps) == 0 {
		return
	}

	r.eraseActive()
	r.redrawActive()
}

func (r *cliRenderer) addActive(stepID string) {
	r.activeSteps = append(r.activeSteps, stepID)
}

func (r *cliRenderer) removeActive(stepID string) {
	for i, id := range r.activeSteps {
		if id == stepID {
			r.activeSteps = append(r.activeSteps[:i], r.activeSteps[i+1:]...)
			return
		}
	}
}

func (r *cliRenderer) inLoop(stepID string) bool {
	return r.loopIteration > 1 && r.loopBody[stepID]
}

func (r *cliRenderer) endLoop() {
	r.printLinef("  %s  %s\n",
		r.c("32", "✓"),
		r.c("1", fmt.Sprintf("loop completed (%d iterations)", r.loopIteration)),
	)

	r.loopBody = nil
	r.loopIteration = 0
	r.loopMaxIter = 0
	r.streamLog = ""
	r.loopStepID = ""
}

func (r *cliRenderer) isStreamLog(ev event.Event) bool {
	if ev.Data == nil {
		return false
	}

	stream, ok := ev.Data["stream"]
	if !ok {
		return false
	}

	b, ok := stream.(bool)

	return ok && b
}

func (r *cliRenderer) isPipelineResult(msg string) bool {
	return strings.Contains(msg, "] step ") && (strings.Contains(msg, " OK (") || strings.Contains(msg, " FAILED ("))
}

func (r *cliRenderer) printPipelineLine(msg string) {
	if strings.Contains(msg, " FAILED (") {
		idx := strings.Index(msg, "): ")
		if idx != -1 {
			status := msg[:idx+1]
			errMsg := msg[idx+3:]

			r.printLinef("  %s   %s\n", r.c("90", "│"), r.c("31", status))
			r.printLinef("  %s     %s\n", r.c("90", "│"), r.c("31", errMsg))
		} else {
			r.printLinef("  %s   %s\n", r.c("90", "│"), r.c("31", msg))
		}
	} else {
		r.printLinef("  %s   %s\n", r.c("90", "│"), r.c("90", msg))
	}
}

func (r *cliRenderer) attemptSuffix(ev event.Event) string {
	if ev.Data == nil {
		return ""
	}

	attempt, ok := ev.Data["attempt"]
	if !ok {
		return ""
	}

	a, ok := attempt.(float64)
	if !ok || a <= 1 {
		return ""
	}

	return fmt.Sprintf(" (attempt %.0f)", a)
}

func (r *cliRenderer) handleEvent(ev event.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch ev.Type {
	case event.WorkflowStarted:
		r.printLinef("  %s %s\n", r.c("1;34", "▶"), r.c("1", "Execution started"))
	case event.StepStarted:
		r.handleStepStarted(ev)
	case event.StepCompleted:
		r.handleStepCompleted(ev)
	case event.StepFailed:
		r.handleStepFailed(ev)
	case event.StepSkipped:
		r.handleStepSkipped(ev)
	case event.StepLog:
		r.handleStepLog(ev)
	case event.StepGoto:
		r.handleStepGoto(ev)
	case event.StepWaiting:
		title := r.stepTitle(ev.StepID)
		r.printLinef("  %s  %s %s\n", r.c("33", "⏳"), title, ev.Message)
	case event.WorkflowCompleted, event.StepInput, event.StepOutput, event.Metrics, event.ExecutionState, event.ExecutionGroup:
		return
	}
}

func (r *cliRenderer) handleStepStarted(ev event.Event) {
	if r.loopIteration > 0 && !r.loopBody[ev.StepID] {
		r.endLoop()
	}

	st := r.steps[ev.StepID]
	if st != nil {
		st.status = statusRunning
		st.start = ev.Timestamp
	}

	r.addActive(ev.StepID)

	switch {
	case r.inLoop(ev.StepID), r.isTTY:
		r.eraseActive()
		r.redrawActive()
	default:
		title := r.stepTitle(ev.StepID)
		extra := r.attemptSuffix(ev)

		fmt.Printf("  %s  %s%s\n", r.c("36", "⟳"), title, extra)
	}
}

func (r *cliRenderer) handleStepCompleted(ev event.Event) {
	st := r.steps[ev.StepID]
	if st != nil {
		st.status = statusCompleted
		st.duration = ev.Timestamp.Sub(st.start)
	}

	r.removeActive(ev.StepID)

	if r.inLoop(ev.StepID) {
		r.eraseActive()
		r.redrawActive()

		return
	}

	r.streamLog = ""

	title := r.stepTitle(ev.StepID)
	dur := r.formatDuration(ev.StepID)

	r.printLinef("  %s  %s %s\n", r.c("32", "✓"), title, r.c("90", dur))
}

func (r *cliRenderer) handleStepFailed(ev event.Event) {
	st := r.steps[ev.StepID]
	if st != nil {
		st.status = statusFailed
		st.duration = ev.Timestamp.Sub(st.start)
		st.errMsg = ev.Message
	}

	r.removeActive(ev.StepID)

	if r.inLoop(ev.StepID) {
		r.eraseActive()
		r.redrawActive()

		return
	}

	r.streamLog = ""

	title := r.stepTitle(ev.StepID)
	dur := r.formatDuration(ev.StepID)

	r.printLinef("  %s  %s %s: %s\n", r.c("31", "✗"), title, r.c("90", dur), r.c("31", ev.Message))
}

func (r *cliRenderer) handleStepSkipped(ev event.Event) {
	st := r.steps[ev.StepID]
	if st != nil {
		st.status = statusSkipped
	}

	r.removeActive(ev.StepID)

	if r.inLoop(ev.StepID) {
		r.eraseActive()
		r.redrawActive()

		return
	}

	title := r.stepTitle(ev.StepID)

	r.printLinef("  %s  %s %s\n", r.c("33", "⏭"), title, r.c("90", "— skipped"))
}

func (r *cliRenderer) handleStepLog(ev event.Event) {
	if !r.isStreamLog(ev) {
		r.printLinef("  %s   %s\n", r.c("90", "│"), r.c("90", ev.Message))

		return
	}

	if r.isPipelineResult(ev.Message) {
		if !r.inLoop(ev.StepID) {
			r.printPipelineLine(ev.Message)
		}

		return
	}

	r.streamLog = ev.Message

	st := r.steps[ev.StepID]
	if st != nil {
		st.lastLog = ev.Message
	}

	if r.isTTY {
		r.eraseActive()
		r.redrawActive()
	}
}

func (r *cliRenderer) handleStepGoto(ev event.Event) {
	iteration := 2

	if ev.Data != nil {
		iter, ok := ev.Data["iteration"].(float64)
		if ok {
			iteration = int(iter)
		}
	}

	if iteration > 2 {
		r.loopIteration = iteration
		r.eraseActive()
		r.redrawActive()

		return
	}

	r.printLinef("  %s  %s\n", r.c("33", "↻"), ev.Message)
	r.initLoopBody(ev, iteration)
}

func (r *cliRenderer) initLoopBody(ev event.Event, iteration int) {
	r.loopBody = make(map[string]bool)
	r.loopStepID = ev.StepID
	r.loopIteration = iteration

	if ev.Data == nil {
		return
	}

	body, bodyOK := ev.Data["body"].([]any)
	if bodyOK {
		for _, b := range body {
			s, sOK := b.(string)
			if !sOK {
				continue
			}

			r.loopBody[s] = true
		}
	}

	maxIter, ok := ev.Data["max_iterations"].(float64)
	if ok {
		r.loopMaxIter = int(maxIter)
	}
}

func (r *cliRenderer) stepTitle(stepID string) string {
	st, ok := r.steps[stepID]
	if !ok {
		return stepID
	}

	return st.title
}

func (r *cliRenderer) formatDuration(stepID string) string {
	st := r.steps[stepID]
	if st == nil {
		return ""
	}

	return fmt.Sprintf("(%s)", st.duration.Round(time.Millisecond))
}

func (r *cliRenderer) printSummary(result *engine.ExecuteResult, elapsed time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.eraseActive()

	if r.loopIteration > 0 {
		r.endLoop()
	}

	succeeded, failed, skipped := r.countStepStatuses()

	fmt.Println()
	r.printWorkflowResult(result)

	fmt.Printf("    %d steps: %s, %s, %s ─ %s\n",
		len(r.steps),
		r.c("32", fmt.Sprintf("%d succeeded", succeeded)),
		r.c("31", fmt.Sprintf("%d failed", failed)),
		r.c("33", fmt.Sprintf("%d skipped", skipped)),
		r.c("1", elapsed.Round(time.Millisecond).String()),
	)
}

func (r *cliRenderer) countStepStatuses() (succeeded, failed, skipped int) {
	for _, st := range r.steps {
		switch st.status { //nolint:exhaustive // only counting terminal states
		case statusCompleted:
			succeeded++
		case statusFailed:
			failed++
		case statusSkipped:
			skipped++
		}
	}

	return succeeded, failed, skipped
}

func (r *cliRenderer) printWorkflowResult(result *engine.ExecuteResult) {
	switch result.Status {
	case runtime.StatusSuccess:
		fmt.Printf("  %s %s\n", r.c("32", "✓"), r.c("1;32", "Workflow completed successfully"))
	case runtime.StatusCompletedWithErrors:
		fmt.Printf("  %s %s\n", r.c("33", "⚠"), r.c("1;33", "Workflow completed with errors"))
	default:
		fmt.Printf("  %s %s\n", r.c("31", "✗"), r.c("1;31", "Workflow failed"))

		if result.Error != nil {
			fmt.Printf("    %s\n", r.c("31", "Error: "+result.Error.Error()))
		}
	}
}

func detectNoColor(flagValue bool) bool {
	if flagValue {
		return true
	}

	_, ok := os.LookupEnv("NO_COLOR")
	if ok {
		return true
	}

	fi, err := os.Stdout.Stat()
	if err != nil {
		return true
	}

	if fi.Mode()&os.ModeCharDevice == 0 {
		return true // piped / not a TTY
	}

	return false
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return fi.Mode()&os.ModeCharDevice != 0
}

func flagOrEnv(flagVal, envName string) string {
	if flagVal != "" {
		return flagVal
	}

	return os.Getenv(envName)
}

func main() {
	_ = godotenv.Load()

	var (
		noColorFlag     bool
		exporterURL     string
		exporterKey     string
		exporterName    string
		otelEndpoint    string
		otelServiceName string
	)

	rootCmd := &cobra.Command{
		Use:          "tailflow",
		Short:        "TailFlow - Workflow Engine",
		Version:      version,
		SilenceUsage: true,
	}
	rootCmd.PersistentFlags().BoolVar(&noColorFlag, "no-color", false, "Disable colour output")
	rootCmd.PersistentFlags().StringVar(&exporterURL, "exporter-url", "", "SaaS endpoint URL for event export (env: TAILFLOW_EXPORTER_URL)")
	rootCmd.PersistentFlags().StringVar(&exporterKey, "exporter-key", "", "API key for SaaS authentication (env: TAILFLOW_EXPORTER_KEY)")
	rootCmd.PersistentFlags().StringVar(&exporterName, "exporter-name", "", "Unique agent name (env: TAILFLOW_EXPORTER_NAME)")
	rootCmd.PersistentFlags().StringVar(&otelEndpoint, "otel-endpoint", "", "OTLP/HTTP endpoint (env: OTEL_EXPORTER_OTLP_ENDPOINT)")
	rootCmd.PersistentFlags().StringVar(&otelServiceName, "otel-service-name", "", "Service name (env: OTEL_SERVICE_NAME, default: tailflow)")

	rootCmd.AddCommand(runCmd(&noColorFlag, &exporterURL, &exporterKey, &exporterName, &otelEndpoint, &otelServiceName))
	rootCmd.AddCommand(validateCmd(&noColorFlag))
	rootCmd.AddCommand(serveCmd(&exporterURL, &exporterKey, &exporterName, &otelEndpoint, &otelServiceName))
	rootCmd.AddCommand(testCmd(&noColorFlag))

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func shutdownOTel(result *tfotel.Result) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	flushErr := result.ForceFlush(ctx)
	if flushErr != nil {
		log.Printf("[otel] flush error: %v", flushErr)
	}

	shutdownErr := result.Shutdown(ctx)
	if shutdownErr != nil {
		log.Printf("[otel] shutdown error: %v", shutdownErr)
	}
}

func resolveOTelConfig(endpoint, serviceName *string) tfotel.Config {
	ep := flagOrEnv(*endpoint, "OTEL_EXPORTER_OTLP_ENDPOINT")
	sn := flagOrEnv(*serviceName, "OTEL_SERVICE_NAME")

	return tfotel.Config{
		Endpoint:    ep,
		ServiceName: sn,
		Debug:       os.Getenv("OTEL_DEBUG") != "",
	}
}

func runCmd(noColorFlag *bool, exporterURL, exporterKey, exporterName, otelEndpoint, otelServiceName *string) *cobra.Command {
	var (
		params []string
		data   string
	)

	cmd := &cobra.Command{
		Use:   "run <workflow.yaml>",
		Short: "Execute a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			noColor := detectNoColor(*noColorFlag)

			url := flagOrEnv(*exporterURL, "TAILFLOW_EXPORTER_URL")
			key := flagOrEnv(*exporterKey, "TAILFLOW_EXPORTER_KEY")
			name := flagOrEnv(*exporterName, "TAILFLOW_EXPORTER_NAME")

			if url != "" && name == "" {
				return errors.New("--exporter-name (or TAILFLOW_EXPORTER_NAME) is required when exporter is enabled")
			}

			otelCfg := resolveOTelConfig(otelEndpoint, otelServiceName)

			return executeRun(args[0], params, data, noColor, url, key, name, otelCfg)
		},
	}
	cmd.Flags().StringArrayVarP(&params, "param", "p", nil, "Parameters (key=value)")
	cmd.Flags().StringVarP(&data, "data", "d", "", "Trigger body as JSON (for trigger-based workflows)")

	return cmd
}

func validateCmd(noColorFlag *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "validate <workflow.yaml>",
		Short: "Validate a workflow file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			noColor := detectNoColor(*noColorFlag)

			return executeValidate(args[0], noColor)
		},
	}
}

func serveCmd(exporterURL, exporterKey, exporterName, otelEndpoint, otelServiceName *string) *cobra.Command {
	var (
		port       int
		maxExecs   int
		selfHosted bool
		editor     bool
	)

	cmd := &cobra.Command{
		Use:   "serve <workflow.yaml>",
		Short: "Start the web server for a single workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := flagOrEnv(*exporterURL, "TAILFLOW_EXPORTER_URL")
			key := flagOrEnv(*exporterKey, "TAILFLOW_EXPORTER_KEY")
			name := flagOrEnv(*exporterName, "TAILFLOW_EXPORTER_NAME")

			if url != "" && name == "" {
				return errors.New("--exporter-name (or TAILFLOW_EXPORTER_NAME) is required when exporter is enabled")
			}

			otelCfg := resolveOTelConfig(otelEndpoint, otelServiceName)

			return executeServe(args[0], port, maxExecs, selfHosted, editor, url, key, name, otelCfg)
		},
	}
	cmd.Flags().IntVarP(&port, "port", "P", 8080, "Server port")
	cmd.Flags().IntVar(&maxExecs, "max-executions", 100, "Max executions to keep in memory")
	cmd.Flags().BoolVar(&selfHosted, "selfhosted", false, "Enable all actions (exec, js, file.*) for self-hosted deployments")
	cmd.Flags().BoolVar(&editor, "editor", false, "Enable workflow editor: persist YAML changes via PUT /api/workflow/raw")

	return cmd
}

type runResources struct {
	bus          *event.Bus
	exporter     export.EventExporter
	exportCancel context.CancelFunc
	tickDone     chan struct{}
	wg           sync.WaitGroup
}

func (rr *runResources) shutdown() {
	rr.bus.Close()
	rr.wg.Wait()

	if rr.tickDone != nil {
		close(rr.tickDone)
	}

	rr.exportCancel()

	if rr.exporter != nil {
		rr.exporter.Shutdown()
	}
}

func executeRun(
	path string, rawParams []string, data string, noColor bool,
	exportURL, apiKey, exporterName string, otelCfg tfotel.Config,
) error {
	otelCfg.Sync = true

	otelResult, err := tfotel.Setup(context.Background(), otelCfg)
	if err != nil {
		return fmt.Errorf("otel setup: %w", err)
	}
	defer shutdownOTel(otelResult)

	wf, parseErr := parser.Parse(path)
	if parseErr != nil {
		return parseErr
	}

	renderer := &cliRenderer{
		noColor: noColor,
		isTTY:   isTerminal(),
		wfName:  wf.Name,
	}

	buildErr := renderer.buildTree(wf)
	if buildErr != nil {
		return buildErr
	}

	bus := event.NewBus()
	defer bus.Close()

	tracer := tfotel.NewTracer(otelResult.TracerProvider)

	bm, bmErr := tfotel.NewBusinessMetrics(otelResult.MeterProvider)
	if bmErr != nil {
		return fmt.Errorf("otel business metrics: %w", bmErr)
	}

	logger := tfotel.NewSlogLogger(slog.LevelError+1, otelResult)

	exec, services, closeFn, setupErr := setupActionRegistry(wf, bus, logger, tracer, bm)
	if setupErr != nil {
		return setupErr
	}

	defer closeFn()

	res := setupRunResources(bus, renderer, exportURL, apiKey, exporterName, wf)
	opts := buildExecutionOptions(renderer, wf, path, data, services)

	renderer.printTree()

	return runAndReport(renderer, exec, wf, parseParams(rawParams), opts, res)
}

func setupActionRegistry(
	wf *parser.Workflow, bus *event.Bus, logger *slog.Logger,
	tracer *tfotel.Tracer, bm *tfotel.BusinessMetrics,
) (*engine.Executor, *runtime.ActionServices, func(), error) {
	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	dbPool := runtime.NewMemoryDBPool()

	services := &runtime.ActionServices{
		DBPool:     dbPool,
		TxRegistry: runtime.NewMemoryTxRegistry(logger),
		Locker:     runtime.NewMemoryLocker(),
		KVStore:    runtime.NewMemoryKVStore(),
	}

	reg.SetAllowlist(cliAllowedActions(reg.Names()))

	for _, step := range wf.Steps {
		_, err := reg.Create(step.Action)
		if err != nil {
			closeErr := dbPool.Close()
			if closeErr != nil {
				logger.Warn("failed to close db pool", "error", closeErr)
			}

			return nil, nil, nil, fmt.Errorf("step %q uses %q which requires 'tailflow serve'", step.ID, step.Action)
		}
	}

	exec := engine.NewExecutor(reg, bus, logger, wf.Sensitive, tracer, bm)

	return exec, services, func() {
		err := dbPool.Close()
		if err != nil {
			logger.Warn("failed to close db pool", "error", err)
		}
	}, nil
}

func setupRunResources(
	bus *event.Bus, renderer *cliRenderer,
	exportURL, apiKey, exporterName string, wf *parser.Workflow,
) *runResources {
	res := &runResources{
		bus:          bus,
		exportCancel: func() {},
	}

	res.exporter, res.exportCancel = setupExporter(exportURL, apiKey, exporterName, bus, wf)

	ch := bus.Subscribe(eventBusBuffer)

	res.wg.Go(func() {
		for ev := range ch {
			renderer.handleEvent(ev)
		}
	})

	if renderer.isTTY {
		res.tickDone = make(chan struct{})

		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					renderer.tick()
				case <-res.tickDone:
					return
				}
			}
		}()
	}

	return res
}

func setupExporter(
	exportURL, apiKey, exporterName string, bus *event.Bus, wf *parser.Workflow,
) (export.EventExporter, context.CancelFunc) {
	if exportURL == "" {
		return export.NewNoopExporter(), func() {}
	}

	exportLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	exporter := saas.NewExporter(saas.Config{
		ExportURL:           exportURL,
		APIKey:              apiKey,
		AgentName:           exporterName,
		EventBus:            bus,
		Logger:              exportLogger,
		WorkflowName:        wf.Name,
		WorkflowDescription: wf.Description,
		WorkflowTags:        wf.Tags,
		TriggerType:         resolveTriggerType(wf),
		StepsCount:          len(wf.Steps),
		Version:             version,
		Revision:            wf.Revision,
	})

	exportCtx, exportCancel := context.WithCancel(context.Background())
	exporter.Start(exportCtx)

	return exporter, exportCancel
}

func resolveTriggerType(wf *parser.Workflow) string {
	t := wf.Trigger
	if t == nil {
		return ""
	}

	switch {
	case t.HTTP != nil:
		return "http"
	case t.Webhook != nil:
		return "webhook"
	case t.Schedule != nil:
		return "schedule"
	case t.RabbitMQ != nil:
		return "rabbitmq"
	default:
		return ""
	}
}

func buildExecutionOptions(
	renderer *cliRenderer, wf *parser.Workflow, path, data string, services *runtime.ActionServices,
) []engine.ExecuteOptions {
	baseOpts := engine.ExecuteOptions{Services: services}

	if wf.Trigger != nil {
		if data == "" {
			fmt.Printf("\n  %s %s\n",
				renderer.c("33", "Note:"),
				renderer.c("33", "this workflow has a trigger. Use --data/-d to provide a JSON body."),
			)
			fmt.Printf("  %s\n",
				renderer.c("33", fmt.Sprintf("  Example: tailflow run %s -d '{\"key\":\"value\"}'", path)),
			)
		}

		baseOpts.TriggerData = buildCLITriggerData(wf, data)
	}

	return []engine.ExecuteOptions{baseOpts}
}

func runAndReport(
	renderer *cliRenderer, exec *engine.Executor,
	wf *parser.Workflow, params map[string]any,
	opts []engine.ExecuteOptions, res *runResources,
) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	start := time.Now()

	result, err := exec.Execute(ctx, wf, params, opts...)
	if err != nil {
		res.shutdown()

		return fmt.Errorf("execution failed: %w", err)
	}

	elapsed := time.Since(start)

	res.shutdown()

	renderer.printSummary(result, elapsed)
	fmt.Println()

	if result.Status != runtime.StatusSuccess && result.Status != runtime.StatusCompletedWithErrors {
		cancel()
		os.Exit(1) //nolint:gocritic // cancel() called explicitly above
	}

	return nil
}

func buildCLITriggerData(wf *parser.Workflow, data string) map[string]any {
	triggerData := map[string]any{
		"method":  "CLI",
		"path":    "",
		"headers": map[string]string{},
		"query":   map[string][]string{},
		"body":    nil,
	}

	if wf.Trigger.HTTP != nil {
		triggerData["method"] = wf.Trigger.HTTP.Method
		triggerData["path"] = wf.Trigger.HTTP.Path
	} else if wf.Trigger.Webhook != nil {
		triggerData["method"] = "POST"
		triggerData["path"] = wf.Trigger.Webhook.Path
	}

	if data != "" {
		var body any

		err := json.Unmarshal([]byte(data), &body)
		if err == nil {
			triggerData["body"] = body
		} else {
			triggerData["body"] = data
		}
	}

	return triggerData
}

func testCmd(noColorFlag *bool) *cobra.Command {
	var (
		caseName string
		listFlag bool
	)

	cmd := &cobra.Command{
		Use:   "test <workflow.yaml>",
		Short: "Run workflow test cases",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			noColor := detectNoColor(*noColorFlag)

			if listFlag {
				return executeTestList(args[0], noColor)
			}

			return executeTest(args[0], caseName, noColor)
		},
	}

	cmd.Flags().StringVar(&caseName, "case", "", "Run a specific test case")
	cmd.Flags().BoolVar(&listFlag, "list", false, "List all test cases")

	return cmd
}

func collectTestCases(wf *parser.Workflow) []string {
	seen := make(map[string]bool)
	var names []string

	for _, step := range wf.Steps {
		for _, tc := range step.Testing {
			if !seen[tc.Name] {
				seen[tc.Name] = true
				names = append(names, tc.Name)
			}
		}
	}

	return names
}

func executeTestList(path string, noColor bool) error {
	r := &cliRenderer{noColor: noColor}

	wf, err := parser.Parse(path)
	if err != nil {
		return err
	}

	cases := collectTestCases(wf)
	if len(cases) == 0 {
		fmt.Printf("  %s No test cases found in %q\n", r.c("33", "⚠"), wf.Name)

		return nil
	}

	fmt.Printf("\n  Test cases for %q:\n\n", wf.Name)

	var headerBuilder strings.Builder

	fmt.Fprintf(&headerBuilder, "  %-20s", "")

	for _, step := range wf.Steps {
		fmt.Fprintf(&headerBuilder, "%-18s", step.ID)
	}

	fmt.Println(r.c("1", headerBuilder.String()))

	for _, caseName := range cases {
		var rowBuilder strings.Builder

		fmt.Fprintf(&rowBuilder, "  %-20s", caseName)

		for _, step := range wf.Steps {
			rowBuilder.WriteString(testCaseCell(r, step, caseName))
		}

		fmt.Println(rowBuilder.String())
	}

	fmt.Println()

	return nil
}

func colorPad(r *cliRenderer, code, text string) string {
	padded := fmt.Sprintf("%-18s", text)
	if r.noColor {
		return padded
	}

	return fmt.Sprintf("\033[%sm%s\033[0m", code, padded)
}

func testCaseCell(r *cliRenderer, step parser.Step, caseName string) string {
	tc := findTestCaseInStep(step, caseName)

	switch {
	case tc == nil:
		return colorPad(r, "90", "(runs)")
	case tc.Error != nil:
		return colorPad(r, "31", "mock error")
	case tc.Output != nil && tc.Expect != nil:
		return colorPad(r, "36", "mock+expect")
	case tc.Output != nil:
		return colorPad(r, "33", "mock")
	case tc.Expect != nil:
		return colorPad(r, "36", "expect")
	default:
		return colorPad(r, "90", "(runs)")
	}
}

func findTestCaseInStep(step parser.Step, caseName string) *parser.TestCase {
	for i := range step.Testing {
		if step.Testing[i].Name == caseName {
			return &step.Testing[i]
		}
	}

	return nil
}

func executeTest(path string, caseName string, noColor bool) error {
	wf, err := parser.Parse(path)
	if err != nil {
		return err
	}

	cases := []string{caseName}
	if caseName == "" {
		cases = collectTestCases(wf)
		if len(cases) == 0 {
			r := &cliRenderer{noColor: noColor}
			fmt.Printf("  %s No test cases found in %q\n", r.c("33", "⚠"), wf.Name)

			return nil
		}
	}

	r := &cliRenderer{noColor: noColor}

	fmt.Printf("\n  Testing %q...\n\n", wf.Name)

	passed := 0
	failed := 0

	for _, cn := range cases {
		start := time.Now()
		testErr := runSingleTestCase(wf, cn)
		elapsed := time.Since(start).Round(time.Millisecond)

		if testErr != nil {
			fmt.Printf("  %s  %-20s %s %s\n", r.c("31", "✗"), cn, r.c("90", fmt.Sprintf("(%s)", elapsed)), r.c("31", testErr.Error()))

			failed++
		} else {
			fmt.Printf("  %s  %-20s %s\n", r.c("32", "✓"), cn, r.c("90", fmt.Sprintf("passed (%s)", elapsed)))

			passed++
		}
	}

	fmt.Printf("\n  %d/%d passed\n\n", passed, len(cases))

	if failed > 0 {
		os.Exit(1)
	}

	return nil
}

func runSingleTestCase(wf *parser.Workflow, caseName string) error {
	bus := event.NewBus()
	defer bus.Close()

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)
	reg.SetAllowlist(cliAllowedActions(reg.Names()))

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))
	exec := engine.NewExecutor(reg, bus, logger, wf.Sensitive, nil, nil)

	services := &runtime.ActionServices{
		DBPool:     runtime.NewMemoryDBPool(),
		TxRegistry: runtime.NewMemoryTxRegistry(logger),
		Locker:     runtime.NewMemoryLocker(),
		KVStore:    runtime.NewMemoryKVStore(),
	}

	ctx := context.Background()

	result, err := exec.Execute(ctx, wf, nil, engine.ExecuteOptions{
		Services:     services,
		TestCaseName: caseName,
	})
	if err != nil {
		return err
	}

	if result.Status == runtime.StatusFailed {
		if result.Error != nil {
			return result.Error
		}

		return errors.New("workflow failed")
	}

	return nil
}

func executeValidate(path string, noColor bool) error {
	r := &cliRenderer{noColor: noColor}

	wf, err := parser.Parse(path)
	if err != nil {
		fmt.Printf("  %s %s\n", r.c("31", "✗"), r.c("31", "Validation failed: "+err.Error()))
		os.Exit(1)
	}

	r.wfName = wf.Name

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	for _, step := range wf.Steps {
		if !reg.Has(step.Action) {
			fmt.Printf("  %s %s\n", r.c("31", "✗"),
				r.c("31", fmt.Sprintf("Validation failed: step %q references unknown action %q", step.ID, step.Action)))
			os.Exit(1)
		}
	}

	buildErr := r.buildTree(wf)
	if buildErr != nil {
		fmt.Printf("  %s %s\n", r.c("31", "✗"), r.c("31", "Validation failed: "+buildErr.Error()))
		os.Exit(1)
	}

	fmt.Printf("  %s %s\n", r.c("32", "✓"),
		r.c("32", fmt.Sprintf("Workflow %q is valid (%d steps, %d params)", wf.Name, len(wf.Steps), len(wf.Params))))

	r.printTree()

	return nil
}

func executeServe(
	path string, port int, maxExecs int, selfHosted, editor bool,
	exportURL, apiKey, exporterName string, otelCfg tfotel.Config,
) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return agentfx.RunApp(ctx, agentfx.Config{
		WorkflowPath: path,
		Port:         port,
		MaxExecs:     maxExecs,
		SelfHosted:   selfHosted,
		Editor:       editor,
		ExportURL:    exportURL,
		APIKey:       apiKey,
		ExporterName: exporterName,
		Version:      version,
		OTel:         otelCfg,
		LogLevel:     slog.LevelInfo,
	})
}

func cliAllowedActions(all []string) []string {
	allowed := make([]string, 0, len(all))

	for _, name := range all {
		if strings.HasPrefix(name, "wait.") ||
			name == "schedule" {
			continue
		}

		allowed = append(allowed, name)
	}

	return allowed
}

func parseParams(raw []string) map[string]any {
	params := make(map[string]any)

	for _, p := range raw {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) == 2 {
			params[parts[0]] = parts[1]
		}
	}

	return params
}

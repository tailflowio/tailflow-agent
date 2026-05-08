package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
)

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

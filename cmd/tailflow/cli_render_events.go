package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/tailflow/tailflow/internal/event"
)

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

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tailflow/tailflow/internal/event"
)

const flushInterval = 50 * time.Millisecond

// jsonMarshalEvent is used to marshal SSE event payloads.
// Override in tests to simulate marshal errors.
var jsonMarshalEvent = json.Marshal

type loopTracker struct {
	Body      map[string]bool
	Iteration int
}

func (lt *loopTracker) Track(ev event.Event) {
	if ev.Type == event.StepGoto && ev.Data != nil {
		lt.trackGoto(ev)
	}

	if ev.Type == event.StepStarted && lt.Iteration > 0 && !lt.Body[ev.StepID] {
		lt.Body = nil
		lt.Iteration = 0
	}
}

func (lt *loopTracker) trackGoto(ev event.Event) {
	iter, ok := ev.Data["iteration"].(float64)
	if ok {
		lt.Iteration = int(iter)
	}

	body, ok := ev.Data["body"].([]any)
	if !ok {
		return
	}

	lt.Body = make(map[string]bool, len(body))

	for _, b := range body {
		sid, ok := b.(string)
		if ok {
			lt.Body[sid] = true
		}
	}
}

// step.goto events are never considered "in loop" (always pass through).
func (lt *loopTracker) InLoop(ev event.Event) bool {
	return lt.Iteration > 1 && lt.Body[ev.StepID] && ev.Type != event.StepGoto
}

func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

func writeSSEEvent(w http.ResponseWriter, ev event.Event) bool {
	data, err := jsonMarshalEvent(ev)
	if err != nil {
		return false
	}

	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, string(data))

	return true
}

func (s *Server) replayStoredEvents(
	w http.ResponseWriter, flusher http.Flusher, executionID string,
) (replayedCount int, workflowDone bool) {
	stored := s.config.ExecutionStore.GetEvents(executionID)
	sent := 0

	for _, ev := range stored {
		writeSSEEvent(w, ev)

		sent++

		if ev.Type == "workflow.completed" {
			workflowDone = true
			break // stop replaying — nothing should follow workflow.completed
		}
	}

	flusher.Flush()

	return sent, workflowDone
}

type sseEventFilter func(ev event.Event) bool

func sseEventLoop(
	w http.ResponseWriter, r *http.Request, flusher http.Flusher,
	ch <-chan event.Event, closeOnComplete bool, filter sseEventFilter,
) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	pendingFlush := false

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}

			if !filter(ev) {
				continue
			}

			if !writeSSEEvent(w, ev) {
				continue
			}

			pendingFlush = true

			if closeOnComplete && ev.Type == "workflow.completed" {
				flusher.Flush()
				return
			}

		case <-ticker.C:
			if pendingFlush {
				flusher.Flush()

				pendingFlush = false
			}

		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	executionID := r.PathValue("id")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(r.Context(), w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	setSSEHeaders(w)

	// Subscribe BEFORE reading history to avoid losing events published
	// between GetEvents and Subscribe.
	ch := s.config.EventBus.Subscribe(10_000)
	defer s.config.EventBus.Unsubscribe(ch)

	fmt.Fprintf(w, "event: connected\ndata: {\"execution_id\":%q}\n\n", executionID)
	flusher.Flush()

	replayedCount, workflowDone := s.replayStoredEvents(w, flusher, executionID)
	if workflowDone {
		return
	}

	skipped := 0
	lt := &loopTracker{}

	sseEventLoop(w, r, flusher, ch, true, func(ev event.Event) bool {
		if ev.ExecutionID != executionID {
			return false
		}

		lt.Track(ev)

		// Loop body events (iteration > 1) are never stored, so they must
		// not consume the skip counter — otherwise real post-replay events
		// get incorrectly skipped.
		inLoop := lt.InLoop(ev)

		if skipped < replayedCount {
			if !inLoop {
				skipped++
			}

			return false
		}

		return !inLoop
	})
}

func (s *Server) handleGlobalSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(r.Context(), w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	setSSEHeaders(w)

	ch := s.config.EventBus.Subscribe(10_000)
	defer s.config.EventBus.Unsubscribe(ch)

	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	loops := map[string]*loopTracker{}

	sseEventLoop(w, r, flusher, ch, false, func(ev event.Event) bool {
		lt := loops[ev.ExecutionID]
		if lt == nil {
			lt = &loopTracker{}
			loops[ev.ExecutionID] = lt
		}

		lt.Track(ev)

		if lt.InLoop(ev) {
			return false
		}

		if ev.Type == event.WorkflowCompleted {
			delete(loops, ev.ExecutionID)
		}

		return true
	})
}

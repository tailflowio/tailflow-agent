package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tailflow/tailflow/internal/event"
)

const flushInterval = 50 * time.Millisecond

// loopTracker tracks goto loop state to filter out redundant events
// for loop body steps after iteration 1. Used by both SSE handlers
// and captureEvents to avoid sending/storing thousands of repetitive events.
type loopTracker struct {
	Body      map[string]bool
	Iteration int
}

// Track updates loop state from an event. Must be called for every event.
func (lt *loopTracker) Track(ev event.Event) {
	if ev.Type == event.StepGoto && ev.Data != nil {
		if iter, ok := ev.Data["iteration"].(float64); ok {
			lt.Iteration = int(iter)
		}

		if body, ok := ev.Data["body"].([]any); ok {
			lt.Body = make(map[string]bool, len(body))
			for _, b := range body {
				if sid, ok := b.(string); ok {
					lt.Body[sid] = true
				}
			}
		}
	}

	// End of loop: a step outside the body starts.
	if ev.Type == event.StepStarted && lt.Iteration > 0 && !lt.Body[ev.StepID] {
		lt.Body = nil
		lt.Iteration = 0
	}
}

// InLoop returns true if the event is a loop body event past iteration 1.
// step.goto events are never considered "in loop" (always pass through).
func (lt *loopTracker) InLoop(ev event.Event) bool {
	return lt.Iteration > 1 && lt.Body[ev.StepID] && ev.Type != event.StepGoto
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	executionID := r.PathValue("id")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Subscribe BEFORE reading history to avoid losing events published
	// between GetEvents and Subscribe.
	ch := s.config.EventBus.Subscribe(100)
	defer s.config.EventBus.Unsubscribe(ch)

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: {\"execution_id\":%q}\n\n", executionID)
	flusher.Flush()

	// Replay stored events (may overlap with events buffered in ch)
	stored := s.config.ExecutionStore.GetEvents(executionID)
	replayedCount := len(stored)
	workflowDone := false

	for _, ev := range stored {
		data, err := json.Marshal(ev)
		if err != nil {
			continue
		}

		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, string(data))

		if ev.Type == "workflow.completed" {
			workflowDone = true
		}
	}

	flusher.Flush()

	// If workflow already finished, close stream
	if workflowDone {
		return
	}

	// Live events — skip events already replayed.
	// Events for this execution are appended sequentially, so we count
	// how many we already sent and skip that many from the bus.
	skipped := 0

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	pendingFlush := false
	lt := &loopTracker{}

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}

			if ev.ExecutionID != executionID {
				continue
			}

			// Skip events that were already part of the replay
			if skipped < replayedCount {
				skipped++
				continue
			}

			lt.Track(ev)

			if lt.InLoop(ev) {
				continue
			}

			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}

			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, string(data))
			pendingFlush = true

			if ev.Type == "workflow.completed" {
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

// handleGlobalSSE streams ALL events (no execution filter).
// The frontend uses this to refresh lists in real-time.
func (s *Server) handleGlobalSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := s.config.EventBus.Subscribe(100)
	defer s.config.EventBus.Unsubscribe(ch)

	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	pendingFlush := false

	// Per-execution loop tracking for the global stream.
	loops := map[string]*loopTracker{}

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}

			lt := loops[ev.ExecutionID]
			if lt == nil {
				lt = &loopTracker{}
				loops[ev.ExecutionID] = lt
			}

			lt.Track(ev)

			if lt.InLoop(ev) {
				continue
			}

			// Clean up finished executions.
			if ev.Type == event.WorkflowCompleted {
				delete(loops, ev.ExecutionID)
			}

			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}

			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, string(data))
			pendingFlush = true

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

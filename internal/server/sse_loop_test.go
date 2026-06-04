package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/tailflow/tailflow/internal/event"
)

func (s *SSETestSuite) TestSseEventLoop_ClosedChannel() {
	synctest.Test(s.T(), func(t *testing.T) {
		w := newFlushRecorder()
		ctx := context.Background()
		req := httptest.NewRequest("GET", "/test", nil).WithContext(ctx)

		ch := make(chan event.Event)
		close(ch)

		exited := false
		go func() {
			sseEventLoop(w, req, w, ch, false, func(ev event.Event) bool {
				return true
			})
			exited = true
		}()

		synctest.Wait()
		s.True(exited)
	})
}

func (s *SSETestSuite) TestSseEventLoop_MarshalError_ContinuesLoop() {
	synctest.Test(s.T(), func(t *testing.T) {
		original := jsonMarshalEvent
		t.Cleanup(func() { jsonMarshalEvent = original })

		callCount := 0
		jsonMarshalEvent = func(v any) ([]byte, error) {
			callCount++
			if callCount == 1 {
				return nil, errors.New("marshal error")
			}
			return json.Marshal(v)
		}

		w := newFlushRecorder()
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("GET", "/test", nil).WithContext(ctx)

		ch := make(chan event.Event, 2)
		ch <- event.Event{Type: event.StepStarted, StepID: "bad"}
		ch <- event.Event{Type: event.StepCompleted, StepID: "good"}

		go func() {
			sseEventLoop(w, req, w, ch, false, func(ev event.Event) bool {
				return true
			})
		}()

		time.Sleep(flushInterval * 2)
		synctest.Wait()

		cancel()
		time.Sleep(flushInterval)
		synctest.Wait()

		body := w.Body.String()
		s.NotContains(body, "step.started")
		s.Contains(body, "step.completed")
	})
}

func (s *SSETestSuite) TestSseEventLoop_FilterReturnsFalse() {
	synctest.Test(s.T(), func(t *testing.T) {
		w := newFlushRecorder()
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("GET", "/test", nil).WithContext(ctx)

		ch := make(chan event.Event, 2)
		ch <- event.Event{Type: event.StepStarted, StepID: "filtered-out"}
		ch <- event.Event{Type: event.StepCompleted, StepID: "passed"}

		go func() {
			sseEventLoop(w, req, w, ch, false, func(ev event.Event) bool {
				return ev.StepID == "passed"
			})
		}()

		time.Sleep(flushInterval * 2)
		synctest.Wait()

		cancel()
		time.Sleep(flushInterval)
		synctest.Wait()

		body := w.Body.String()
		s.NotContains(body, "filtered-out")
		s.Contains(body, "passed")
	})
}

func (s *SSETestSuite) TestHandleGlobalSSE_WorkflowCompleted_CleansUpLoopTracker() {
	synctest.Test(s.T(), func(t *testing.T) {
		srv := newTestServer(t)

		w := newFlushRecorder()
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("GET", "/api/events", nil).WithContext(ctx)

		go srv.Handler().ServeHTTP(w, req)
		time.Sleep(flushInterval)
		synctest.Wait()

		srv.config.EventBus.Publish(event.Event{
			Type:        event.StepGoto,
			ExecutionID: "exec-1",
			StepID:      "loop",
			Data: map[string]any{
				"iteration": float64(2),
				"body":      []any{"step-a"},
			},
		})

		time.Sleep(flushInterval)
		synctest.Wait()

		srv.config.EventBus.Publish(event.Event{
			Type:        event.WorkflowCompleted,
			ExecutionID: "exec-1",
		})

		time.Sleep(flushInterval * 2)
		synctest.Wait()

		cancel()
		time.Sleep(flushInterval)
		synctest.Wait()

		body := w.Body.String()
		s.Contains(body, "event: workflow.completed")
	})
}

func (s *SSETestSuite) TestHandleGlobalSSE_InLoopEventFiltered() {
	synctest.Test(s.T(), func(t *testing.T) {
		srv := newTestServer(t)

		w := newFlushRecorder()
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("GET", "/api/events", nil).WithContext(ctx)

		go srv.Handler().ServeHTTP(w, req)
		time.Sleep(flushInterval)
		synctest.Wait()

		srv.config.EventBus.Publish(event.Event{
			Type:        event.StepGoto,
			ExecutionID: "exec-1",
			StepID:      "loop",
			Data: map[string]any{
				"iteration": float64(2),
				"body":      []any{"step-a"},
			},
		})

		time.Sleep(flushInterval)
		synctest.Wait()

		srv.config.EventBus.Publish(event.Event{
			Type:        event.StepStarted,
			ExecutionID: "exec-1",
			StepID:      "step-a",
		})

		srv.config.EventBus.Publish(event.Event{
			Type:        event.StepStarted,
			ExecutionID: "exec-1",
			StepID:      "step-b",
		})

		time.Sleep(flushInterval * 2)
		synctest.Wait()

		cancel()
		time.Sleep(flushInterval)
		synctest.Wait()

		body := w.Body.String()
		s.Contains(body, "step-b")
	})
}

func (s *SSETestSuite) TestHandleSSE_SkipsAlreadyReplayedEvents() {
	synctest.Test(s.T(), func(t *testing.T) {
		srv := newTestServer(t)

		srv.config.ExecutionStore.AppendEvent(context.Background(), "exec-1", event.Event{
			Type:        event.StepStarted,
			ExecutionID: "exec-1",
			StepID:      "step-a",
		})

		w := newFlushRecorder()
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("GET", "/api/executions/exec-1/events", nil).WithContext(ctx)

		go srv.Handler().ServeHTTP(w, req)
		time.Sleep(flushInterval)
		synctest.Wait()

		srv.config.EventBus.Publish(event.Event{
			Type:        event.StepStarted,
			ExecutionID: "exec-1",
			StepID:      "step-a",
		})

		srv.config.EventBus.Publish(event.Event{
			Type:        event.StepCompleted,
			ExecutionID: "exec-1",
			StepID:      "step-a",
		})

		time.Sleep(flushInterval * 2)
		synctest.Wait()

		cancel()
		time.Sleep(flushInterval)
		synctest.Wait()

		body := w.Body.String()
		startedCount := strings.Count(body, "event: step.started")
		s.Equal(1, startedCount, "replayed events in live stream should be skipped")

		s.Contains(body, "event: step.completed")
	})
}

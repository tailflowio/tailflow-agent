package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/event"
)

type SSETestSuite struct {
	suite.Suite
}

func TestSSE(t *testing.T) {
	suite.Run(t, new(SSETestSuite))
}

func (s *SSETestSuite) SetupTest() {}

// plainResponseWriter implements http.ResponseWriter but NOT http.Flusher.
type plainResponseWriter struct {
	code   int
	header http.Header
	body   strings.Builder
}

func newPlainResponseWriter() *plainResponseWriter {
	return &plainResponseWriter{header: make(http.Header)}
}

func (w *plainResponseWriter) Header() http.Header        { return w.header }
func (w *plainResponseWriter) WriteHeader(statusCode int)  { w.code = statusCode }
func (w *plainResponseWriter) Write(b []byte) (int, error) { return w.body.Write(b) }

// flushRecorder wraps httptest.ResponseRecorder to implement http.Flusher.
type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed int
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (f *flushRecorder) Flush() { f.flushed++ }

// Unwrap lets net/http see the underlying ResponseRecorder if needed.
func (f *flushRecorder) Unwrap() http.ResponseWriter { return f.ResponseRecorder }

func (s *SSETestSuite) TestHandleSSE_StreamNotSupported() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/executions/exec-1/events", nil)
	w := newPlainResponseWriter()
	srv.handleSSE(w, req)

	s.Equal(http.StatusInternalServerError, w.code)
	s.Contains(w.body.String(), "streaming not supported")
}

func (s *SSETestSuite) TestHandleSSE_ConnectsAndSendsConnectedEvent() {
	synctest.Test(s.T(), func(t *testing.T) {
		srv := newTestServer(t)

		w := newFlushRecorder()
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("GET", "/api/executions/exec-1/events", nil).WithContext(ctx)

		go srv.Handler().ServeHTTP(w, req)
		synctest.Wait()

		cancel()
		synctest.Wait()

		body := w.Body.String()
		s.Contains(body, "event: connected")
		s.Contains(body, `"execution_id":"exec-1"`)
		s.GreaterOrEqual(w.flushed, 1)
	})
}

func (s *SSETestSuite) TestHandleSSE_ReplaysStoredEvents() {
	synctest.Test(s.T(), func(t *testing.T) {
		srv := newTestServer(t)

		srv.config.ExecutionStore.AppendEvent(context.Background(), "exec-1", event.Event{
			Type:        event.StepStarted,
			ExecutionID: "exec-1",
			StepID:      "step-a",
		})
		srv.config.ExecutionStore.AppendEvent(context.Background(), "exec-1", event.Event{
			Type:        event.StepCompleted,
			ExecutionID: "exec-1",
			StepID:      "step-a",
		})

		w := newFlushRecorder()
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("GET", "/api/executions/exec-1/events", nil).WithContext(ctx)

		go srv.Handler().ServeHTTP(w, req)
		time.Sleep(flushInterval)
		synctest.Wait()

		cancel()
		time.Sleep(flushInterval)
		synctest.Wait()

		body := w.Body.String()
		s.Contains(body, "event: connected")
		s.Contains(body, "event: step.started")
		s.Contains(body, "event: step.completed")

		count := strings.Count(body, "event: step.started")
		s.Equal(1, count, "step.started should appear exactly once")
	})
}

func (s *SSETestSuite) TestHandleSSE_WorkflowCompletedClosesStream() {
	synctest.Test(s.T(), func(t *testing.T) {
		srv := newTestServer(t)

		w := newFlushRecorder()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		req := httptest.NewRequest("GET", "/api/executions/exec-1/events", nil).WithContext(ctx)

		done := make(chan struct{})
		go func() {
			srv.Handler().ServeHTTP(w, req)
			close(done)
		}()

		time.Sleep(flushInterval)
		synctest.Wait()

		srv.config.EventBus.Publish(event.Event{
			Type:        event.WorkflowCompleted,
			ExecutionID: "exec-1",
		})

		time.Sleep(flushInterval)
		synctest.Wait()

		<-done

		body := w.Body.String()
		s.Contains(body, "event: workflow.completed")
	})
}

func (s *SSETestSuite) TestHandleSSE_FiltersDifferentExecution() {
	synctest.Test(s.T(), func(t *testing.T) {
		srv := newTestServer(t)

		w := newFlushRecorder()
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("GET", "/api/executions/exec-1/events", nil).WithContext(ctx)

		go srv.Handler().ServeHTTP(w, req)
		time.Sleep(flushInterval)
		synctest.Wait()

		srv.config.EventBus.Publish(event.Event{
			Type:        event.StepStarted,
			ExecutionID: "exec-OTHER",
			StepID:      "step-x",
		})

		srv.config.EventBus.Publish(event.Event{
			Type:        event.StepStarted,
			ExecutionID: "exec-1",
			StepID:      "step-y",
		})

		time.Sleep(flushInterval * 2)
		synctest.Wait()

		cancel()
		time.Sleep(flushInterval)
		synctest.Wait()

		body := w.Body.String()
		s.NotContains(body, "step-x", "events from other executions should be filtered")
		s.Contains(body, "step-y", "events from matching execution should appear")
	})
}

func (s *SSETestSuite) TestHandleGlobalSSE_StreamsAllExecutions() {
	synctest.Test(s.T(), func(t *testing.T) {
		srv := newTestServer(t)

		w := newFlushRecorder()
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("GET", "/api/events", nil).WithContext(ctx)

		go srv.Handler().ServeHTTP(w, req)
		time.Sleep(flushInterval)
		synctest.Wait()

		srv.config.EventBus.Publish(event.Event{
			Type:        event.StepStarted,
			ExecutionID: "exec-A",
			StepID:      "step-1",
		})
		srv.config.EventBus.Publish(event.Event{
			Type:        event.StepStarted,
			ExecutionID: "exec-B",
			StepID:      "step-2",
		})

		time.Sleep(flushInterval * 2)
		synctest.Wait()

		cancel()
		time.Sleep(flushInterval)
		synctest.Wait()

		body := w.Body.String()
		s.Contains(body, "event: connected")
		s.Contains(body, "step-1")
		s.Contains(body, "step-2")
	})
}

func (s *SSETestSuite) TestHandleGlobalSSE_StreamNotSupported() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/events", nil)
	w := newPlainResponseWriter()
	srv.handleGlobalSSE(w, req)

	s.Equal(http.StatusInternalServerError, w.code)
	s.Contains(w.body.String(), "streaming not supported")
}

func (s *SSETestSuite) TestWriteSSEEvent_MarshalError() {
	original := jsonMarshalEvent
	s.T().Cleanup(func() { jsonMarshalEvent = original })

	jsonMarshalEvent = func(v any) ([]byte, error) {
		return nil, errors.New("marshal boom")
	}

	w := httptest.NewRecorder()
	ok := writeSSEEvent(w, event.Event{Type: event.StepStarted})

	s.False(ok)
	s.Empty(w.Body.String())
}

func (s *SSETestSuite) TestSetSSEHeaders() {
	w := httptest.NewRecorder()
	setSSEHeaders(w)

	s.Equal("text/event-stream", w.Header().Get("Content-Type"))
	s.Equal("no-cache", w.Header().Get("Cache-Control"))
	s.Equal("keep-alive", w.Header().Get("Connection"))
	s.Equal("*", w.Header().Get("Access-Control-Allow-Origin"))
}

func parseSSEFrames(raw string) []map[string]string {
	var frames []map[string]string

	scanner := bufio.NewScanner(strings.NewReader(raw))
	current := map[string]string{}

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			current["event"] = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			current["data"] = strings.TrimPrefix(line, "data: ")
		case line == "":
			if len(current) > 0 {
				frames = append(frames, current)
				current = map[string]string{}
			}
		}
	}

	if len(current) > 0 {
		frames = append(frames, current)
	}

	return frames
}

func (s *SSETestSuite) TestWriteSSEEvent_Format() {
	w := httptest.NewRecorder()

	ev := event.Event{
		Type:        event.StepStarted,
		ExecutionID: "exec-1",
		StepID:      "step-a",
	}
	ok := writeSSEEvent(w, ev)
	s.True(ok)

	frames := parseSSEFrames(w.Body.String())
	s.Require().Len(frames, 1)
	s.Equal("step.started", frames[0]["event"])

	var parsed event.Event
	err := json.Unmarshal([]byte(frames[0]["data"]), &parsed)
	s.Require().NoError(err)
	s.Equal(event.StepStarted, parsed.Type)
	s.Equal("exec-1", parsed.ExecutionID)
	s.Equal("step-a", parsed.StepID)
}

func (s *SSETestSuite) TestReplayStoredEvents_WithWorkflowCompleted() {
	synctest.Test(s.T(), func(t *testing.T) {
		srv := newTestServer(t)

		srv.config.ExecutionStore.AppendEvent(context.Background(), "exec-1", event.Event{
			Type:        event.StepStarted,
			ExecutionID: "exec-1",
			StepID:      "step-a",
		})
		srv.config.ExecutionStore.AppendEvent(context.Background(), "exec-1", event.Event{
			Type:        event.WorkflowCompleted,
			ExecutionID: "exec-1",
		})

		w := newFlushRecorder()
		count, done := srv.replayStoredEvents(w, w, "exec-1")

		s.Equal(2, count)
		s.True(done, "should return workflowDone=true when workflow.completed is in stored events")
		s.Contains(w.Body.String(), "event: workflow.completed")
	})
}

func (s *SSETestSuite) TestHandleSSE_StoredWorkflowCompleted_ClosesImmediately() {
	synctest.Test(s.T(), func(t *testing.T) {
		srv := newTestServer(t)

		srv.config.ExecutionStore.AppendEvent(context.Background(), "exec-1", event.Event{
			Type:        event.WorkflowCompleted,
			ExecutionID: "exec-1",
		})

		w := newFlushRecorder()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		req := httptest.NewRequest("GET", "/api/executions/exec-1/events", nil).WithContext(ctx)

		done := make(chan struct{})
		go func() {
			srv.Handler().ServeHTTP(w, req)
			close(done)
		}()

		time.Sleep(flushInterval)
		synctest.Wait()

		<-done

		body := w.Body.String()
		s.Contains(body, "event: workflow.completed")
	})
}

func (s *SSETestSuite) TestReplayStoredEvents_EmptyStore() {
	srv := newTestServer(s.T())

	w := newFlushRecorder()
	count, done := srv.replayStoredEvents(w, w, "nonexistent")

	s.Equal(0, count)
	s.False(done)
}


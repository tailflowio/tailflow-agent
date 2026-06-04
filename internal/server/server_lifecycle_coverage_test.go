package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

// TestRecoverExecutions_RecovererError covers recoverExecutions – Recoverer returns error.
func (s *ServerTestSuite) TestRecoverExecutions_RecovererError() {
	srv := newTestServerWithRecoverer(s.T(), &stubRecoverer{err: errors.New("recoverer failed")})
	srv.ctx = context.Background()

	s.NotPanics(func() {
		srv.recoverExecutions(context.Background())
	})
}

// TestRunWorkflowAsync_ShutdownWithRecovery covers runWorkflowAsync – execCtx cancelled + Recovery=true
// (shutdownWithRecovery branch skips ensureWorkflowCompleted).
func (s *ServerTestSuite) TestRunWorkflowAsync_ShutdownWithRecovery() {
	srv := newTestServer(s.T())
	srv.config.Workflow.Recovery = true

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv.ctx = ctx

	execID := srv.runWorkflowAsync(nil)
	s.NotEmpty(execID)

	s.Eventually(func() bool {
		exec, err := srv.config.ExecutionStore.Get(context.Background(), execID)
		return err == nil && exec.Status != runtime.StatusRunning
	}, 3*time.Second, 10*time.Millisecond)
}

// TestRunWorkflowAsync_StoreAddError covers runWorkflowAsync – store.Add fails.
func (s *ServerTestSuite) TestRunWorkflowAsync_StoreAddError() {
	es := newErrStore()
	es.failAdd = true
	srv := newTestServerWithErrStore(s.T(), es)

	execID := srv.runWorkflowAsync(nil)
	s.NotEmpty(execID)

	s.Eventually(func() bool {
		_, err := es.inner.Get(context.Background(), execID)
		return err != nil
	}, 3*time.Second, 10*time.Millisecond)
}

// TestEnsureWorkflowCompleted_GetEventsError covers ensureWorkflowCompleted – GetEvents error.
func (s *ServerTestSuite) TestEnsureWorkflowCompleted_GetEventsError() {
	es := newErrStore()
	es.failGetEvents = true
	srv := newTestServerWithErrStore(s.T(), es)

	execID := "ewc-err"

	s.NotPanics(func() {
		srv.ensureWorkflowCompleted(execID, nil, nil, context.Background())
	})
}

// TestEnsureWorkflowCompleted_AppendEventError covers ensureWorkflowCompleted – AppendEvent fails.
func (s *ServerTestSuite) TestEnsureWorkflowCompleted_AppendEventError() {
	es := newErrStore()
	es.failAppendEvent = true
	srv := newTestServerWithErrStore(s.T(), es)

	execID := "ewc-append-err"
	es.inner.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: "running",
		StartedAt: time.Now(),
	})

	s.NotPanics(func() {
		srv.ensureWorkflowCompleted(execID, nil, nil, context.Background())
	})
}

// TestEnsureWorkflowCompleted_PublishesAfterAppend covers ensureWorkflowCompleted – event is published.
func (s *ServerTestSuite) TestEnsureWorkflowCompleted_PublishesAfterAppend() {
	srv := newTestServer(s.T())

	execID := "ewc-publish-after"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	ch := srv.config.EventBus.Subscribe(10)
	defer srv.config.EventBus.Unsubscribe(ch)

	srv.ensureWorkflowCompleted(execID, nil, errors.New("exec error"), context.Background())

	select {
	case ev := <-ch:
		s.Equal(event.WorkflowCompleted, ev.Type)
		s.Equal(runtime.StatusFailed, ev.Data["status"])
	case <-time.After(2 * time.Second):
		s.Fail("event not published")
	}
}

// TestReplayStoredEvents_GetEventsError covers replayStoredEvents – GetEvents returns error.
func (s *SSETestSuite) TestReplayStoredEvents_GetEventsError() {
	es := newErrStore()
	es.failGetEvents = true
	srv := newTestServerWithErrStore(s.T(), es)

	w := newFlushRecorder()
	count, done := srv.replayStoredEvents(w, w, "any-id")

	s.Equal(0, count)
	s.False(done)
}

// TestPrepareTriggerExecution_StoreAddError covers prepareTriggerExecution – store.Add error is logged, not fatal.
func (s *HandlersTestSuite) TestPrepareTriggerExecution_StoreAddError() {
	es := newErrStore()
	es.failAdd = true
	srv := newTestServerWithErrStore(s.T(), es)

	srv.config.Workflow.Trigger = &parser.Trigger{
		HTTP: &parser.HTTPTrigger{
			Method: "POST",
			Path:   "/submit",
		},
	}

	req := httptest.NewRequest("POST", "/api/public/submit", nil)
	w := httptest.NewRecorder()

	s.NotPanics(func() {
		srv.Handler().ServeHTTP(w, req)
	})
}

// TestHandleGetAllStepMetrics_WithPrecomputedMetrics covers the allMetrics loop body
// and the >maxHistory branch.
func (s *HandlersTestSuite) TestHandleGetAllStepMetrics_WithPrecomputedMetrics() {
	memStore := store.NewExecutionStore(200)
	srv := newTestServerWithCustomStore(s.T(), memStore)

	now := time.Now()
	finished := now.Add(500 * time.Millisecond)

	for i := range 25 {
		execID := fmt.Sprintf("m-exec-%d", i)
		srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
			ID:           execID,
			WorkflowName: "test-workflow",
			Status:       runtime.StatusSuccess,
			StartedAt:    now,
			FinishedAt:   &finished,
			Steps: map[string]*runtime.StepResult{
				"greet": {
					Status:     runtime.StatusSuccess,
					StartedAt:  &now,
					FinishedAt: &finished,
				},
			},
		})
	}

	memStore.RefreshStepMetrics()

	req := httptest.NewRequest("GET", "/api/workflow/steps/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)

	greetStats, ok := resp["greet"].(map[string]any)
	s.True(ok, "greet step should have stats")
	s.Greater(greetStats["total_executions"].(float64), float64(0))
}

// TestBuildActionServices_ScheduleExecution_StoreAddError covers buildActionServices –
// ScheduleExecution branch when store.Add fails.
func (s *HandlersTestSuite) TestBuildActionServices_ScheduleExecution_StoreAddError() {
	es := newErrStore()
	es.failAdd = true
	srv := newTestServerWithErrStore(s.T(), es)

	services := srv.buildActionServices()

	_, err := services.ScheduleExecution(0, map[string]any{"key": "val"})
	s.Error(err)
	s.Contains(err.Error(), "schedule execution")
}

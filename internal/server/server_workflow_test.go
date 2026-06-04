package server

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

func (s *ServerTestSuite) TestRunWorkflowAsync_WithOnComplete() {
	srv := newTestServer(s.T())

	completeCalled := make(chan bool, 1)
	opts := asyncRunOpts{
		OnComplete: func(executionID string, success bool) {
			completeCalled <- success
		},
	}

	execID := srv.runWorkflowAsync(nil, opts)
	s.NotEmpty(execID)

	select {
	case success := <-completeCalled:
		s.True(success)
	case <-time.After(5 * time.Second):
		s.Fail("OnComplete was not called")
	}
}

func (s *ServerTestSuite) TestRunWorkflowAsync_WithTriggerData() {
	srv := newTestServer(s.T())

	triggerData := map[string]any{
		"body": "hello",
	}
	opts := asyncRunOpts{
		TriggerData: triggerData,
	}

	execID := srv.runWorkflowAsync(nil, opts)
	s.NotEmpty(execID)

	s.Eventually(func() bool {
		exec, err := srv.config.ExecutionStore.Get(context.Background(), execID)
		return err == nil && exec.Status != "running"
	}, 5*time.Second, 50*time.Millisecond)
}

func (s *ServerTestSuite) TestEnsureWorkflowCompleted_AlreadyStored() {
	srv := newTestServer(s.T())

	execID := "ewc-already"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusSuccess,
		StartedAt: time.Now(),
	})

	srv.config.ExecutionStore.AppendEvent(context.Background(), execID, event.Event{
		Type:        event.WorkflowCompleted,
		ExecutionID: execID,
		Data:        map[string]any{"status": "success"},
	})

	ch := srv.config.EventBus.Subscribe(10)
	defer srv.config.EventBus.Unsubscribe(ch)

	result := &engine.ExecuteResult{Status: runtime.StatusSuccess}
	srv.ensureWorkflowCompleted(execID, result, nil, context.Background())

	events, _ := srv.config.ExecutionStore.GetEvents(context.Background(), execID)
	s.Len(events, 1)

	select {
	case <-ch:
		s.Fail("no event should be published when WorkflowCompleted already stored")
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *ServerTestSuite) TestEnsureWorkflowCompleted_ErrorCancelled() {
	srv := newTestServer(s.T())

	execID := "ewc-cancel"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	srv.ensureWorkflowCompleted(execID, nil, errors.New("context cancelled"), ctx)

	events, _ := srv.config.ExecutionStore.GetEvents(context.Background(), execID)
	s.Require().Len(events, 1)
	s.Equal(event.WorkflowCompleted, events[0].Type)
	s.Equal(runtime.StatusCancelled, events[0].Data["status"])
}

func (s *ServerTestSuite) TestEnsureWorkflowCompleted_ErrorFailed() {
	srv := newTestServer(s.T())

	execID := "ewc-fail"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	srv.ensureWorkflowCompleted(execID, nil, errors.New("exec failed"), context.Background())

	events, _ := srv.config.ExecutionStore.GetEvents(context.Background(), execID)
	s.Require().Len(events, 1)
	s.Equal(event.WorkflowCompleted, events[0].Type)
	s.Equal(runtime.StatusFailed, events[0].Data["status"])
}

func (s *ServerTestSuite) TestEnsureWorkflowCompleted_SuccessWithResult() {
	srv := newTestServer(s.T())

	execID := "ewc-result"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	result := &engine.ExecuteResult{Status: runtime.StatusCompletedWithErrors}
	srv.ensureWorkflowCompleted(execID, result, nil, context.Background())

	events, _ := srv.config.ExecutionStore.GetEvents(context.Background(), execID)
	s.Require().Len(events, 1)
	s.Equal(event.WorkflowCompleted, events[0].Type)
	s.Equal(runtime.StatusCompletedWithErrors, events[0].Data["status"])
}

func (s *ServerTestSuite) TestEnsureWorkflowCompleted_NilResultNilError() {
	srv := newTestServer(s.T())

	execID := "ewc-default"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	srv.ensureWorkflowCompleted(execID, nil, nil, context.Background())

	events, _ := srv.config.ExecutionStore.GetEvents(context.Background(), execID)
	s.Require().Len(events, 1)
	s.Equal(event.WorkflowCompleted, events[0].Type)
	s.Equal(runtime.StatusSuccess, events[0].Data["status"])
}

func (s *ServerTestSuite) TestEnsureWorkflowCompleted_PublishesEvent() {
	srv := newTestServer(s.T())

	execID := "ewc-publish"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	ch := srv.config.EventBus.Subscribe(10)
	defer srv.config.EventBus.Unsubscribe(ch)

	result := &engine.ExecuteResult{Status: runtime.StatusSuccess}
	srv.ensureWorkflowCompleted(execID, result, nil, context.Background())

	select {
	case ev := <-ch:
		s.Equal(event.WorkflowCompleted, ev.Type)
		s.Equal(execID, ev.ExecutionID)
		s.Equal(runtime.StatusSuccess, ev.Data["status"])
	case <-time.After(time.Second):
		s.Fail("timeout waiting for published event")
	}
}

func (s *ServerTestSuite) TestRecoverExecutions_ResumesIncompleteExecutions() {
	now := time.Now()
	srv := newTestServer(s.T())
	srv.config.Workflow.Recovery = true
	srv.config.Recoverer = &stubRecoverer{executions: []export.RecoveredExecution{{
		ExecutionID:  "exec-recovered",
		WorkflowName: "test-workflow",
		Status:       "running",
		Params:       map[string]any{"env": "staging"},
		Steps: map[string]*runtime.StepResult{
			"greet": {Status: "success", StartedAt: &now, FinishedAt: &now},
		},
	}}}
	srv.ctx = context.Background()

	var started atomic.Int32

	ch := srv.config.EventBus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.WorkflowStarted && ev.ExecutionID == "exec-recovered" {
				started.Add(1)
			}
		}
	}()

	srv.recoverExecutions(context.Background())

	s.Eventually(func() bool {
		return started.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond)
}

func (s *ServerTestSuite) TestRecoverExecutions_SkipsUnknownWorkflow() {
	srv := newTestServer(s.T())
	srv.config.Workflow.Recovery = true
	srv.config.Recoverer = &stubRecoverer{executions: []export.RecoveredExecution{{
		ExecutionID:  "exec-unknown",
		WorkflowName: "unknown-workflow",
		Status:       "running",
	}}}
	srv.ctx = context.Background()

	srv.recoverExecutions(context.Background())

	execs, _ := srv.config.ExecutionStore.List(context.Background())
	for _, exec := range execs {
		s.NotEqual("exec-unknown", exec.ID)
	}
}

func (s *ServerTestSuite) TestRecoverExecutions_NoopWhenRecoveryDisabled() {
	srv := newTestServer(s.T())
	srv.config.Workflow.Recovery = false
	srv.ctx = context.Background()

	srv.recoverExecutions(context.Background())

	allExecs, _ := srv.config.ExecutionStore.List(context.Background())
	s.Empty(allExecs)
}

func (s *ServerTestSuite) TestNewRedisKVStoreFn_DefaultDialFails() {
	_, err := newRedisKVStoreFn(context.Background(), "redis://localhost:59999")
	s.Require().Error(err)
}

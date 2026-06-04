package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

func (s *HandlersTestSuite) TestFinalizeExecution_NotFound() {
	srv := newTestServer(s.T())

	s.NotPanics(func() {
		srv.finalizeExecution("nonexistent-id", nil, nil, context.Background())
	})
}

// TestApplyStepEvent_AllTypes covers applyStepEvent – all event types.
func (s *HandlersTestSuite) TestApplyStepEvent_AllTypes() {
	srv := newTestServer(s.T())

	execID := "apply-test"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	srv.applyStepEvent(execID, event.Event{
		Type: event.StepStarted, StepID: "s1",
	})
	exec, _ := srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal(runtime.StatusRunning, exec.Steps["s1"].Status)

	srv.applyStepEvent(execID, event.Event{
		Type: event.StepWaiting, StepID: "s1",
	})
	exec, _ = srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal(runtime.StatusWaiting, exec.Steps["s1"].Status)

	srv.applyStepEvent(execID, event.Event{
		Type: event.StepInput, StepID: "s1",
		Data: map[string]any{"input_key": "input_val"},
	})
	exec, _ = srv.config.ExecutionStore.Get(context.Background(), execID)
	s.NotNil(exec.Steps["s1"].Input)

	srv.applyStepEvent(execID, event.Event{
		Type: event.StepCompleted, StepID: "s1",
		Data: map[string]any{"output": map[string]any{"result": 42}},
	})
	exec, _ = srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal(runtime.StatusSuccess, exec.Steps["s1"].Status)
	s.NotNil(exec.Steps["s1"].Output)

	srv.applyStepEvent(execID, event.Event{
		Type: event.StepFailed, StepID: "s2", Message: "boom",
	})
	exec, _ = srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal(runtime.StatusFailed, exec.Steps["s2"].Status)
	s.Equal("boom", exec.Steps["s2"].Error.Message)

	srv.applyStepEvent(execID, event.Event{
		Type: event.StepSkipped, StepID: "s3",
	})
	exec, _ = srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal(runtime.StatusSkipped, exec.Steps["s3"].Status)

	srv.applyStepEvent(execID, event.Event{
		Type: event.StepOutput, StepID: "s1",
		Data: map[string]any{"output": "new-output"},
	})
	exec, _ = srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal("new-output", exec.Steps["s1"].Output)

	srv.applyStepEvent(execID, event.Event{Type: event.Metrics, StepID: "s1"})
	srv.applyStepEvent(execID, event.Event{Type: event.WorkflowStarted, StepID: "s1"})
	srv.applyStepEvent(execID, event.Event{Type: event.StepLog, StepID: "s1"})
	srv.applyStepEvent(execID, event.Event{Type: event.StepGoto, StepID: "s1"})
}

// TestApplyWorkflowCompleted_ValidStatus covers applyWorkflowCompleted.
func (s *HandlersTestSuite) TestApplyWorkflowCompleted_ValidStatus() {
	srv := newTestServer(s.T())

	execID := "wfc-1"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	ts := time.Now()
	srv.applyWorkflowCompleted(execID, event.Event{
		Type:      event.WorkflowCompleted,
		Timestamp: ts,
		Data:      map[string]any{"status": "success"},
	})

	exec, _ := srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal(runtime.StatusSuccess, exec.Status)
	s.NotNil(exec.FinishedAt)
}

func (s *HandlersTestSuite) TestApplyWorkflowCompleted_NoStatus() {
	srv := newTestServer(s.T())

	execID := "wfc-2"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	srv.applyWorkflowCompleted(execID, event.Event{
		Type: event.WorkflowCompleted,
		Data: map[string]any{},
	})

	exec, _ := srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal(runtime.StatusRunning, exec.Status)
}

// TestApplyStepEvent_WorkflowCompleted covers applyStepEvent – WorkflowCompleted dispatches to applyWorkflowCompleted.
func (s *HandlersTestSuite) TestApplyStepEvent_WorkflowCompleted() {
	srv := newTestServer(s.T())

	execID := "wfc-dispatch"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	ts := time.Now()
	srv.applyStepEvent(execID, event.Event{
		Type:      event.WorkflowCompleted,
		StepID:    "s1",
		Timestamp: ts,
		Data:      map[string]any{"status": "failed"},
	})

	exec, _ := srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal(runtime.StatusFailed, exec.Status)
}

// TestProcessEvent_StepGotoResetsBody covers processEvent – StepGoto with body resets pending.
func (s *HandlersTestSuite) TestProcessEvent_StepGotoResetsBody() {
	srv := newTestServer(s.T())

	execID := "pe-goto"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
		Steps: map[string]*runtime.StepResult{
			"step_b": {Status: runtime.StatusSuccess},
		},
	})

	lt := &loopTracker{}

	completedSeen := false
	srv.processEvent(execID, event.Event{
		Type:        event.StepGoto,
		ExecutionID: execID,
		StepID:      "step_a",
		Data: map[string]any{
			"iteration": float64(1),
			"body":      []any{"step_b"},
		},
	}, lt, &completedSeen)

	srv.processEvent(execID, event.Event{
		Type:        event.StepGoto,
		ExecutionID: execID,
		StepID:      "step_a",
		Data: map[string]any{
			"iteration": float64(2),
			"body":      []any{"step_b"},
		},
	}, lt, &completedSeen)

	exec, _ := srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal("pending", exec.Steps["step_b"].Status)
}

// TestProcessEvent_EmptyStepID covers processEvent – event with empty StepID (no applyStepEvent call).
func (s *HandlersTestSuite) TestProcessEvent_EmptyStepID() {
	srv := newTestServer(s.T())

	execID := "pe-empty"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	lt := &loopTracker{}
	completedSeen := false

	srv.processEvent(execID, event.Event{
		Type:        event.WorkflowStarted,
		ExecutionID: execID,
	}, lt, &completedSeen)
}

// TestProcessEvent_LoopBodyEventSkipped covers processEvent – loop body event after iteration 1 is skipped from event store.
func (s *HandlersTestSuite) TestProcessEvent_LoopBodyEventSkipped() {
	srv := newTestServer(s.T())

	execID := "pe-loop"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	lt := &loopTracker{}
	completedSeen := false

	lt.Track(event.Event{
		Type: event.StepGoto,
		Data: map[string]any{
			"iteration": float64(2),
			"body":      []any{"step_b"},
		},
	})

	srv.processEvent(execID, event.Event{
		Type:        event.StepStarted,
		ExecutionID: execID,
		StepID:      "step_b",
	}, lt, &completedSeen)

	events, _ := srv.config.ExecutionStore.GetEvents(context.Background(), execID)
	s.Len(events, 0)
}

// TestCaptureEvents_IgnoresOtherExecution covers captureEvents – events for a different executionID are ignored.
func (s *HandlersTestSuite) TestCaptureEvents_IgnoresOtherExecution() {
	srv := newTestServer(s.T())

	execID := "cap-1"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	stopCapture := srv.captureEvents(execID)

	srv.config.EventBus.Publish(event.Event{
		Type:        event.StepStarted,
		ExecutionID: "other-exec",
		StepID:      "s1",
	})

	time.Sleep(50 * time.Millisecond)

	stopCapture()

	events, _ := srv.config.ExecutionStore.GetEvents(context.Background(), execID)
	s.Len(events, 0)
}

// TestFinalizeExecution_ErrorCancelled covers finalizeExecution – error with cancelled context.
func (s *HandlersTestSuite) TestFinalizeExecution_ErrorCancelled() {
	srv := newTestServer(s.T())

	execID := "fin-cancel"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	srv.finalizeExecution(execID, nil, errors.New("context cancelled"), ctx)

	exec, _ := srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal(runtime.StatusCancelled, exec.Status)
	s.Equal("execution cancelled", exec.Error)
	s.NotNil(exec.FinishedAt)
}

// TestFinalizeExecution_ErrorNotCancelled covers finalizeExecution – error without cancelled context.
func (s *HandlersTestSuite) TestFinalizeExecution_ErrorNotCancelled() {
	srv := newTestServer(s.T())

	execID := "fin-err"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	srv.finalizeExecution(execID, nil, errors.New("something failed"), context.Background())

	exec, _ := srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal(runtime.StatusFailed, exec.Status)
	s.Equal("something failed", exec.Error)
	s.NotNil(exec.FinishedAt)
}

// TestFinalizeExecution_SuccessWithResultError covers finalizeExecution – success with result error.
func (s *HandlersTestSuite) TestFinalizeExecution_SuccessWithResultError() {
	srv := newTestServer(s.T())

	execID := "fin-res-err"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	finishedAt := time.Now()
	result := &engine.ExecuteResult{
		Status:     runtime.StatusCompletedWithErrors,
		FinishedAt: finishedAt,
		Error:      fmt.Errorf("partial failure"),
		Steps: map[string]*runtime.StepResult{
			"s1": {Status: runtime.StatusSuccess, Output: "ok"},
		},
	}

	srv.finalizeExecution(execID, result, nil, context.Background())

	exec, _ := srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal(runtime.StatusCompletedWithErrors, exec.Status)
	s.Equal("partial failure", exec.Error)
	s.NotNil(exec.FinishedAt)
	s.NotNil(exec.Steps["s1"])
}

// TestFinalizeExecution_SuccessWithSteps covers finalizeExecution – success with steps merge.
func (s *HandlersTestSuite) TestFinalizeExecution_SuccessWithSteps() {
	srv := newTestServer(s.T())

	execID := "fin-ok"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
		Steps: map[string]*runtime.StepResult{
			"s1": {Status: runtime.StatusRunning, Input: "input-data"},
		},
	})

	finishedAt := time.Now()
	started := time.Now().Add(-1 * time.Second)
	result := &engine.ExecuteResult{
		Status:     runtime.StatusSuccess,
		FinishedAt: finishedAt,
		Steps: map[string]*runtime.StepResult{
			"s1": {Status: runtime.StatusSuccess, Output: "result", StartedAt: &started, FinishedAt: &finishedAt},
			"s2": {Status: runtime.StatusSuccess, Output: "new-step"},
		},
	}

	srv.finalizeExecution(execID, result, nil, context.Background())

	exec, _ := srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal(runtime.StatusSuccess, exec.Status)
	s.Equal("input-data", exec.Steps["s1"].Input)
	s.Equal("result", exec.Steps["s1"].Output)
	s.Equal("new-step", exec.Steps["s2"].Output)
}

// TestMergeStepResults_NilEngineSteps covers mergeStepResults – nil engineSteps.
func (s *HandlersTestSuite) TestMergeStepResults_NilEngineSteps() {
	exec := &store.Execution{
		Steps: map[string]*runtime.StepResult{
			"s1": {Status: runtime.StatusRunning},
		},
	}

	mergeStepResults(exec, nil)
	s.Equal(runtime.StatusRunning, exec.Steps["s1"].Status)
}

// TestMergeStepResults_NilExecSteps covers mergeStepResults – nil exec.Steps.
func (s *HandlersTestSuite) TestMergeStepResults_NilExecSteps() {
	exec := &store.Execution{}

	engineSteps := map[string]*runtime.StepResult{
		"s1": {Status: runtime.StatusSuccess, Output: "hello"},
	}

	mergeStepResults(exec, engineSteps)
	s.Equal(engineSteps, exec.Steps)
}

// TestMergeStepResults_MergeExisting covers mergeStepResults – merge with existing.
func (s *HandlersTestSuite) TestMergeStepResults_MergeExisting() {
	exec := &store.Execution{
		Steps: map[string]*runtime.StepResult{
			"s1": {Status: runtime.StatusRunning, Input: "preserved"},
		},
	}

	started := time.Now()
	finished := started.Add(1 * time.Second)
	engineSteps := map[string]*runtime.StepResult{
		"s1": {Status: runtime.StatusSuccess, Output: "out", StartedAt: &started, FinishedAt: &finished},
		"s2": {Status: runtime.StatusSuccess},
	}

	mergeStepResults(exec, engineSteps)
	s.Equal(runtime.StatusSuccess, exec.Steps["s1"].Status)
	s.Equal("out", exec.Steps["s1"].Output)
	s.Equal("preserved", exec.Steps["s1"].Input)
	s.NotNil(exec.Steps["s2"])
}

// TestProcessEvent_DuplicateWorkflowCompleted covers processEvent – duplicate WorkflowCompleted is deduplicated (completedSeen branch).
func (s *HandlersTestSuite) TestProcessEvent_DuplicateWorkflowCompleted() {
	srv := newTestServer(s.T())

	execID := "pe-dedup"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	lt := &loopTracker{}
	completedSeen := false

	srv.processEvent(execID, event.Event{
		Type:        event.WorkflowCompleted,
		ExecutionID: execID,
		Data:        map[string]any{"status": "success"},
	}, lt, &completedSeen)

	s.True(completedSeen)
	events, _ := srv.config.ExecutionStore.GetEvents(context.Background(), execID)
	s.Len(events, 1)

	srv.processEvent(execID, event.Event{
		Type:        event.WorkflowCompleted,
		ExecutionID: execID,
		Data:        map[string]any{"status": "success"},
	}, lt, &completedSeen)

	events, _ = srv.config.ExecutionStore.GetEvents(context.Background(), execID)
	s.Len(events, 1)
}

// TestCaptureEvents_FullLifecycle covers captureEvents full lifecycle.
func (s *HandlersTestSuite) TestCaptureEvents_FullLifecycle() {
	srv := newTestServer(s.T())

	execID := "cap-full"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	stopCapture := srv.captureEvents(execID)

	srv.config.EventBus.Publish(event.Event{
		Type:        event.StepStarted,
		ExecutionID: execID,
		StepID:      "s1",
	})
	srv.config.EventBus.Publish(event.Event{
		Type:        event.StepCompleted,
		ExecutionID: execID,
		StepID:      "s1",
		Data:        map[string]any{"output": "done"},
	})

	time.Sleep(50 * time.Millisecond)

	stopCapture()

	events, _ := srv.config.ExecutionStore.GetEvents(context.Background(), execID)
	s.Len(events, 2)

	exec, _ := srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Equal(runtime.StatusSuccess, exec.Steps["s1"].Status)
}

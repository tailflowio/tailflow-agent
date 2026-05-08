package server

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

func (s *Server) buildActionServices() *runtime.ActionServices {
	return &runtime.ActionServices{
		WaitWebhookRegister:  s.waitRegistry.Register,
		WaitRabbitMQRegister: s.rmqWaitMgr.Register,
		EmitWaiting: func(executionID, stepID string, waitType string, details map[string]any) {
			data := map[string]any{"wait_type": waitType}
			for k, v := range details {
				data[k] = v
			}

			s.config.EventBus.Publish(event.Event{
				Type:        event.StepWaiting,
				Timestamp:   time.Now(),
				ExecutionID: executionID,
				StepID:      stepID,
				Message:     fmt.Sprintf("step %q waiting for %s", stepID, waitType),
				Data:        data,
			})
		},
		ScheduleExecution: func(delay time.Duration, params map[string]any) (string, error) {
			executionID := uuid.New().String()
			exec := &store.Execution{
				ID:           executionID,
				WorkflowName: s.config.Workflow.Name,
				Status:       runtime.StatusScheduled,
				Params:       params,
				StartedAt:    time.Now(),
			}
			s.config.ExecutionStore.Add(exec)

			timer := time.AfterFunc(delay, func() {
				s.runWorkflowAsync(params)
			})
			s.addScheduledTimer(timer)

			return executionID, nil
		},
		Locker:     s.locker,
		DBPool:     s.dbPool,
		KVStore:    s.kvStore,
		TxRegistry: runtime.NewMemoryTxRegistry(s.config.Logger),
	}
}

func (s *Server) captureEvents(executionID string) func() {
	// Blocking subscription: the store is the source of truth for the API,
	// so we can't afford dropped events. The buffer is generous (10k) to
	// absorb bursts without back-pressuring the engine in practice.
	ch := s.config.EventBus.SubscribeBlocking(10_000)
	done := make(chan struct{})

	go func() {
		defer close(done)

		lt := &loopTracker{}
		completedSeen := false

		for ev := range ch {
			if ev.ExecutionID != executionID {
				continue
			}

			s.processEvent(executionID, ev, lt, &completedSeen)
		}
	}()

	return func() {
		s.config.EventBus.Unsubscribe(ch)
		<-done // wait for goroutine to drain
	}
}

func (s *Server) processEvent(executionID string, ev event.Event, lt *loopTracker, completedSeen *bool) {
	lt.Track(ev)

	// Deduplicate workflow.completed — only store the first one
	if ev.Type == event.WorkflowCompleted {
		if *completedSeen {
			return
		}

		*completedSeen = true
	}

	// Reset body steps to pending on goto so dashboard stays coherent during loops
	if ev.Type == event.StepGoto && lt.Body != nil {
		for sid := range lt.Body {
			s.config.ExecutionStore.UpdateStep(executionID, sid, func(r *runtime.StepResult) {
				r.Status = "pending"
			})
		}
	}

	// Skip loop body events after iteration 1 to prevent unbounded memory growth
	if !lt.InLoop(ev) {
		s.config.ExecutionStore.AppendEvent(executionID, ev)
	}

	if ev.StepID == "" {
		return
	}

	s.applyStepEvent(executionID, ev)
}

func (s *Server) applyStepEvent(executionID string, ev event.Event) {
	switch ev.Type {
	case event.StepStarted:
		s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
			r.Status = runtime.StatusRunning
		})
		s.config.ExecutionStore.IncrStepExecCount(ev.StepID)
	case event.StepWaiting:
		s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
			r.Status = runtime.StatusWaiting
		})
	case event.StepInput:
		s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
			r.Input = ev.Data
		})
	case event.StepCompleted:
		s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
			r.Status = runtime.StatusSuccess

			o, ok := ev.Data["output"]
			if ok {
				r.Output = o
			}
		})
	case event.StepFailed:
		s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
			r.Status = runtime.StatusFailed
			r.Error = &runtime.StepError{Message: ev.Message, Code: "action_failed", StepID: ev.StepID}
		})
	case event.StepSkipped:
		s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
			r.Status = runtime.StatusSkipped
		})
	case event.StepOutput:
		s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
			o, ok := ev.Data["output"]
			if ok {
				r.Output = o
			}
		})
	case event.WorkflowCompleted:
		s.applyWorkflowCompleted(executionID, ev)
	case event.Metrics, event.WorkflowStarted, event.StepLog, event.StepGoto, event.ExecutionState, event.ExecutionGroup:
	}
}

func (s *Server) applyWorkflowCompleted(executionID string, ev event.Event) {
	statusStr, ok := ev.Data["status"].(string)
	if !ok {
		return
	}

	ts := ev.Timestamp

	s.config.ExecutionStore.UpdateExecution(executionID, func(exec *store.Execution) {
		exec.Status = statusStr
		exec.FinishedAt = &ts
	})
}

func (s *Server) finalizeExecution(
	executionID string, result *engine.ExecuteResult, err error, execCtx context.Context,
) {
	s.config.ExecutionStore.UpdateExecution(executionID, func(exec *store.Execution) {
		switch {
		case err != nil && execCtx.Err() != nil:
			exec.Status = runtime.StatusCancelled
			exec.Error = "execution cancelled"
		case err != nil:
			exec.Status = runtime.StatusFailed
			exec.Error = err.Error()
		default:
			exec.Status = result.Status
			mergeStepResults(exec, result.Steps)

			now := result.FinishedAt
			exec.FinishedAt = &now

			if result.Error != nil {
				exec.Error = result.Error.Error()
			}
		}

		now := time.Now()
		if exec.FinishedAt == nil {
			exec.FinishedAt = &now
		}
	})
}

func mergeStepResults(exec *store.Execution, engineSteps map[string]*runtime.StepResult) {
	if engineSteps == nil {
		return
	}

	if exec.Steps == nil {
		exec.Steps = engineSteps
		return
	}

	for id, sr := range engineSteps {
		existing, ok := exec.Steps[id]
		if !ok {
			exec.Steps[id] = sr
			continue
		}
		// Keep Input from event tracking, take everything else from engine
		existing.Status = sr.Status
		existing.Output = sr.Output
		existing.Error = sr.Error
		existing.StartedAt = sr.StartedAt
		existing.FinishedAt = sr.FinishedAt
	}
}

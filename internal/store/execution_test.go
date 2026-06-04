package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
)

type ExecutionStoreTestSuite struct {
	suite.Suite

	ctx context.Context
}

func TestExecutionStore(t *testing.T) {
	suite.Run(t, new(ExecutionStoreTestSuite))
}

func (s *ExecutionStoreTestSuite) SetupTest() {
	s.ctx = context.Background()
}

func (s *ExecutionStoreTestSuite) TestAddAndGet() {
	st := NewExecutionStore(10)

	exec := &Execution{
		ID:           "exec-1",
		WorkflowName: "test-wf",
		Status:       "running",
		StartedAt:    time.Now(),
	}
	s.Require().NoError(st.Add(s.ctx, exec))

	got, err := st.Get(s.ctx, "exec-1")
	s.Require().NoError(err)
	s.Equal("exec-1", got.ID)
	s.Equal("running", got.Status)
}

func (s *ExecutionStoreTestSuite) TestNotFound() {
	st := NewExecutionStore(10)
	_, err := st.Get(s.ctx, "nonexistent")
	s.Error(err)
	s.True(errors.Is(err, ErrNotFound), "error should wrap ErrNotFound")
}

func (s *ExecutionStoreTestSuite) TestList_ReturnsReverseOrder() {
	st := NewExecutionStore(10)

	for i := 0; i < 5; i++ {
		s.Require().NoError(st.Add(s.ctx, &Execution{
			ID:           fmt.Sprintf("exec-%d", i),
			WorkflowName: "test",
			Status:       "success",
			StartedAt:    time.Now(),
		}))
	}

	list, err := st.List(s.ctx)
	s.Require().NoError(err)
	s.Len(list, 5)
	// Most recent first
	s.Equal("exec-4", list[0].ID)
	s.Equal("exec-0", list[4].ID)
}

func (s *ExecutionStoreTestSuite) TestRingBuffer() {
	st := NewExecutionStore(3) // Only 3 slots

	for i := 0; i < 5; i++ {
		s.Require().NoError(st.Add(s.ctx, &Execution{
			ID:           fmt.Sprintf("exec-%d", i),
			WorkflowName: "test",
			Status:       "success",
			StartedAt:    time.Now(),
		}))
	}

	count, err := st.Count(s.ctx)
	s.Require().NoError(err)
	s.Equal(3, count)

	// exec-0 and exec-1 should be evicted
	_, err = st.Get(s.ctx, "exec-0")
	s.Error(err)
	_, err = st.Get(s.ctx, "exec-1")
	s.Error(err)

	// exec-2, exec-3, exec-4 should exist
	_, err = st.Get(s.ctx, "exec-2")
	s.NoError(err)
	_, err = st.Get(s.ctx, "exec-3")
	s.NoError(err)
	_, err = st.Get(s.ctx, "exec-4")
	s.NoError(err)
}

func (s *ExecutionStoreTestSuite) TestUpdate_StatusAndFinishedAt() {
	st := NewExecutionStore(10)

	exec := &Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       "running",
		StartedAt:    time.Now(),
	}
	s.Require().NoError(st.Add(s.ctx, exec))

	now := time.Now()
	s.Require().NoError(st.Update(s.ctx, &Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       "success",
		StartedAt:    exec.StartedAt,
		FinishedAt:   &now,
	}))

	got, _ := st.Get(s.ctx, "exec-1")
	s.Equal("success", got.Status)
	s.NotNil(got.FinishedAt)
}

func (s *ExecutionStoreTestSuite) TestDefaultCapacity() {
	st := NewExecutionStore(0)
	s.Equal(100, st.capacity)
}

func (s *ExecutionStoreTestSuite) TestUpdateExecution() {
	st := NewExecutionStore(10)
	exec := &Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       "running",
		StartedAt:    time.Now(),
	}
	s.Require().NoError(st.Add(s.ctx, exec))

	err := st.UpdateExecution(s.ctx, "exec-1", func(e *Execution) {
		e.Status = "success"
	})
	s.Require().NoError(err)

	got, err := st.Get(s.ctx, "exec-1")
	s.Require().NoError(err)
	s.Equal("success", got.Status)
}

func (s *ExecutionStoreTestSuite) TestUpdateExecution_NotFound() {
	st := NewExecutionStore(10)
	// Should not panic and should not error when target is missing.
	s.NotPanics(func() {
		err := st.UpdateExecution(s.ctx, "nonexistent", func(e *Execution) {
			e.Status = "success"
		})
		s.NoError(err)
	}, "UpdateExecution with nonexistent ID should not panic")
}

func (s *ExecutionStoreTestSuite) TestAppendAndGetEvents() {
	st := NewExecutionStore(10)
	s.Require().NoError(st.Add(s.ctx, &Execution{ID: "exec-1", WorkflowName: "test", Status: "running", StartedAt: time.Now()}))

	ev1 := event.Event{Type: event.StepStarted, ExecutionID: "exec-1", StepID: "step1"}
	ev2 := event.Event{Type: event.StepCompleted, ExecutionID: "exec-1", StepID: "step1"}

	s.Require().NoError(st.AppendEvent(s.ctx, "exec-1", ev1))
	s.Require().NoError(st.AppendEvent(s.ctx, "exec-1", ev2))

	events, err := st.GetEvents(s.ctx, "exec-1")
	s.Require().NoError(err)
	s.Len(events, 2)
	s.Equal(event.StepStarted, events[0].Type)
	s.Equal(event.StepCompleted, events[1].Type)
}

func (s *ExecutionStoreTestSuite) TestGetEvents_Empty() {
	st := NewExecutionStore(10)
	events, err := st.GetEvents(s.ctx, "nonexistent")
	s.Require().NoError(err)
	s.Nil(events)
}

func (s *ExecutionStoreTestSuite) TestGetEvents_ReturnsCopy() {
	st := NewExecutionStore(10)
	s.Require().NoError(st.AppendEvent(s.ctx, "exec-1", event.Event{Type: event.StepStarted, ExecutionID: "exec-1"}))

	events1, err := st.GetEvents(s.ctx, "exec-1")
	s.Require().NoError(err)
	events2, err := st.GetEvents(s.ctx, "exec-1")
	s.Require().NoError(err)
	s.Len(events1, 1)
	s.Len(events2, 1)
	// Modifying returned slice should not affect the store
	events1[0].StepID = "modified"
	events2Again, err := st.GetEvents(s.ctx, "exec-1")
	s.Require().NoError(err)
	s.Empty(events2Again[0].StepID)
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_NewStep() {
	st := NewExecutionStore(10)
	s.Require().NoError(st.Add(s.ctx, &Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusRunning,
		StartedAt:    time.Now(),
	}))

	err := st.UpdateStep(s.ctx, "exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusRunning
	})
	s.Require().NoError(err)

	got, _ := st.Get(s.ctx, "exec-1")
	s.NotNil(got.Steps["step1"])
	s.Equal(runtime.StatusRunning, got.Steps["step1"].Status)
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_ExistingStep() {
	st := NewExecutionStore(10)
	s.Require().NoError(st.Add(s.ctx, &Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusRunning,
		Steps:        map[string]*runtime.StepResult{"step1": {Status: runtime.StatusRunning}},
		StartedAt:    time.Now(),
	}))

	err := st.UpdateStep(s.ctx, "exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusSuccess
		step.Output = "done"
	})
	s.Require().NoError(err)

	got, _ := st.Get(s.ctx, "exec-1")
	s.Equal(runtime.StatusSuccess, got.Steps["step1"].Status)
	s.Equal("done", got.Steps["step1"].Output)
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_AutoDerivesRunning() {
	st := NewExecutionStore(10)
	s.Require().NoError(st.Add(s.ctx, &Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusWaiting,
		StartedAt:    time.Now(),
	}))

	s.Require().NoError(st.UpdateStep(s.ctx, "exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusRunning
	}))

	got, _ := st.Get(s.ctx, "exec-1")
	s.Equal(runtime.StatusRunning, got.Status)
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_AutoDerivesWaiting() {
	st := NewExecutionStore(10)
	s.Require().NoError(st.Add(s.ctx, &Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusRunning,
		StartedAt:    time.Now(),
	}))

	s.Require().NoError(st.UpdateStep(s.ctx, "exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusWaiting
	}))

	got, _ := st.Get(s.ctx, "exec-1")
	s.Equal(runtime.StatusWaiting, got.Status)
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_NoAutoDerive_TerminalStatus() {
	st := NewExecutionStore(10)
	s.Require().NoError(st.Add(s.ctx, &Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusSuccess,
		StartedAt:    time.Now(),
	}))

	s.Require().NoError(st.UpdateStep(s.ctx, "exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusRunning
	}))

	got, _ := st.Get(s.ctx, "exec-1")
	// Should NOT change from success to running
	s.Equal(runtime.StatusSuccess, got.Status)
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_NotFound() {
	st := NewExecutionStore(10)
	// Should not panic when updating a step on a nonexistent execution
	s.NotPanics(func() {
		err := st.UpdateStep(s.ctx, "nonexistent", "step1", func(step *runtime.StepResult) {
			step.Status = runtime.StatusRunning
		})
		s.NoError(err)
	}, "UpdateStep with nonexistent execution should not panic")
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_NilStepsMap() {
	st := NewExecutionStore(10)
	s.Require().NoError(st.Add(s.ctx, &Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusRunning,
		StartedAt:    time.Now(),
		Steps:        nil,
	}))

	s.Require().NoError(st.UpdateStep(s.ctx, "exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusRunning
	}))

	got, _ := st.Get(s.ctx, "exec-1")
	s.NotNil(got.Steps)
	s.Equal(runtime.StatusRunning, got.Steps["step1"].Status)
}

func (s *ExecutionStoreTestSuite) TestCapacity() {
	st := NewExecutionStore(42)
	s.Equal(42, st.Capacity())
}

func (s *ExecutionStoreTestSuite) TestGetEventsPaginated_Normal() {
	st := NewExecutionStore(10)

	for i := 0; i < 5; i++ {
		ev := event.Event{Type: event.StepStarted, ExecutionID: "exec-1", StepID: fmt.Sprintf("step%d", i)}
		s.Require().NoError(st.AppendEvent(s.ctx, "exec-1", ev))
	}

	evts, total, err := st.GetEventsPaginated(s.ctx, "exec-1", 1, 2)
	s.Require().NoError(err)
	s.Equal(5, total)
	s.Len(evts, 2)
	s.Equal("step1", evts[0].StepID)
	s.Equal("step2", evts[1].StepID)
}

func (s *ExecutionStoreTestSuite) TestGetEventsPaginated_OffsetBeyondTotal() {
	st := NewExecutionStore(10)

	s.Require().NoError(st.AppendEvent(s.ctx, "exec-1", event.Event{Type: event.StepStarted, ExecutionID: "exec-1"}))

	evts, total, err := st.GetEventsPaginated(s.ctx, "exec-1", 10, 5)
	s.Require().NoError(err)
	s.Equal(1, total)
	s.Nil(evts)
}

func (s *ExecutionStoreTestSuite) TestGetEventsPaginated_LimitClampedToEnd() {
	st := NewExecutionStore(10)

	for i := 0; i < 3; i++ {
		s.Require().NoError(st.AppendEvent(s.ctx, "exec-1", event.Event{
			Type:        event.StepStarted,
			ExecutionID: "exec-1",
			StepID:      fmt.Sprintf("step%d", i),
		}))
	}

	evts, total, err := st.GetEventsPaginated(s.ctx, "exec-1", 2, 100)
	s.Require().NoError(err)
	s.Equal(3, total)
	s.Len(evts, 1)
	s.Equal("step2", evts[0].StepID)
}

func (s *ExecutionStoreTestSuite) TestComputeStepMetrics_ExportedWrapper() {
	now := time.Now()
	started := now.Add(-50 * time.Millisecond)

	execs := []*Execution{
		{
			ID:           "exec-1",
			WorkflowName: "test",
			Status:       runtime.StatusSuccess,
			StartedAt:    now,
			Steps: map[string]*runtime.StepResult{
				"step1": {
					Status:     runtime.StatusSuccess,
					StartedAt:  &started,
					FinishedAt: &now,
				},
			},
		},
	}

	metrics := ComputeStepMetrics(execs)
	s.Require().NotNil(metrics["step1"])
	s.Equal(1, metrics["step1"].TotalExecutions)
	s.Equal(1, metrics["step1"].SuccessCount)
}


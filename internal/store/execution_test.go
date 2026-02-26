package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
)

type ExecutionStoreTestSuite struct {
	suite.Suite
}

func TestExecutionStore(t *testing.T) {
	suite.Run(t, new(ExecutionStoreTestSuite))
}

func (s *ExecutionStoreTestSuite) SetupTest() {
	// required by convention
}

func (s *ExecutionStoreTestSuite) TestAddAndGet() {
	st := NewExecutionStore(10)

	exec := &Execution{
		ID:           "exec-1",
		WorkflowName: "test-wf",
		Status:       "running",
		StartedAt:    time.Now(),
	}
	st.Add(exec)

	got, err := st.Get("exec-1")
	s.Require().NoError(err)
	s.Equal("exec-1", got.ID)
	s.Equal("running", got.Status)
}

func (s *ExecutionStoreTestSuite) TestNotFound() {
	st := NewExecutionStore(10)
	_, err := st.Get("nonexistent")
	s.Error(err)
}

func (s *ExecutionStoreTestSuite) TestList_ReturnsReverseOrder() {
	st := NewExecutionStore(10)

	for i := 0; i < 5; i++ {
		st.Add(&Execution{
			ID:           fmt.Sprintf("exec-%d", i),
			WorkflowName: "test",
			Status:       "success",
			StartedAt:    time.Now(),
		})
	}

	list := st.List()
	s.Len(list, 5)
	// Most recent first
	s.Equal("exec-4", list[0].ID)
	s.Equal("exec-0", list[4].ID)
}

func (s *ExecutionStoreTestSuite) TestRingBuffer() {
	st := NewExecutionStore(3) // Only 3 slots

	for i := 0; i < 5; i++ {
		st.Add(&Execution{
			ID:           fmt.Sprintf("exec-%d", i),
			WorkflowName: "test",
			Status:       "success",
			StartedAt:    time.Now(),
		})
	}

	s.Equal(3, st.Count())

	// exec-0 and exec-1 should be evicted
	_, err := st.Get("exec-0")
	s.Error(err)
	_, err = st.Get("exec-1")
	s.Error(err)

	// exec-2, exec-3, exec-4 should exist
	_, err = st.Get("exec-2")
	s.NoError(err)
	_, err = st.Get("exec-3")
	s.NoError(err)
	_, err = st.Get("exec-4")
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
	st.Add(exec)

	now := time.Now()
	st.Update(&Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       "success",
		StartedAt:    exec.StartedAt,
		FinishedAt:   &now,
	})

	got, _ := st.Get("exec-1")
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
	st.Add(exec)

	st.UpdateExecution("exec-1", func(e *Execution) {
		e.Status = "success"
	})

	got, err := st.Get("exec-1")
	s.Require().NoError(err)
	s.Equal("success", got.Status)
}

func (s *ExecutionStoreTestSuite) TestUpdateExecution_NotFound() {
	st := NewExecutionStore(10)
	// Should not panic when updating a nonexistent execution
	s.NotPanics(func() {
		st.UpdateExecution("nonexistent", func(e *Execution) {
			e.Status = "success"
		})
	}, "UpdateExecution with nonexistent ID should not panic")
}

func (s *ExecutionStoreTestSuite) TestAppendAndGetEvents() {
	st := NewExecutionStore(10)
	st.Add(&Execution{ID: "exec-1", WorkflowName: "test", Status: "running", StartedAt: time.Now()})

	ev1 := event.Event{Type: event.StepStarted, ExecutionID: "exec-1", StepID: "step1"}
	ev2 := event.Event{Type: event.StepCompleted, ExecutionID: "exec-1", StepID: "step1"}

	st.AppendEvent("exec-1", ev1)
	st.AppendEvent("exec-1", ev2)

	events := st.GetEvents("exec-1")
	s.Len(events, 2)
	s.Equal(event.StepStarted, events[0].Type)
	s.Equal(event.StepCompleted, events[1].Type)
}

func (s *ExecutionStoreTestSuite) TestGetEvents_Empty() {
	st := NewExecutionStore(10)
	events := st.GetEvents("nonexistent")
	s.Nil(events)
}

func (s *ExecutionStoreTestSuite) TestGetEvents_ReturnsCopy() {
	st := NewExecutionStore(10)
	st.AppendEvent("exec-1", event.Event{Type: event.StepStarted, ExecutionID: "exec-1"})

	events1 := st.GetEvents("exec-1")
	events2 := st.GetEvents("exec-1")
	s.Len(events1, 1)
	s.Len(events2, 1)
	// Modifying returned slice should not affect the store
	events1[0].StepID = "modified"
	events2Again := st.GetEvents("exec-1")
	s.Empty(events2Again[0].StepID)
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_NewStep() {
	st := NewExecutionStore(10)
	st.Add(&Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusRunning,
		StartedAt:    time.Now(),
	})

	st.UpdateStep("exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusRunning
	})

	got, _ := st.Get("exec-1")
	s.NotNil(got.Steps["step1"])
	s.Equal(runtime.StatusRunning, got.Steps["step1"].Status)
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_ExistingStep() {
	st := NewExecutionStore(10)
	st.Add(&Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusRunning,
		Steps:        map[string]*runtime.StepResult{"step1": {Status: runtime.StatusRunning}},
		StartedAt:    time.Now(),
	})

	st.UpdateStep("exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusSuccess
		step.Output = "done"
	})

	got, _ := st.Get("exec-1")
	s.Equal(runtime.StatusSuccess, got.Steps["step1"].Status)
	s.Equal("done", got.Steps["step1"].Output)
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_AutoDerivesRunning() {
	st := NewExecutionStore(10)
	st.Add(&Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusWaiting,
		StartedAt:    time.Now(),
	})

	st.UpdateStep("exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusRunning
	})

	got, _ := st.Get("exec-1")
	s.Equal(runtime.StatusRunning, got.Status)
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_AutoDerivesWaiting() {
	st := NewExecutionStore(10)
	st.Add(&Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusRunning,
		StartedAt:    time.Now(),
	})

	st.UpdateStep("exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusWaiting
	})

	got, _ := st.Get("exec-1")
	s.Equal(runtime.StatusWaiting, got.Status)
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_NoAutoDerive_TerminalStatus() {
	st := NewExecutionStore(10)
	st.Add(&Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusSuccess,
		StartedAt:    time.Now(),
	})

	st.UpdateStep("exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusRunning
	})

	got, _ := st.Get("exec-1")
	// Should NOT change from success to running
	s.Equal(runtime.StatusSuccess, got.Status)
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_NotFound() {
	st := NewExecutionStore(10)
	// Should not panic when updating a step on a nonexistent execution
	s.NotPanics(func() {
		st.UpdateStep("nonexistent", "step1", func(step *runtime.StepResult) {
			step.Status = runtime.StatusRunning
		})
	}, "UpdateStep with nonexistent execution should not panic")
}

func (s *ExecutionStoreTestSuite) TestUpdateStep_NilStepsMap() {
	st := NewExecutionStore(10)
	st.Add(&Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusRunning,
		StartedAt:    time.Now(),
		Steps:        nil,
	})

	st.UpdateStep("exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusRunning
	})

	got, _ := st.Get("exec-1")
	s.NotNil(got.Steps)
	s.Equal(runtime.StatusRunning, got.Steps["step1"].Status)
}

func (s *ExecutionStoreTestSuite) TestIncrStepExecCount() {
	st := NewExecutionStore(10)

	st.IncrStepExecCount("step1")
	st.IncrStepExecCount("step1")
	st.IncrStepExecCount("step2")

	counts := st.StepExecCounts()
	s.Equal(2, counts["step1"])
	s.Equal(1, counts["step2"])
}

func (s *ExecutionStoreTestSuite) TestStepExecCounts_ReturnsCopy() {
	st := NewExecutionStore(10)
	st.IncrStepExecCount("step1")

	counts := st.StepExecCounts()
	counts["step1"] = 999

	counts2 := st.StepExecCounts()
	s.Equal(1, counts2["step1"])
}

func (s *ExecutionStoreTestSuite) TestRefreshStepMetrics() {
	st := NewExecutionStore(10)

	now := time.Now()
	started := now.Add(-100 * time.Millisecond)
	finished := now

	st.Add(&Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusSuccess,
		StartedAt:    now,
		Steps: map[string]*runtime.StepResult{
			"step1": {
				Status:     runtime.StatusSuccess,
				StartedAt:  &started,
				FinishedAt: &finished,
			},
			"step2": {
				Status:     runtime.StatusFailed,
				StartedAt:  &started,
				FinishedAt: &finished,
			},
		},
	})

	st.RefreshStepMetrics()

	m1 := st.GetStepMetrics("step1")
	s.Require().NotNil(m1)
	s.Equal(1, m1.TotalExecutions)
	s.Equal(1, m1.SuccessCount)
	s.Equal(0, m1.FailureCount)
	s.Greater(m1.AvgDurationMs, int64(0))

	m2 := st.GetStepMetrics("step2")
	s.Require().NotNil(m2)
	s.Equal(1, m2.TotalExecutions)
	s.Equal(0, m2.SuccessCount)
	s.Equal(1, m2.FailureCount)
}

func (s *ExecutionStoreTestSuite) TestGetStepMetrics_Nil() {
	st := NewExecutionStore(10)
	m := st.GetStepMetrics("nonexistent")
	s.Nil(m)
}

func (s *ExecutionStoreTestSuite) TestGetAllStepMetrics() {
	st := NewExecutionStore(10)

	now := time.Now()
	started := now.Add(-50 * time.Millisecond)

	st.Add(&Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusSuccess,
		StartedAt:    now,
		Steps: map[string]*runtime.StepResult{
			"step1": {Status: runtime.StatusSuccess, StartedAt: &started, FinishedAt: &now},
			"step2": {Status: runtime.StatusSuccess, StartedAt: &started, FinishedAt: &now},
		},
	})

	st.RefreshStepMetrics()

	all := st.GetAllStepMetrics()
	s.Len(all, 2)
	s.NotNil(all["step1"])
	s.NotNil(all["step2"])
}

func (s *ExecutionStoreTestSuite) TestGetAllStepMetrics_ReturnsCopy() {
	st := NewExecutionStore(10)

	now := time.Now()
	st.Add(&Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusSuccess,
		StartedAt:    now,
		Steps: map[string]*runtime.StepResult{
			"step1": {Status: runtime.StatusSuccess},
		},
	})

	st.RefreshStepMetrics()

	all := st.GetAllStepMetrics()
	all["step1"].TotalExecutions = 999

	all2 := st.GetAllStepMetrics()
	s.Equal(1, all2["step1"].TotalExecutions)
}

func (s *ExecutionStoreTestSuite) TestRefreshStepMetrics_LastExecution() {
	st := NewExecutionStore(10)

	t1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)

	st.Add(&Execution{
		ID:        "exec-1",
		WorkflowName: "test",
		Status:    runtime.StatusSuccess,
		StartedAt: t1,
		Steps:     map[string]*runtime.StepResult{"step1": {Status: runtime.StatusSuccess}},
	})
	st.Add(&Execution{
		ID:        "exec-2",
		WorkflowName: "test",
		Status:    runtime.StatusSuccess,
		StartedAt: t2,
		Steps:     map[string]*runtime.StepResult{"step1": {Status: runtime.StatusSuccess}},
	})

	st.RefreshStepMetrics()

	m := st.GetStepMetrics("step1")
	s.Require().NotNil(m)
	s.Equal(2, m.TotalExecutions)
	s.Equal(t2.Format(time.RFC3339), m.LastExecution)
}

package store

import (
	"time"

	"github.com/tailflow/tailflow/internal/runtime"
)

func (s *ExecutionStoreTestSuite) TestIncrStepExecCount() {
	st := NewExecutionStore(10)

	s.Require().NoError(st.IncrStepExecCount(s.ctx, "step1"))
	s.Require().NoError(st.IncrStepExecCount(s.ctx, "step1"))
	s.Require().NoError(st.IncrStepExecCount(s.ctx, "step2"))

	counts, err := st.StepExecCounts(s.ctx)
	s.Require().NoError(err)
	s.Equal(2, counts["step1"])
	s.Equal(1, counts["step2"])
}

func (s *ExecutionStoreTestSuite) TestStepExecCounts_ReturnsCopy() {
	st := NewExecutionStore(10)
	s.Require().NoError(st.IncrStepExecCount(s.ctx, "step1"))

	counts, err := st.StepExecCounts(s.ctx)
	s.Require().NoError(err)
	counts["step1"] = 999

	counts2, err := st.StepExecCounts(s.ctx)
	s.Require().NoError(err)
	s.Equal(1, counts2["step1"])
}

func (s *ExecutionStoreTestSuite) TestRefreshStepMetrics() {
	st := NewExecutionStore(10)

	now := time.Now()
	started := now.Add(-100 * time.Millisecond)
	finished := now

	s.Require().NoError(st.Add(s.ctx, &Execution{
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
	}))

	st.RefreshStepMetrics()

	m1, err := st.GetStepMetrics(s.ctx, "step1")
	s.Require().NoError(err)
	s.Require().NotNil(m1)
	s.Equal(1, m1.TotalExecutions)
	s.Equal(1, m1.SuccessCount)
	s.Equal(0, m1.FailureCount)
	s.Greater(m1.AvgDurationMs, int64(0))

	m2, err := st.GetStepMetrics(s.ctx, "step2")
	s.Require().NoError(err)
	s.Require().NotNil(m2)
	s.Equal(1, m2.TotalExecutions)
	s.Equal(0, m2.SuccessCount)
	s.Equal(1, m2.FailureCount)
}

func (s *ExecutionStoreTestSuite) TestGetStepMetrics_Nil() {
	st := NewExecutionStore(10)
	m, err := st.GetStepMetrics(s.ctx, "nonexistent")
	s.Require().NoError(err)
	s.Nil(m)
}

func (s *ExecutionStoreTestSuite) TestGetStepMetrics_StepNotInMetrics() {
	st := NewExecutionStore(10)

	now := time.Now()
	s.Require().NoError(st.Add(s.ctx, &Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusSuccess,
		StartedAt:    now,
		Steps: map[string]*runtime.StepResult{
			"step1": {Status: runtime.StatusSuccess},
		},
	}))

	// Refresh populates stepMetrics map with step1 only
	st.RefreshStepMetrics()

	// stepMetrics is now non-nil but "unknown-step" is not in the map
	m, err := st.GetStepMetrics(s.ctx, "unknown-step")
	s.Require().NoError(err)
	s.Nil(m)
}

func (s *ExecutionStoreTestSuite) TestGetAllStepMetrics() {
	st := NewExecutionStore(10)

	now := time.Now()
	started := now.Add(-50 * time.Millisecond)

	s.Require().NoError(st.Add(s.ctx, &Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusSuccess,
		StartedAt:    now,
		Steps: map[string]*runtime.StepResult{
			"step1": {Status: runtime.StatusSuccess, StartedAt: &started, FinishedAt: &now},
			"step2": {Status: runtime.StatusSuccess, StartedAt: &started, FinishedAt: &now},
		},
	}))

	st.RefreshStepMetrics()

	all, err := st.GetAllStepMetrics(s.ctx)
	s.Require().NoError(err)
	s.Len(all, 2)
	s.NotNil(all["step1"])
	s.NotNil(all["step2"])
}

func (s *ExecutionStoreTestSuite) TestGetAllStepMetrics_ReturnsCopy() {
	st := NewExecutionStore(10)

	now := time.Now()
	s.Require().NoError(st.Add(s.ctx, &Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusSuccess,
		StartedAt:    now,
		Steps: map[string]*runtime.StepResult{
			"step1": {Status: runtime.StatusSuccess},
		},
	}))

	st.RefreshStepMetrics()

	all, err := st.GetAllStepMetrics(s.ctx)
	s.Require().NoError(err)
	all["step1"].TotalExecutions = 999

	all2, err := st.GetAllStepMetrics(s.ctx)
	s.Require().NoError(err)
	s.Equal(1, all2["step1"].TotalExecutions)
}

func (s *ExecutionStoreTestSuite) TestSnapshot_NilStepsNilParamsNilFinishedAt() {
	st := NewExecutionStore(10)

	s.Require().NoError(st.Add(s.ctx, &Execution{
		ID:           "exec-nil",
		WorkflowName: "test",
		Status:       runtime.StatusRunning,
		StartedAt:    time.Now(),
		Steps:        nil,
		Params:       nil,
		FinishedAt:   nil,
	}))

	got, err := st.Get(s.ctx, "exec-nil")
	s.Require().NoError(err)
	s.Nil(got.Steps)
	s.Nil(got.Params)
	s.Nil(got.FinishedAt)
}

func (s *ExecutionStoreTestSuite) TestSnapshot_WithStepsParamsAndFinishedAt() {
	st := NewExecutionStore(10)

	now := time.Now()
	finished := now.Add(1 * time.Second)

	s.Require().NoError(st.Add(s.ctx, &Execution{
		ID:           "exec-full",
		WorkflowName: "test",
		Status:       runtime.StatusSuccess,
		StartedAt:    now,
		FinishedAt:   &finished,
		Params:       map[string]any{"env": "prod", "count": 42},
		Steps: map[string]*runtime.StepResult{
			"step1": {Status: runtime.StatusSuccess, Output: "ok"},
			"step2": {Status: runtime.StatusFailed, Error: &runtime.StepError{Message: "fail"}},
		},
	}))

	got, err := st.Get(s.ctx, "exec-full")
	s.Require().NoError(err)

	// Verify deep copy of Steps
	s.Len(got.Steps, 2)
	s.Equal(runtime.StatusSuccess, got.Steps["step1"].Status)
	s.Equal(runtime.StatusFailed, got.Steps["step2"].Status)

	// Verify deep copy of Params
	s.Equal("prod", got.Params["env"])
	s.Equal(42, got.Params["count"])

	// Verify deep copy of FinishedAt
	s.NotNil(got.FinishedAt)
	s.Equal(finished, *got.FinishedAt)

	// Mutating the snapshot should not affect the original
	got.Params["env"] = "staging"
	got.Steps["step1"].Status = runtime.StatusFailed
	newFinished := now.Add(99 * time.Second)
	got.FinishedAt = &newFinished

	original, err := st.Get(s.ctx, "exec-full")
	s.Require().NoError(err)
	s.Equal("prod", original.Params["env"])
	s.Equal(runtime.StatusSuccess, original.Steps["step1"].Status)
	s.Equal(finished, *original.FinishedAt)
}

func (s *ExecutionStoreTestSuite) TestRefreshStepMetrics_LastExecution() {
	st := NewExecutionStore(10)

	t1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)

	s.Require().NoError(st.Add(s.ctx, &Execution{
		ID:           "exec-1",
		WorkflowName: "test",
		Status:       runtime.StatusSuccess,
		StartedAt:    t1,
		Steps:        map[string]*runtime.StepResult{"step1": {Status: runtime.StatusSuccess}},
	}))
	s.Require().NoError(st.Add(s.ctx, &Execution{
		ID:           "exec-2",
		WorkflowName: "test",
		Status:       runtime.StatusSuccess,
		StartedAt:    t2,
		Steps:        map[string]*runtime.StepResult{"step1": {Status: runtime.StatusSuccess}},
	}))

	st.RefreshStepMetrics()

	m, err := st.GetStepMetrics(s.ctx, "step1")
	s.Require().NoError(err)
	s.Require().NotNil(m)
	s.Equal(2, m.TotalExecutions)
	s.Equal(t2.Format(time.RFC3339), m.LastExecution)
}

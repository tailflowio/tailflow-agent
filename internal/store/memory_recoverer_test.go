package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/tailflow/tailflow/internal/runtime"
)

type MemoryRecovererTestSuite struct {
	suite.Suite

	context   context.Context
	store     *MemoryExecutionStore
	recoverer *MemoryRecoverer
}

func TestMemoryRecoverer(t *testing.T) {
	suite.Run(t, new(MemoryRecovererTestSuite))
}

func (s *MemoryRecovererTestSuite) SetupTest() {
	s.context = context.Background()
	s.store = NewExecutionStore(10)
	s.recoverer = NewMemoryRecoverer(s.store)
}

func (s *MemoryRecovererTestSuite) TestRecoverExecutions_ReturnsRunningAndWaiting() {
	now := time.Now()

	s.Require().NoError(s.store.Add(s.context, &Execution{
		ID:           "exec-success",
		WorkflowName: "wf",
		Status:       runtime.StatusSuccess,
		StartedAt:    now,
	}))
	s.Require().NoError(s.store.Add(s.context, &Execution{
		ID:           "exec-running",
		WorkflowName: "wf",
		Status:       runtime.StatusRunning,
		Params:       map[string]any{"env": "prod"},
		Steps: map[string]*runtime.StepResult{
			"step1": {Status: runtime.StatusRunning},
		},
		StartedAt: now,
	}))
	s.Require().NoError(s.store.Add(s.context, &Execution{
		ID:           "exec-waiting",
		WorkflowName: "wf",
		Status:       runtime.StatusWaiting,
		StartedAt:    now,
	}))
	s.Require().NoError(s.store.Add(s.context, &Execution{
		ID:           "exec-failed",
		WorkflowName: "wf",
		Status:       runtime.StatusFailed,
		StartedAt:    now,
	}))

	recovered, err := s.recoverer.RecoverExecutions(s.context, "ignored-agent")
	s.Require().NoError(err)
	s.Require().Len(recovered, 2)

	ids := map[string]bool{}
	for _, rec := range recovered {
		ids[rec.ExecutionID] = true
	}

	s.True(ids["exec-running"])
	s.True(ids["exec-waiting"])
	s.False(ids["exec-success"])
	s.False(ids["exec-failed"])
}

func (s *MemoryRecovererTestSuite) TestRecoverExecutions_MapsParamsAndSteps() {
	now := time.Now()

	s.Require().NoError(s.store.Add(s.context, &Execution{
		ID:           "exec-running",
		WorkflowName: "wf",
		Status:       runtime.StatusRunning,
		Params:       map[string]any{"env": "prod"},
		Steps: map[string]*runtime.StepResult{
			"step1": {Status: runtime.StatusRunning},
		},
		StartedAt: now,
	}))

	recovered, err := s.recoverer.RecoverExecutions(s.context, "")
	s.Require().NoError(err)
	s.Require().Len(recovered, 1)

	rec := recovered[0]
	s.Equal("exec-running", rec.ExecutionID)
	s.Equal("wf", rec.WorkflowName)
	s.Equal(runtime.StatusRunning, rec.Status)
	s.Equal(map[string]any{"env": "prod"}, rec.Params)
	s.Require().Contains(rec.Steps, "step1")
	s.Equal(runtime.StatusRunning, rec.Steps["step1"].Status)
}

func (s *MemoryRecovererTestSuite) TestRecoverExecutions_EmptyStoreYieldsNothing() {
	recovered, err := s.recoverer.RecoverExecutions(s.context, "")
	s.Require().NoError(err)
	s.Empty(recovered)
}

func (s *MemoryRecovererTestSuite) TestRecoverExecutions_PreservesNewestFirst() {
	now := time.Now()

	s.Require().NoError(s.store.Add(s.context, &Execution{
		ID:           "exec-old",
		WorkflowName: "wf",
		Status:       runtime.StatusRunning,
		StartedAt:    now,
	}))
	s.Require().NoError(s.store.Add(s.context, &Execution{
		ID:           "exec-new",
		WorkflowName: "wf",
		Status:       runtime.StatusWaiting,
		StartedAt:    now,
	}))

	recovered, err := s.recoverer.RecoverExecutions(s.context, "")
	s.Require().NoError(err)
	s.Require().Len(recovered, 2)
	s.Equal("exec-new", recovered[0].ExecutionID)
	s.Equal("exec-old", recovered[1].ExecutionID)
}

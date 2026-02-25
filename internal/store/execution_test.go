package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type ExecutionStoreTestSuite struct {
	suite.Suite
}

func TestExecutionStore(t *testing.T) {
	suite.Run(t, new(ExecutionStoreTestSuite))
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

func (s *ExecutionStoreTestSuite) TestList() {
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

func (s *ExecutionStoreTestSuite) TestUpdate() {
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

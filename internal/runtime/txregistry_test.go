package runtime

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type MemoryTxRegistryTestSuite struct {
	suite.Suite
}

func TestMemoryTxRegistry(t *testing.T) {
	suite.Run(t, new(MemoryTxRegistryTestSuite))
}

func (s *MemoryTxRegistryTestSuite) TestGetUnknown() {
	r := NewMemoryTxRegistry()
	_, err := r.Get("nonexistent")
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

func (s *MemoryTxRegistryTestSuite) TestCommitUnknown() {
	r := NewMemoryTxRegistry()
	err := r.Commit("nonexistent")
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

func (s *MemoryTxRegistryTestSuite) TestRollbackUnknown() {
	r := NewMemoryTxRegistry()
	err := r.Rollback("nonexistent")
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

func (s *MemoryTxRegistryTestSuite) TestRollbackAllEmpty() {
	r := NewMemoryTxRegistry()
	// Should not panic on empty registry
	r.RollbackAll()
}

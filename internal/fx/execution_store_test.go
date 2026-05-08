package fx

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ExecutionStoreTestSuite struct {
	suite.Suite
}

func TestExecutionStoreTestSuite(t *testing.T) {
	suite.Run(t, new(ExecutionStoreTestSuite))
}

func (s *ExecutionStoreTestSuite) TestNewExecutionStore_UsesConfigMaxExecs() {
	out := NewExecutionStore(ExecutionStoreIn{Config: Config{MaxExecs: 42}})
	s.NotNil(out.Store)
}

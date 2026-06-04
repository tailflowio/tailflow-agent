package runtime

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ExprEvaluatorTestSuite struct {
	suite.Suite
}

func TestExprEvaluator(t *testing.T) {
	suite.Run(t, new(ExprEvaluatorTestSuite))
}

func (s *ExprEvaluatorTestSuite) SetupTest() {
	// required by convention
}

func (s *ExprEvaluatorTestSuite) TestNewExprEvaluator() {
	e := NewExprEvaluator()
	s.NotNil(e)
}

//go:build !saas

package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ExecActionTestSuite struct {
	suite.Suite
}

func TestExecAction(t *testing.T) {
	suite.Run(t, new(ExecActionTestSuite))
}

func (s *ExecActionTestSuite) SetupTest() {}

func (s *ExecActionTestSuite) TestExecuteStringCommand() {
	a := NewExecAction()
	ctx := newTestContext(map[string]any{
		"command": "echo hello",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	outMap := out.(map[string]any)
	s.Equal("hello\n", outMap["stdout"])
	s.Equal(0, outMap["exit_code"])
}

func (s *ExecActionTestSuite) TestExecuteArrayCommand() {
	a := NewExecAction()
	ctx := newTestContext(map[string]any{
		"command": []any{"echo", "hello", "world"},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	outMap := out.(map[string]any)
	s.Equal("hello world\n", outMap["stdout"])
}

func (s *ExecActionTestSuite) TestValidateMissingCommand() {
	a := NewExecAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
}

func (s *ExecActionTestSuite) TestExecuteFailedCommand() {
	a := NewExecAction()
	ctx := newTestContext(map[string]any{
		"command": "false",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
}

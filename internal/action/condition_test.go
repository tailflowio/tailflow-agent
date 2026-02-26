package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ConditionActionTestSuite struct {
	suite.Suite
}

func TestConditionAction(t *testing.T) {
	suite.Run(t, new(ConditionActionTestSuite))
}

func (s *ConditionActionTestSuite) SetupTest() {}

func (s *ConditionActionTestSuite) TestTrue_ReturnsThenBranch() {
	a := NewConditionAction()
	ctx := newTestContext(map[string]any{
		"if":   "true",
		"then": "yes",
		"else": "no",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(true, m["result"])
	s.Equal("then", m["branch"])
	s.Equal("yes", m["value"])
}

func (s *ConditionActionTestSuite) TestFalse_ReturnsElseBranch() {
	a := NewConditionAction()
	ctx := newTestContext(map[string]any{
		"if":   "false",
		"then": "yes",
		"else": "no",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(false, m["result"])
	s.Equal("else", m["branch"])
	s.Equal("no", m["value"])
}

func (s *ConditionActionTestSuite) TestWithParams() {
	a := NewConditionAction()
	ctx := newTestContext(map[string]any{
		"if": "params.env == 'prod'",
	})
	ctx.ExecCtx.Params = map[string]any{"env": "prod"}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal(true, out.(map[string]any)["result"])
}

func (s *ConditionActionTestSuite) TestValidateMissingIf() {
	a := NewConditionAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
}

func (s *ConditionActionTestSuite) TestValidateOK() {
	a := NewConditionAction()
	err := a.Validate(newTestContext(map[string]any{"if": "true"}))
	s.NoError(err)
}

func (s *ConditionActionTestSuite) TestExecuteInvalidExpression() {
	a := NewConditionAction()
	ctx := newTestContext(map[string]any{
		"if": "!!!invalid",
	})
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "condition")
}

func (s *ConditionActionTestSuite) TestFalseWithoutElseValue() {
	a := NewConditionAction()
	ctx := newTestContext(map[string]any{
		"if":   "false",
		"then": "yes",
		// no "else" key
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(false, m["result"])
	s.Equal("else", m["branch"])
	_, hasValue := m["value"]
	s.False(hasValue)
}

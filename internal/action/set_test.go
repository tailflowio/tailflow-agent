package action

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/runtime"
)

func newTestContext(config map[string]any) *ActionContext {
	return &ActionContext{
		Context: context.Background(),
		Config:  config,
		ExecCtx: runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:  "test-step",
		Logger:  slog.Default(),
	}
}

type SetActionTestSuite struct {
	suite.Suite
}

func TestSetAction(t *testing.T) {
	suite.Run(t, new(SetActionTestSuite))
}

func (s *SetActionTestSuite) SetupTest() {}

func (s *SetActionTestSuite) TestExecute() {
	a := NewSetAction()
	ctx := newTestContext(map[string]any{
		"name":  "Alice",
		"count": 42,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.NotNil(out)

	v, ok := ctx.ExecCtx.GetVariable("name")
	s.Require().True(ok)
	s.Equal("Alice", v)

	v, ok = ctx.ExecCtx.GetVariable("count")
	s.Require().True(ok)
	s.Equal(42, v)
}

func (s *SetActionTestSuite) TestValidateEmpty() {
	a := NewSetAction()
	err := a.Validate(newTestContext(nil))
	s.Error(err)
}

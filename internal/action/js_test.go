//go:build !saas

package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type JSActionTestSuite struct {
	suite.Suite
}

func TestJSAction(t *testing.T) {
	suite.Run(t, new(JSActionTestSuite))
}

func (s *JSActionTestSuite) SetupTest() {}

func (s *JSActionTestSuite) TestExecute() {
	a := NewJSAction()
	ctx := newTestContext(map[string]any{
		"script": "return 1 + 2",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal(int64(3), out)
}

func (s *JSActionTestSuite) TestAccessContext() {
	a := NewJSAction()
	ctx := newTestContext(map[string]any{
		"script": "return params.name",
	})
	ctx.ExecCtx.Params = map[string]any{"name": "Alice"}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("Alice", out)
}

func (s *JSActionTestSuite) TestCtxGetSet() {
	a := NewJSAction()
	ctx := newTestContext(map[string]any{
		"script": `ctx.set("greeting", "hello"); return ctx.get("greeting")`,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("hello", out)

	v, ok := ctx.ExecCtx.GetVariable("greeting")
	s.Require().True(ok)
	s.Equal("hello", v)
}

func (s *JSActionTestSuite) TestThrowError() {
	a := NewJSAction()
	ctx := newTestContext(map[string]any{
		"script": `throw new Error("validation failed")`,
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "validation failed")
}

func (s *JSActionTestSuite) TestValidateMissingScript() {
	a := NewJSAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
}

func (s *JSActionTestSuite) TestReturnObject() {
	a := NewJSAction()
	ctx := newTestContext(map[string]any{
		"script": `return {name: "Alice", age: 30}`,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal("Alice", m["name"])
	s.Equal(int64(30), m["age"])
}

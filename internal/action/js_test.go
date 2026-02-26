//go:build !saas

package action

import (
	"fmt"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/suite"
)

type JSActionTestSuite struct {
	suite.Suite
}

func TestJSAction(t *testing.T) {
	suite.Run(t, new(JSActionTestSuite))
}

func (s *JSActionTestSuite) SetupTest() {}

func (s *JSActionTestSuite) TestExecute_SimpleExpression() {
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

func (s *JSActionTestSuite) TestValidateOK() {
	a := NewJSAction()
	err := a.Validate(newTestContext(map[string]any{"script": "return 1"}))
	s.NoError(err)
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

func (s *JSActionTestSuite) TestReturnNull() {
	a := NewJSAction()
	ctx := newTestContext(map[string]any{
		"script": "return null",
	})

	out, err := a.Execute(ctx)
	s.NoError(err)
	s.Nil(out)
}

func (s *JSActionTestSuite) TestReturnUndefined() {
	a := NewJSAction()
	ctx := newTestContext(map[string]any{
		"script": "return undefined",
	})

	out, err := a.Execute(ctx)
	s.NoError(err)
	s.Nil(out)
}

func (s *JSActionTestSuite) TestCtxGetMissing() {
	a := NewJSAction()
	ctx := newTestContext(map[string]any{
		"script": `return ctx.get("nonexistent")`,
	})

	out, err := a.Execute(ctx)
	s.NoError(err)
	s.Nil(out)
}

func (s *JSActionTestSuite) TestExecute_VMSetError() {
	orig := gojaVMSet
	defer func() { gojaVMSet = orig }()
	gojaVMSet = func(vm *goja.Runtime, name string, value any) error {
		return fmt.Errorf("vm set failed")
	}

	a := NewJSAction()
	ctx := newTestContext(map[string]any{
		"script": "return 1",
	})
	// Ensure ctxMap has entries so the loop in Execute iterates at least once.
	ctx.ExecCtx.Params = map[string]any{"key": "value"}

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "set")
}

func (s *JSActionTestSuite) TestExecute_ObjSetGetError() {
	orig := gojaObjSet
	defer func() { gojaObjSet = orig }()
	gojaObjSet = func(obj *goja.Object, name string, value any) error {
		if name == "get" {
			return fmt.Errorf("obj set get failed")
		}

		return nil
	}

	a := NewJSAction()
	ctx := newTestContext(map[string]any{"script": "return 1"})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "js: set ctx.get")
}

func (s *JSActionTestSuite) TestExecute_ObjSetSetError() {
	orig := gojaObjSet
	defer func() { gojaObjSet = orig }()
	gojaObjSet = func(obj *goja.Object, name string, value any) error {
		if name == "set" {
			return fmt.Errorf("obj set set failed")
		}

		return nil
	}

	a := NewJSAction()
	ctx := newTestContext(map[string]any{"script": "return 1"})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "js: set ctx.set")
}

func (s *JSActionTestSuite) TestExecute_VMSetCtxError() {
	orig := gojaVMSet
	defer func() { gojaVMSet = orig }()
	gojaVMSet = func(vm *goja.Runtime, name string, value any) error {
		if name == "ctx" {
			return fmt.Errorf("vm set ctx failed")
		}

		return nil
	}

	a := NewJSAction()
	ctx := newTestContext(map[string]any{"script": "return 1"})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "js: set ctx")
}

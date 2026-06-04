package action

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/suite"
)

func contains(s, substr string) bool { return strings.Contains(s, substr) }

type LoopActionTestSuite struct {
	suite.Suite
}

func TestLoopAction(t *testing.T) {
	suite.Run(t, new(LoopActionTestSuite))
}

func (s *LoopActionTestSuite) SetupTest() {}

func (s *LoopActionTestSuite) TestExecute_IteratesItems() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items": []any{"a", "b", "c"},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(3, m["iterations"])

	v, ok := ctx.ExecCtx.GetVariable("item")
	s.Require().True(ok)
	s.Equal("c", v)
}

func (s *LoopActionTestSuite) TestCustomVarNames() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items": []any{1, 2},
		"as":    "val",
		"index": "i",
	})

	_, err := a.Execute(ctx)
	s.Require().NoError(err)

	v, ok := ctx.ExecCtx.GetVariable("val")
	s.Require().True(ok)
	s.Equal(2, v)

	idx, ok := ctx.ExecCtx.GetVariable("i")
	s.Require().True(ok)
	s.Equal(1, idx)
}

func (s *LoopActionTestSuite) TestValidateMissingItems() {
	a := NewLoopAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
}

func (s *LoopActionTestSuite) TestNotArray() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items": "not an array",
	})
	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *LoopActionTestSuite) TestWithBodyAction() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items":  []any{"alice", "bob", "charlie"},
		"as":     "user",
		"index":  "i",
		"action": "echo",
		"action_config": map[string]any{
			"msg": "hello",
		},
	})

	var calls int32
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		atomic.AddInt32(&calls, 1)
		s.Equal("echo", actionName)
		s.Contains(loopVars, "user")
		s.Contains(loopVars, "i")
		return map[string]any{"greeted": loopVars["user"]}, nil
	}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(3, m["iterations"])
	s.Len(m["results"], 3)
	s.Equal(int32(3), atomic.LoadInt32(&calls))

	v, _ := ctx.ExecCtx.GetVariable("user")
	s.Equal("charlie", v)
}

func (s *LoopActionTestSuite) TestConcurrency_ParallelExecution() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items":       []any{1, 2, 3, 4},
		"concurrency": 2,
		"action":      "noop",
		"action_config": map[string]any{
			"x": "y",
		},
	})

	var calls int32
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		atomic.AddInt32(&calls, 1)
		return nil, nil
	}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(4, m["iterations"])
	s.Equal(int32(4), atomic.LoadInt32(&calls))
}

func (s *LoopActionTestSuite) TestBodyActionError() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items":         []any{"a", "b"},
		"action":        "fail",
		"action_config": map[string]any{},
	})

	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return nil, fmt.Errorf("boom")
	}

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "loop iteration failed")
}

func (s *LoopActionTestSuite) TestMissingRunAction() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items":         []any{"a"},
		"action":        "echo",
		"action_config": map[string]any{},
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "RunAction callback not available")
}

func (s *LoopActionTestSuite) TestValidateOK() {
	a := NewLoopAction()
	err := a.Validate(newTestContext(map[string]any{"items": []any{1, 2}}))
	s.NoError(err)
}

func (s *LoopActionTestSuite) TestToIntInt64() {
	v, ok := toInt(int64(42))
	s.True(ok)
	s.Equal(42, v)
}

func (s *LoopActionTestSuite) TestToIntUnsupported() {
	v, ok := toInt("notanumber")
	s.False(ok)
	s.Equal(0, v)
}

func (s *LoopActionTestSuite) TestDeepCopyMapNil() {
	result := deepCopyMap(nil)
	s.Nil(result)
}

func (s *LoopActionTestSuite) TestDeepCopyMapNested() {
	original := map[string]any{
		"nested": map[string]any{"a": 1},
		"arr":    []any{1, 2, 3},
		"str":    "hello",
	}
	copied := deepCopyMap(original)

	s.Equal(1, copied["nested"].(map[string]any)["a"])
	s.Equal([]any{1, 2, 3}, copied["arr"])
	s.Equal("hello", copied["str"])

	original["nested"].(map[string]any)["a"] = 99
	s.Equal(1, copied["nested"].(map[string]any)["a"])
}

func (s *LoopActionTestSuite) TestLoopWithEmitLog() {
	a := NewLoopAction()
	var logs []string

	ctx := newTestContext(map[string]any{
		"items":         []any{"x"},
		"action":        "noop",
		"action_config": map[string]any{},
	})
	ctx.EmitLog = func(msg string) {
		logs = append(logs, msg)
	}
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return nil, nil
	}

	_, err := a.Execute(ctx)
	s.NoError(err)
	s.Len(logs, 1)
	s.Contains(logs[0], "[1/1]")
}

func (s *LoopActionTestSuite) TestEmptyItemsLegacy() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items": []any{},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(0, m["iterations"])
}

func (s *LoopActionTestSuite) TestFirstErrIdxAllNil() {
	idx := firstErrIdx([]error{nil, nil, nil})
	s.Equal(0, idx)
}

package action

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/suite"
)

type LoopActionTestSuite struct {
	suite.Suite
}

func TestLoopAction(t *testing.T) {
	suite.Run(t, new(LoopActionTestSuite))
}

func (s *LoopActionTestSuite) SetupTest() {}

func (s *LoopActionTestSuite) TestExecute() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items": []any{"a", "b", "c"},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(3, m["iterations"])

	// Last item should be set
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

	// Backward compat: last item set as variable
	v, _ := ctx.ExecCtx.GetVariable("user")
	s.Equal("charlie", v)
}

func (s *LoopActionTestSuite) TestConcurrency() {
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
	// RunAction is nil by default from newTestContext

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "RunAction callback not available")
}

func (s *LoopActionTestSuite) TestPipelineBasic() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items": []any{"alice", "bob"},
		"as":    "user",
		"index": "i",
		"actions": []any{
			map[string]any{
				"action": "fetch",
				"config": map[string]any{"url": "http://example.com"},
			},
			map[string]any{
				"action": "transform",
				"config": map[string]any{"key": "name"},
			},
		},
	})

	var calls int32
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		atomic.AddInt32(&calls, 1)
		s.Contains(loopVars, "user")
		s.Contains(loopVars, "i")

		if actionName == "fetch" {
			return map[string]any{"body": map[string]any{"name": loopVars["user"]}}, nil
		}
		// transform: should receive prev from fetch
		prev, hasPrev := loopVars["prev"]
		s.True(hasPrev, "transform should receive loop.prev")
		return map[string]any{"result": prev}, nil
	}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(2, m["iterations"])
	s.Equal(int32(4), atomic.LoadInt32(&calls)) // 2 items x 2 actions
}

func (s *LoopActionTestSuite) TestPipelineErrorStops() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items": []any{"x"},
		"actions": []any{
			map[string]any{"action": "step1", "config": map[string]any{}},
			map[string]any{"action": "step2", "config": map[string]any{}},
			map[string]any{"action": "step3", "config": map[string]any{}},
		},
	})

	var calls []string
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		calls = append(calls, actionName)
		if actionName == "step2" {
			return nil, fmt.Errorf("step2 failed")
		}
		return map[string]any{"ok": true}, nil
	}

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "loop iteration failed")
	// step3 should never be called
	s.Equal([]string{"step1", "step2"}, calls)
}

func (s *LoopActionTestSuite) TestPipelineConcurrency() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items":       []any{1, 2, 3},
		"concurrency": 3,
		"actions": []any{
			map[string]any{"action": "a1", "config": map[string]any{}},
			map[string]any{"action": "a2", "config": map[string]any{}},
		},
	})

	var calls int32
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		atomic.AddInt32(&calls, 1)
		return map[string]any{"v": actionName}, nil
	}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(3, m["iterations"])
	s.Equal(int32(6), atomic.LoadInt32(&calls)) // 3 items x 2 actions
}

func (s *LoopActionTestSuite) TestPipelineValidation() {
	a := NewLoopAction()

	// actions is not an array
	ctx := newTestContext(map[string]any{
		"items":   []any{"a"},
		"actions": "not-an-array",
	})
	ctx.RunAction = func(string, map[string]any, map[string]any) (any, error) { return nil, nil }
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "'actions' must be an array")

	// empty array
	ctx2 := newTestContext(map[string]any{
		"items":   []any{"a"},
		"actions": []any{},
	})
	ctx2.RunAction = func(string, map[string]any, map[string]any) (any, error) { return nil, nil }
	_, err = a.Execute(ctx2)
	s.Error(err)
	s.Contains(err.Error(), "must not be empty")

	// element without action field
	ctx3 := newTestContext(map[string]any{
		"items":   []any{"a"},
		"actions": []any{map[string]any{"config": map[string]any{}}},
	})
	ctx3.RunAction = func(string, map[string]any, map[string]any) (any, error) { return nil, nil }
	_, err = a.Execute(ctx3)
	s.Error(err)
	s.Contains(err.Error(), "missing or invalid 'action' field")
}

func (s *LoopActionTestSuite) TestErrorPolicyContinue() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items":         []any{"a", "b", "c"},
		"action":        "maybe_fail",
		"action_config": map[string]any{},
		"error_policy":  "continue",
	})

	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		idx := loopVars["index"].(int)
		if idx == 1 {
			return nil, fmt.Errorf("fail on index 1")
		}
		return map[string]any{"ok": true}, nil
	}

	out, err := a.Execute(ctx)
	s.Require().NoError(err) // error_policy=continue means no error propagated

	m := out.(map[string]any)
	s.Equal(3, m["iterations"])
	s.Equal(1, m["failed"])
	s.Equal(2, m["succeeded"])

	loopErrors, ok := m["errors"].([]map[string]any)
	s.Require().True(ok)
	s.Len(loopErrors, 1)
	s.Equal(1, loopErrors[0]["index"])
	s.Contains(loopErrors[0]["message"], "fail on index 1")
}

func (s *LoopActionTestSuite) TestErrorPolicyDefaultFailFast() {
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

func (s *LoopActionTestSuite) TestPipelineErrorPolicyContinue() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items":        []any{"x", "y"},
		"error_policy": "continue",
		"actions": []any{
			map[string]any{"action": "step1", "config": map[string]any{}},
		},
	})

	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		idx := loopVars["index"].(int)
		if idx == 0 {
			return nil, fmt.Errorf("step1 failed for x")
		}
		return map[string]any{"ok": true}, nil
	}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(2, m["iterations"])
	s.Equal(1, m["failed"])
	s.Equal(1, m["succeeded"])
}

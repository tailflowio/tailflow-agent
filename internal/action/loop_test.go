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

func (s *LoopActionTestSuite) TestExecute_IteratesItems() {
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

	// Verify the copy is independent
	original["nested"].(map[string]any)["a"] = 99
	s.Equal(1, copied["nested"].(map[string]any)["a"])
}

func (s *LoopActionTestSuite) TestPipelineActionsNotArray() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items":   []any{"a"},
		"actions": 123,
	})
	ctx.RunAction = func(string, map[string]any, map[string]any) (any, error) { return nil, nil }
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "'actions' must be an array")
}

func (s *LoopActionTestSuite) TestPipelineMissingRunAction() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items": []any{"a"},
		"actions": []any{
			map[string]any{"action": "step1", "config": map[string]any{}},
		},
	})
	// RunAction is nil

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "RunAction callback not available")
}

func (s *LoopActionTestSuite) TestPipelineNonObjectElement() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items":   []any{"a"},
		"actions": []any{"not-an-object"},
	})
	ctx.RunAction = func(string, map[string]any, map[string]any) (any, error) { return nil, nil }

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "must be an object")
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

func (s *LoopActionTestSuite) TestPipelineWithEmitLogSuccess() {
	a := NewLoopAction()
	var logs []string

	ctx := newTestContext(map[string]any{
		"items": []any{"x"},
		"actions": []any{
			map[string]any{"action": "step1", "config": map[string]any{}},
		},
	})
	ctx.EmitLog = func(msg string) {
		logs = append(logs, msg)
	}
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	}

	_, err := a.Execute(ctx)
	s.NoError(err)
	s.GreaterOrEqual(len(logs), 2) // start + OK
}

func (s *LoopActionTestSuite) TestPipelineWithEmitLogError() {
	a := NewLoopAction()
	var logs []string

	ctx := newTestContext(map[string]any{
		"items":        []any{"x"},
		"error_policy": "continue",
		"actions": []any{
			map[string]any{"action": "step1", "config": map[string]any{}},
		},
	})
	ctx.EmitLog = func(msg string) {
		logs = append(logs, msg)
	}
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return nil, fmt.Errorf("fail")
	}

	_, err := a.Execute(ctx)
	s.NoError(err) // error_policy=continue
	// Should have a FAILED log
	found := false
	for _, l := range logs {
		if len(l) > 0 {
			found = true
		}
	}
	s.True(found)
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
	// This tests the firstErrIdx function when all errors are nil
	idx := firstErrIdx([]error{nil, nil, nil})
	s.Equal(0, idx)
}

func (s *LoopActionTestSuite) TestConcurrencyFloat64() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items":         []any{1, 2},
		"concurrency":   float64(2),
		"action":        "noop",
		"action_config": map[string]any{},
	})

	var calls int32
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		atomic.AddInt32(&calls, 1)
		return nil, nil
	}

	_, err := a.Execute(ctx)
	s.NoError(err)
	s.Equal(int32(2), atomic.LoadInt32(&calls))
}

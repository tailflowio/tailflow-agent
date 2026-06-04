package action

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/suite"
)

type LoopPipelineTestSuite struct {
	suite.Suite
}

func TestLoopPipeline(t *testing.T) {
	suite.Run(t, new(LoopPipelineTestSuite))
}

func (s *LoopPipelineTestSuite) SetupTest() {}

func (s *LoopPipelineTestSuite) TestPipelineBasic() {
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
		prev, hasPrev := loopVars["prev"]
		s.True(hasPrev, "transform should receive loop.prev")
		return map[string]any{"result": prev}, nil
	}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(2, m["iterations"])
	s.Equal(int32(4), atomic.LoadInt32(&calls))
}

func (s *LoopPipelineTestSuite) TestPipelineErrorStops() {
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
	s.Equal([]string{"step1", "step2"}, calls)
}

func (s *LoopPipelineTestSuite) TestPipelineConcurrency() {
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
	s.Equal(int32(6), atomic.LoadInt32(&calls))
}

func (s *LoopPipelineTestSuite) TestPipelineValidation() {
	a := NewLoopAction()

	ctx := newTestContext(map[string]any{
		"items":   []any{"a"},
		"actions": "not-an-array",
	})
	ctx.RunAction = func(string, map[string]any, map[string]any) (any, error) { return nil, nil }
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "'actions' must be an array")

	ctx2 := newTestContext(map[string]any{
		"items":   []any{"a"},
		"actions": []any{},
	})
	ctx2.RunAction = func(string, map[string]any, map[string]any) (any, error) { return nil, nil }
	_, err = a.Execute(ctx2)
	s.Error(err)
	s.Contains(err.Error(), "must not be empty")

	ctx3 := newTestContext(map[string]any{
		"items":   []any{"a"},
		"actions": []any{map[string]any{"config": map[string]any{}}},
	})
	ctx3.RunAction = func(string, map[string]any, map[string]any) (any, error) { return nil, nil }
	_, err = a.Execute(ctx3)
	s.Error(err)
	s.Contains(err.Error(), "missing or invalid 'action' field")
}

func (s *LoopPipelineTestSuite) TestPipelineActionsNotArray() {
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

func (s *LoopPipelineTestSuite) TestPipelineMissingRunAction() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items": []any{"a"},
		"actions": []any{
			map[string]any{"action": "step1", "config": map[string]any{}},
		},
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "RunAction callback not available")
}

func (s *LoopPipelineTestSuite) TestPipelineNonObjectElement() {
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

func (s *LoopPipelineTestSuite) TestErrorPolicyContinue() {
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
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(3, m["iterations"])
	s.Equal(1, m["failed"])
	s.Equal(2, m["succeeded"])

	loopErrors, ok := m["errors"].([]any)
	s.Require().True(ok)
	s.Len(loopErrors, 1)
	errEntry := loopErrors[0].(map[string]any)
	s.Equal(1, errEntry["index"])
	s.Contains(errEntry["message"], "fail on index 1")
}

func (s *LoopPipelineTestSuite) TestErrorPolicyDefaultFailFast() {
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

func (s *LoopPipelineTestSuite) TestPipelineErrorPolicyContinue() {
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

func (s *LoopPipelineTestSuite) TestPipelineWithEmitLogSuccess() {
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
	s.GreaterOrEqual(len(logs), 2)
}

func (s *LoopPipelineTestSuite) TestPipelineWithEmitLogError() {
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
	s.NoError(err)
	found := false
	for _, l := range logs {
		if len(l) > 0 {
			found = true
		}
	}
	s.True(found)
}

func (s *LoopPipelineTestSuite) TestPipelineNilEmitLog_Success() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items": []any{"x"},
		"actions": []any{
			map[string]any{"action": "step1", "config": map[string]any{}},
		},
	})
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	}

	out, err := a.Execute(ctx)
	s.NoError(err)

	m := out.(map[string]any)
	s.Equal(1, m["iterations"])
}

func (s *LoopPipelineTestSuite) TestPipelineNilEmitLog_Error() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items":        []any{"x"},
		"error_policy": "continue",
		"actions": []any{
			map[string]any{"action": "step1", "config": map[string]any{}},
		},
	})
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return nil, fmt.Errorf("boom")
	}

	out, err := a.Execute(ctx)
	s.NoError(err)

	m := out.(map[string]any)
	s.Equal(1, m["failed"])
}

func (s *LoopPipelineTestSuite) TestConcurrencyFloat64() {
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

func (s *LoopPipelineTestSuite) TestPipelineEmitLog_LabelFromMapItem() {
	a := NewLoopAction()
	var logs []string

	ctx := newTestContext(map[string]any{
		"items": []any{
			map[string]any{"name": "resource-1"},
		},
		"actions": []any{
			map[string]any{"action": "step1", "config": map[string]any{}},
		},
	})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	}

	_, err := a.Execute(ctx)
	s.NoError(err)

	s.Require().GreaterOrEqual(len(logs), 2)
	s.Contains(logs[0], "resource-1")
	s.Contains(logs[1], "resource-1")
	s.Contains(logs[1], "OK")
}

func (s *LoopPipelineTestSuite) TestPipelineEmitLog_FailWithLabelAndStdout() {
	a := NewLoopAction()
	var logs []string

	ctx := newTestContext(map[string]any{
		"items": []any{
			map[string]any{"path": "/etc/config.yaml"},
		},
		"error_policy": "continue",
		"actions": []any{
			map[string]any{"action": "exec", "config": map[string]any{}},
		},
	})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return map[string]any{"stdout": "permission denied"}, fmt.Errorf("exec failed")
	}

	out, err := a.Execute(ctx)
	s.NoError(err)

	foundFailed := false
	for _, l := range logs {
		if foundFailed {
			break
		}
		if contains(l, "FAILED") && contains(l, "/etc/config.yaml") && contains(l, "permission denied") {
			foundFailed = true
		}
	}
	s.True(foundFailed, "expected FAILED log with label and hint, got: %v", logs)

	m := out.(map[string]any)
	loopErrors := m["errors"].([]any)
	s.Require().Len(loopErrors, 1)
	entry := loopErrors[0].(map[string]any)
	s.Equal("permission denied", entry["detail"])
}

func (s *LoopPipelineTestSuite) TestPipelineEmitLog_FailWithLabelNoStdout() {
	a := NewLoopAction()
	var logs []string

	ctx := newTestContext(map[string]any{
		"items": []any{
			map[string]any{"id": "abc-123"},
		},
		"error_policy": "continue",
		"actions": []any{
			map[string]any{"action": "exec", "config": map[string]any{}},
		},
	})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return map[string]any{"stderr": "some error"}, fmt.Errorf("exec failed")
	}

	_, err := a.Execute(ctx)
	s.NoError(err)

	foundFailed := false
	for _, l := range logs {
		if foundFailed {
			break
		}
		if contains(l, "FAILED") && contains(l, "abc-123") && contains(l, "ms)") {
			foundFailed = true
		}
	}
	s.True(foundFailed, "expected FAILED log with label and duration, got: %v", logs)
}

func (s *LoopPipelineTestSuite) TestPipelineEmitLog_FailNoLabelNoStdout() {
	a := NewLoopAction()
	var logs []string

	ctx := newTestContext(map[string]any{
		"items":        []any{42},
		"error_policy": "continue",
		"actions": []any{
			map[string]any{"action": "exec", "config": map[string]any{}},
		},
	})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return nil, fmt.Errorf("exec failed")
	}

	_, err := a.Execute(ctx)
	s.NoError(err)

	foundFailed := false
	for _, l := range logs {
		if foundFailed {
			break
		}
		if contains(l, "FAILED") && contains(l, "step") && contains(l, "exec") {
			foundFailed = true
		}
	}
	s.True(foundFailed, "expected FAILED log with step info, got: %v", logs)
}

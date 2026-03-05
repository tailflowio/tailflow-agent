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

	loopErrors, ok := m["errors"].([]any)
	s.Require().True(ok)
	s.Len(loopErrors, 1)
	errEntry := loopErrors[0].(map[string]any)
	s.Equal(1, errEntry["index"])
	s.Contains(errEntry["message"], "fail on index 1")
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

func (s *LoopActionTestSuite) TestLoopItemLabel_StringValue() {
	label := loopItemLabel(map[string]any{"item": "hello", "index": 0})
	s.Equal("hello", label)
}

func (s *LoopActionTestSuite) TestLoopItemLabel_MapWithPath() {
	label := loopItemLabel(map[string]any{
		"item": map[string]any{"path": "/tmp/file.txt", "extra": 42},
	})
	s.Equal("/tmp/file.txt", label)
}

func (s *LoopActionTestSuite) TestLoopItemLabel_MapWithName() {
	label := loopItemLabel(map[string]any{
		"item": map[string]any{"name": "my-resource"},
	})
	s.Equal("my-resource", label)
}

func (s *LoopActionTestSuite) TestLoopItemLabel_MapWithId() {
	label := loopItemLabel(map[string]any{
		"item": map[string]any{"id": "abc-123"},
	})
	s.Equal("abc-123", label)
}

func (s *LoopActionTestSuite) TestLoopItemLabel_MapWithTitle() {
	label := loopItemLabel(map[string]any{
		"item": map[string]any{"title": "My Title"},
	})
	s.Equal("My Title", label)
}

func (s *LoopActionTestSuite) TestLoopItemLabel_MapNoRecognizedField() {
	// Map value without any of path/name/id/title – falls through, returns ""
	label := loopItemLabel(map[string]any{
		"item": map[string]any{"foo": "bar"},
	})
	s.Equal("", label)
}

func (s *LoopActionTestSuite) TestLoopItemLabel_SkipsIndexAndPrev() {
	// Only keys are "index" and "prev", both should be skipped → ""
	label := loopItemLabel(map[string]any{
		"index": 0,
		"prev":  "something",
	})
	s.Equal("", label)
}

func (s *LoopActionTestSuite) TestLoopItemLabel_EmptyMap() {
	label := loopItemLabel(map[string]any{})
	s.Equal("", label)
}

func (s *LoopActionTestSuite) TestLoopItemLabel_NonStringNonMapValue() {
	// Numeric value – not a string, not a map → returns ""
	label := loopItemLabel(map[string]any{
		"item": 42,
	})
	s.Equal("", label)
}

func (s *LoopActionTestSuite) TestLoopItemLabel_MapFieldEmpty() {
	// Map value where path/name/id/title exist but are empty strings – should not match
	label := loopItemLabel(map[string]any{
		"item": map[string]any{"path": "", "name": "", "id": "", "title": ""},
	})
	s.Equal("", label)
}

func (s *LoopActionTestSuite) TestLastStdoutLine_NonMapOutput() {
	result := lastStdoutLine("not a map")
	s.Equal("", result)
}

func (s *LoopActionTestSuite) TestLastStdoutLine_NilOutput() {
	result := lastStdoutLine(nil)
	s.Equal("", result)
}

func (s *LoopActionTestSuite) TestLastStdoutLine_EmptyStdout() {
	result := lastStdoutLine(map[string]any{"stdout": ""})
	s.Equal("", result)
}

func (s *LoopActionTestSuite) TestLastStdoutLine_WhitespaceOnlyStdout() {
	result := lastStdoutLine(map[string]any{"stdout": "   \n\n   "})
	s.Equal("", result)
}

func (s *LoopActionTestSuite) TestLastStdoutLine_SingleLine() {
	result := lastStdoutLine(map[string]any{"stdout": "only line"})
	s.Equal("only line", result)
}

func (s *LoopActionTestSuite) TestLastStdoutLine_MultipleLines() {
	result := lastStdoutLine(map[string]any{"stdout": "first\nsecond\nthird"})
	s.Equal("third", result)
}

func (s *LoopActionTestSuite) TestLastStdoutLine_TrailingNewlines() {
	result := lastStdoutLine(map[string]any{"stdout": "first\nsecond\n\n"})
	s.Equal("second", result)
}

func (s *LoopActionTestSuite) TestLastStdoutLine_NoStdoutKey() {
	result := lastStdoutLine(map[string]any{"stderr": "some error"})
	s.Equal("", result)
}

func (s *LoopActionTestSuite) TestExtractResultDetail_NonIterationResult() {
	// When T is any (not iterationResult), returns ""
	result := extractResultDetail[any]("just a string")
	s.Equal("", result)
}

func (s *LoopActionTestSuite) TestExtractResultDetail_WithStdout() {
	ir := iterationResult{
		Actions: []actionResult{
			{
				Action: "step1",
				Output: map[string]any{"stdout": "line1\nline2"},
			},
		},
	}
	result := extractResultDetail(ir)
	s.Equal("line2", result)
}

func (s *LoopActionTestSuite) TestExtractResultDetail_EmptyStdout() {
	ir := iterationResult{
		Actions: []actionResult{
			{
				Action: "step1",
				Output: map[string]any{"stdout": ""},
			},
		},
	}
	result := extractResultDetail(ir)
	s.Equal("", result)
}

func (s *LoopActionTestSuite) TestExtractResultDetail_NonMapOutput() {
	ir := iterationResult{
		Actions: []actionResult{
			{
				Action: "step1",
				Output: "not a map",
			},
		},
	}
	result := extractResultDetail(ir)
	s.Equal("", result)
}

func (s *LoopActionTestSuite) TestExtractResultDetail_MultipleActions_LastHasStdout() {
	ir := iterationResult{
		Actions: []actionResult{
			{Action: "step1", Output: map[string]any{"stdout": ""}},
			{Action: "step2", Output: map[string]any{"stdout": "final output"}},
		},
	}
	result := extractResultDetail(ir)
	s.Equal("final output", result)
}

func (s *LoopActionTestSuite) TestExtractResultDetail_MultipleActions_FirstHasStdoutLastEmpty() {
	// Reverse iteration: last action has empty stdout, first has output
	ir := iterationResult{
		Actions: []actionResult{
			{Action: "step1", Output: map[string]any{"stdout": "good output"}},
			{Action: "step2", Output: map[string]any{"stdout": ""}},
		},
	}
	result := extractResultDetail(ir)
	s.Equal("good output", result)
}

func (s *LoopActionTestSuite) TestExtractResultDetail_EmptyActions() {
	ir := iterationResult{
		Actions: []actionResult{},
	}
	result := extractResultDetail(ir)
	s.Equal("", result)
}

func (s *LoopActionTestSuite) TestEmitStepStart_NilEmitLog() {
	ctx := newTestContext(map[string]any{})
	// EmitLog is nil by default; should not panic
	emitStepStart(ctx, 0, 1, 0, 1, "step1", "label")
}

func (s *LoopActionTestSuite) TestEmitStepSuccess_NilEmitLog() {
	ctx := newTestContext(map[string]any{})
	// EmitLog is nil by default; should not panic
	emitStepSuccess(ctx, 0, 1, 0, 1, "step1", "label", 100)
}

func (s *LoopActionTestSuite) TestEmitStepStart_WithLabel() {
	var logs []string
	ctx := newTestContext(map[string]any{})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }

	emitStepStart(ctx, 0, 3, 0, 2, "step1", "my-file.txt")

	s.Require().Len(logs, 1)
	s.Equal("[1/3] my-file.txt", logs[0])
}

func (s *LoopActionTestSuite) TestEmitStepStart_WithoutLabel() {
	var logs []string
	ctx := newTestContext(map[string]any{})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }

	emitStepStart(ctx, 1, 3, 0, 2, "fetch", "")

	s.Require().Len(logs, 1)
	s.Equal("[2/3] step 1/2 fetch", logs[0])
}

func (s *LoopActionTestSuite) TestEmitStepSuccess_WithLabel() {
	var logs []string
	ctx := newTestContext(map[string]any{})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }

	emitStepSuccess(ctx, 0, 3, 0, 2, "step1", "my-file.txt", 150)

	s.Require().Len(logs, 1)
	s.Equal("[1/3] my-file.txt OK (150ms)", logs[0])
}

func (s *LoopActionTestSuite) TestEmitStepSuccess_WithoutLabel() {
	var logs []string
	ctx := newTestContext(map[string]any{})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }

	emitStepSuccess(ctx, 2, 5, 1, 3, "transform", "", 250)

	s.Require().Len(logs, 1)
	s.Equal("[3/5] step 2/3 transform OK (250ms)", logs[0])
}

func (s *LoopActionTestSuite) TestAppendFailedStep_LabelAndHint() {
	var logs []string
	ctx := newTestContext(map[string]any{})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }

	ps := pipelineStep{Action: "exec", Config: map[string]any{}}
	output := map[string]any{"stdout": "line1\nsome error hint"}

	results, err := appendFailedStep(ctx, nil, ps, output, 100,
		fmt.Errorf("exec failed"), 0, 3, 0, 2, "my-file.txt")

	s.Error(err)
	s.Contains(err.Error(), "exec")
	s.Len(results, 1)
	s.Equal("exec failed", results[0].Error)

	s.Require().Len(logs, 1)
	s.Equal("[1/3] my-file.txt FAILED: some error hint", logs[0])
}

func (s *LoopActionTestSuite) TestAppendFailedStep_LabelNoHint() {
	var logs []string
	ctx := newTestContext(map[string]any{})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }

	ps := pipelineStep{Action: "exec", Config: map[string]any{}}
	// nil output → lastStdoutLine returns ""
	results, err := appendFailedStep(ctx, nil, ps, nil, 200,
		fmt.Errorf("exec failed"), 1, 5, 0, 2, "my-resource")

	s.Error(err)
	s.Len(results, 1)
	s.Require().Len(logs, 1)
	s.Equal("[2/5] my-resource FAILED (200ms)", logs[0])
}

func (s *LoopActionTestSuite) TestAppendFailedStep_NoLabelNoHint() {
	var logs []string
	ctx := newTestContext(map[string]any{})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }

	ps := pipelineStep{Action: "exec", Config: map[string]any{}}

	results, err := appendFailedStep(ctx, nil, ps, nil, 300,
		fmt.Errorf("boom"), 2, 4, 1, 3, "")

	s.Error(err)
	s.Len(results, 1)
	s.Require().Len(logs, 1)
	s.Equal("[3/4] step 2/3 exec FAILED (300ms): boom", logs[0])
}

func (s *LoopActionTestSuite) TestAppendFailedStep_NilEmitLog() {
	ctx := newTestContext(map[string]any{})
	// EmitLog is nil by default

	ps := pipelineStep{Action: "exec", Config: map[string]any{}}
	results, err := appendFailedStep(ctx, nil, ps, nil, 100,
		fmt.Errorf("boom"), 0, 1, 0, 1, "label")

	s.Error(err)
	s.Len(results, 1)
	s.Equal("boom", results[0].Error)
}

func (s *LoopActionTestSuite) TestAppendFailedStep_AppendsToExistingResults() {
	ctx := newTestContext(map[string]any{})

	existing := []actionResult{
		{Action: "step1", Output: "ok", DurationMs: 50},
	}

	ps := pipelineStep{Action: "step2", Config: map[string]any{}}
	results, err := appendFailedStep(ctx, existing, ps, nil, 100,
		fmt.Errorf("step2 failed"), 0, 1, 1, 2, "")

	s.Error(err)
	s.Len(results, 2)
	s.Equal("step1", results[0].Action)
	s.Equal("step2", results[1].Action)
}

func (s *LoopActionTestSuite) TestBuildLoopOutput_WithDetail() {
	ctx := newTestContext(map[string]any{
		"error_policy": "continue",
	})

	items := []any{"a"}
	results := []iterationResult{
		{
			Actions: []actionResult{
				{
					Action: "exec",
					Output: map[string]any{"stdout": "some detail line"},
					Error:  "fail",
				},
			},
		},
	}
	errs := []error{fmt.Errorf("iteration failed")}

	out, err := buildLoopOutput(ctx, items, results, errs)
	s.NoError(err) // error_policy=continue

	m := out.(map[string]any)
	loopErrors := m["errors"].([]any)
	s.Require().Len(loopErrors, 1)

	entry := loopErrors[0].(map[string]any)
	s.Equal("some detail line", entry["detail"])
}

func (s *LoopActionTestSuite) TestBuildLoopOutput_NoErrors() {
	ctx := newTestContext(map[string]any{})

	items := []any{"a", "b"}
	results := []any{"r1", "r2"}
	errs := []error{nil, nil}

	out, err := buildLoopOutput(ctx, items, results, errs)
	s.NoError(err)

	m := out.(map[string]any)
	s.Equal(2, m["iterations"])
	s.Nil(m["errors"])
	s.Nil(m["failed"])
}

func (s *LoopActionTestSuite) TestPipelineEmitLog_LabelFromMapItem() {
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

	// Should have emitted label-based log messages
	s.Require().GreaterOrEqual(len(logs), 2)
	s.Contains(logs[0], "resource-1")
	s.Contains(logs[1], "resource-1")
	s.Contains(logs[1], "OK")
}

func (s *LoopActionTestSuite) TestPipelineEmitLog_FailWithLabelAndStdout() {
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
	s.NoError(err) // error_policy=continue

	// Verify label+hint FAILED log was emitted
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

	// Verify detail is captured in errors output
	m := out.(map[string]any)
	loopErrors := m["errors"].([]any)
	s.Require().Len(loopErrors, 1)
	entry := loopErrors[0].(map[string]any)
	s.Equal("permission denied", entry["detail"])
}

func (s *LoopActionTestSuite) TestPipelineEmitLog_FailWithLabelNoStdout() {
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
		// No stdout in output → hint is empty
		return map[string]any{"stderr": "some error"}, fmt.Errorf("exec failed")
	}

	_, err := a.Execute(ctx)
	s.NoError(err)

	// Verify label FAILED log without hint (duration-based message)
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

func (s *LoopActionTestSuite) TestPipelineEmitLog_FailNoLabelNoStdout() {
	a := NewLoopAction()
	var logs []string

	ctx := newTestContext(map[string]any{
		"items": []any{42}, // numeric item → no label
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

	// Verify default FAILED log with step info
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

func (s *LoopActionTestSuite) TestPipelineNilEmitLog_Success() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items": []any{"x"},
		"actions": []any{
			map[string]any{"action": "step1", "config": map[string]any{}},
		},
	})
	// EmitLog is nil
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	}

	out, err := a.Execute(ctx)
	s.NoError(err)

	m := out.(map[string]any)
	s.Equal(1, m["iterations"])
}

func (s *LoopActionTestSuite) TestPipelineNilEmitLog_Error() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items":        []any{"x"},
		"error_policy": "continue",
		"actions": []any{
			map[string]any{"action": "step1", "config": map[string]any{}},
		},
	})
	// EmitLog is nil
	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return nil, fmt.Errorf("boom")
	}

	out, err := a.Execute(ctx)
	s.NoError(err) // error_policy=continue

	m := out.(map[string]any)
	s.Equal(1, m["failed"])
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

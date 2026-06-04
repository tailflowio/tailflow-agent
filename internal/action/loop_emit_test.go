package action

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type LoopEmitTestSuite struct {
	suite.Suite
}

func TestLoopEmit(t *testing.T) {
	suite.Run(t, new(LoopEmitTestSuite))
}

func (s *LoopEmitTestSuite) SetupTest() {}

func (s *LoopEmitTestSuite) TestLoopItemLabel_StringValue() {
	label := loopItemLabel(map[string]any{"item": "hello", "index": 0})
	s.Equal("hello", label)
}

func (s *LoopEmitTestSuite) TestLoopItemLabel_MapWithPath() {
	label := loopItemLabel(map[string]any{
		"item": map[string]any{"path": "/tmp/file.txt", "extra": 42},
	})
	s.Equal("/tmp/file.txt", label)
}

func (s *LoopEmitTestSuite) TestLoopItemLabel_MapWithName() {
	label := loopItemLabel(map[string]any{
		"item": map[string]any{"name": "my-resource"},
	})
	s.Equal("my-resource", label)
}

func (s *LoopEmitTestSuite) TestLoopItemLabel_MapWithId() {
	label := loopItemLabel(map[string]any{
		"item": map[string]any{"id": "abc-123"},
	})
	s.Equal("abc-123", label)
}

func (s *LoopEmitTestSuite) TestLoopItemLabel_MapWithTitle() {
	label := loopItemLabel(map[string]any{
		"item": map[string]any{"title": "My Title"},
	})
	s.Equal("My Title", label)
}

func (s *LoopEmitTestSuite) TestLoopItemLabel_MapNoRecognizedField() {
	label := loopItemLabel(map[string]any{
		"item": map[string]any{"foo": "bar"},
	})
	s.Equal("", label)
}

func (s *LoopEmitTestSuite) TestLoopItemLabel_SkipsIndexAndPrev() {
	label := loopItemLabel(map[string]any{
		"index": 0,
		"prev":  "something",
	})
	s.Equal("", label)
}

func (s *LoopEmitTestSuite) TestLoopItemLabel_EmptyMap() {
	label := loopItemLabel(map[string]any{})
	s.Equal("", label)
}

func (s *LoopEmitTestSuite) TestLoopItemLabel_NonStringNonMapValue() {
	label := loopItemLabel(map[string]any{
		"item": 42,
	})
	s.Equal("", label)
}

func (s *LoopEmitTestSuite) TestLoopItemLabel_MapFieldEmpty() {
	label := loopItemLabel(map[string]any{
		"item": map[string]any{"path": "", "name": "", "id": "", "title": ""},
	})
	s.Equal("", label)
}

func (s *LoopEmitTestSuite) TestLastStdoutLine_NonMapOutput() {
	result := lastStdoutLine("not a map")
	s.Equal("", result)
}

func (s *LoopEmitTestSuite) TestLastStdoutLine_NilOutput() {
	result := lastStdoutLine(nil)
	s.Equal("", result)
}

func (s *LoopEmitTestSuite) TestLastStdoutLine_EmptyStdout() {
	result := lastStdoutLine(map[string]any{"stdout": ""})
	s.Equal("", result)
}

func (s *LoopEmitTestSuite) TestLastStdoutLine_WhitespaceOnlyStdout() {
	result := lastStdoutLine(map[string]any{"stdout": "   \n\n   "})
	s.Equal("", result)
}

func (s *LoopEmitTestSuite) TestLastStdoutLine_SingleLine() {
	result := lastStdoutLine(map[string]any{"stdout": "only line"})
	s.Equal("only line", result)
}

func (s *LoopEmitTestSuite) TestLastStdoutLine_MultipleLines() {
	result := lastStdoutLine(map[string]any{"stdout": "first\nsecond\nthird"})
	s.Equal("third", result)
}

func (s *LoopEmitTestSuite) TestLastStdoutLine_TrailingNewlines() {
	result := lastStdoutLine(map[string]any{"stdout": "first\nsecond\n\n"})
	s.Equal("second", result)
}

func (s *LoopEmitTestSuite) TestLastStdoutLine_NoStdoutKey() {
	result := lastStdoutLine(map[string]any{"stderr": "some error"})
	s.Equal("", result)
}

func (s *LoopEmitTestSuite) TestExtractResultDetail_NonIterationResult() {
	result := extractResultDetail[any]("just a string")
	s.Equal("", result)
}

func (s *LoopEmitTestSuite) TestExtractResultDetail_WithStdout() {
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

func (s *LoopEmitTestSuite) TestExtractResultDetail_EmptyStdout() {
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

func (s *LoopEmitTestSuite) TestExtractResultDetail_NonMapOutput() {
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

func (s *LoopEmitTestSuite) TestExtractResultDetail_MultipleActions_LastHasStdout() {
	ir := iterationResult{
		Actions: []actionResult{
			{Action: "step1", Output: map[string]any{"stdout": ""}},
			{Action: "step2", Output: map[string]any{"stdout": "final output"}},
		},
	}
	result := extractResultDetail(ir)
	s.Equal("final output", result)
}

func (s *LoopEmitTestSuite) TestExtractResultDetail_MultipleActions_FirstHasStdoutLastEmpty() {
	ir := iterationResult{
		Actions: []actionResult{
			{Action: "step1", Output: map[string]any{"stdout": "good output"}},
			{Action: "step2", Output: map[string]any{"stdout": ""}},
		},
	}
	result := extractResultDetail(ir)
	s.Equal("good output", result)
}

func (s *LoopEmitTestSuite) TestExtractResultDetail_EmptyActions() {
	ir := iterationResult{
		Actions: []actionResult{},
	}
	result := extractResultDetail(ir)
	s.Equal("", result)
}

func (s *LoopEmitTestSuite) TestEmitStepStart_NilEmitLog() {
	ctx := newTestContext(map[string]any{})
	emitStepStart(ctx, 0, 1, 0, 1, "step1", "label")
}

func (s *LoopEmitTestSuite) TestEmitStepSuccess_NilEmitLog() {
	ctx := newTestContext(map[string]any{})
	emitStepSuccess(ctx, 0, 1, 0, 1, "step1", "label", 100)
}

func (s *LoopEmitTestSuite) TestEmitStepStart_WithLabel() {
	var logs []string
	ctx := newTestContext(map[string]any{})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }

	emitStepStart(ctx, 0, 3, 0, 2, "step1", "my-file.txt")

	s.Require().Len(logs, 1)
	s.Equal("[1/3] my-file.txt", logs[0])
}

func (s *LoopEmitTestSuite) TestEmitStepStart_WithoutLabel() {
	var logs []string
	ctx := newTestContext(map[string]any{})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }

	emitStepStart(ctx, 1, 3, 0, 2, "fetch", "")

	s.Require().Len(logs, 1)
	s.Equal("[2/3] step 1/2 fetch", logs[0])
}

func (s *LoopEmitTestSuite) TestEmitStepSuccess_WithLabel() {
	var logs []string
	ctx := newTestContext(map[string]any{})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }

	emitStepSuccess(ctx, 0, 3, 0, 2, "step1", "my-file.txt", 150)

	s.Require().Len(logs, 1)
	s.Equal("[1/3] my-file.txt OK (150ms)", logs[0])
}

func (s *LoopEmitTestSuite) TestEmitStepSuccess_WithoutLabel() {
	var logs []string
	ctx := newTestContext(map[string]any{})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }

	emitStepSuccess(ctx, 2, 5, 1, 3, "transform", "", 250)

	s.Require().Len(logs, 1)
	s.Equal("[3/5] step 2/3 transform OK (250ms)", logs[0])
}

func (s *LoopEmitTestSuite) TestAppendFailedStep_LabelAndHint() {
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

func (s *LoopEmitTestSuite) TestAppendFailedStep_LabelNoHint() {
	var logs []string
	ctx := newTestContext(map[string]any{})
	ctx.EmitLog = func(msg string) { logs = append(logs, msg) }

	ps := pipelineStep{Action: "exec", Config: map[string]any{}}
	results, err := appendFailedStep(ctx, nil, ps, nil, 200,
		fmt.Errorf("exec failed"), 1, 5, 0, 2, "my-resource")

	s.Error(err)
	s.Len(results, 1)
	s.Require().Len(logs, 1)
	s.Equal("[2/5] my-resource FAILED (200ms)", logs[0])
}

func (s *LoopEmitTestSuite) TestAppendFailedStep_NoLabelNoHint() {
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

func (s *LoopEmitTestSuite) TestAppendFailedStep_NilEmitLog() {
	ctx := newTestContext(map[string]any{})

	ps := pipelineStep{Action: "exec", Config: map[string]any{}}
	results, err := appendFailedStep(ctx, nil, ps, nil, 100,
		fmt.Errorf("boom"), 0, 1, 0, 1, "label")

	s.Error(err)
	s.Len(results, 1)
	s.Equal("boom", results[0].Error)
}

func (s *LoopEmitTestSuite) TestAppendFailedStep_AppendsToExistingResults() {
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

func (s *LoopEmitTestSuite) TestBuildLoopOutput_WithDetail() {
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
	s.NoError(err)

	m := out.(map[string]any)
	loopErrors := m["errors"].([]any)
	s.Require().Len(loopErrors, 1)

	entry := loopErrors[0].(map[string]any)
	s.Equal("some detail line", entry["detail"])
}

func (s *LoopEmitTestSuite) TestBuildLoopOutput_NoErrors() {
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


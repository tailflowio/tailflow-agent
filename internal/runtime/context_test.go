package runtime

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ExecutionContextTestSuite struct {
	suite.Suite
}

func TestExecutionContext(t *testing.T) {
	suite.Run(t, new(ExecutionContextTestSuite))
}

func (s *ExecutionContextTestSuite) SetupTest() {
	// required by convention
}

func (s *ExecutionContextTestSuite) TestNewExecutionContext() {
	ctx := NewExecutionContext("exec-1", "my-workflow", map[string]any{"key": "val"}, map[string]string{"ENV": "prod"})

	s.Equal("exec-1", ctx.ExecutionID)
	s.Equal("my-workflow", ctx.WorkflowName)
	s.Equal("val", ctx.Params["key"])
	s.Equal("prod", ctx.Env["ENV"])
	s.NotNil(ctx.Steps)
	s.NotNil(ctx.Variables)
	s.NotNil(ctx.TriggerData)
}

func (s *ExecutionContextTestSuite) TestNewExecutionContext_NilParams() {
	ctx := NewExecutionContext("exec-1", "wf", nil, nil)

	s.NotNil(ctx.Params)
	s.NotNil(ctx.Env)
}

func (s *ExecutionContextTestSuite) TestSetAndGetStepResult() {
	ctx := NewExecutionContext("exec-1", "wf", nil, nil)

	result := &StepResult{Status: StatusSuccess, Output: "hello"}
	ctx.SetStepResult("step1", result)

	got, ok := ctx.GetStepResult("step1")
	s.True(ok)
	s.Equal(StatusSuccess, got.Status)
	s.Equal("hello", got.Output)
}

func (s *ExecutionContextTestSuite) TestGetStepResult_NotFound() {
	ctx := NewExecutionContext("exec-1", "wf", nil, nil)

	_, ok := ctx.GetStepResult("nonexistent")
	s.False(ok)
}

func (s *ExecutionContextTestSuite) TestClearStepResult() {
	ctx := NewExecutionContext("exec-1", "wf", nil, nil)

	ctx.SetStepResult("step1", &StepResult{Status: StatusSuccess})
	ctx.ClearStepResult("step1")

	_, ok := ctx.GetStepResult("step1")
	s.False(ok)
}

func (s *ExecutionContextTestSuite) TestSetAndGetVariable() {
	ctx := NewExecutionContext("exec-1", "wf", nil, nil)

	ctx.SetVariable("count", 42)

	val, ok := ctx.GetVariable("count")
	s.True(ok)
	s.Equal(42, val)
}

func (s *ExecutionContextTestSuite) TestGetVariable_NotFound() {
	ctx := NewExecutionContext("exec-1", "wf", nil, nil)

	_, ok := ctx.GetVariable("nonexistent")
	s.False(ok)
}

func (s *ExecutionContextTestSuite) TestToMap_Basic() {
	ctx := NewExecutionContext("exec-1", "wf",
		map[string]any{"env": "prod"},
		map[string]string{"API_URL": "http://example.com"},
	)
	ctx.SetVariable("counter", 10)
	ctx.TriggerData["method"] = "POST"

	m := ctx.ToMap()

	s.Equal(map[string]any{"env": "prod"}, m["params"])
	s.Equal(map[string]string{"API_URL": "http://example.com"}, m["env"])
	s.Equal(map[string]any{"counter": 10}, m["vars"])
	s.Equal(map[string]any{"method": "POST"}, m["trigger"])
}

func (s *ExecutionContextTestSuite) TestToMap_WithSteps() {
	ctx := NewExecutionContext("exec-1", "wf", nil, nil)
	ctx.SetStepResult("step1", &StepResult{
		Status: StatusSuccess,
		Output: "result1",
	})

	m := ctx.ToMap()

	steps := m["steps"].(map[string]any)
	step1 := steps["step1"].(map[string]any)
	s.Equal(StatusSuccess, step1["status"])
	s.Equal("result1", step1["output"])
}

func (s *ExecutionContextTestSuite) TestToMap_WithStepError() {
	ctx := NewExecutionContext("exec-1", "wf", nil, nil)
	ctx.SetStepResult("step1", &StepResult{
		Status: StatusFailed,
		Error:  &StepError{Message: "something failed", Code: "action_failed", StepID: "step1"},
	})

	m := ctx.ToMap()

	steps := m["steps"].(map[string]any)
	step1 := steps["step1"].(map[string]any)
	errMap := step1["error"].(map[string]any)
	s.Equal("something failed", errMap["message"])
	s.Equal("action_failed", errMap["code"])
	s.Equal("step1", errMap["step_id"])
}

func (s *ExecutionContextTestSuite) TestToMap_DeepCopyIsolation() {
	ctx := NewExecutionContext("exec-1", "wf", nil, nil)
	ctx.SetStepResult("step1", &StepResult{
		Status: StatusSuccess,
		Output: map[string]any{
			"items": []any{"a", "b"},
			"nested": map[string]any{
				"key": "val",
			},
		},
	})
	ctx.SetVariable("list", []any{1, 2, 3})

	m := ctx.ToMap()

	steps := m["steps"].(map[string]any)
	step1 := steps["step1"].(map[string]any)
	output := step1["output"].(map[string]any)
	items := output["items"].([]any)
	s.Equal([]any{"a", "b"}, items)

	vars := m["vars"].(map[string]any)
	varList := vars["list"].([]any)
	s.Equal([]any{1, 2, 3}, varList)

	items[0] = "MUTATED"
	varList[0] = 999

	original, _ := ctx.GetStepResult("step1")
	origOutput := original.Output.(map[string]any)
	origItems := origOutput["items"].([]any)
	s.Equal("a", origItems[0])

	origVar, _ := ctx.GetVariable("list")
	origList := origVar.([]any)
	s.Equal(1, origList[0])
}

func (s *ExecutionContextTestSuite) TestResolveParam_Found() {
	ctx := NewExecutionContext("exec-1", "wf", map[string]any{"env": "prod"}, nil)

	val, err := ctx.ResolveParam("env")
	s.Require().NoError(err)
	s.Equal("prod", val)
}

func (s *ExecutionContextTestSuite) TestResolveParam_NotFound() {
	ctx := NewExecutionContext("exec-1", "wf", nil, nil)

	_, err := ctx.ResolveParam("missing")
	s.Error(err)
	s.Contains(err.Error(), "not provided")
}

func (s *ExecutionContextTestSuite) TestStepError_Error() {
	e := &StepError{Message: "timeout reached"}
	s.Equal("timeout reached", e.Error())
}

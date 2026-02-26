package engine

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/runtime"
)

type ContextTestSuite struct {
	suite.Suite
}

func TestContext(t *testing.T) {
	suite.Run(t, new(ContextTestSuite))
}

func (s *ContextTestSuite) SetupTest() {}

func (s *ContextTestSuite) TestStepResults() {
	ctx := NewExecutionContext("exec-1", "test-wf", map[string]any{"env": "staging"}, nil)

	ctx.SetStepResult("step1", &StepResult{Status: "success", Output: "hello"})
	r, ok := ctx.GetStepResult("step1")
	s.Require().True(ok)
	s.Equal("success", r.Status)
	s.Equal("hello", r.Output)

	_, ok = ctx.GetStepResult("nonexistent")
	s.False(ok)
}

func (s *ContextTestSuite) TestVariables() {
	ctx := NewExecutionContext("exec-1", "test-wf", nil, nil)

	ctx.SetVariable("foo", "bar")
	v, ok := ctx.GetVariable("foo")
	s.Require().True(ok)
	s.Equal("bar", v)

	_, ok = ctx.GetVariable("nonexistent")
	s.False(ok)
}

func (s *ContextTestSuite) TestToMap() {
	ctx := NewExecutionContext("exec-1", "test-wf",
		map[string]any{"env": "staging"},
		map[string]string{"API_URL": "https://api.example.com"},
	)
	ctx.SetStepResult("step1", &StepResult{Status: "success", Output: "data"})
	ctx.SetVariable("counter", 42)
	ctx.TriggerData = map[string]any{"method": "POST"}

	m := ctx.ToMap()
	s.Equal("staging", m["params"].(map[string]any)["env"])
	s.Equal("https://api.example.com", m["env"].(map[string]string)["API_URL"])
	s.Equal(42, m["vars"].(map[string]any)["counter"])
	s.Equal("POST", m["trigger"].(map[string]any)["method"])

	steps := m["steps"].(map[string]any)
	step1 := steps["step1"].(map[string]any)
	s.Equal("success", step1["status"])
	s.Equal("data", step1["output"])
}

func (s *ContextTestSuite) TestToMap_StructuredError() {
	ctx := NewExecutionContext("exec-1", "test-wf", nil, nil)
	ctx.SetStepResult("fail_step", &StepResult{
		Status: "failed",
		Error:  &runtime.StepError{Message: "something broke", Code: "action_failed", StepID: "fail_step"},
	})

	m := ctx.ToMap()
	steps := m["steps"].(map[string]any)
	step := steps["fail_step"].(map[string]any)

	s.Equal("failed", step["status"])

	errMap, ok := step["error"].(map[string]any)
	s.Require().True(ok)
	s.Equal("something broke", errMap["message"])
	s.Equal("action_failed", errMap["code"])
	s.Equal("fail_step", errMap["step_id"])
}

func (s *ContextTestSuite) TestToMap_NoError() {
	ctx := NewExecutionContext("exec-1", "test-wf", nil, nil)
	ctx.SetStepResult("ok_step", &StepResult{Status: "success", Output: "data"})

	m := ctx.ToMap()
	steps := m["steps"].(map[string]any)
	step := steps["ok_step"].(map[string]any)

	s.Equal("success", step["status"])
	_, hasError := step["error"]
	s.False(hasError, "error key should not be present for successful steps")
}

func (s *ContextTestSuite) TestConcurrentAccess() {
	ctx := NewExecutionContext("exec-1", "test-wf", nil, nil)

	s.NotPanics(func() {
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(2)
			stepID := "step" + string(rune('a'+i%26))
			go func() {
				defer wg.Done()
				ctx.SetStepResult(stepID, &StepResult{Status: "success"})
			}()
			go func() {
				defer wg.Done()
				ctx.GetStepResult(stepID)
			}()
		}
		wg.Wait()
	}, "concurrent access to ExecutionContext should not panic")
}

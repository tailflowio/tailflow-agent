package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/parser"
)

type EngineUnsafeTestSuite struct {
	suite.Suite
}

func TestEngineUnsafe(t *testing.T) {
	suite.Run(t, new(EngineUnsafeTestSuite))
}

func (s *EngineUnsafeTestSuite) SetupTest() {}

func (s *EngineUnsafeTestSuite) TestFailedStep() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "failure",
		Steps: []parser.Step{
			{
				ID:     "fail",
				Action: "exec",
				Config: map[string]any{"command": "false"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err) // Execute itself doesn't error, the result has the error
	s.Equal("failed", result.Status)
	s.Equal("failed", result.Steps["fail"].Status)
}

func (s *EngineUnsafeTestSuite) TestRetryStep() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	// Use a JS script that fails on first calls
	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "retry",
		Steps: []parser.Step{
			{
				ID:     "set_counter",
				Action: "set",
				Config: map[string]any{"attempt_count": 0},
			},
			{
				ID:        "retry_step",
				Action:    "js",
				DependsOn: []string{"set_counter"},
				Config: map[string]any{
					"script": `
						var count = ctx.get("attempt_count");
						count++;
						ctx.set("attempt_count", count);
						if (count < 3) throw new Error("not yet: " + count);
						return "success on attempt " + count;
					`,
				},
				Retry: &parser.RetryConfig{MaxAttempts: 5, Delay: "1ms"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["retry_step"].Status)
}

func (s *EngineUnsafeTestSuite) TestOnError() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "onerror",
		Steps: []parser.Step{
			{
				ID:     "fail_step",
				Action: "exec",
				Config: map[string]any{"command": "false"},
				OnError: []parser.Step{
					{
						ID:     "recovery",
						Action: "set",
						Config: map[string]any{"recovered": true},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status) // Overall still fails
	s.Equal("success", result.Steps["recovery"].Status)
}

func (s *EngineUnsafeTestSuite) TestSkipDownstreamOnFailure() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "skip-downstream",
		Steps: []parser.Step{
			{ID: "fail", Action: "exec", Config: map[string]any{"command": "false"}},
			{ID: "after", Action: "log", DependsOn: []string{"fail"}, Config: map[string]any{"message": "should skip"}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
	s.Equal("failed", result.Steps["fail"].Status)
	s.Equal("skipped", result.Steps["after"].Status)
}

func (s *EngineUnsafeTestSuite) TestStepOutputAccess() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "output-access",
		Steps: []parser.Step{
			{
				ID:     "produce",
				Action: "set",
				Config: map[string]any{"value": "hello-from-produce"},
			},
			{
				ID:        "consume",
				Action:    "js",
				DependsOn: []string{"produce"},
				Config: map[string]any{
					"script": `return steps.produce.output.value`,
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("hello-from-produce", result.Steps["consume"].Output)
}

func (s *EngineUnsafeTestSuite) TestStepErrorStructured() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "step-error-structured",
		Steps: []parser.Step{
			{
				ID:     "fail",
				Action: "exec",
				Config: map[string]any{"command": "false"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)

	sr := result.Steps["fail"]
	s.Require().NotNil(sr.Error)
	s.Equal("action_failed", sr.Error.Code)
	s.Equal("fail", sr.Error.StepID)
	s.NotEmpty(sr.Error.Message)
}

func (s *EngineUnsafeTestSuite) TestStepErrorTimeout() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "step-error-timeout",
		Steps: []parser.Step{
			{
				ID:      "slow",
				Action:  "delay",
				Timeout: "1ms",
				Config:  map[string]any{"duration": "10s"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)

	sr := result.Steps["slow"]
	s.Require().NotNil(sr.Error)
	s.Equal("timeout", sr.Error.Code)
	s.Equal("slow", sr.Error.StepID)
}

//go:build !saas

package engine

import (
	"context"
	"fmt"
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
					"script": fmt.Sprintf(`return steps.produce.output.value`),
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("hello-from-produce", result.Steps["consume"].Output)
}

func (s *EngineUnsafeTestSuite) TestErrorPolicyStop() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "error-policy-stop",
		Steps: []parser.Step{
			{
				ID:          "fail",
				Action:      "exec",
				ErrorPolicy: "stop",
				Config:      map[string]any{"command": "false"},
			},
			{
				ID:        "after",
				Action:    "log",
				DependsOn: []string{"fail"},
				Config:    map[string]any{"message": "should skip"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
	s.Equal("failed", result.Steps["fail"].Status)
	s.Equal("skipped", result.Steps["after"].Status)
}

func (s *EngineUnsafeTestSuite) TestErrorPolicyContinue() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "error-policy-continue",
		Steps: []parser.Step{
			{
				ID:          "fail",
				Action:      "exec",
				ErrorPolicy: "continue",
				Config:      map[string]any{"command": "false"},
			},
			{
				ID:        "after",
				Action:    "log",
				DependsOn: []string{"fail"},
				Config:    map[string]any{"message": "should run"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("completed_with_errors", result.Status)
	s.True(result.HasErrors)
	s.Equal("failed", result.Steps["fail"].Status)
	s.Equal("success", result.Steps["after"].Status)
}

func (s *EngineUnsafeTestSuite) TestErrorPolicyIgnore() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "error-policy-ignore",
		Steps: []parser.Step{
			{
				ID:          "fail",
				Action:      "exec",
				ErrorPolicy: "ignore",
				Config:      map[string]any{"command": "false"},
			},
			{
				ID:        "after",
				Action:    "log",
				DependsOn: []string{"fail"},
				Config:    map[string]any{"message": "should run"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.False(result.HasErrors)
	s.Equal("success", result.Steps["fail"].Status)
	s.Equal("success", result.Steps["after"].Status)

	// Check suppressed error in output
	out, ok := result.Steps["fail"].Output.(map[string]any)
	s.Require().True(ok)
	s.NotEmpty(out["_suppressed_error"])
}

func (s *EngineUnsafeTestSuite) TestErrorPolicyContinueWithOnError() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "error-policy-continue-onerror",
		Steps: []parser.Step{
			{
				ID:          "fail",
				Action:      "exec",
				ErrorPolicy: "continue",
				Config:      map[string]any{"command": "false"},
				OnError: []parser.Step{
					{
						ID:     "recovery",
						Action: "set",
						Config: map[string]any{"recovered": true},
					},
				},
			},
			{
				ID:        "after",
				Action:    "log",
				DependsOn: []string{"fail"},
				Config:    map[string]any{"message": "should run"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("completed_with_errors", result.Status)
	s.Equal("success", result.Steps["recovery"].Status)
	s.Equal("success", result.Steps["after"].Status)
}

func (s *EngineUnsafeTestSuite) TestLoopPipeline() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "loop-pipeline",
		Steps: []parser.Step{
			{
				ID:     "prepare",
				Action: "set",
				Config: map[string]any{
					"users": []any{
						map[string]any{"name": "Alice"},
						map[string]any{"name": "Bob"},
					},
				},
			},
			{
				ID:        "pipeline_loop",
				Action:    "loop",
				DependsOn: []string{"prepare"},
				Config: map[string]any{
					"items": "{{ vars.users }}",
					"as":    "user",
					"index": "i",
					"actions": []any{
						map[string]any{
							"action": "js",
							"config": map[string]any{
								"script": `return { greeting: "Hello {{ loop.user.name }}" }`,
							},
						},
						map[string]any{
							"action": "js",
							"config": map[string]any{
								"script": `return { message: "{{ loop.prev.greeting }}!" }`,
							},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["pipeline_loop"].Status)

	// Check structured output
	out, ok := result.Steps["pipeline_loop"].Output.(map[string]any)
	s.Require().True(ok)
	s.Equal(2, out["iterations"])
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

func (s *EngineUnsafeTestSuite) TestWorkflowOnError_Basic() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "wf-onerror",
		OnError: []parser.Step{
			{
				ID:     "wf_recovery",
				Action: "set",
				Config: map[string]any{"wf_failed": true},
			},
		},
		Steps: []parser.Step{
			{
				ID:     "fail_step",
				Action: "exec",
				Config: map[string]any{"command": "false"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
	// Workflow on_error should have run
	s.Equal("success", result.Steps["wf_recovery"].Status)
}

func (s *EngineUnsafeTestSuite) TestWorkflowOnError_NotRunOnSuccess() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "wf-onerror-success",
		OnError: []parser.Step{
			{
				ID:     "wf_recovery",
				Action: "set",
				Config: map[string]any{"wf_failed": true},
			},
		},
		Steps: []parser.Step{
			{
				ID:     "ok_step",
				Action: "log",
				Config: map[string]any{"message": "all good"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	// Workflow on_error should NOT have run
	_, exists := result.Steps["wf_recovery"]
	s.False(exists)
}

func (s *EngineUnsafeTestSuite) TestWorkflowOnError_AccessErrorCode() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "wf-onerror-code",
		OnError: []parser.Step{
			{
				ID:     "check_error",
				Action: "js",
				Config: map[string]any{
					"script": `return { code: steps.fail_step.error.code, msg: steps.fail_step.error.message }`,
				},
			},
		},
		Steps: []parser.Step{
			{
				ID:     "fail_step",
				Action: "exec",
				Config: map[string]any{"command": "false"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)

	sr := result.Steps["check_error"]
	s.Require().NotNil(sr)
	s.Equal("success", sr.Status)
	out, ok := sr.Output.(map[string]any)
	s.Require().True(ok)
	s.Equal("action_failed", out["code"])
	s.NotEmpty(out["msg"])
}

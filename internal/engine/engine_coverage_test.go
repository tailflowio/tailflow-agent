//go:build !saas

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/parser"
)

// EngineCoverageTestSuite adds tests to reach 100% coverage on
// skipNode, executeWithRetry, waitForRetry, applyTimeout, executeOnError, hasFailedSteps.
type EngineCoverageTestSuite struct {
	suite.Suite
}

func TestEngineCoverage(t *testing.T) {
	suite.Run(t, new(EngineCoverageTestSuite))
}

func (s *EngineCoverageTestSuite) SetupTest() {}

// TestSkipNode_CloseDoneChannel covers the branch where skipNode detects
// that all nodes are completed and closes the done channel.
// This is exercised when the only step fails and its single child is skipped,
// bringing completed == total. The existing TestSkipDownstreamOnFailure already
// covers the "not done yet" path; this test ensures two downstream children
// are both skipped and the second one triggers the close.
func (s *EngineCoverageTestSuite) TestSkipNode_MultipleChildrenAllSkipped() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "skip-multi-children",
		Steps: []parser.Step{
			{ID: "fail", Action: "exec", Config: map[string]any{"command": "false"}},
			{ID: "child1", Action: "log", DependsOn: []string{"fail"}, Config: map[string]any{"message": "a"}},
			{ID: "child2", Action: "log", DependsOn: []string{"fail"}, Config: map[string]any{"message": "b"}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
	s.Equal("skipped", result.Steps["child1"].Status)
	s.Equal("skipped", result.Steps["child2"].Status)
}

// TestExecuteWithRetry_Exhaustion covers the branch where all retry attempts
// are exhausted and the last error is returned.
func (s *EngineCoverageTestSuite) TestExecuteWithRetry_Exhaustion() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "retry-exhausted",
		Steps: []parser.Step{
			{
				ID:     "always_fail",
				Action: "exec",
				Config: map[string]any{"command": "false"},
				Retry:  &parser.RetryConfig{MaxAttempts: 3, Delay: "1ms"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
	s.Equal("failed", result.Steps["always_fail"].Status)
}

// TestExecuteWithRetry_TimeoutDuringRetry covers the branch where a timeout
// fires during a retry attempt.
func (s *EngineCoverageTestSuite) TestExecuteWithRetry_TimeoutDuringRetry() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "retry-timeout",
		Steps: []parser.Step{
			{
				ID:      "slow_retry",
				Action:  "delay",
				Timeout: "10ms",
				Config:  map[string]any{"duration": "10s"},
				Retry:   &parser.RetryConfig{MaxAttempts: 2, Delay: "1ms"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
	s.Require().NotNil(result.Steps["slow_retry"].Error)
	s.Equal("timeout", result.Steps["slow_retry"].Error.Code)
}

// TestWaitForRetry_ContextCancellation covers the branch where context is
// cancelled during the retry wait delay.
func (s *EngineCoverageTestSuite) TestWaitForRetry_ContextCancellation() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	// Cancel context after a very short delay, while retry delay is long
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "retry-cancel",
		Steps: []parser.Step{
			{
				ID:     "fail_then_wait",
				Action: "exec",
				Config: map[string]any{"command": "false"},
				// Long retry delay so the context cancels during wait
				Retry: &parser.RetryConfig{MaxAttempts: 10, Delay: "30s"},
			},
		},
	}

	result, err := exec.Execute(ctx, wf, nil)
	s.Require().NoError(err)
	s.Equal("cancelled", result.Status)
}

// TestApplyTimeout_WithStepTimeout covers the branch where a valid step
// timeout is configured and a timeout context is created.
func (s *EngineCoverageTestSuite) TestApplyTimeout_WithStepTimeout() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "timeout-configured",
		Steps: []parser.Step{
			{
				ID:      "fast_step",
				Action:  "log",
				Timeout: "5s",
				Config:  map[string]any{"message": "within timeout"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["fast_step"].Status)
}

// TestApplyTimeout_InvalidDuration covers the branch where step timeout
// cannot be parsed, so the original context is used.
func (s *EngineCoverageTestSuite) TestApplyTimeout_InvalidDuration() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "timeout-invalid",
		Steps: []parser.Step{
			{
				ID:      "bad_timeout",
				Action:  "log",
				Timeout: "not-a-duration",
				Config:  map[string]any{"message": "runs anyway"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["bad_timeout"].Status)
}

// TestExecuteOnError_ResolveConfigError covers the on_error branch where
// config resolution fails.
func (s *EngineCoverageTestSuite) TestExecuteOnError_ResolveConfigError() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "onerror-resolve-fail",
		Steps: []parser.Step{
			{
				ID:     "fail_step",
				Action: "exec",
				Config: map[string]any{"command": "false"},
				OnError: []parser.Step{
					{
						ID:     "bad_onerror",
						Action: "log",
						Config: map[string]any{"message": "{{ invalid syntax === }}"},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
	// on_error step should NOT have a success result since config resolution failed
	_, exists := result.Steps["bad_onerror"]
	s.False(exists)
}

// TestExecuteOnError_UnknownAction covers the on_error branch where the
// action is unknown.
func (s *EngineCoverageTestSuite) TestExecuteOnError_UnknownAction() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "onerror-unknown-action",
		Steps: []parser.Step{
			{
				ID:     "fail_step",
				Action: "exec",
				Config: map[string]any{"command": "false"},
				OnError: []parser.Step{
					{
						ID:     "bad_action_onerror",
						Action: "nonexistent_xyz",
						Config: map[string]any{},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
	// on_error step should NOT succeed since the action does not exist
	_, exists := result.Steps["bad_action_onerror"]
	s.False(exists)
}

// TestExecuteOnError_ExecutionError covers the on_error branch where the
// on_error step itself fails during execution.
func (s *EngineCoverageTestSuite) TestExecuteOnError_ExecutionError() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "onerror-exec-fail",
		Steps: []parser.Step{
			{
				ID:     "fail_step",
				Action: "exec",
				Config: map[string]any{"command": "false"},
				OnError: []parser.Step{
					{
						ID:     "onerror_also_fails",
						Action: "exec",
						Config: map[string]any{"command": "false"},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
	// on_error step ran but failed
	s.Equal("failed", result.Steps["onerror_also_fails"].Status)
}

// TestHasFailedSteps_NoFailedSteps covers the false branch of hasFailedSteps
// when all steps succeed. Validated by confirming workflow-level on_error is
// NOT triggered.
func (s *EngineCoverageTestSuite) TestHasFailedSteps_NoFailedSteps() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "no-failures",
		OnError: []parser.Step{
			{ID: "should_not_run", Action: "set", Config: map[string]any{"ran": true}},
		},
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "ok"}},
			{ID: "s2", Action: "set", DependsOn: []string{"s1"}, Config: map[string]any{"val": 1}},
			{ID: "s3", Action: "log", DependsOn: []string{"s2"}, Config: map[string]any{"message": "ok2"}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.False(result.HasErrors)
	_, exists := result.Steps["should_not_run"]
	s.False(exists)
}

// TestExecuteWithRetry_InvalidRetryDelay covers the branch where the retry
// delay string is invalid and defaults to 1 second.
func (s *EngineCoverageTestSuite) TestExecuteWithRetry_InvalidRetryDelay() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "retry-bad-delay",
		Steps: []parser.Step{
			{
				ID:     "set_counter",
				Action: "set",
				Config: map[string]any{"attempt_count": 0},
			},
			{
				ID:        "retry_bad_delay",
				Action:    "js",
				DependsOn: []string{"set_counter"},
				Config: map[string]any{
					"script": `
						var count = ctx.get("attempt_count");
						count++;
						ctx.set("attempt_count", count);
						if (count < 2) throw new Error("not yet: " + count);
						return "ok";
					`,
				},
				// "bad" is not a valid Go duration string
				Retry: &parser.RetryConfig{MaxAttempts: 2, Delay: "bad"},
			},
		},
	}

	// This will log a warning about invalid retry delay and use 1s default.
	// We use a short test timeout to avoid actually waiting 1s.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := exec.Execute(ctx, wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
}

// TestWorkflowOnError_WithFailedStepsAndContinuePolicy covers the on_error
// workflow-level handler triggered by hasFailedSteps (error_policy=continue
// means execErr is nil but hasFailedSteps returns true).
func (s *EngineCoverageTestSuite) TestWorkflowOnError_WithFailedStepsAndContinuePolicy() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "wf-onerror-continue",
		OnError: []parser.Step{
			{ID: "wf_handler", Action: "set", Config: map[string]any{"handled": true}},
		},
		Steps: []parser.Step{
			{
				ID:          "fail_continue",
				Action:      "exec",
				ErrorPolicy: "continue",
				Config:      map[string]any{"command": "false"},
			},
			{
				ID:        "after",
				Action:    "log",
				DependsOn: []string{"fail_continue"},
				Config:    map[string]any{"message": "still runs"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("completed_with_errors", result.Status)
	// Workflow on_error should have run because hasFailedSteps returns true
	s.Equal("success", result.Steps["wf_handler"].Status)
}

func (s *EngineCoverageTestSuite) TestEmitLogActionOutput_WithStream() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "log-stream",
		Steps: []parser.Step{
			{
				ID:     "stream_log",
				Action: "log",
				Config: map[string]any{"message": "streaming", "stream": true},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
}

func (s *EngineCoverageTestSuite) TestStepErrorCode_Canceled() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "cancel-step",
		Steps: []parser.Step{
			{
				ID:     "slow",
				Action: "delay",
				Config: map[string]any{"duration": "10s"},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	result, err := exec.Execute(ctx, wf, nil)
	s.Require().NoError(err)
	s.Equal("cancelled", result.Status)

	sr := result.Steps["slow"]
	s.Require().NotNil(sr.Error)
	s.Equal("cancelled", sr.Error.Code)
}

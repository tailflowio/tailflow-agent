//go:build !saas

package engine

import (
	"context"

	"github.com/tailflow/tailflow/internal/parser"
)

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

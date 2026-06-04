package engine

import (
	"context"

	"github.com/tailflow/tailflow/internal/parser"
)

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

package engine

import (
	"context"
	"time"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
)

// TestParamPatternNotProvided covers the case where a param has a pattern
// but the param value is not provided (and not required), so the pattern
// validation is skipped.
func (s *EngineTestSuite) TestParamPatternNotProvided() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "pattern-not-provided",
		Params: []parser.Param{
			{Name: "host", Type: "string", Pattern: "[a-z]+"},
		},
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "no host"}},
		},
	}

	// Don't provide the "host" param at all
	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
}

// TestParamPatternNonString covers the case where a param has a pattern
// but the provided value is not a string (e.g. int), so the pattern check is skipped.
func (s *EngineTestSuite) TestParamPatternNonString() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "pattern-non-string",
		Params: []parser.Param{
			{Name: "count", Type: "int", Pattern: "[0-9]+"},
		},
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "count is set"}},
		},
	}

	// Provide an integer instead of a string
	result, err := exec.Execute(context.Background(), wf, map[string]any{"count": 42})
	s.Require().NoError(err)
	s.Equal("success", result.Status)
}

// TestInvalidPatternRegex covers the case where the pattern itself is not a valid regex.
func (s *EngineTestSuite) TestInvalidPatternRegex() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "invalid-pattern",
		Params: []parser.Param{
			{Name: "host", Type: "string", Pattern: "[invalid(regex"},
		},
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "test"}},
		},
	}

	_, err := exec.Execute(context.Background(), wf, map[string]any{"host": "test"})
	s.Error(err)
	s.Contains(err.Error(), "invalid pattern")
}

// TestEnvResolutionError covers the error branch in env variable resolution.
func (s *EngineTestSuite) TestEnvResolutionError() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "env-error",
		Env:     map[string]string{"BAD": "{{ invalid syntax === }}"},
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "test"}},
		},
	}

	_, err := exec.Execute(context.Background(), wf, nil)
	s.Error(err)
	s.Contains(err.Error(), "resolve env")
}

// TestBuildDAGError covers the DAG build error branch (unknown dependency).
func (s *EngineTestSuite) TestBuildDAGError() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "dag-error",
		Steps: []parser.Step{
			{ID: "s1", Action: "log", DependsOn: []string{"nonexistent"}, Config: map[string]any{"message": "test"}},
		},
	}

	_, err := exec.Execute(context.Background(), wf, nil)
	s.Error(err)
	s.Contains(err.Error(), "build DAG")
}

// TestUnknownAction covers the branch where a step references an unregistered action.
func (s *EngineTestSuite) TestUnknownAction() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "unknown-action",
		Steps: []parser.Step{
			{ID: "s1", Action: "nonexistent_action", Config: map[string]any{}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

// TestActionValidateError covers the branch where a step's action Validate returns an error.
func (s *EngineTestSuite) TestActionValidateError() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	// The "log" action requires a "message" config key. Omitting it should fail validation.
	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "validate-error",
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

// TestWhenConditionError covers the branch where the when expression evaluation fails.
func (s *EngineTestSuite) TestWhenConditionError() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "when-error",
		Steps: []parser.Step{
			{
				ID:     "s1",
				Action: "log",
				When:   "invalid syntax ===",
				Config: map[string]any{"message": "test"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

// TestResolveConfigError covers the branch where config template resolution fails.
func (s *EngineTestSuite) TestResolveConfigError() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "resolve-config-error",
		Steps: []parser.Step{
			{
				ID:     "s1",
				Action: "log",
				Config: map[string]any{"message": "{{ invalid syntax === }}"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

// TestHasFailedStepsAllSuccess verifies hasFailedSteps returns false when all steps succeed.
// This is implicitly tested via a fully successful workflow.
func (s *EngineTestSuite) TestHasFailedStepsAllSuccess() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "all-success",
		OnError: []parser.Step{
			{ID: "cleanup", Action: "set", Config: map[string]any{"ran": true}},
		},
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "ok"}},
			{ID: "s2", Action: "log", DependsOn: []string{"s1"}, Config: map[string]any{"message": "ok2"}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.False(result.HasErrors)
	// on_error should NOT have run
	_, exists := result.Steps["cleanup"]
	s.False(exists)
}

// TestTriggerDataInStepInput covers the trigger data in step input event emission.
func (s *EngineTestSuite) TestTriggerDataInStepInput() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	ch := bus.Subscribe(100)

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "trigger-input",
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "hi"}},
		},
	}

	opts := ExecuteOptions{
		TriggerData: map[string]any{"method": "POST", "path": "/hook"},
	}

	result, err := exec.Execute(context.Background(), wf, nil, opts)
	s.Require().NoError(err)
	s.Equal("success", result.Status)

	// Verify a StepInput event includes trigger data
	var found bool
	s.Eventually(func() bool {
		for {
			select {
			case ev := <-ch:
				if ev.Type == event.StepInput && ev.Data != nil {
					_, ok := ev.Data["trigger"]
					if ok {
						found = true
						return true
					}
				}
			default:
				return found
			}
		}
	}, 2*time.Second, 10*time.Millisecond)
	s.True(found)
}

// TestLoopActionConfigDeferred covers the engine's special handling of loop action's
// action_config key — it must be kept unresolved until the loop action resolves it.
func (s *EngineTestSuite) TestLoopActionConfigDeferred() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "loop-action-config",
		Steps: []parser.Step{
			{
				ID:     "prepare",
				Action: "set",
				Config: map[string]any{
					"items": []any{"a", "b"},
				},
			},
			{
				ID:        "loop_step",
				Action:    "loop",
				DependsOn: []string{"prepare"},
				Config: map[string]any{
					"items": "{{ vars.items }}",
					"as":    "item",
					"action_config": map[string]any{
						"val": "{{ loop.item }}",
					},
					"action": "set",
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["loop_step"].Status)
}

// TestLoopRunActionResolveConfigError covers the RunAction closure's config
// resolution error path (engine.go line 519-521).
func (s *EngineTestSuite) TestLoopRunActionResolveConfigError() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "loop-resolve-error",
		Steps: []parser.Step{
			{
				ID:     "prepare",
				Action: "set",
				Config: map[string]any{
					"items": []any{"x"},
				},
			},
			{
				ID:        "loop_step",
				Action:    "loop",
				DependsOn: []string{"prepare"},
				Config: map[string]any{
					"items":  "{{ vars.items }}",
					"as":     "item",
					"action": "set",
					"action_config": map[string]any{
						"val": "{{ invalid syntax === }}",
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

// TestLoopRunActionUnknownAction covers the RunAction closure's unknown action
// error path (engine.go line 525-527).
func (s *EngineTestSuite) TestLoopRunActionUnknownAction() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "loop-unknown-action",
		Steps: []parser.Step{
			{
				ID:     "prepare",
				Action: "set",
				Config: map[string]any{
					"items": []any{"x"},
				},
			},
			{
				ID:        "loop_step",
				Action:    "loop",
				DependsOn: []string{"prepare"},
				Config: map[string]any{
					"items":         "{{ vars.items }}",
					"as":            "item",
					"action":        "nonexistent_action_xyz",
					"action_config": map[string]any{},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

// TestLoopRunActionValidateError covers the RunAction closure's validate error
// path (engine.go line 541-543).
func (s *EngineTestSuite) TestLoopRunActionValidateError() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "loop-validate-error",
		Steps: []parser.Step{
			{
				ID:     "prepare",
				Action: "set",
				Config: map[string]any{
					"items": []any{"x"},
				},
			},
			{
				ID:        "loop_step",
				Action:    "loop",
				DependsOn: []string{"prepare"},
				Config: map[string]any{
					"items":  "{{ vars.items }}",
					"as":     "item",
					"action": "log",
					"action_config": map[string]any{
						// "log" action requires "message" — omitting it causes validate error
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

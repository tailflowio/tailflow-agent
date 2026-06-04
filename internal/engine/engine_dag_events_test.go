package engine

import (
	"context"
	"time"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

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

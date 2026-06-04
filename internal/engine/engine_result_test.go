package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

type EngineResultTestSuite struct {
	suite.Suite
}

func TestEngineResult(t *testing.T) {
	suite.Run(t, new(EngineResultTestSuite))
}

func (s *EngineResultTestSuite) SetupTest() {}

// TestExecute_RecoveryWorkflow_CancelledDuringExecution covers two uncovered branches:
//  1. engine.go: `if ctx.Err() != nil && wf.Recovery` → true path (lines 100-112)
//  2. engine_result.go emitInitialExecutionState: `if wf.Recovery` → true path (line 88)
func (s *EngineResultTestSuite) TestExecute_RecoveryWorkflow_CancelledDuringExecution() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	wf := &parser.Workflow{
		Version:  "2.0",
		Name:     "recovery-cancel",
		Recovery: true,
		Steps: []parser.Step{
			{
				ID:     "long_step",
				Action: "delay",
				Config: map[string]any{"duration": "10s"},
			},
		},
	}

	result, err := exec.Execute(ctx, wf, nil)
	s.Require().NoError(err)
	s.Equal(runtime.StatusCancelled, result.Status)
}

// TestEmitInitialExecutionState_WithRecoveryFlag exercises the
// `if wf.Recovery { data["recovery"] = true }` branch in emitInitialExecutionState
// for a workflow that completes successfully (no cancellation).
func (s *EngineResultTestSuite) TestEmitInitialExecutionState_WithRecoveryFlag() {
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
		Version:  "2.0",
		Name:     "recovery-flag-emit",
		Recovery: true,
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "ok"}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)

	s.Eventually(func() bool {
		return stateEvents.load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	events := stateEvents.snapshot()
	s.NotEmpty(events)
	recoveryVal, _ := events[0].Data["recovery"].(bool)
	s.True(recoveryVal, "initial state event should carry recovery=true")
}

// TestResolveParams_TemplateDefaultSkippedWhenParamProvided covers the true branch of
// `if _, ok := params[p.Name]; ok { continue }` inside resolveParams second loop.
// The param has a template-string default but the caller supplies a value,
// so the template expression evaluation is skipped.
func (s *EngineResultTestSuite) TestResolveParams_TemplateDefaultSkippedWhenParamProvided() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "template-default-provided",
		Params: []parser.Param{
			{
				Name:    "base",
				Type:    "string",
				Default: "staging",
			},
			{
				Name: "region",
				Type: "string",
				// Template default — would reference params.base, but caller provides "us-east"
				Default: "{{ params.base }}-default",
			},
		},
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "{{ params.region }}"}},
		},
	}

	// Provide "region" explicitly → template default must NOT be evaluated (the continue branch).
	result, err := exec.Execute(context.Background(), wf, map[string]any{"region": "us-east"})
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)
	out, _ := result.Steps["s1"].Output.(map[string]any)
	s.Equal("us-east", out["message"])
}

// TestResolveParams_TemplateDefaultEvaluated covers the Eval path inside resolveParams
// second loop: when a param has a template-string default and the caller does NOT provide
// a value, the expression is compiled and evaluated.
func (s *EngineResultTestSuite) TestResolveParams_TemplateDefaultEvaluated() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "template-default-eval",
		Params: []parser.Param{
			{
				Name:    "base",
				Type:    "string",
				Default: "staging",
			},
			{
				Name: "derived",
				Type: "string",
				// Template default references another param — Eval branch is exercised.
				Default: "{{ params.base }}",
			},
		},
		Steps: []parser.Step{
			{ID: "s1", Action: "log", Config: map[string]any{"message": "{{ params.derived }}"}},
		},
	}

	// Do NOT provide "derived" → template default {{ params.base }} must be evaluated.
	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal(runtime.StatusSuccess, result.Status)
	out, _ := result.Steps["s1"].Output.(map[string]any)
	s.Equal("staging", out["message"])
}

//go:build !saas

package engine

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

type GotoTestSuite struct {
	suite.Suite
}

func TestGoto(t *testing.T) {
	suite.Run(t, new(GotoTestSuite))
}

func (s *GotoTestSuite) SetupTest() {}

func (s *GotoTestSuite) TestHandleGoto_NoLoop() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	node := &DAGNode{Step: parser.Step{
		ID:     "x",
		Action: "log",
		Goto:   &parser.GotoConfig{Target: "a", When: "true", MaxIterations: 5},
	}}

	ready, triggered := exec.handleGoto(node, nil, nil, map[string]loopInfo{}, nil, nil, new(int))
	s.Nil(ready)
	s.False(triggered)
}

func (s *GotoTestSuite) TestGotoBasic() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	// init sets count=0, increment adds 1 via JS, update stores it back,
	// check gotos back to increment while count < 2.
	// Expected: loops 2 times, final count = 2.
	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "goto-basic",
		Steps: []parser.Step{
			{
				ID:     "init",
				Action: "set",
				Config: map[string]any{"count": 0},
			},
			{
				ID:        "increment",
				Action:    "js",
				DependsOn: []string{"init"},
				Config: map[string]any{
					"script": `return { count: vars.count + 1 };`,
				},
			},
			{
				ID:        "update",
				Action:    "set",
				DependsOn: []string{"increment"},
				Config:    map[string]any{"count": "{{ steps.increment.output.count }}"},
			},
			{
				ID:        "check",
				Action:    "log",
				DependsOn: []string{"update"},
				Config:    map[string]any{"message": "count={{ vars.count }}"},
				Goto: &parser.GotoConfig{
					Target:        "increment",
					When:          "vars.count < 2",
					MaxIterations: 10,
				},
			},
			{
				ID:        "done",
				Action:    "log",
				DependsOn: []string{"check"},
				Config:    map[string]any{"message": "final"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["done"].Status)
}

func (s *GotoTestSuite) TestGotoMaxIterations() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	ch := bus.Subscribe(100)

	// This goto condition is always true, so it should hit max_iterations (2)
	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "goto-max",
		Steps: []parser.Step{
			{
				ID:     "init",
				Action: "set",
				Config: map[string]any{"x": 0},
			},
			{
				ID:        "work",
				Action:    "js",
				DependsOn: []string{"init"},
				Config: map[string]any{
					"script": `return { x: vars.x + 1 };`,
				},
			},
			{
				ID:        "save",
				Action:    "set",
				DependsOn: []string{"work"},
				Config:    map[string]any{"x": "{{ steps.work.output.x }}"},
			},
			{
				ID:        "loop",
				Action:    "log",
				DependsOn: []string{"save"},
				Config:    map[string]any{"message": "looping"},
				Goto: &parser.GotoConfig{
					Target:        "work",
					When:          "true",
					MaxIterations: 2,
				},
			},
			{
				ID:        "end",
				Action:    "log",
				DependsOn: []string{"loop"},
				Config:    map[string]any{"message": "end"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["end"].Status)

	// Count step.goto events
	gotoCount := 0
	s.Eventually(func() bool {
		for {
			select {
			case e := <-ch:
				if e.Type == event.StepGoto {
					gotoCount++
				}
			default:
				return gotoCount == 2
			}
		}
	}, 2*time.Second, 10*time.Millisecond)
}

func (s *GotoTestSuite) TestGotoConditionFalse() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	ch := bus.Subscribe(100)

	// Goto condition is false from the start -- should never loop
	wf := &parser.Workflow{
		Version: "2.0",
		Name:    "goto-false",
		Steps: []parser.Step{
			{
				ID:     "a",
				Action: "log",
				Config: map[string]any{"message": "a"},
			},
			{
				ID:        "b",
				Action:    "log",
				DependsOn: []string{"a"},
				Config:    map[string]any{"message": "b"},
				Goto: &parser.GotoConfig{
					Target:        "a",
					When:          "false",
					MaxIterations: 10,
				},
			},
			{
				ID:        "c",
				Action:    "log",
				DependsOn: []string{"b"},
				Config:    map[string]any{"message": "c"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)
	s.Equal("success", result.Steps["c"].Status)

	// Verify no step.goto events — wait until WorkflowCompleted is received, then check
	var collected []event.Event
	s.Eventually(func() bool {
		for {
			select {
			case e := <-ch:
				collected = append(collected, e)
			default:
				for _, e := range collected {
					if e.Type == event.WorkflowCompleted {
						return true
					}
				}
				return false
			}
		}
	}, 2*time.Second, 10*time.Millisecond)

	for _, e := range collected {
		s.NotEqual(event.StepGoto, e.Type, "should not have any step.goto events")
	}
}

func (s *GotoTestSuite) gotoLoopWorkflow() *parser.Workflow {
	return &parser.Workflow{
		Version: "2.0",
		Name:    "goto-body",
		Steps: []parser.Step{
			{
				ID:     "init",
				Action: "set",
				Config: map[string]any{"x": 0},
			},
			{
				ID:        "work",
				Action:    "js",
				DependsOn: []string{"init"},
				Config:    map[string]any{"script": `return { x: vars.x + 1 };`},
			},
			{
				ID:        "save",
				Action:    "set",
				DependsOn: []string{"work"},
				Config:    map[string]any{"x": "{{ steps.work.output.x }}"},
			},
			{
				ID:        "loop",
				Action:    "log",
				DependsOn: []string{"save"},
				Config:    map[string]any{"message": "looping"},
				Goto: &parser.GotoConfig{
					Target:        "work",
					When:          "vars.x < 2",
					MaxIterations: 10,
				},
			},
			{
				ID:        "end",
				Action:    "log",
				DependsOn: []string{"loop"},
				Config:    map[string]any{"message": "end"},
			},
		},
	}
}

func (s *GotoTestSuite) TestGotoEvent_BodyIsResetList() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	ch := bus.Subscribe(200)

	result, err := exec.Execute(context.Background(), s.gotoLoopWorkflow(), nil)
	s.Require().NoError(err)
	s.Equal("success", result.Status)

	var collected []event.Event

	s.Eventually(func() bool {
		for {
			select {
			case e := <-ch:
				collected = append(collected, e)
			default:
				for _, e := range collected {
					if e.Type == event.WorkflowCompleted {
						return true
					}
				}

				return false
			}
		}
	}, 2*time.Second, 10*time.Millisecond)

	var firstGoto *event.Event

	workStartedCount := 0
	gotoBeforeSecondWorkStart := false

	for i := range collected {
		e := collected[i]

		if e.Type == event.StepStarted && e.StepID == "work" {
			workStartedCount++
			if workStartedCount == 2 && firstGoto != nil {
				gotoBeforeSecondWorkStart = true
			}
		}

		if e.Type == event.StepGoto && firstGoto == nil {
			firstGoto = &collected[i]
		}
	}

	s.Require().NotNil(firstGoto, "expected at least one step.goto event")

	body, ok := firstGoto.Data["body"].([]any)
	s.Require().True(ok, "step.goto Data[body] must be a list")

	sorted := make([]string, 0, len(body))

	for _, bid := range body {
		id, isString := bid.(string)
		s.Require().True(isString, "each body entry must be a step id string")

		sorted = append(sorted, id)
	}

	sort.Strings(sorted)
	s.Equal([]string{"loop", "save", "work"}, sorted)
	s.True(gotoBeforeSecondWorkStart, "step.goto must precede the 2nd iteration step.started of work")
}

func (s *GotoTestSuite) TestGotoEvent_BodyNeverNil() {
	exec, bus := newTestExecutor()
	defer bus.Close()

	ch := bus.Subscribe(10)

	node := &DAGNode{Step: parser.Step{ID: "loop", Goto: &parser.GotoConfig{Target: "work", MaxIterations: 1}}}
	execCtx := runtime.NewExecutionContext("exec-1", "wf", nil, nil)

	exec.publishGotoEvent(execCtx, node, 0, 1, nil)
	exec.publishGotoEvent(execCtx, node, 0, 1, []string{})

	for i := 0; i < 2; i++ {
		e := <-ch
		body, ok := e.Data["body"].([]any)
		s.Require().True(ok, "step.goto Data[body] must always be a list")
		s.NotNil(body, "step.goto Data[body] must never be nil")
	}
}

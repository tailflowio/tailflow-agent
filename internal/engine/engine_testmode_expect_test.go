package engine

import (
	"context"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

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

package engine

import (
	"context"

	"github.com/tailflow/tailflow/internal/parser"
)

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

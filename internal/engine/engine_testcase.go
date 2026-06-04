package engine

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

func findTestCase(step parser.Step, caseName string) *parser.TestCase {
	for i := range step.Testing {
		if step.Testing[i].Name == caseName {
			return &step.Testing[i]
		}
	}

	return nil
}

func (e *Executor) applyTestMock(step parser.Step, tc *parser.TestCase, execCtx *runtime.ExecutionContext, logger *slog.Logger) error {
	now := time.Now()

	if tc.Error != nil {
		code := tc.Error.Code
		if code == "" {
			code = "action_failed"
		}

		execCtx.SetStepResult(step.ID, &runtime.StepResult{
			Status:     runtime.StatusFailed,
			Error:      &runtime.StepError{Message: tc.Error.Message, Code: code, StepID: step.ID},
			StartedAt:  &now,
			FinishedAt: &now,
		})

		e.bus.Publish(event.Event{
			Type: event.StepFailed, ExecutionID: execCtx.ExecutionID,
			StepID: step.ID, Message: tc.Error.Message, Timestamp: now,
		})

		if step.ErrorPolicy == "continue" || step.ErrorPolicy == "ignore" {
			return nil
		}

		return errors.New(tc.Error.Message)
	}

	e.recordStepSuccess(step, tc.Output, execCtx, &now, logger)

	return nil
}

func (e *Executor) CheckTestExpectExposed(step parser.Step, expect *parser.TestCaseExpect, execCtx *runtime.ExecutionContext) error {
	return e.checkTestExpect(step, expect, execCtx)
}

func (e *Executor) checkTestExpect(step parser.Step, expect *parser.TestCaseExpect, execCtx *runtime.ExecutionContext) error {
	result, ok := execCtx.GetStepResult(step.ID)
	if !ok {
		return fmt.Errorf("test expect: step %q has no result", step.ID)
	}

	if expect.Status != "" && result.Status != expect.Status {
		return fmt.Errorf("test expect: step %q status = %q, want %q", step.ID, result.Status, expect.Status)
	}

	if expect.Output != nil {
		err := partialMatch(step.ID, "output", expect.Output, result.Output)
		if err != nil {
			return err
		}
	}

	if expect.Error != nil {
		if result.Error == nil {
			return fmt.Errorf("test expect: step %q expected error but got none", step.ID)
		}

		if expect.Error.Message != "" && result.Error.Message != expect.Error.Message {
			return fmt.Errorf("test expect: step %q error message = %q, want %q", step.ID, result.Error.Message, expect.Error.Message)
		}

		if expect.Error.Code != "" && result.Error.Code != expect.Error.Code {
			return fmt.Errorf("test expect: step %q error code = %q, want %q", step.ID, result.Error.Code, expect.Error.Code)
		}
	}

	return nil
}

func partialMatch(stepID, path string, expected, actual any) error {
	switch exp := expected.(type) {
	case map[string]any:
		actMap, ok := actual.(map[string]any)
		if !ok {
			return fmt.Errorf("test expect: step %q %s expected map, got %T", stepID, path, actual)
		}

		for k, v := range exp {
			av, ok := actMap[k]
			if !ok {
				return fmt.Errorf("test expect: step %q %s.%s missing", stepID, path, k)
			}

			err := partialMatch(stepID, path+"."+k, v, av)
			if err != nil {
				return err
			}
		}

	case []any:
		actSlice, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("test expect: step %q %s expected slice, got %T", stepID, path, actual)
		}

		if len(exp) != len(actSlice) {
			return fmt.Errorf("test expect: step %q %s length = %d, want %d", stepID, path, len(actSlice), len(exp))
		}

		for i := range exp {
			err := partialMatch(stepID, fmt.Sprintf("%s[%d]", path, i), exp[i], actSlice[i])
			if err != nil {
				return err
			}
		}

	default:
		if !scalarEqual(expected, actual) {
			return fmt.Errorf("test expect: step %q %s = %v, want %v", stepID, path, actual, expected)
		}
	}

	return nil
}

func scalarEqual(a, b any) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

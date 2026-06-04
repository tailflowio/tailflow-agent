package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"time"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

func (e *Executor) handleStepError(
	ctx context.Context, step parser.Step, output any, execErr error,
	execCtx *runtime.ExecutionContext, stepStartedAt *time.Time, logger *slog.Logger,
) error {
	if step.ErrorPolicy == "ignore" {
		return e.handleIgnoredError(step, output, execErr, execCtx, stepStartedAt, logger)
	}

	failedAt := time.Now()
	execCtx.SetStepResult(step.ID, &runtime.StepResult{
		Status:     runtime.StatusFailed,
		Output:     output,
		Error:      stepError(step.ID, execErr),
		StartedAt:  stepStartedAt,
		FinishedAt: &failedAt,
	})

	e.bus.Publish(event.Event{
		Type:        event.StepFailed,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     execErr.Error(),
	})

	if len(step.OnError) > 0 {
		e.executeOnError(ctx, step, execCtx)
	}

	if step.ErrorPolicy == "continue" {
		logger.Warn("continuing despite error (error_policy=continue)")

		return nil
	}

	return execErr
}

func (e *Executor) handleIgnoredError(
	step parser.Step, output any, execErr error,
	execCtx *runtime.ExecutionContext, stepStartedAt *time.Time, logger *slog.Logger,
) error {
	suppressedOutput := map[string]any{"_suppressed_error": execErr.Error()}

	outMap, ok := output.(map[string]any)
	if ok {
		maps.Copy(suppressedOutput, outMap)
	}

	now := time.Now()
	execCtx.SetStepResult(step.ID, &runtime.StepResult{
		Status:     runtime.StatusSuccess,
		Output:     suppressedOutput,
		StartedAt:  stepStartedAt,
		FinishedAt: &now,
	})
	logger.Warn("error ignored (error_policy=ignore)", "error", execErr)

	e.bus.Publish(e.sensitive.MaskEvent(event.Event{
		Type:        event.StepCompleted,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     fmt.Sprintf("step %q completed (error ignored)", step.ID),
		Data:        map[string]any{"output": suppressedOutput},
	}))

	return nil
}

func (e *Executor) recordStepSuccess(
	step parser.Step, output any, execCtx *runtime.ExecutionContext,
	stepStartedAt *time.Time, logger *slog.Logger,
) {
	completedAt := time.Now()

	execCtx.SetStepResult(step.ID, &runtime.StepResult{
		Status:     runtime.StatusSuccess,
		Output:     output,
		StartedAt:  stepStartedAt,
		FinishedAt: &completedAt,
	})

	e.emitLogActionOutput(step, output, execCtx)

	e.bus.Publish(e.sensitive.MaskEvent(event.Event{
		Type:        event.StepOutput,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     fmt.Sprintf("step %q output", step.ID),
		Data:        map[string]any{"output": output},
	}))

	e.bus.Publish(e.sensitive.MaskEvent(event.Event{
		Type:        event.StepCompleted,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     fmt.Sprintf("step %q completed", step.ID),
		Data:        map[string]any{"output": output},
	}))
	logger.Info("completed")
}

func (e *Executor) emitLogActionOutput(step parser.Step, output any, execCtx *runtime.ExecutionContext) {
	outMap, ok := output.(map[string]any)
	if !ok {
		return
	}

	var logMsg, logLevel string

	switch step.Action {
	case "log":
		logMsg, _ = outMap["message"].(string)
		logLevel, _ = outMap["level"].(string)
	default:
		return
	}

	stream, _ := step.Config["stream"].(bool)

	for _, line := range strings.Split(strings.TrimRight(logMsg, "\n"), "\n") {
		data := map[string]any{"level": logLevel}
		if stream {
			data["stream"] = true
		}

		e.bus.Publish(event.Event{
			Type:        event.StepLog,
			Timestamp:   time.Now(),
			ExecutionID: execCtx.ExecutionID,
			StepID:      step.ID,
			Message:     line,
			Data:        data,
		})
	}
}

func (e *Executor) executeOnError(ctx context.Context, step parser.Step, execCtx *runtime.ExecutionContext) {
	for _, errStep := range step.OnError {
		logger := e.logger.With("step", errStep.ID, "on_error_of", step.ID)
		logger.Info("executing on_error step")

		resolvedConfig, err := e.eval.ResolveConfig(errStep.Config, execCtx.ToMap())
		if err != nil {
			logger.Error("failed to resolve on_error config", "error", err)
			continue
		}

		act, err := e.registry.Create(errStep.Action)
		if err != nil {
			logger.Error("failed to create on_error action", "error", err)
			continue
		}

		actCtx := &action.ActionContext{
			Context:  ctx,
			Config:   resolvedConfig,
			ExecCtx:  execCtx,
			StepID:   errStep.ID,
			Logger:   logger,
			Services: execCtx.Services,
		}

		output, err := act.Execute(actCtx)
		if err != nil {
			logger.Error("on_error step failed", "error", err)
			execCtx.SetStepResult(errStep.ID, &runtime.StepResult{Status: runtime.StatusFailed, Error: stepError(errStep.ID, err)})

			continue
		}

		execCtx.SetStepResult(errStep.ID, &runtime.StepResult{Status: runtime.StatusSuccess, Output: output})
	}
}

func stepErrorCode(err error) string {
	if err == nil {
		return ""
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}

	if errors.Is(err, context.Canceled) {
		return "canceled"
	}

	return "action_failed"
}

func stepError(stepID string, err error) *runtime.StepError {
	code := "action_failed"
	if errors.Is(err, context.DeadlineExceeded) {
		code = "timeout"
	} else if errors.Is(err, context.Canceled) {
		code = "cancelled"
	}

	return &runtime.StepError{Message: err.Error(), Code: code, StepID: stepID}
}

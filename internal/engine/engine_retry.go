package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

func (e *Executor) executeWithRetry(
	ctx context.Context, step parser.Step, act action.Action,
	actCtx *action.ActionContext, execCtx *runtime.ExecutionContext, logger *slog.Logger,
) (any, error) {
	maxAttempts := 1

	var retryDelay time.Duration

	if step.Retry != nil {
		maxAttempts = step.Retry.MaxAttempts

		var delayErr error

		retryDelay, delayErr = step.Retry.ParsedDelay()
		if delayErr != nil {
			logger.WarnContext(ctx, "invalid retry delay, using default",
				"delay", step.Retry.Delay,
				"error", delayErr,
			)

			retryDelay = time.Second
		}
	}

	var output any
	var execErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			err := e.waitForRetry(ctx, step, attempt, maxAttempts, retryDelay, execCtx, logger)
			if err != nil {
				return nil, err
			}
		}

		retryCtx := ctx
		var finishRetry func(error)

		if maxAttempts > 1 {
			retryCtx, finishRetry = e.tracer.StartRetry(ctx, attempt, maxAttempts)
			e.businessMetrics.RecordStepRetry(ctx, step.ID, step.Action)
		}

		timeoutCtx, timeoutCancel := e.applyTimeout(retryCtx, step)
		actCtx.Context = timeoutCtx

		output, execErr = act.Execute(actCtx)

		timeoutCancel()

		if finishRetry != nil {
			finishRetry(execErr)
		}

		if execErr == nil {
			break
		}
	}

	return output, execErr
}

func (e *Executor) waitForRetry(
	ctx context.Context, step parser.Step, attempt, maxAttempts int,
	retryDelay time.Duration, execCtx *runtime.ExecutionContext, logger *slog.Logger,
) error {
	logger.Info("retrying", "attempt", attempt, "max", maxAttempts)
	e.bus.Publish(event.Event{
		Type:        event.StepStarted,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     fmt.Sprintf("retry attempt %d/%d", attempt, maxAttempts),
		Data:        map[string]any{"attempt": attempt},
	})

	select {
	case <-time.After(retryDelay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Executor) applyTimeout(ctx context.Context, step parser.Step) (context.Context, context.CancelFunc) {
	if step.Timeout == "" {
		return ctx, func() {}
	}

	dur, err := time.ParseDuration(step.Timeout)
	if err == nil {
		return context.WithTimeout(ctx, dur)
	}

	return ctx, func() {}
}

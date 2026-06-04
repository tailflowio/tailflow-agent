package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/event"
	tfotel "github.com/tailflow/tailflow/internal/otel"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

func (e *Executor) handleRecovery(execCtx *runtime.ExecutionContext, step *parser.Step) (skip bool, err error) {
	if !execCtx.Resumed {
		return false, nil
	}

	sr, ok := execCtx.GetStepResult(step.ID)
	if !ok {
		return false, nil
	}

	if sr.Status == runtime.StatusSuccess || sr.Status == runtime.StatusSkipped {
		return true, nil
	}

	if sr.Status != runtime.StatusRunning {
		return false, nil
	}

	strategy := step.OnRecovery
	if strategy == "" {
		strategy = "retry"
	}

	now := time.Now()

	switch strategy {
	case "skip":
		execCtx.SetStepResult(step.ID, &runtime.StepResult{
			Status:     runtime.StatusSuccess,
			StartedAt:  sr.StartedAt,
			FinishedAt: &now,
		})

		return true, nil
	case "fail":
		execCtx.SetStepResult(step.ID, &runtime.StepResult{
			Status:     runtime.StatusFailed,
			StartedAt:  sr.StartedAt,
			FinishedAt: &now,
			Error: &runtime.StepError{
				Message: "step was running at crash time and on_recovery=fail",
				Code:    "recovery_failed",
				StepID:  step.ID,
			},
		})

		return true, fmt.Errorf("step %q: recovery failed, manual intervention required", step.ID)
	default:
		execCtx.ClearStepResult(step.ID)

		return false, nil
	}
}

func (e *Executor) executeNode(ctx context.Context, node *DAGNode, execCtx *runtime.ExecutionContext) error {
	step := node.Step
	logger := e.logger.With("step", step.ID)

	skip, recoveryErr := e.handleRecovery(execCtx, &node.Step)
	if skip {
		return recoveryErr
	}

	skipped, err := e.evaluateWhenCondition(step, execCtx, logger)
	if skipped || err != nil {
		return err
	}

	if execCtx.TestCaseName != "" {
		tc := findTestCase(step, execCtx.TestCaseName)
		if tc != nil && (tc.Output != nil || tc.Error != nil) {
			mockErr := e.applyTestMock(step, tc, execCtx, logger)
			if mockErr == nil && tc.Expect != nil {
				expectErr := e.checkTestExpect(step, tc.Expect, execCtx)
				if expectErr != nil {
					return expectErr
				}
			}

			return mockErr
		}
	}

	e.bus.Publish(event.NewEvent(event.StepStarted, execCtx.ExecutionID, step.ID, fmt.Sprintf("step %q started", step.ID)))
	logger.Info("started", "action", step.Action)

	stepStartedAt := time.Now()

	resolvedConfig, err := e.resolveStepConfig(step, execCtx)
	if err != nil {
		_, finishStep := e.tracer.StartStep(ctx, step.ID, step.Action, nil)
		finishStep("failed", err, nil, nil)

		return err
	}

	ctx, finishStep := e.tracer.StartStep(ctx, step.ID, step.Action, resolvedConfig)

	act, err := e.registry.Create(step.Action)
	if err != nil {
		finishStep("failed", err, nil, nil)

		return err
	}

	e.emitStepInput(step, resolvedConfig, execCtx)

	emitLog := e.newLogEmitter(step.ID, execCtx)
	actCtx := e.createActionContext(ctx, step, resolvedConfig, execCtx, logger, emitLog)

	err = act.Validate(actCtx)
	if err != nil {
		finishStep("failed", fmt.Errorf("validate: %w", err), nil, nil)

		return fmt.Errorf("validate: %w", err)
	}

	output, execErr := e.executeWithRetry(ctx, step, act, actCtx, execCtx, logger)
	if execErr != nil {
		maskedInput := e.sensitive.MaskMap(e.prepareStepInput(step, resolvedConfig, execCtx))
		finishStep("failed", execErr, maskedInput, nil)
		e.businessMetrics.RecordStepCompleted(
			ctx, step.ID, step.Action, "failed", tfotel.DurationMs(stepStartedAt),
		)
		e.businessMetrics.RecordStepError(ctx, step.ID, step.Action, stepErrorCode(execErr))

		return e.handleStepError(ctx, step, output, execErr, execCtx, &stepStartedAt, logger)
	}

	maskedInput := e.sensitive.MaskMap(e.prepareStepInput(step, resolvedConfig, execCtx))
	maskedOutput := e.sensitive.MaskAny(output)
	finishStep("success", nil, maskedInput, maskedOutput)
	e.businessMetrics.RecordStepCompleted(
		ctx, step.ID, step.Action, "success", tfotel.DurationMs(stepStartedAt),
	)

	e.recordStepSuccess(step, output, execCtx, &stepStartedAt, logger)

	if execCtx.TestCaseName != "" {
		tc := findTestCase(step, execCtx.TestCaseName)
		if tc != nil && tc.Expect != nil {
			expectErr := e.checkTestExpect(step, tc.Expect, execCtx)
			if expectErr != nil {
				return expectErr
			}
		}
	}

	return nil
}

func (e *Executor) evaluateWhenCondition(
	step parser.Step, execCtx *runtime.ExecutionContext, logger *slog.Logger,
) (skipped bool, err error) {
	if step.When == "" {
		return false, nil
	}

	result, err := e.eval.EvalBool(step.When, execCtx.ToMap())
	if err != nil {
		return false, fmt.Errorf("eval when condition: %w", err)
	}

	if !result {
		execCtx.SetStepResult(step.ID, &runtime.StepResult{Status: runtime.StatusSkipped})
		e.bus.Publish(event.NewEvent(event.StepSkipped, execCtx.ExecutionID, step.ID, "condition not met"))
		logger.Info("skipped (when condition false)")

		return true, nil
	}

	return false, nil
}

func (e *Executor) resolveStepConfig(step parser.Step, execCtx *runtime.ExecutionContext) (map[string]any, error) {
	configToResolve, rawActionConfig, rawActionsArray := e.extractLoopConfig(step)

	resolvedConfig, err := e.eval.ResolveConfig(configToResolve, execCtx.ToMap())
	if err != nil {
		return nil, fmt.Errorf("resolve config: %w", err)
	}

	if rawActionConfig != nil {
		resolvedConfig["action_config"] = rawActionConfig
	}

	if rawActionsArray != nil {
		resolvedConfig["actions"] = rawActionsArray
	}

	return resolvedConfig, nil
}

func (e *Executor) extractLoopConfig(
	step parser.Step,
) (configToResolve map[string]any, rawActionConfig map[string]any, rawActionsArray []any) {
	configToResolve = step.Config

	if step.Action != "loop" {
		return configToResolve, nil, nil
	}

	needsCopy := false

	ac, ok := step.Config["action_config"]
	if ok {
		rawActionConfig, _ = ac.(map[string]any)
		needsCopy = true
	}

	aa, ok := step.Config["actions"]
	if ok {
		rawActionsArray, _ = aa.([]any)
		needsCopy = true
	}

	if needsCopy {
		configToResolve = make(map[string]any, len(step.Config))

		for k, v := range step.Config {
			if k != "action_config" && k != "actions" {
				configToResolve[k] = v
			}
		}
	}

	return configToResolve, rawActionConfig, rawActionsArray
}

func (e *Executor) emitStepInput(step parser.Step, resolvedConfig map[string]any, execCtx *runtime.ExecutionContext) {
	inputData := e.prepareStepInput(step, resolvedConfig, execCtx)

	e.bus.Publish(e.sensitive.MaskEvent(event.Event{
		Type:        event.StepInput,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      step.ID,
		Message:     fmt.Sprintf("step %q input", step.ID),
		Data:        inputData,
	}))
}

func (e *Executor) prepareStepInput(step parser.Step, resolvedConfig map[string]any, execCtx *runtime.ExecutionContext) map[string]any {
	inputData := map[string]any{}

	filteredConfig := make(map[string]any, len(resolvedConfig))

	for k, v := range resolvedConfig {
		if k != "script" {
			filteredConfig[k] = v
		}
	}

	if len(filteredConfig) > 0 {
		inputData["config"] = filteredConfig
	}

	if len(execCtx.TriggerData) > 0 {
		inputData["trigger"] = execCtx.TriggerData
	}

	deps := make(map[string]any)

	for _, dep := range step.DependsOn {
		r, ok := execCtx.GetStepResult(dep)
		if ok && r.Output != nil {
			deps[dep] = r.Output
		}
	}

	if len(deps) > 0 {
		inputData["deps"] = deps
	}

	return inputData
}

func (e *Executor) newLogEmitter(stepID string, execCtx *runtime.ExecutionContext) func(string) {
	return func(msg string) {
		e.bus.Publish(event.Event{
			Type:        event.StepLog,
			Timestamp:   time.Now(),
			ExecutionID: execCtx.ExecutionID,
			StepID:      stepID,
			Message:     msg,
			Data:        map[string]any{"stream": true},
		})
	}
}

func (e *Executor) newPrintEmitter(stepID string, execCtx *runtime.ExecutionContext) func(string) {
	return func(msg string) {
		e.bus.Publish(event.Event{
			Type:        event.StepLog,
			Timestamp:   time.Now(),
			ExecutionID: execCtx.ExecutionID,
			StepID:      stepID,
			Message:     msg,
			Data:        map[string]any{"print": true},
		})
	}
}

func (e *Executor) newGroupEmitter(stepID string, execCtx *runtime.ExecutionContext) func(string) {
	return func(key string) {
		e.bus.Publish(event.Event{
			Type:        event.ExecutionGroup,
			Timestamp:   time.Now(),
			ExecutionID: execCtx.ExecutionID,
			StepID:      stepID,
			Data:        map[string]any{"group_key": key},
		})
	}
}

func (e *Executor) createActionContext(
	ctx context.Context, step parser.Step, resolvedConfig map[string]any,
	execCtx *runtime.ExecutionContext, logger *slog.Logger, emitLog func(string),
) *action.ActionContext {
	return &action.ActionContext{
		Context:   ctx,
		Config:    resolvedConfig,
		ExecCtx:   execCtx,
		StepID:    step.ID,
		Logger:    logger,
		Services:  execCtx.Services,
		EmitLog:   emitLog,
		EmitPrint: e.newPrintEmitter(step.ID, execCtx),
		EmitGroup: e.newGroupEmitter(step.ID, execCtx),
		RunAction: e.newRunAction(ctx, step, execCtx, logger, emitLog),
	}
}

func (e *Executor) newRunAction(
	ctx context.Context, step parser.Step,
	execCtx *runtime.ExecutionContext, logger *slog.Logger, emitLog func(string),
) func(string, map[string]any, map[string]any) (any, error) {
	return func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		ctxMap := execCtx.ToMap()
		ctxMap["loop"] = loopVars

		resolved, resolveErr := e.eval.ResolveConfig(rawConfig, ctxMap)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve loop action config: %w", resolveErr)
		}

		subAct, createErr := e.registry.Create(actionName)
		if createErr != nil {
			return nil, fmt.Errorf("create loop action %q: %w", actionName, createErr)
		}

		subCtx := &action.ActionContext{
			Context:   ctx,
			Config:    resolved,
			ExecCtx:   execCtx,
			StepID:    step.ID,
			Logger:    logger,
			Services:  execCtx.Services,
			EmitLog:   emitLog,
			EmitPrint: e.newPrintEmitter(step.ID, execCtx),
			EmitGroup: e.newGroupEmitter(step.ID, execCtx),
		}

		validateErr := subAct.Validate(subCtx)
		if validateErr != nil {
			return nil, fmt.Errorf("validate loop action %q: %w", actionName, validateErr)
		}

		return subAct.Execute(subCtx)
	}
}

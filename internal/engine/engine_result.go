package engine

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

type ExecuteResult struct {
	ExecutionID string
	Status      string // see runtime.Status* constants
	HasErrors   bool   // true when some steps failed but error_policy allowed continuation
	Steps       map[string]*runtime.StepResult
	Error       error
	StartedAt   time.Time
	FinishedAt  time.Time
}

type ExecuteOptions struct {
	ExecutionID    string
	TriggerData    map[string]any
	Services       *runtime.ActionServices
	TestCaseName   string
	Resumed        bool
	RecoveredSteps map[string]*runtime.StepResult
}

func (e *Executor) emitExecutionState(wf *parser.Workflow, execCtx *runtime.ExecutionContext, result *ExecuteResult) {
	var idempotencyKey string

	if wf.Trigger != nil && wf.Trigger.HTTP != nil && wf.Trigger.HTTP.IdempotencyKey != "" {
		resolved, evalErr := e.eval.ResolveTemplate(wf.Trigger.HTTP.IdempotencyKey, execCtx.ToMap())
		if evalErr == nil {
			idempotencyKey = resolved
			execCtx.IdempotencyKey = resolved
		}
	}

	stepsSnapshot := make(map[string]any, len(execCtx.Steps))

	for stepID, sr := range execCtx.StepsCopy() {
		stepsSnapshot[stepID] = map[string]any{
			"status":      sr.Status,
			"started_at":  sr.StartedAt,
			"finished_at": sr.FinishedAt,
		}
	}

	e.bus.Publish(event.Event{
		Type:        event.ExecutionState,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		Data: map[string]any{
			"workflow_name":   wf.Name,
			"status":          result.Status,
			"steps":           stepsSnapshot,
			"idempotency_key": idempotencyKey,
			"params":          execCtx.Params,
		},
	})
}

func (e *Executor) emitInitialExecutionState(wf *parser.Workflow, execCtx *runtime.ExecutionContext) {
	var idempotencyKey string

	if wf.Trigger != nil && wf.Trigger.HTTP != nil && wf.Trigger.HTTP.IdempotencyKey != "" {
		resolved, evalErr := e.eval.ResolveTemplate(wf.Trigger.HTTP.IdempotencyKey, execCtx.ToMap())
		if evalErr == nil {
			idempotencyKey = resolved
			execCtx.IdempotencyKey = resolved
		}
	}

	data := map[string]any{
		"workflow_name":   wf.Name,
		"status":          "running",
		"idempotency_key": idempotencyKey,
		"params":          execCtx.Params,
	}
	if wf.Recovery {
		data["recovery"] = true
	}

	e.bus.Publish(event.Event{
		Type:        event.ExecutionState,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		Data:        data,
	})
}

func resolveExecutionID(opts []ExecuteOptions) string {
	if len(opts) > 0 && opts[0].ExecutionID != "" {
		return opts[0].ExecutionID
	}

	return uuid.New().String()
}

func (e *Executor) buildExecutionContext(
	executionID, wfName string, resolvedParams map[string]any,
	resolvedEnv map[string]string, opts []ExecuteOptions,
) *runtime.ExecutionContext {
	execCtx := runtime.NewExecutionContext(executionID, wfName, resolvedParams, resolvedEnv)
	if len(opts) > 0 && opts[0].TriggerData != nil {
		execCtx.TriggerData = opts[0].TriggerData
	}

	if len(opts) > 0 && opts[0].Services != nil {
		execCtx.Services = opts[0].Services
	}

	return execCtx
}

func (e *Executor) buildExecuteResult(
	ctx context.Context, wfName string, execCtx *runtime.ExecutionContext,
	execErr error, executionID string, startedAt time.Time,
) *ExecuteResult {
	status, hasErrors := e.determineStatus(ctx, execErr, execCtx)
	finishedAt := time.Now()
	e.publishWorkflowCompleted(executionID, wfName, status, finishedAt)

	return &ExecuteResult{
		ExecutionID: executionID,
		Status:      status,
		HasErrors:   hasErrors,
		Steps:       copyStepResults(execCtx.Steps),
		Error:       execErr,
		StartedAt:   startedAt,
		FinishedAt:  finishedAt,
	}
}

func (e *Executor) publishWorkflowCompleted(executionID, wfName, status string, ts time.Time) {
	e.bus.Publish(event.Event{
		Type:        event.WorkflowCompleted,
		Timestamp:   ts,
		ExecutionID: executionID,
		Message:     fmt.Sprintf("workflow %q %s", wfName, status),
		Data:        map[string]any{"status": status},
	})
}

func copyStepResults(steps map[string]*runtime.StepResult) map[string]*runtime.StepResult {
	stepsCopy := make(map[string]*runtime.StepResult, len(steps))
	for k, v := range steps {
		cp := *v
		stepsCopy[k] = &cp
	}

	return stepsCopy
}

func (e *Executor) resolveParams(wf *parser.Workflow, params map[string]any) (map[string]any, error) {
	resolved := make(map[string]any)

	for _, p := range wf.Params {
		v, ok := params[p.Name]

		switch {
		case ok:
			resolved[p.Name] = v
		case p.Default != nil:
			resolved[p.Name] = p.Default
		case p.Required:
			return nil, fmt.Errorf("required parameter %q not provided", p.Name)
		}
	}

	ctx := map[string]any{"params": resolved}

	for _, p := range wf.Params {
		if p.Default == nil {
			continue
		}

		if _, ok := params[p.Name]; ok {
			continue
		}

		s, ok := resolved[p.Name].(string)
		if !ok || !strings.Contains(s, "{{") {
			continue
		}

		val, err := e.eval.Eval(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "{{"), "}}")), ctx)
		if err == nil {
			resolved[p.Name] = val
			ctx["params"] = resolved
		}
	}

	for _, p := range wf.Params {
		if p.Pattern == "" {
			continue
		}

		v, ok := resolved[p.Name]
		if !ok {
			continue
		}

		s, ok := v.(string)
		if !ok {
			continue
		}

		re, err := regexp.Compile("^(?:" + p.Pattern + ")$")
		if err != nil {
			return nil, fmt.Errorf("invalid pattern for parameter %q: %w", p.Name, err)
		}

		if !re.MatchString(s) {
			return nil, fmt.Errorf("parameter %q value %q does not match pattern %q", p.Name, s, p.Pattern)
		}
	}

	for k, v := range params {
		_, exists := resolved[k]
		if !exists {
			resolved[k] = v
		}
	}

	return resolved, nil
}

func (e *Executor) resolveEnv(wf *parser.Workflow, resolvedParams map[string]any) (map[string]string, error) {
	resolved := make(map[string]string, len(wf.Env))

	for k, v := range wf.Env {
		expanded := os.ExpandEnv(v)
		ctxMap := map[string]any{"params": resolvedParams}

		val, err := e.eval.ResolveTemplate(expanded, ctxMap)
		if err != nil {
			return nil, fmt.Errorf("resolve env %q: %w", k, err)
		}

		resolved[k] = val
	}

	return resolved, nil
}

func (e *Executor) determineStatus(ctx context.Context, execErr error, execCtx *runtime.ExecutionContext) (string, bool) {
	if execErr != nil && ctx.Err() != nil {
		return runtime.StatusCancelled, false
	}

	if execErr != nil {
		return runtime.StatusFailed, false
	}

	if execCtx.HasFailedSteps() {
		return runtime.StatusCompletedWithErrors, true
	}

	return runtime.StatusSuccess, false
}

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

type Executor struct {
	registry        *action.Registry
	bus             *event.Bus
	eval            *runtime.ExprEvaluator
	logger          *slog.Logger
	sensitive       *SensitiveRegistry
	tracer          *tfotel.Tracer
	businessMetrics *tfotel.BusinessMetrics
}

func NewExecutor(
	registry *action.Registry, bus *event.Bus, logger *slog.Logger,
	sensitiveKeys []string, tracer *tfotel.Tracer, businessMetrics *tfotel.BusinessMetrics,
) *Executor {
	if tracer == nil {
		tracer = tfotel.NewTracer(nil)
	}

	return &Executor{
		registry:        registry,
		bus:             bus,
		eval:            runtime.NewExprEvaluator(),
		logger:          logger,
		sensitive:       NewSensitiveRegistry(sensitiveKeys),
		tracer:          tracer,
		businessMetrics: businessMetrics,
	}
}

func (e *Executor) Execute(
	ctx context.Context, wf *parser.Workflow, params map[string]any, opts ...ExecuteOptions,
) (*ExecuteResult, error) {
	executionID := resolveExecutionID(opts)
	startedAt := time.Now()

	resolvedParams, err := e.resolveParams(wf, params)
	if err != nil {
		return nil, err
	}

	resolvedEnv, err := e.resolveEnv(wf, resolvedParams)
	if err != nil {
		return nil, err
	}

	execCtx := e.buildExecutionContext(executionID, wf.Name, resolvedParams, resolvedEnv, opts)

	if len(opts) > 0 && opts[0].Resumed {
		execCtx.Resumed = true

		for stepID, sr := range opts[0].RecoveredSteps {
			execCtx.SetStepResult(stepID, sr)
		}
	}

	if len(opts) > 0 && opts[0].TestCaseName != "" {
		execCtx.TestCaseName = opts[0].TestCaseName
	}

	triggerType := ""
	if len(opts) > 0 && opts[0].TriggerData != nil {
		triggerType, _ = opts[0].TriggerData["method"].(string)
	}

	ctx, finishWorkflow := e.tracer.StartWorkflow(ctx, executionID, wf.Name, triggerType)
	e.businessMetrics.RecordWorkflowStarted(ctx, wf.Name)

	e.bus.Publish(event.NewEvent(event.WorkflowStarted, executionID, "", fmt.Sprintf("workflow %q started", wf.Name)))

	e.emitInitialExecutionState(wf, execCtx)

	dag, err := BuildDAG(wf.Steps)
	if err != nil {
		finishWorkflow("failed", err)

		return nil, fmt.Errorf("build DAG: %w", err)
	}

	execErr := e.executeDAG(ctx, dag, execCtx)

	if len(wf.OnError) > 0 && (execErr != nil || execCtx.HasFailedSteps()) {
		e.executeOnError(ctx, parser.Step{OnError: wf.OnError}, execCtx)
	}

	if ctx.Err() != nil && wf.Recovery {
		result := &ExecuteResult{
			ExecutionID: executionID,
			Status:      runtime.StatusCancelled,
			Steps:       copyStepResults(execCtx.Steps),
			Error:       execErr,
			StartedAt:   startedAt,
			FinishedAt:  time.Now(),
		}
		finishWorkflow(result.Status, result.Error)

		return result, nil
	}

	result := e.buildExecuteResult(ctx, wf.Name, execCtx, execErr, executionID, startedAt)
	finishWorkflow(result.Status, result.Error)
	e.businessMetrics.RecordWorkflowCompleted(ctx, wf.Name, result.Status, tfotel.DurationMs(startedAt))

	e.emitExecutionState(wf, execCtx, result)

	return result, nil
}

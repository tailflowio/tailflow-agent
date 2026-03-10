package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type BusinessMetrics struct {
	workflowExecutionsTotal metric.Int64Counter
	workflowDuration        metric.Float64Histogram
	activeExecutions        metric.Int64UpDownCounter
	stepDuration            metric.Float64Histogram
	stepErrorsTotal         metric.Int64Counter
	stepRetriesTotal        metric.Int64Counter
}

func NewBusinessMetrics(mp *sdkmetric.MeterProvider) (*BusinessMetrics, error) {
	if mp == nil {
		return nil, nil
	}

	meter := newMeter(mp)
	bm := &BusinessMetrics{}

	err := bm.registerWorkflowMetrics(meter)
	if err != nil {
		return nil, err
	}

	err = bm.registerStepMetrics(meter)
	if err != nil {
		return nil, err
	}

	return bm, nil
}

func (m *BusinessMetrics) registerWorkflowMetrics(meter metric.Meter) error {
	var err error

	m.workflowExecutionsTotal, err = meter.Int64Counter(
		"tailflow.workflow.executions.total",
		metric.WithDescription("Total workflow executions"),
	)
	if err != nil {
		return fmt.Errorf("failed to create workflow executions counter: %w", err)
	}

	m.workflowDuration, err = meter.Float64Histogram(
		"tailflow.workflow.execution.duration",
		metric.WithDescription("Workflow execution duration in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return fmt.Errorf("failed to create workflow duration histogram: %w", err)
	}

	m.activeExecutions, err = meter.Int64UpDownCounter(
		"tailflow.workflow.active_executions",
		metric.WithDescription("Currently running workflow executions"),
	)
	if err != nil {
		return fmt.Errorf("failed to create active executions counter: %w", err)
	}

	return nil
}

func (m *BusinessMetrics) registerStepMetrics(meter metric.Meter) error {
	var err error

	m.stepDuration, err = meter.Float64Histogram(
		"tailflow.step.execution.duration",
		metric.WithDescription("Step execution duration in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return fmt.Errorf("failed to create step duration histogram: %w", err)
	}

	m.stepErrorsTotal, err = meter.Int64Counter(
		"tailflow.step.errors.total",
		metric.WithDescription("Total step errors"),
	)
	if err != nil {
		return fmt.Errorf("failed to create step errors counter: %w", err)
	}

	m.stepRetriesTotal, err = meter.Int64Counter(
		"tailflow.step.retries.total",
		metric.WithDescription("Total step retry attempts"),
	)
	if err != nil {
		return fmt.Errorf("failed to create step retries counter: %w", err)
	}

	return nil
}

func (m *BusinessMetrics) RecordWorkflowStarted(ctx context.Context, workflowName string) {
	if m == nil {
		return
	}

	attrs := metric.WithAttributes(
		attribute.String("workflow.name", workflowName),
	)
	m.workflowExecutionsTotal.Add(ctx, 1, attrs)
	m.activeExecutions.Add(ctx, 1, attrs)
}

func (m *BusinessMetrics) RecordWorkflowCompleted(
	ctx context.Context,
	workflowName, status string,
	durationMs float64,
) {
	if m == nil {
		return
	}

	attrs := metric.WithAttributes(
		attribute.String("workflow.name", workflowName),
		attribute.String("workflow.status", status),
	)
	m.workflowDuration.Record(ctx, durationMs, attrs)
	m.activeExecutions.Add(ctx, -1,
		metric.WithAttributes(
			attribute.String("workflow.name", workflowName),
		),
	)
}

func (m *BusinessMetrics) RecordStepCompleted(
	ctx context.Context,
	stepID, actionName, status string,
	durationMs float64,
) {
	if m == nil {
		return
	}

	attrs := metric.WithAttributes(
		attribute.String("step.name", stepID),
		attribute.String("step.action", actionName),
		attribute.String("step.status", status),
	)
	m.stepDuration.Record(ctx, durationMs, attrs)
}

func (m *BusinessMetrics) RecordStepError(
	ctx context.Context,
	stepID, actionName, errorCode string,
) {
	if m == nil {
		return
	}

	attrs := metric.WithAttributes(
		attribute.String("step.name", stepID),
		attribute.String("step.action", actionName),
		attribute.String("error.code", errorCode),
	)
	m.stepErrorsTotal.Add(ctx, 1, attrs)
}

func (m *BusinessMetrics) RecordStepRetry(ctx context.Context, stepID, actionName string) {
	if m == nil {
		return
	}

	attrs := metric.WithAttributes(
		attribute.String("step.name", stepID),
		attribute.String("step.action", actionName),
	)
	m.stepRetriesTotal.Add(ctx, 1, attrs)
}

func DurationMs(start time.Time) float64 {
	return float64(time.Since(start).Milliseconds())
}

package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

type Tracer struct {
	tracer trace.Tracer
}

func NewTracer(tp trace.TracerProvider) *Tracer {
	if tp == nil {
		tp = nooptrace.NewTracerProvider()
	}

	return &Tracer{tracer: tp.Tracer("tailflow")}
}

func (t *Tracer) StartWorkflow(
	ctx context.Context,
	executionID, workflowName, triggerType string,
) (context.Context, func(status string, err error)) {
	ctx, span := t.tracer.Start(ctx, "workflow "+workflowName,
		trace.WithAttributes(
			attribute.String("workflow.name", workflowName),
			attribute.String("workflow.execution_id", executionID),
			attribute.String("workflow.trigger", triggerType),
		),
	)

	return ctx, func(status string, err error) {
		span.SetAttributes(attribute.String("workflow.status", status))
		finishSpan(span, err)
	}
}

func (t *Tracer) StartStep(
	ctx context.Context,
	stepID, actionName string, config map[string]any,
) (context.Context, func(status string, err error, input any, output any)) {
	spanKind := resolveSpanKind(actionName)

	ctx, span := t.tracer.Start(ctx, "step "+stepID,
		trace.WithSpanKind(spanKind),
		trace.WithAttributes(
			attribute.String("step.name", stepID),
			attribute.String("step.action", actionName),
		),
	)

	return ctx, func(status string, err error, input any, output any) {
		span.SetAttributes(attribute.String("step.status", status))

		if input != nil {
			span.SetAttributes(
				attribute.String("step.input", fmt.Sprintf("%v", input)),
			)
		}

		if output != nil {
			span.SetAttributes(
				attribute.String("step.output", fmt.Sprintf("%v", output)),
			)
		}

		EnrichStepSpan(span, actionName, config, output)
		finishSpan(span, err)
	}
}

func resolveSpanKind(actionName string) trace.SpanKind {
	switch actionName {
	case "http":
		return trace.SpanKindClient
	case "sql.query", "sql.exec", "sql.begin", "sql.commit", "sql.rollback":
		return trace.SpanKindClient
	case "kv.get", "kv.set", "kv.delete":
		return trace.SpanKindClient
	case "rabbitmq.shovel", "wait.rabbitmq":
		return trace.SpanKindClient
	default:
		return trace.SpanKindInternal
	}
}

func (t *Tracer) StartRetry(
	ctx context.Context,
	attempt, maxAttempts int,
) (context.Context, func(err error)) {
	ctx, span := t.tracer.Start(ctx,
		fmt.Sprintf("retry %d/%d", attempt, maxAttempts),
		trace.WithAttributes(
			attribute.Int("retry.attempt", attempt),
			attribute.Int("retry.max_attempts", maxAttempts),
		),
	)

	return ctx, func(err error) {
		finishSpan(span, err)
	}
}

func finishSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err, trace.WithStackTrace(true))
		span.SetStatus(codes.Error, err.Error())
		span.End()

		return
	}

	span.SetStatus(codes.Ok, "")
	span.End()
}

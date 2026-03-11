package otel

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type TracerTestSuite struct {
	suite.Suite
	ctx      context.Context
	exporter *tracetest.InMemoryExporter
	provider *sdktrace.TracerProvider
	tracer   *Tracer
}

func TestTracer(t *testing.T) {
	suite.Run(t, new(TracerTestSuite))
}

func (s *TracerTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.exporter = tracetest.NewInMemoryExporter()
	s.provider = sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(s.exporter),
	)
	s.tracer = NewTracer(s.provider)
}

func (s *TracerTestSuite) TestStartWorkflow_CreatesRootSpan() {
	ctx, finish := s.tracer.StartWorkflow(
		s.ctx, "exec-123", "my-workflow", "http",
	)
	s.NotNil(ctx)

	finish("success", nil)

	spans := s.exporter.GetSpans()
	s.Require().Len(spans, 1)
	s.Equal("workflow my-workflow", spans[0].Name)
	s.Equal(codes.Ok, spans[0].Status.Code)

	attrs := spanAttrMap(spans[0])
	s.Equal("my-workflow", attrs["workflow.name"])
	s.Equal("exec-123", attrs["workflow.execution_id"])
	s.Equal("http", attrs["workflow.trigger"])
	s.Equal("success", attrs["workflow.status"])
}

func (s *TracerTestSuite) TestStartWorkflow_WithError() {
	_, finish := s.tracer.StartWorkflow(
		s.ctx, "exec-456", "fail-wf", "cron",
	)

	testErr := errors.New("workflow failed")
	finish("failed", testErr)

	spans := s.exporter.GetSpans()
	s.Require().Len(spans, 1)
	s.Equal(codes.Error, spans[0].Status.Code)
	s.Equal("workflow failed", spans[0].Status.Description)
	s.Require().Len(spans[0].Events, 1)
	s.Equal("exception", spans[0].Events[0].Name)
}

func (s *TracerTestSuite) TestStartStep_CreatesChildSpan() {
	ctx, finishWf := s.tracer.StartWorkflow(
		s.ctx, "exec-123", "my-workflow", "http",
	)
	stepCtx, finishStep := s.tracer.StartStep(ctx, "call-api", "http", nil)
	s.NotNil(stepCtx)

	finishStep("success", nil, nil, nil)
	finishWf("success", nil)

	spans := s.exporter.GetSpans()
	s.Require().Len(spans, 2)
	s.Equal("step call-api", spans[0].Name)
	s.Equal("workflow my-workflow", spans[1].Name)

	attrs := spanAttrMap(spans[0])
	s.Equal("call-api", attrs["step.name"])
	s.Equal("http", attrs["step.action"])
	s.Equal("success", attrs["step.status"])
}

func (s *TracerTestSuite) TestStartStep_WithInputAndOutput() {
	_, finishWf := s.tracer.StartWorkflow(
		s.ctx, "exec-789", "io-wf", "manual",
	)
	_, finishStep := s.tracer.StartStep(s.ctx, "transform", "map", nil)

	finishStep("success", nil, "input-data", "output-data")
	finishWf("success", nil)

	spans := s.exporter.GetSpans()
	s.Require().Len(spans, 2)

	attrs := spanAttrMap(spans[0])
	s.Equal("input-data", attrs["step.input"])
	s.Equal("output-data", attrs["step.output"])
}

func (s *TracerTestSuite) TestStartStep_WithError() {
	_, finishStep := s.tracer.StartStep(
		s.ctx, "bad-step", "http", nil,
	)

	testErr := errors.New("step failed")
	finishStep("failed", testErr, nil, nil)

	spans := s.exporter.GetSpans()
	s.Require().Len(spans, 1)
	s.Equal(codes.Error, spans[0].Status.Code)
	s.Equal("step failed", spans[0].Status.Description)
	s.Require().Len(spans[0].Events, 1)
	s.Equal("exception", spans[0].Events[0].Name)
}

func (s *TracerTestSuite) TestStartRetry_CreatesRetrySpans() {
	ctx, finishWf := s.tracer.StartWorkflow(
		s.ctx, "exec-123", "my-workflow", "",
	)
	stepCtx, finishStep := s.tracer.StartStep(ctx, "flaky-step", "http", nil)

	retryCtx, finishRetry := s.tracer.StartRetry(stepCtx, 1, 3)
	s.NotNil(retryCtx)
	finishRetry(errors.New("temporary failure"))

	_, finishRetry2 := s.tracer.StartRetry(stepCtx, 2, 3)
	finishRetry2(nil)

	finishStep("success", nil, nil, nil)
	finishWf("success", nil)

	spans := s.exporter.GetSpans()
	s.Require().Len(spans, 4)
	s.Equal("retry 1/3", spans[0].Name)
	s.Equal("retry 2/3", spans[1].Name)

	s.Equal(codes.Error, spans[0].Status.Code)
	s.Equal(codes.Ok, spans[1].Status.Code)

	attrs0 := spanAttrMap(spans[0])
	s.Equal(int64(1), attrs0["retry.attempt"])
	s.Equal(int64(3), attrs0["retry.max_attempts"])

	attrs1 := spanAttrMap(spans[1])
	s.Equal(int64(2), attrs1["retry.attempt"])
	s.Equal(int64(3), attrs1["retry.max_attempts"])
}

func (s *TracerTestSuite) TestNewTracer_WhenProviderIsNil() {
	tracer := NewTracer(nil)

	ctx, finish := tracer.StartWorkflow(s.ctx, "exec-1", "wf", "")
	s.NotNil(ctx)
	finish("success", nil)
}

func (s *TracerTestSuite) TestResolveSpanKind() {
	for _, tc := range []struct {
		action   string
		expected trace.SpanKind
	}{
		{"http", trace.SpanKindClient},
		{"sql.query", trace.SpanKindClient},
		{"sql.exec", trace.SpanKindClient},
		{"sql.begin", trace.SpanKindClient},
		{"sql.commit", trace.SpanKindClient},
		{"sql.rollback", trace.SpanKindClient},
		{"kv.get", trace.SpanKindClient},
		{"kv.set", trace.SpanKindClient},
		{"kv.delete", trace.SpanKindClient},
		{"rabbitmq.shovel", trace.SpanKindClient},
		{"wait.rabbitmq", trace.SpanKindClient},
		{"set", trace.SpanKindInternal},
		{"log", trace.SpanKindInternal},
	} {
		s.Equal(tc.expected, resolveSpanKind(tc.action), "action: %s", tc.action)
	}
}

func spanAttrMap(span tracetest.SpanStub) map[string]any {
	attrs := make(map[string]any)

	for _, a := range span.Attributes {
		attrs[string(a.Key)] = a.Value.AsInterface()
	}

	return attrs
}

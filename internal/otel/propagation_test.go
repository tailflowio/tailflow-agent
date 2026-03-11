package otel

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type PropagationTestSuite struct {
	suite.Suite
	exporter *tracetest.InMemoryExporter
	provider *sdktrace.TracerProvider
}

func TestPropagation(t *testing.T) {
	suite.Run(t, new(PropagationTestSuite))
}

func (s *PropagationTestSuite) SetupTest() {
	s.exporter = tracetest.NewInMemoryExporter()
	s.provider = sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(s.exporter),
	)
	otel.SetTracerProvider(s.provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
}

func (s *PropagationTestSuite) TestInjectTraceContext_WhenSpanIsActive() {
	ctx, span := s.provider.Tracer("test").Start(
		context.Background(), "test-span",
	)
	defer span.End()

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, "http://example.com", nil,
	)
	s.Require().NoError(err)

	InjectTraceContext(req)

	s.NotEmpty(req.Header.Get("Traceparent"))
}

func (s *PropagationTestSuite) TestInjectTraceContext_WhenNoSpanIsActive() {
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, "http://example.com", nil,
	)
	s.Require().NoError(err)

	InjectTraceContext(req)

	s.Empty(req.Header.Get("Traceparent"))
}

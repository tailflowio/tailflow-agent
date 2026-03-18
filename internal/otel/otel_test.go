package otel

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	sdkotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

var errStub = errors.New("stub error")

type SetupTestSuite struct {
	suite.Suite
	ctx context.Context
}

func TestSetup(t *testing.T) {
	suite.Run(t, new(SetupTestSuite))
}

func (s *SetupTestSuite) SetupTest() {
	s.ctx = context.Background()
	newResource = defaultResource
	newTraceExporter = defaultTraceExporter
	newMetricExporter = defaultMetricExporter
	newLogExporter = defaultLogExporter
}

func (s *SetupTestSuite) TestSetup_WhenEndpointIsEmpty() {
	cfg := Config{ServiceName: "test-svc"}

	res, err := Setup(s.ctx, cfg)

	s.Require().NoError(err)
	s.NotNil(res.TracerProvider)
	s.NotNil(res.MeterProvider)
	s.NotNil(res.LoggerProvider)
	s.NotNil(res.Shutdown)
	s.IsType(&trace.TracerProvider{}, res.TracerProvider)
	s.IsType(&metric.MeterProvider{}, res.MeterProvider)
	s.IsType(&log.LoggerProvider{}, res.LoggerProvider)

	s.NoError(res.Shutdown(s.ctx))
}

func (s *SetupTestSuite) TestSetup_WhenEndpointIsConfigured() {
	cfg := Config{
		Endpoint:    "http://localhost:4318",
		ServiceName: "test-svc",
	}

	res, err := Setup(s.ctx, cfg)

	s.Require().NoError(err)
	s.NotNil(res.TracerProvider)
	s.NotNil(res.MeterProvider)
	s.NotNil(res.LoggerProvider)
	s.NotNil(res.Shutdown)
	s.IsType(&trace.TracerProvider{}, res.TracerProvider)
	s.IsType(&metric.MeterProvider{}, res.MeterProvider)
	s.IsType(&log.LoggerProvider{}, res.LoggerProvider)

	cancelCtx, cancel := context.WithCancel(s.ctx)
	cancel()

	_ = res.Shutdown(cancelCtx)
}

func (s *SetupTestSuite) TestSetup_WhenResourceFails() {
	newResource = func(_ context.Context, _ Config) (*resource.Resource, error) {
		return nil, errStub
	}

	cfg := Config{
		Endpoint:    "http://localhost:4318",
		ServiceName: "test-svc",
	}

	res, err := Setup(s.ctx, cfg)

	s.Nil(res)
	s.ErrorIs(err, errStub)
}

func (s *SetupTestSuite) TestSetup_WhenTraceExporterFails() {
	newTraceExporter = func(_ context.Context, _ string, _ bool) (trace.SpanExporter, error) {
		return nil, errStub
	}

	cfg := Config{
		Endpoint:    "http://localhost:4318",
		ServiceName: "test-svc",
	}

	res, err := Setup(s.ctx, cfg)

	s.Nil(res)
	s.ErrorIs(err, errStub)
}

func (s *SetupTestSuite) TestSetup_WhenMetricExporterFails() {
	newMetricExporter = func(_ context.Context, _ string, _ bool) (metric.Exporter, error) {
		return nil, errStub
	}

	cfg := Config{
		Endpoint:    "http://localhost:4318",
		ServiceName: "test-svc",
	}

	res, err := Setup(s.ctx, cfg)

	s.Nil(res)
	s.ErrorIs(err, errStub)
}

func (s *SetupTestSuite) TestSetup_WhenLogExporterFails() {
	newLogExporter = func(_ context.Context, _ string, _ bool) (log.Exporter, error) {
		return nil, errStub
	}

	cfg := Config{
		Endpoint:    "http://localhost:4318",
		ServiceName: "test-svc",
	}

	res, err := Setup(s.ctx, cfg)

	s.Nil(res)
	s.ErrorIs(err, errStub)
}

func (s *SetupTestSuite) TestEnabled_WhenEndpointIsEmpty() {
	cfg := Config{}

	s.False(cfg.Enabled())
}

func (s *SetupTestSuite) TestEnabled_WhenEndpointIsSet() {
	cfg := Config{Endpoint: "http://localhost:4318"}

	s.True(cfg.Enabled())
}

func (s *SetupTestSuite) TestSetup_WithSyncAndDebug() {
	cfg := Config{
		Endpoint:    "http://localhost:4318",
		ServiceName: "test-svc",
		Sync:        true,
		Debug:       true,
	}

	res, err := Setup(s.ctx, cfg)

	s.Require().NoError(err)
	s.NotNil(res.TracerProvider)
	s.NotNil(res.ForceFlush)

	cancelCtx, cancel := context.WithCancel(s.ctx)
	cancel()

	_ = res.Shutdown(cancelCtx)
}

func (s *SetupTestSuite) TestSetup_WithHTTPSEndpoint() {
	cfg := Config{
		Endpoint:    "https://otel.example.com",
		ServiceName: "test-svc",
	}

	res, err := Setup(s.ctx, cfg)

	s.Require().NoError(err)
	s.NotNil(res.TracerProvider)

	cancelCtx, cancel := context.WithCancel(s.ctx)
	cancel()

	_ = res.Shutdown(cancelCtx)
}

func (s *SetupTestSuite) TestForceFlush_IsCallable() {
	cfg := Config{ServiceName: "test-svc"}

	res, err := Setup(s.ctx, cfg)

	s.Require().NoError(err)
	s.NotNil(res.ForceFlush)

	flushErr := res.ForceFlush(s.ctx)
	s.NoError(flushErr)

	s.NoError(res.Shutdown(s.ctx))
}

func (s *SetupTestSuite) TestParseEndpoint_HTTP() {
	host, insecure := parseEndpoint("http://localhost:4318")

	s.Equal("localhost:4318", host)
	s.True(insecure)
}

func (s *SetupTestSuite) TestParseEndpoint_HTTPS() {
	host, insecure := parseEndpoint("https://otel.example.com")

	s.Equal("otel.example.com", host)
	s.False(insecure)
}

func (s *SetupTestSuite) TestParseEndpoint_NoScheme() {
	host, insecure := parseEndpoint("otel.example.com")

	s.Equal("otel.example.com", host)
	s.False(insecure)
}

func (s *SetupTestSuite) TestSetup_DebugErrorHandlerIsInvoked() {
	cfg := Config{
		Endpoint:    "http://localhost:4318",
		ServiceName: "test-svc",
		Debug:       true,
	}

	res, err := Setup(s.ctx, cfg)
	s.Require().NoError(err)

	sdkotel.Handle(errors.New("synthetic otel error"))

	cancelCtx, cancel := context.WithCancel(s.ctx)
	cancel()

	_ = res.Shutdown(cancelCtx)
}

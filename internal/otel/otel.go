package otel

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
)

type Config struct {
	Endpoint    string
	ServiceName string
	Debug       bool
	Sync        bool // use synchronous export (for short-lived processes)
}

type Result struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	LoggerProvider *sdklog.LoggerProvider
	Shutdown       func(ctx context.Context) error
	ForceFlush     func(ctx context.Context) error
}

var (
	newTraceExporter  = defaultTraceExporter
	newMetricExporter = defaultMetricExporter
	newLogExporter    = defaultLogExporter
	newResource       = defaultResource
)

func (c Config) Enabled() bool {
	return c.Endpoint != ""
}

func Setup(ctx context.Context, cfg Config) (*Result, error) {
	if !cfg.Enabled() {
		return noopResult(), nil
	}

	if cfg.Debug {
		otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
			log.Printf("[otel] error: %v", err)
		}))
		log.Printf("[otel] endpoint=%s service=%s", cfg.Endpoint, cfg.ServiceName)
	}

	res, err := newResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel resource: %w", err)
	}

	tp, err := setupTraceProvider(ctx, cfg, res)
	if err != nil {
		return nil, fmt.Errorf("failed to setup trace provider: %w", err)
	}

	mp, err := setupMeterProvider(ctx, cfg, res)
	if err != nil {
		return nil, fmt.Errorf("failed to setup meter provider: %w", err)
	}

	lp, err := setupLoggerProvider(ctx, cfg, res)
	if err != nil {
		return nil, fmt.Errorf("failed to setup logger provider: %w", err)
	}

	return &Result{
		TracerProvider: tp,
		MeterProvider:  mp,
		LoggerProvider: lp,
		Shutdown:       shutdownFunc(tp, mp, lp),
		ForceFlush:     flushFunc(tp, mp, lp),
	}, nil
}

func noopResult() *Result {
	tp := sdktrace.NewTracerProvider()
	mp := sdkmetric.NewMeterProvider()
	lp := sdklog.NewLoggerProvider()

	return &Result{
		TracerProvider: tp,
		MeterProvider:  mp,
		LoggerProvider: lp,
		Shutdown:       shutdownFunc(tp, mp, lp),
		ForceFlush:     flushFunc(tp, mp, lp),
	}
}

func defaultResource(
	ctx context.Context,
	cfg Config,
) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
	)
}

func defaultTraceExporter(
	ctx context.Context,
	endpoint string,
	insecure bool,
) (sdktrace.SpanExporter, error) {
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpoint)}
	if insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	return otlptracehttp.New(ctx, opts...)
}

func defaultMetricExporter(
	ctx context.Context,
	endpoint string,
	insecure bool,
) (sdkmetric.Exporter, error) {
	opts := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpoint(endpoint)}
	if insecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	return otlpmetrichttp.New(ctx, opts...)
}

func defaultLogExporter(
	ctx context.Context,
	endpoint string,
	insecure bool,
) (sdklog.Exporter, error) {
	opts := []otlploghttp.Option{otlploghttp.WithEndpoint(endpoint)}
	if insecure {
		opts = append(opts, otlploghttp.WithInsecure())
	}

	return otlploghttp.New(ctx, opts...)
}

func setupTraceProvider(
	ctx context.Context,
	cfg Config,
	res *resource.Resource,
) (*sdktrace.TracerProvider, error) {
	endpoint, insecure := parseEndpoint(cfg.Endpoint)

	exp, err := newTraceExporter(ctx, endpoint, insecure)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
	}

	if cfg.Sync {
		opts = append(opts, sdktrace.WithSyncer(exp))
	} else {
		opts = append(opts, sdktrace.WithBatcher(exp))
	}

	if cfg.Debug {
		debugExp, debugErr := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if debugErr == nil {
			opts = append(opts, sdktrace.WithSyncer(debugExp))
		}
	}

	return sdktrace.NewTracerProvider(opts...), nil
}

func setupMeterProvider(
	ctx context.Context,
	cfg Config,
	res *resource.Resource,
) (*sdkmetric.MeterProvider, error) {
	endpoint, insecure := parseEndpoint(cfg.Endpoint)

	exp, err := newMetricExporter(ctx, endpoint, insecure)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	opts := []sdkmetric.Option{
		sdkmetric.WithResource(res),
	}

	if cfg.Sync {
		opts = append(opts, sdkmetric.WithReader(sdkmetric.NewManualReader()))
	}

	opts = append(opts, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)))

	return sdkmetric.NewMeterProvider(opts...), nil
}

func setupLoggerProvider(
	ctx context.Context,
	cfg Config,
	res *resource.Resource,
) (*sdklog.LoggerProvider, error) {
	endpoint, insecure := parseEndpoint(cfg.Endpoint)

	exp, err := newLogExporter(ctx, endpoint, insecure)
	if err != nil {
		return nil, fmt.Errorf("failed to create log exporter: %w", err)
	}

	// Always batch logs — they are not critical and batching reduces pressure on the collector.
	processor := sdklog.NewBatchProcessor(exp)

	return sdklog.NewLoggerProvider(
		sdklog.WithProcessor(processor),
		sdklog.WithResource(res),
	), nil
}

type shutdowner interface {
	Shutdown(ctx context.Context) error
	ForceFlush(ctx context.Context) error
}

func flushFunc(providers ...shutdowner) func(context.Context) error {
	return func(ctx context.Context) error {
		errs := make([]error, 0, len(providers))

		for _, p := range providers {
			errs = append(errs, p.ForceFlush(ctx))
		}

		return errors.Join(errs...)
	}
}

func shutdownFunc(providers ...shutdowner) func(context.Context) error {
	return func(ctx context.Context) error {
		errs := make([]error, 0, len(providers))

		for _, p := range providers {
			errs = append(errs, p.Shutdown(ctx))
		}

		return errors.Join(errs...)
	}
}

func parseEndpoint(endpoint string) (string, bool) {
	host, found := strings.CutPrefix(endpoint, "http://")
	if found {
		return host, true
	}

	host, _ = strings.CutPrefix(endpoint, "https://")

	return host, false
}

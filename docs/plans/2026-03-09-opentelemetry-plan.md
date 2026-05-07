# OpenTelemetry Integration — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add opt-in OpenTelemetry support (traces, metrics, logs) to TailFlow Agent, activated via `--otel-endpoint` flag or `OTEL_EXPORTER_OTLP_ENDPOINT` env var, with zero overhead when disabled.

**Architecture:** A new `internal/otel/` package encapsulates all OTel logic. When no endpoint is configured, all providers return noop implementations (zero allocation). The package exposes a single `Setup()` function that returns a `Shutdown` func. Tracing hooks are injected into the engine executor via a `Tracer` interface. Metrics read from the existing `metrics.Collector` snapshots. Logs use the official `otelslog` bridge.

**Tech Stack:** `go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`, `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp`, `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp`, `go.opentelemetry.io/contrib/bridges/otelslog`

---

## Project Rules (from `.claude/rules.md` and `.claude/conventions.md`)

**MANDATORY — all code in this plan MUST comply with:**

### Testing
- **100% code coverage** — verify with `make test-coverage` before each commit
- **testify/suite pattern** for all tests — `<FeatureName>TestSuite` with `SetupTest()`
- **Mockery** for mocks — add interfaces in `.mockery.yaml`, generate with `make gen-mocks`
- **Test naming**: `Test<Feature>_<Scenario>` (e.g., `TestSetup_WhenEndpointIsEmpty`)
- **No `time.Sleep()`** — use `GOEXPERIMENT=synctest`
- **Recreate mocks in `SetupTest()`** — never reuse between tests

### Code Style
- **Early returns mandatory** — no deep nesting
- **No comments** unless non-obvious constraint (1 line max, explain WHY not WHAT)
- **No separator/divider comments**
- **No assignment in `if` condition** — separate into two lines
- **`_` for unused parameters**
- **Explicit English names** — no abbreviations except standard (HTTP, ID, URL, etc.)
- **Max 140 char line length**
- **Preallocate slices** when size is known

### Error Handling
- Never swallow errors — always propagate
- Do not log AND return the error — choose one
- Wrap with context: `fmt.Errorf("failed to ...: %w", err)`
- Use `errors.Is()`/`errors.As()` — never `==`

### Validation Workflow
- **`make lint && make test`** must pass before every commit
- **Zero tolerance** for lint warnings (~80 linters via golangci-lint)
- Key linters: errcheck, govet, staticcheck, contextcheck, noctx, perfsprint, wsl, lll, gofumpt

### Architecture
- **Imports grouped**: stdlib → external → internal (blank lines between)
- **File structure**: imports → constants → types → constructors → methods → helpers
- Functions < 50 lines
- Context as first parameter, always propagated

---

## Task 0: Create feature branch

**Step 1: Create and switch to feature branch**

Run:
```bash
cd /Users/jackgianesini/workspace/tailflow/tailflow-agent
git checkout -b feat/opentelemetry
```

---

## Task 1: Add OTel dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Add OTel modules**

Run:
```bash
cd /Users/jackgianesini/workspace/tailflow/tailflow-agent
go get go.opentelemetry.io/otel@latest \
  go.opentelemetry.io/otel/sdk@latest \
  go.opentelemetry.io/otel/sdk/metric@latest \
  go.opentelemetry.io/otel/sdk/log@latest \
  go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@latest \
  go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp@latest \
  go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp@latest \
  go.opentelemetry.io/contrib/bridges/otelslog@latest
```

**Step 2: Tidy**

Run: `go mod tidy`
Expected: clean go.mod/go.sum

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add OpenTelemetry SDK and OTLP exporters"
```

---

## Task 2: OTel provider — noop by default

**Files:**
- Create: `internal/otel/otel.go`
- Test: `internal/otel/otel_test.go`

**Step 1: Write the failing test**

```go
// internal/otel/otel_test.go
package otel_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	tfotel "github.com/tailflow/tailflow/internal/otel"
)

type SetupTestSuite struct {
	suite.Suite

	context context.Context
}

func TestSetup(t *testing.T) {
	suite.Run(t, new(SetupTestSuite))
}

func (s *SetupTestSuite) SetupTest() {
	s.context = context.Background()
}

func (s *SetupTestSuite) TestSetup_WhenEndpointIsEmpty() {
	cfg := tfotel.Config{}

	result, err := tfotel.Setup(s.context, cfg)

	s.NoError(err)
	s.NotNil(result.TracerProvider)
	s.NotNil(result.Shutdown)

	err = result.Shutdown(s.context)
	s.NoError(err)
}

func (s *SetupTestSuite) TestSetup_WhenEndpointIsConfigured() {
	cfg := tfotel.Config{
		Endpoint:    "http://localhost:4318",
		ServiceName: "test-tailflow",
	}

	result, err := tfotel.Setup(s.context, cfg)

	s.NoError(err)
	s.NotNil(result.TracerProvider)

	err = result.Shutdown(s.context)
	s.NoError(err)
}

func (s *SetupTestSuite) TestEnabled_WhenEndpointIsEmpty() {
	cfg := tfotel.Config{}

	s.False(cfg.Enabled())
}

func (s *SetupTestSuite) TestEnabled_WhenEndpointIsSet() {
	cfg := tfotel.Config{Endpoint: "http://localhost:4318"}

	s.True(cfg.Enabled())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/otel/ -v -run TestSetup`
Expected: FAIL (package doesn't exist)

**Step 3: Write implementation**

```go
// internal/otel/otel.go
package otel

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

type Config struct {
	Endpoint    string
	ServiceName string
}

type Result struct {
	TracerProvider trace.TracerProvider
	LoggerProvider *sdklog.LoggerProvider
	MeterProvider  *sdkmetric.MeterProvider
	Shutdown       func(ctx context.Context) error
}

func (c Config) Enabled() bool {
	return c.Endpoint != ""
}

func Setup(ctx context.Context, cfg Config) (*Result, error) {
	if !cfg.Enabled() {
		return &Result{
			TracerProvider: nooptrace.NewTracerProvider(),
			Shutdown:       func(context.Context) error { return nil },
		}, nil
	}

	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = "tailflow"
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, err
	}

	tp, err := setupTraceProvider(ctx, cfg.Endpoint, res)
	if err != nil {
		return nil, err
	}

	mp, err := setupMeterProvider(ctx, cfg.Endpoint, res)
	if err != nil {
		return nil, err
	}

	lp, err := setupLoggerProvider(ctx, cfg.Endpoint, res)
	if err != nil {
		return nil, err
	}

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Result{
		TracerProvider: tp,
		LoggerProvider: lp,
		MeterProvider:  mp,
		Shutdown:       shutdownFunc(tp, mp, lp),
	}, nil
}

func setupTraceProvider(ctx context.Context, endpoint string, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(stripScheme(endpoint)),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	), nil
}

func setupMeterProvider(ctx context.Context, endpoint string, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
	exporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(stripScheme(endpoint)),
		otlpmetrichttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	return sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
	), nil
}

func setupLoggerProvider(ctx context.Context, endpoint string, res *resource.Resource) (*sdklog.LoggerProvider, error) {
	exporter, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpoint(stripScheme(endpoint)),
		otlploghttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	return sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		sdklog.WithResource(res),
	), nil
}

func shutdownFunc(tp *sdktrace.TracerProvider, mp *sdkmetric.MeterProvider, lp *sdklog.LoggerProvider) func(context.Context) error {
	return func(ctx context.Context) error {
		return errors.Join(
			tp.Shutdown(ctx),
			mp.Shutdown(ctx),
			lp.Shutdown(ctx),
		)
	}
}

// OTel SDK expects host:port, not full URL
func stripScheme(endpoint string) string {
	for _, prefix := range []string{"http://", "https://"} {
		if len(endpoint) > len(prefix) && endpoint[:len(prefix)] == prefix {
			return endpoint[len(prefix):]
		}
	}

	return endpoint
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/otel/ -v -run TestSetup`
Expected: PASS

**Step 5: Commit**

```bash
make lint && make test
git add internal/otel/
git commit -m "feat(otel): add provider setup with noop fallback"
```

---

## Task 3: Tracing helpers — workflow/step/retry spans

**Files:**
- Create: `internal/otel/tracing.go`
- Test: `internal/otel/tracing_test.go`

**Step 1: Write the failing test**

```go
// internal/otel/tracing_test.go
package otel_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	tfotel "github.com/tailflow/tailflow/internal/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type TracerTestSuite struct {
	suite.Suite

	context  context.Context
	exporter *tracetest.InMemoryExporter
	provider *sdktrace.TracerProvider
	tracer   *tfotel.Tracer
}

func TestTracer(t *testing.T) {
	suite.Run(t, new(TracerTestSuite))
}

func (s *TracerTestSuite) SetupTest() {
	s.context = context.Background()
	s.exporter = tracetest.NewInMemoryExporter()
	s.provider = sdktrace.NewTracerProvider(sdktrace.WithSyncer(s.exporter))
	s.tracer = tfotel.NewTracer(s.provider)
}

func (s *TracerTestSuite) TestStartWorkflow_CreatesRootSpan() {
	ctx, finish := s.tracer.StartWorkflow(s.context, "exec-123", "my-workflow", "http")
	s.NotNil(ctx)

	finish("success", nil)

	spans := s.exporter.GetSpans()
	s.Len(spans, 1)
	s.Equal("workflow my-workflow", spans[0].Name)
}

func (s *TracerTestSuite) TestStartStep_CreatesChildSpan() {
	ctx, finishWf := s.tracer.StartWorkflow(s.context, "exec-123", "my-workflow", "http")
	stepCtx, finishStep := s.tracer.StartStep(ctx, "call-api", "http")
	s.NotNil(stepCtx)

	finishStep("success", nil, nil, nil)
	finishWf("success", nil)

	spans := s.exporter.GetSpans()
	s.Len(spans, 2)
	s.Equal("step call-api", spans[0].Name)
	s.Equal("workflow my-workflow", spans[1].Name)
}

func (s *TracerTestSuite) TestStartRetry_CreatesRetrySpans() {
	ctx, finishWf := s.tracer.StartWorkflow(s.context, "exec-123", "my-workflow", "")
	stepCtx, finishStep := s.tracer.StartStep(ctx, "flaky-step", "http")

	retryCtx, finishRetry := s.tracer.StartRetry(stepCtx, 1, 3)
	s.NotNil(retryCtx)
	finishRetry(assert.AnError)

	_, finishRetry2 := s.tracer.StartRetry(stepCtx, 2, 3)
	finishRetry2(nil)

	finishStep("success", nil, nil, nil)
	finishWf("success", nil)

	spans := s.exporter.GetSpans()
	s.Len(spans, 4)
	s.Equal("retry 1/3", spans[0].Name)
	s.Equal("retry 2/3", spans[1].Name)
}

func (s *TracerTestSuite) TestNewTracer_WhenProviderIsNil() {
	tracer := tfotel.NewTracer(nil)

	ctx, finish := tracer.StartWorkflow(s.context, "exec-1", "wf", "")
	s.NotNil(ctx)
	finish("success", nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/otel/ -v -run TestTracer`
Expected: FAIL (NewTracer not defined)

**Step 3: Write implementation**

```go
// internal/otel/tracing.go
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
	ctx context.Context, executionID, workflowName, triggerType string,
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
	ctx context.Context, stepID, actionName string,
) (context.Context, func(status string, err error, input any, output any)) {
	ctx, span := t.tracer.Start(ctx, "step "+stepID,
		trace.WithAttributes(
			attribute.String("step.name", stepID),
			attribute.String("step.action", actionName),
		),
	)

	return ctx, func(status string, err error, input any, output any) {
		span.SetAttributes(attribute.String("step.status", status))

		if input != nil {
			span.SetAttributes(attribute.String("step.input", fmt.Sprintf("%v", input)))
		}

		if output != nil {
			span.SetAttributes(attribute.String("step.output", fmt.Sprintf("%v", output)))
		}

		finishSpan(span, err)
	}
}

func (t *Tracer) StartRetry(
	ctx context.Context, attempt, maxAttempts int,
) (context.Context, func(err error)) {
	ctx, span := t.tracer.Start(ctx, fmt.Sprintf("retry %d/%d", attempt, maxAttempts),
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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	span.End()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/otel/ -v -run TestTracer`
Expected: PASS

**Step 5: Commit**

```bash
make lint && make test
git add internal/otel/tracing.go internal/otel/tracing_test.go
git commit -m "feat(otel): add tracing helpers for workflow/step/retry spans"
```

---

## Task 4: Metrics — OTel instruments from existing collector

**Files:**
- Create: `internal/otel/metrics.go`
- Test: `internal/otel/metrics_test.go`

**Step 1: Write the failing test**

```go
// internal/otel/metrics_test.go
package otel_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	tfotel "github.com/tailflow/tailflow/internal/otel"
	"github.com/tailflow/tailflow/internal/metrics"
)

type fakeSnapshotProvider struct{}

func (f *fakeSnapshotProvider) Snapshot() metrics.ProcessMetrics {
	return metrics.ProcessMetrics{
		CPUPercent: 12.5,
		RSSKB:      1024,
		Goroutines: 42,
		HeapMB:     10.5,
		NetRxBytes: 1000,
		NetTxBytes: 2000,
		UptimeS:    60,
		Available:  true,
	}
}

type MetricsTestSuite struct {
	suite.Suite

	context context.Context
}

func TestMetrics(t *testing.T) {
	suite.Run(t, new(MetricsTestSuite))
}

func (s *MetricsTestSuite) SetupTest() {
	s.context = context.Background()
}

func (s *MetricsTestSuite) TestRegisterMetrics_Success() {
	cfg := tfotel.Config{
		Endpoint:    "http://localhost:4318",
		ServiceName: "test",
	}

	result, err := tfotel.Setup(s.context, cfg)
	s.NoError(err)

	defer result.Shutdown(s.context) //nolint:errcheck

	err = tfotel.RegisterMetrics(result.MeterProvider, &fakeSnapshotProvider{})
	s.NoError(err)
}

func (s *MetricsTestSuite) TestRegisterMetrics_WhenProviderIsNil() {
	err := tfotel.RegisterMetrics(nil, &fakeSnapshotProvider{})
	s.NoError(err)
}

func (s *MetricsTestSuite) TestRegisterMetrics_WhenCollectorIsNil() {
	cfg := tfotel.Config{
		Endpoint:    "http://localhost:4318",
		ServiceName: "test",
	}

	result, err := tfotel.Setup(s.context, cfg)
	s.NoError(err)

	defer result.Shutdown(s.context) //nolint:errcheck

	err = tfotel.RegisterMetrics(result.MeterProvider, nil)
	s.NoError(err)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/otel/ -v -run TestRegisterMetrics`
Expected: FAIL (RegisterMetrics not defined)

**Step 3: Write implementation**

```go
// internal/otel/metrics.go
package otel

import (
	"context"

	"github.com/tailflow/tailflow/internal/metrics"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type SnapshotProvider interface {
	Snapshot() metrics.ProcessMetrics
}

func RegisterMetrics(mp *sdkmetric.MeterProvider, collector SnapshotProvider) error {
	if mp == nil || collector == nil {
		return nil
	}

	meter := mp.Meter("tailflow")

	_, err := meter.Float64ObservableGauge("tailflow.cpu.usage",
		metric.WithDescription("CPU usage percentage"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			o.Observe(collector.Snapshot().CPUPercent)
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableGauge("tailflow.memory.rss",
		metric.WithDescription("Resident set size in KB"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(collector.Snapshot().RSSKB)
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableGauge("tailflow.goroutines",
		metric.WithDescription("Number of goroutines"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(int64(collector.Snapshot().Goroutines))
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Float64ObservableGauge("tailflow.heap.alloc",
		metric.WithDescription("Heap allocation in MB"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			o.Observe(collector.Snapshot().HeapMB)
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableGauge("tailflow.network.rx_bytes",
		metric.WithDescription("Network bytes received"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(collector.Snapshot().NetRxBytes)
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableGauge("tailflow.network.tx_bytes",
		metric.WithDescription("Network bytes transmitted"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(collector.Snapshot().NetTxBytes)
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableGauge("tailflow.uptime",
		metric.WithDescription("Uptime in seconds"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(collector.Snapshot().UptimeS)
			return nil
		}),
	)
	if err != nil {
		return err
	}

	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/otel/ -v -run TestRegisterMetrics`
Expected: PASS

**Step 5: Commit**

```bash
make lint && make test
git add internal/otel/metrics.go internal/otel/metrics_test.go
git commit -m "feat(otel): add OTel metric instruments reading from existing collector"
```

---

## Task 4b: Business metrics — workflow/step counters and histograms

These metrics complement the system metrics (Task 4) with workflow-level observability. They are recorded from the same places where tracing spans are created (engine executor), so they share the same data with near-zero overhead.

**Files:**
- Create: `internal/otel/business_metrics.go`
- Test: `internal/otel/business_metrics_test.go`

**Step 1: Write the failing test**

```go
// internal/otel/business_metrics_test.go
package otel_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	tfotel "github.com/tailflow/tailflow/internal/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type BusinessMetricsTestSuite struct {
	suite.Suite

	context  context.Context
	reader   *sdkmetric.ManualReader
	provider *sdkmetric.MeterProvider
}

func TestBusinessMetrics(t *testing.T) {
	suite.Run(t, new(BusinessMetricsTestSuite))
}

func (s *BusinessMetricsTestSuite) SetupTest() {
	s.context = context.Background()
	s.reader = sdkmetric.NewManualReader()
	s.provider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(s.reader))
}

func (s *BusinessMetricsTestSuite) TestNewBusinessMetrics_Success() {
	bm, err := tfotel.NewBusinessMetrics(s.provider)

	s.NoError(err)
	s.NotNil(bm)
}

func (s *BusinessMetricsTestSuite) TestNewBusinessMetrics_WhenProviderIsNil() {
	bm, err := tfotel.NewBusinessMetrics(nil)

	s.NoError(err)
	s.Nil(bm)
}

func (s *BusinessMetricsTestSuite) TestRecordWorkflowExecution_IncrementsCounter() {
	bm, err := tfotel.NewBusinessMetrics(s.provider)
	s.NoError(err)

	bm.RecordWorkflowStarted("my-workflow")
	bm.RecordWorkflowCompleted("my-workflow", "success", 1500)

	var rm metricdata.ResourceMetrics
	err = s.reader.Collect(s.context, &rm)
	s.NoError(err)
	s.NotEmpty(rm.ScopeMetrics)
}

func (s *BusinessMetricsTestSuite) TestRecordStepExecution_IncrementsCounterAndHistogram() {
	bm, err := tfotel.NewBusinessMetrics(s.provider)
	s.NoError(err)

	bm.RecordStepCompleted("fetch-data", "http", "success", 250)
	bm.RecordStepCompleted("fetch-data", "http", "failed", 100)

	var rm metricdata.ResourceMetrics
	err = s.reader.Collect(s.context, &rm)
	s.NoError(err)
	s.NotEmpty(rm.ScopeMetrics)
}

func (s *BusinessMetricsTestSuite) TestRecordStepError_IncrementsErrorCounter() {
	bm, err := tfotel.NewBusinessMetrics(s.provider)
	s.NoError(err)

	bm.RecordStepError("fetch-data", "http", "timeout")
	bm.RecordStepError("fetch-data", "http", "action_failed")

	var rm metricdata.ResourceMetrics
	err = s.reader.Collect(s.context, &rm)
	s.NoError(err)
	s.NotEmpty(rm.ScopeMetrics)
}

func (s *BusinessMetricsTestSuite) TestRecordStepRetry_IncrementsRetryCounter() {
	bm, err := tfotel.NewBusinessMetrics(s.provider)
	s.NoError(err)

	bm.RecordStepRetry("fetch-data", "http")

	var rm metricdata.ResourceMetrics
	err = s.reader.Collect(s.context, &rm)
	s.NoError(err)
	s.NotEmpty(rm.ScopeMetrics)
}

func (s *BusinessMetricsTestSuite) TestActiveExecutions_UpDown() {
	bm, err := tfotel.NewBusinessMetrics(s.provider)
	s.NoError(err)

	bm.RecordWorkflowStarted("my-workflow")
	bm.RecordWorkflowStarted("my-workflow")
	bm.RecordWorkflowCompleted("my-workflow", "success", 500)

	var rm metricdata.ResourceMetrics
	err = s.reader.Collect(s.context, &rm)
	s.NoError(err)
	s.NotEmpty(rm.ScopeMetrics)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/otel/ -v -run TestBusinessMetrics`
Expected: FAIL (NewBusinessMetrics not defined)

**Step 3: Write implementation**

```go
// internal/otel/business_metrics.go
package otel

import (
	"context"
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

	meter := mp.Meter("tailflow")

	workflowExecutionsTotal, err := meter.Int64Counter("tailflow.workflow.executions.total",
		metric.WithDescription("Total workflow executions"),
	)
	if err != nil {
		return nil, err
	}

	workflowDuration, err := meter.Float64Histogram("tailflow.workflow.execution.duration",
		metric.WithDescription("Workflow execution duration in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	activeExecutions, err := meter.Int64UpDownCounter("tailflow.workflow.active_executions",
		metric.WithDescription("Currently running workflow executions"),
	)
	if err != nil {
		return nil, err
	}

	stepDuration, err := meter.Float64Histogram("tailflow.step.execution.duration",
		metric.WithDescription("Step execution duration in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	stepErrorsTotal, err := meter.Int64Counter("tailflow.step.errors.total",
		metric.WithDescription("Total step errors"),
	)
	if err != nil {
		return nil, err
	}

	stepRetriesTotal, err := meter.Int64Counter("tailflow.step.retries.total",
		metric.WithDescription("Total step retry attempts"),
	)
	if err != nil {
		return nil, err
	}

	return &BusinessMetrics{
		workflowExecutionsTotal: workflowExecutionsTotal,
		workflowDuration:        workflowDuration,
		activeExecutions:        activeExecutions,
		stepDuration:            stepDuration,
		stepErrorsTotal:         stepErrorsTotal,
		stepRetriesTotal:        stepRetriesTotal,
	}, nil
}

func (m *BusinessMetrics) RecordWorkflowStarted(workflowName string) {
	if m == nil {
		return
	}

	attrs := metric.WithAttributes(attribute.String("workflow.name", workflowName))
	m.workflowExecutionsTotal.Add(context.Background(), 1, attrs)
	m.activeExecutions.Add(context.Background(), 1, attrs)
}

func (m *BusinessMetrics) RecordWorkflowCompleted(workflowName, status string, durationMs float64) {
	if m == nil {
		return
	}

	attrs := metric.WithAttributes(
		attribute.String("workflow.name", workflowName),
		attribute.String("workflow.status", status),
	)
	m.workflowDuration.Record(context.Background(), durationMs, attrs)
	m.activeExecutions.Add(context.Background(), -1,
		metric.WithAttributes(attribute.String("workflow.name", workflowName)),
	)
}

func (m *BusinessMetrics) RecordStepCompleted(stepID, actionName, status string, durationMs float64) {
	if m == nil {
		return
	}

	attrs := metric.WithAttributes(
		attribute.String("step.name", stepID),
		attribute.String("step.action", actionName),
		attribute.String("step.status", status),
	)
	m.stepDuration.Record(context.Background(), durationMs, attrs)
}

func (m *BusinessMetrics) RecordStepError(stepID, actionName, errorCode string) {
	if m == nil {
		return
	}

	attrs := metric.WithAttributes(
		attribute.String("step.name", stepID),
		attribute.String("step.action", actionName),
		attribute.String("error.code", errorCode),
	)
	m.stepErrorsTotal.Add(context.Background(), 1, attrs)
}

func (m *BusinessMetrics) RecordStepRetry(stepID, actionName string) {
	if m == nil {
		return
	}

	attrs := metric.WithAttributes(
		attribute.String("step.name", stepID),
		attribute.String("step.action", actionName),
	)
	m.stepRetriesTotal.Add(context.Background(), 1, attrs)
}

func DurationMs(start time.Time) float64 {
	return float64(time.Since(start).Milliseconds())
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/otel/ -v -run TestBusinessMetrics`
Expected: PASS

**Step 5: Commit**

```bash
make lint && make test
git add internal/otel/business_metrics.go internal/otel/business_metrics_test.go
git commit -m "feat(otel): add business metrics (workflow/step counters and histograms)"
```

---

## Task 5: Logging — slog bridge to OTel

**Files:**
- Create: `internal/otel/logging.go`
- Test: `internal/otel/logging_test.go`

**Step 1: Write the failing test**

```go
// internal/otel/logging_test.go
package otel_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/suite"

	tfotel "github.com/tailflow/tailflow/internal/otel"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

type LoggingTestSuite struct {
	suite.Suite

	context context.Context
}

func TestLogging(t *testing.T) {
	suite.Run(t, new(LoggingTestSuite))
}

func (s *LoggingTestSuite) SetupTest() {
	s.context = context.Background()
}

func (s *LoggingTestSuite) TestNewLogHandler_WhenProviderIsNil() {
	handler := tfotel.NewLogHandler(nil)

	s.Nil(handler)
}

func (s *LoggingTestSuite) TestNewLogHandler_WhenProviderIsConfigured() {
	loggerProvider := sdklog.NewLoggerProvider()
	defer loggerProvider.Shutdown(s.context) //nolint:errcheck

	handler := tfotel.NewLogHandler(loggerProvider)

	s.NotNil(handler)

	logger := slog.New(handler)
	s.NotNil(logger)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/otel/ -v -run TestNewLogHandler`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/otel/logging.go
package otel

import (
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

func NewLogHandler(lp *sdklog.LoggerProvider) slog.Handler {
	if lp == nil {
		return nil
	}

	return otelslog.NewHandler("tailflow", otelslog.WithLoggerProvider(lp))
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/otel/ -v -run TestNewLogHandler`
Expected: PASS

**Step 5: Commit**

```bash
make lint && make test
git add internal/otel/logging.go internal/otel/logging_test.go
git commit -m "feat(otel): add slog-to-OTel log bridge"
```

---

## Task 6: W3C trace propagation in HTTP action

**Files:**
- Create: `internal/otel/propagation.go`
- Test: `internal/otel/propagation_test.go`
- Modify: `internal/action/http.go:27-68`

**Step 1: Write the failing test for propagation helper**

```go
// internal/otel/propagation_test.go
package otel_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"

	tfotel "github.com/tailflow/tailflow/internal/otel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type PropagationTestSuite struct {
	suite.Suite

	context  context.Context
	exporter *tracetest.InMemoryExporter
	provider *sdktrace.TracerProvider
}

func TestPropagation(t *testing.T) {
	suite.Run(t, new(PropagationTestSuite))
}

func (s *PropagationTestSuite) SetupTest() {
	s.context = context.Background()
	s.exporter = tracetest.NewInMemoryExporter()
	s.provider = sdktrace.NewTracerProvider(sdktrace.WithSyncer(s.exporter))
	otel.SetTracerProvider(s.provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
}

func (s *PropagationTestSuite) TestInjectTraceContext_WhenSpanIsActive() {
	ctx, span := s.provider.Tracer("test").Start(s.context, "test-span")
	defer span.End()

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
	tfotel.InjectTraceContext(req)

	s.NotEmpty(req.Header.Get("Traceparent"))
}

func (s *PropagationTestSuite) TestInjectTraceContext_WhenNoSpanIsActive() {
	req, _ := http.NewRequestWithContext(s.context, "GET", "http://example.com", nil)
	tfotel.InjectTraceContext(req)

	s.Empty(req.Header.Get("Traceparent"))
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/otel/ -v -run TestInjectTraceContext`
Expected: FAIL

**Step 3: Write propagation helper**

```go
// internal/otel/propagation.go
package otel

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func InjectTraceContext(req *http.Request) {
	otel.GetTextMapPropagator().Inject(req.Context(), propagation.HeaderCarrier(req.Header))
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/otel/ -v -run TestInjectTraceContext`
Expected: PASS

**Step 5: Integrate into HTTP action**

Modify `internal/action/http.go`. After the request is created (line 42) and headers are applied (line 47), inject trace context:

```go
// In Execute method, after line 47 (applyHTTPHeaders):
tfotel.InjectTraceContext(req)
```

The import section gets:
```go
tfotel "github.com/tailflow/tailflow/internal/otel"
```

**Step 6: Run existing HTTP action tests**

Run: `go test ./internal/action/ -v -run TestHTTP`
Expected: PASS (injection is no-op when no active span)

**Step 7: Commit**

```bash
make lint && make test
git add internal/otel/propagation.go internal/otel/propagation_test.go internal/action/http.go
git commit -m "feat(otel): add W3C trace context propagation to HTTP action"
```

---

## Task 7: Integrate tracing and business metrics into the engine executor

This is the core integration. We add the `Tracer` and `BusinessMetrics` to the `Executor` and record spans + metrics at workflow/step/retry boundaries.

**Files:**
- Modify: `internal/engine/engine.go:23-39` (Executor struct + constructor)
- Modify: `internal/engine/engine.go:58-94` (Execute method — workflow span + metrics)
- Modify: `internal/engine/engine.go:401-468` (executeNode — step span + metrics)
- Modify: `internal/engine/engine.go:676-724` (executeWithRetry — retry spans + metrics)
- Test: `internal/engine/engine_test.go` (add OTel integration test)

**Step 1: Add Tracer and BusinessMetrics to Executor struct**

In `internal/engine/engine.go`, modify the Executor struct and constructor:

```go
// Add import:
tfotel "github.com/tailflow/tailflow/internal/otel"

// Modify Executor struct (line 23):
type Executor struct {
	registry        *action.Registry
	bus             *event.Bus
	eval            *runtime.ExprEvaluator
	logger          *slog.Logger
	sensitive       *SensitiveRegistry
	tracer          *tfotel.Tracer
	businessMetrics *tfotel.BusinessMetrics
}

// Modify constructor (line 31):
func NewExecutor(
	registry *action.Registry, bus *event.Bus, logger *slog.Logger,
	sensitiveKeys []string, tracer *tfotel.Tracer, businessMetrics *tfotel.BusinessMetrics,
) *Executor {
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
```

**Step 2: Add workflow span to Execute method**

In `Execute` (line 58), after building the execution context (line 74), wrap the DAG execution with a workflow span:

```go
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

	if len(opts) > 0 && opts[0].TestCaseName != "" {
		execCtx.TestCaseName = opts[0].TestCaseName
	}

	triggerType := ""
	if len(opts) > 0 && opts[0].TriggerData != nil {
		triggerType, _ = opts[0].TriggerData["method"].(string)
	}

	ctx, finishWorkflow := e.tracer.StartWorkflow(ctx, executionID, wf.Name, triggerType)
	e.businessMetrics.RecordWorkflowStarted(wf.Name)

	e.bus.Publish(event.NewEvent(event.WorkflowStarted, executionID, "", fmt.Sprintf("workflow %q started", wf.Name)))

	dag, err := BuildDAG(wf.Steps)
	if err != nil {
		finishWorkflow("failed", err)
		return nil, fmt.Errorf("build DAG: %w", err)
	}

	execErr := e.executeDAG(ctx, dag, execCtx)

	if len(wf.OnError) > 0 && (execErr != nil || execCtx.HasFailedSteps()) {
		e.executeOnError(ctx, parser.Step{OnError: wf.OnError}, execCtx)
	}

	result := e.buildExecuteResult(ctx, wf.Name, execCtx, execErr, executionID, startedAt)
	finishWorkflow(result.Status, result.Error)
	e.businessMetrics.RecordWorkflowCompleted(wf.Name, result.Status, tfotel.DurationMs(startedAt))

	return result, nil
}
```

**Step 3: Add step span to executeNode**

In `executeNode` (line 401), wrap the step execution with a span. The key change is to start a span after the "when" condition check and finish it at the end:

```go
func (e *Executor) executeNode(ctx context.Context, node *DAGNode, execCtx *runtime.ExecutionContext) error {
	step := node.Step
	logger := e.logger.With("step", step.ID)

	skipped, err := e.evaluateWhenCondition(step, execCtx, logger)
	if skipped || err != nil {
		return err
	}

	// ... test mock handling stays the same (lines 410-423) ...

	// Start OTel step span
	ctx, finishStep := e.tracer.StartStep(ctx, step.ID, step.Action)

	e.bus.Publish(event.NewEvent(event.StepStarted, execCtx.ExecutionID, step.ID, fmt.Sprintf("step %q started", step.ID)))
	logger.Info("started", "action", step.Action)

	stepStartedAt := time.Now()

	resolvedConfig, err := e.resolveStepConfig(step, execCtx)
	if err != nil {
		finishStep("failed", err, nil, nil)
		return err
	}

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
		e.businessMetrics.RecordStepCompleted(step.ID, step.Action, "failed", tfotel.DurationMs(stepStartedAt))
		e.businessMetrics.RecordStepError(step.ID, step.Action, stepErrorCode(execErr))
		return e.handleStepError(ctx, step, output, execErr, execCtx, &stepStartedAt, logger)
	}

	maskedInput := e.sensitive.MaskMap(e.prepareStepInput(step, resolvedConfig, execCtx))
	maskedOutput := e.sensitive.MaskAny(output)
	finishStep("success", nil, maskedInput, maskedOutput)
	e.businessMetrics.RecordStepCompleted(step.ID, step.Action, "success", tfotel.DurationMs(stepStartedAt))

	e.recordStepSuccess(step, output, execCtx, &stepStartedAt, logger)

	// ... test expect handling stays the same (lines 457-465) ...

	return nil
}
```

Note: We need a small helper on SensitiveRegistry for masking output (which can be `any`, not just `map[string]any`):

Add to `internal/engine/sensitive.go`:
```go
// MaskAny masks sensitive fields in any value type.
func (r *SensitiveRegistry) MaskAny(v any) any {
	if len(r.keys) == 0 || v == nil {
		return v
	}
	return r.maskValue(v)
}
```

**Step 4: Add retry spans to executeWithRetry**

In `executeWithRetry` (line 676), wrap each attempt in a retry span when retries are configured:

```go
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
			e.businessMetrics.RecordStepRetry(step.ID, step.Action)
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
```

**Step 5: Update all callers of NewExecutor**

All call sites must pass the new `tracer` parameter. Search for `NewExecutor(` across the codebase:

- `cmd/tailflow/main.go:1239` — `engine.NewExecutor(reg, bus, logger, wf.Sensitive)` → add tracer
- `cmd/tailflow/main.go:1608` — same in `runSingleTestCase`
- `internal/server/server.go` (via Config.Executor) — created in main.go
- All test files in `internal/engine/` — pass `nil` tracer (noop behavior)

For `cmd/tailflow/main.go`, the tracer comes from the OTel setup result. For test files, pass `tfotel.NewTracer(nil)` or adjust the constructor to accept nil.

**Step 6: Run all engine tests**

Run: `go test ./internal/engine/ -v`
Expected: PASS (all existing tests work with nil tracer)

**Step 7: Write integration test**

Add to `internal/engine/engine_test.go` (or a new file `internal/engine/engine_otel_test.go`):

```go
func TestExecute_OTelSpans(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tracer := tfotel.NewTracer(tp)

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)
	bus := event.NewBus()
	defer bus.Close()

	exec := engine.NewExecutor(reg, bus, slog.Default(), nil, tracer)

	wf := &parser.Workflow{
		Name: "otel-test",
		Steps: []parser.Step{
			{ID: "step1", Action: "set", Config: map[string]any{"value": "hello"}},
			{ID: "step2", Action: "set", Config: map[string]any{"value": "world"}, DependsOn: []string{"step1"}},
		},
	}

	result, err := exec.Execute(context.Background(), wf, nil)
	require.NoError(t, err)
	assert.Equal(t, runtime.StatusSuccess, result.Status)

	spans := exporter.GetSpans()
	// Should have: step1, step2, workflow (in finish order)
	require.GreaterOrEqual(t, len(spans), 3)

	spanNames := make([]string, len(spans))
	for i, s := range spans {
		spanNames[i] = s.Name
	}
	assert.Contains(t, spanNames, "workflow otel-test")
	assert.Contains(t, spanNames, "step step1")
	assert.Contains(t, spanNames, "step step2")
}
```

**Step 8: Run integration test**

Run: `go test ./internal/engine/ -v -run TestExecute_OTelSpans`
Expected: PASS

**Step 9: Commit**

```bash
make lint && make test
git add internal/engine/engine.go internal/engine/sensitive.go internal/engine/engine_otel_test.go
git commit -m "feat(otel): integrate tracing spans into engine executor"
```

---

## Task 7b: Enrich step spans with semantic conventions

Add action-specific OTel semantic convention attributes to step spans. Only for actions that interact with external systems — in-process actions (`set`, `log`, `json.*`, etc.) keep the generic span.

**Files:**
- Create: `internal/otel/enrich.go`
- Test: `internal/otel/enrich_test.go`

**Step 1: Write the failing test**

```go
// internal/otel/enrich_test.go
package otel_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	tfotel "github.com/tailflow/tailflow/internal/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type EnrichTestSuite struct {
	suite.Suite

	context  context.Context
	exporter *tracetest.InMemoryExporter
	provider *sdktrace.TracerProvider
}

func TestEnrich(t *testing.T) {
	suite.Run(t, new(EnrichTestSuite))
}

func (s *EnrichTestSuite) SetupTest() {
	s.context = context.Background()
	s.exporter = tracetest.NewInMemoryExporter()
	s.provider = sdktrace.NewTracerProvider(sdktrace.WithSyncer(s.exporter))
}

func (s *EnrichTestSuite) TestEnrichStepSpan_HTTP() {
	_, span := s.provider.Tracer("test").Start(s.context, "step call-api")

	config := map[string]any{
		"method": "POST",
		"url":    "https://api.example.com/users",
	}
	output := map[string]any{
		"status": 201,
	}

	tfotel.EnrichStepSpan(span, "http", config, output)
	span.End()

	spans := s.exporter.GetSpans()
	s.Len(spans, 1)

	attrs := spanAttrMap(spans[0])
	s.Equal("POST", attrs["http.request.method"])
	s.Equal("https://api.example.com/users", attrs["url.full"])
	s.Equal(int64(201), attrs["http.response.status_code"])
}

func (s *EnrichTestSuite) TestEnrichStepSpan_SQLQuery() {
	_, span := s.provider.Tracer("test").Start(s.context, "step fetch-users")

	config := map[string]any{
		"url":   "postgres://localhost:5432/mydb",
		"query": "SELECT * FROM users WHERE active = true",
	}

	tfotel.EnrichStepSpan(span, "sql.query", config, nil)
	span.End()

	spans := s.exporter.GetSpans()
	attrs := spanAttrMap(spans[0])
	s.Equal("postgresql", attrs["db.system"])
	s.Equal("SELECT", attrs["db.operation"])
	s.Equal("SELECT * FROM users WHERE active = true", attrs["db.statement"])
}

func (s *EnrichTestSuite) TestEnrichStepSpan_SQLExec() {
	_, span := s.provider.Tracer("test").Start(s.context, "step insert-user")

	config := map[string]any{
		"url":   "mysql://localhost:3306/mydb",
		"query": "INSERT INTO users (name) VALUES (?)",
	}

	tfotel.EnrichStepSpan(span, "sql.exec", config, nil)
	span.End()

	spans := s.exporter.GetSpans()
	attrs := spanAttrMap(spans[0])
	s.Equal("mysql", attrs["db.system"])
	s.Equal("INSERT", attrs["db.operation"])
}

func (s *EnrichTestSuite) TestEnrichStepSpan_SQLTransaction() {
	for _, action := range []string{"sql.begin", "sql.commit", "sql.rollback"} {
		_, span := s.provider.Tracer("test").Start(s.context, "step tx")

		config := map[string]any{
			"url": "postgres://localhost/db",
		}

		tfotel.EnrichStepSpan(span, action, config, nil)
		span.End()
	}

	spans := s.exporter.GetSpans()
	s.Len(spans, 3)

	for i, expectedOp := range []string{"BEGIN", "COMMIT", "ROLLBACK"} {
		attrs := spanAttrMap(spans[i])
		s.Equal("postgresql", attrs["db.system"])
		s.Equal(expectedOp, attrs["db.operation"])
	}
}

func (s *EnrichTestSuite) TestEnrichStepSpan_KV() {
	for _, tc := range []struct {
		action    string
		operation string
	}{
		{"kv.get", "GET"},
		{"kv.set", "SET"},
		{"kv.delete", "DELETE"},
	} {
		_, span := s.provider.Tracer("test").Start(s.context, "step cache")

		config := map[string]any{
			"key": "user:123",
		}

		tfotel.EnrichStepSpan(span, tc.action, config, nil)
		span.End()

		spans := s.exporter.GetSpans()
		latest := spans[len(spans)-1]
		attrs := spanAttrMap(latest)
		s.Equal("redis", attrs["db.system"])
		s.Equal(tc.operation, attrs["db.operation"])
		s.Equal("user:123", attrs["db.redis.key"])
	}
}

func (s *EnrichTestSuite) TestEnrichStepSpan_RabbitMQ() {
	_, span := s.provider.Tracer("test").Start(s.context, "step forward")

	config := map[string]any{
		"queue": "notifications",
	}

	tfotel.EnrichStepSpan(span, "rabbitmq.shovel", config, nil)
	span.End()

	spans := s.exporter.GetSpans()
	attrs := spanAttrMap(spans[0])
	s.Equal("rabbitmq", attrs["messaging.system"])
	s.Equal("notifications", attrs["messaging.destination.name"])
}

func (s *EnrichTestSuite) TestEnrichStepSpan_WaitWebhook() {
	_, span := s.provider.Tracer("test").Start(s.context, "step wait-payment")

	config := map[string]any{
		"path": "/callback/payment",
	}

	tfotel.EnrichStepSpan(span, "wait.webhook", config, nil)
	span.End()

	spans := s.exporter.GetSpans()
	attrs := spanAttrMap(spans[0])
	s.Equal("webhook", attrs["tailflow.wait.type"])
	s.Equal("/callback/payment", attrs["tailflow.wait.path"])
}

func (s *EnrichTestSuite) TestEnrichStepSpan_WaitRabbitMQ() {
	_, span := s.provider.Tracer("test").Start(s.context, "step wait-response")

	config := map[string]any{
		"queue": "responses",
	}

	tfotel.EnrichStepSpan(span, "wait.rabbitmq", config, nil)
	span.End()

	spans := s.exporter.GetSpans()
	attrs := spanAttrMap(spans[0])
	s.Equal("rabbitmq", attrs["tailflow.wait.type"])
	s.Equal("responses", attrs["messaging.destination.name"])
}

func (s *EnrichTestSuite) TestEnrichStepSpan_Lock() {
	_, span := s.provider.Tracer("test").Start(s.context, "step acquire")

	config := map[string]any{
		"key": "order:456",
	}

	tfotel.EnrichStepSpan(span, "lock", config, nil)
	span.End()

	spans := s.exporter.GetSpans()
	attrs := spanAttrMap(spans[0])
	s.Equal("order:456", attrs["tailflow.lock.key"])
}

func (s *EnrichTestSuite) TestEnrichStepSpan_Exec() {
	_, span := s.provider.Tracer("test").Start(s.context, "step run-script")

	config := map[string]any{
		"command": "curl -s https://api.example.com/health",
	}

	tfotel.EnrichStepSpan(span, "exec", config, nil)
	span.End()

	spans := s.exporter.GetSpans()
	attrs := spanAttrMap(spans[0])
	s.Equal("curl -s https://api.example.com/health", attrs["process.command"])
}

func (s *EnrichTestSuite) TestEnrichStepSpan_FileReadWrite() {
	for _, action := range []string{"file.read", "file.write"} {
		_, span := s.provider.Tracer("test").Start(s.context, "step file-op")

		config := map[string]any{
			"path": "/tmp/data.json",
		}

		tfotel.EnrichStepSpan(span, action, config, nil)
		span.End()
	}

	spans := s.exporter.GetSpans()
	s.Len(spans, 2)

	for _, sp := range spans {
		attrs := spanAttrMap(sp)
		s.Equal("/tmp/data.json", attrs["tailflow.file.path"])
	}
}

func (s *EnrichTestSuite) TestEnrichStepSpan_UnknownAction() {
	_, span := s.provider.Tracer("test").Start(s.context, "step set-var")

	tfotel.EnrichStepSpan(span, "set", map[string]any{"value": "hello"}, nil)
	span.End()

	spans := s.exporter.GetSpans()
	attrs := spanAttrMap(spans[0])
	s.Empty(attrs)
}

func spanAttrMap(span tracetest.SpanStub) map[string]any {
	result := make(map[string]any, len(span.Attributes))

	for _, attr := range span.Attributes {
		result[string(attr.Key)] = attr.Value.AsInterface()
	}

	return result
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/otel/ -v -run TestEnrich`
Expected: FAIL (EnrichStepSpan not defined)

**Step 3: Write implementation**

```go
// internal/otel/enrich.go
package otel

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func EnrichStepSpan(span trace.Span, actionName string, config map[string]any, output any) {
	switch actionName {
	case "http":
		enrichHTTP(span, config, output)
	case "sql.query", "sql.exec":
		enrichSQL(span, config)
	case "sql.begin":
		enrichSQLTx(span, config, "BEGIN")
	case "sql.commit":
		enrichSQLTx(span, config, "COMMIT")
	case "sql.rollback":
		enrichSQLTx(span, config, "ROLLBACK")
	case "kv.get":
		enrichKV(span, config, "GET")
	case "kv.set":
		enrichKV(span, config, "SET")
	case "kv.delete":
		enrichKV(span, config, "DELETE")
	case "rabbitmq.shovel":
		enrichRabbitMQ(span, config)
	case "wait.webhook":
		enrichWaitWebhook(span, config)
	case "wait.rabbitmq":
		enrichWaitRabbitMQ(span, config)
	case "lock", "unlock":
		enrichLock(span, config)
	case "exec":
		enrichExec(span, config)
	case "file.read", "file.write":
		enrichFile(span, config)
	}
}

func enrichHTTP(span trace.Span, config map[string]any, output any) {
	if method, ok := configString(config, "method"); ok {
		span.SetAttributes(attribute.String("http.request.method", strings.ToUpper(method)))
	}

	if url, ok := configString(config, "url"); ok {
		span.SetAttributes(attribute.String("url.full", url))
	}

	outMap, ok := output.(map[string]any)
	if !ok {
		return
	}

	if status, ok := outMap["status"]; ok {
		span.SetAttributes(attribute.Int64("http.response.status_code", toInt64(status)))
	}
}

func enrichSQL(span trace.Span, config map[string]any) {
	if url, ok := configString(config, "url"); ok {
		span.SetAttributes(attribute.String("db.system", dbSystemFromURL(url)))
	}

	if query, ok := configString(config, "query"); ok {
		span.SetAttributes(attribute.String("db.statement", query))
		span.SetAttributes(attribute.String("db.operation", sqlOperation(query)))
	}
}

func enrichSQLTx(span trace.Span, config map[string]any, operation string) {
	if url, ok := configString(config, "url"); ok {
		span.SetAttributes(attribute.String("db.system", dbSystemFromURL(url)))
	}

	span.SetAttributes(attribute.String("db.operation", operation))
}

func enrichKV(span trace.Span, config map[string]any, operation string) {
	span.SetAttributes(
		attribute.String("db.system", "redis"),
		attribute.String("db.operation", operation),
	)

	if key, ok := configString(config, "key"); ok {
		span.SetAttributes(attribute.String("db.redis.key", key))
	}
}

func enrichRabbitMQ(span trace.Span, config map[string]any) {
	span.SetAttributes(attribute.String("messaging.system", "rabbitmq"))

	if queue, ok := configString(config, "queue"); ok {
		span.SetAttributes(attribute.String("messaging.destination.name", queue))
	}
}

func enrichWaitWebhook(span trace.Span, config map[string]any) {
	span.SetAttributes(attribute.String("tailflow.wait.type", "webhook"))

	if path, ok := configString(config, "path"); ok {
		span.SetAttributes(attribute.String("tailflow.wait.path", path))
	}
}

func enrichWaitRabbitMQ(span trace.Span, config map[string]any) {
	span.SetAttributes(attribute.String("tailflow.wait.type", "rabbitmq"))

	if queue, ok := configString(config, "queue"); ok {
		span.SetAttributes(attribute.String("messaging.destination.name", queue))
	}
}

func enrichLock(span trace.Span, config map[string]any) {
	if key, ok := configString(config, "key"); ok {
		span.SetAttributes(attribute.String("tailflow.lock.key", key))
	}
}

func enrichExec(span trace.Span, config map[string]any) {
	if command, ok := configString(config, "command"); ok {
		span.SetAttributes(attribute.String("process.command", command))
	}
}

func enrichFile(span trace.Span, config map[string]any) {
	if path, ok := configString(config, "path"); ok {
		span.SetAttributes(attribute.String("tailflow.file.path", path))
	}
}

func configString(config map[string]any, key string) (string, bool) {
	val, ok := config[key]
	if !ok {
		return "", false
	}

	str, ok := val.(string)

	return str, ok
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func dbSystemFromURL(url string) string {
	switch {
	case strings.HasPrefix(url, "postgres"):
		return "postgresql"
	case strings.HasPrefix(url, "mysql"):
		return "mysql"
	default:
		return "other"
	}
}

func sqlOperation(query string) string {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return ""
	}

	firstWord := strings.ToUpper(strings.Fields(trimmed)[0])

	return firstWord
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/otel/ -v -run TestEnrich`
Expected: PASS

**Step 5: Integrate into engine**

In `internal/engine/engine.go`, in the `executeNode` method, call `EnrichStepSpan` just before `finishStep`. The config and output passed to `EnrichStepSpan` must go through the sensitive masking:

```go
// Before finishStep calls in executeNode, add:
tfotel.EnrichStepSpan(trace.SpanFromContext(ctx), step.Action, e.sensitive.MaskMap(resolvedConfig), e.sensitive.MaskAny(output))
```

This requires extracting the span from context. Alternatively, modify `StartStep` to return the raw span, or pass the span through the finish function. The cleanest approach: `EnrichStepSpan` is called inside the `finishStep` closure by modifying `StartStep` to capture and expose the span.

**Step 6: Run all tests**

Run: `make lint && make test`
Expected: PASS

**Step 7: Commit**

```bash
make lint && make test
git add internal/otel/enrich.go internal/otel/enrich_test.go internal/engine/engine.go
git commit -m "feat(otel): enrich step spans with semantic conventions per action type"
```

---

## Task 8: CLI flags and env var configuration

**Files:**
- Modify: `cmd/tailflow/main.go:1046-1076` (rootCmd — add flags)
- Modify: `cmd/tailflow/main.go:1078-1106` (runCmd — init OTel)
- Modify: `cmd/tailflow/main.go:1121-1149` (serveCmd — init OTel)
- Modify: `cmd/tailflow/main.go:1174-1207` (executeRun — pass tracer)
- Modify: `cmd/tailflow/main.go:1674-1714` (executeServe — pass tracer)

**Step 1: Add OTel flags to rootCmd**

In `main()`, add two new persistent flags:

```go
var (
	noColorFlag    bool
	exporterURL    string
	exporterKey    string
	exporterName   string
	otelEndpoint   string
	otelServiceName string
)

// After existing PersistentFlags:
rootCmd.PersistentFlags().StringVar(&otelEndpoint, "otel-endpoint", "", "OTLP/HTTP endpoint for OpenTelemetry export (env: OTEL_EXPORTER_OTLP_ENDPOINT)")
rootCmd.PersistentFlags().StringVar(&otelServiceName, "otel-service-name", "", "Service name for OpenTelemetry (env: OTEL_SERVICE_NAME, default: tailflow)")
```

**Step 2: Create OTel config resolution helper**

```go
func resolveOTelConfig(flagEndpoint, flagServiceName string) tfotel.Config {
	endpoint := flagOrEnv(flagEndpoint, "OTEL_EXPORTER_OTLP_ENDPOINT")
	serviceName := flagOrEnv(flagServiceName, "OTEL_SERVICE_NAME")

	return tfotel.Config{
		Endpoint:    endpoint,
		ServiceName: serviceName,
	}
}
```

**Step 3: Integrate into executeRun**

At the start of `executeRun`, setup OTel and defer shutdown. Pass the tracer to NewExecutor:

```go
func executeRun(path string, rawParams []string, data string, noColor bool, exportURL, apiKey, exporterName string, otelCfg tfotel.Config) error {
	// Setup OTel (noop if no endpoint)
	otelResult, err := tfotel.Setup(context.Background(), otelCfg)
	if err != nil {
		return fmt.Errorf("otel setup: %w", err)
	}
	defer otelResult.Shutdown(context.Background()) //nolint:errcheck

	wf, err := parser.Parse(path)
	if err != nil {
		return err
	}

	// ... existing code ...

	// Modify the logger to include OTel log bridge if enabled
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))
	if otelCfg.Enabled() {
		otelHandler := tfotel.NewLogHandler(otelResult.LoggerProvider)
		if otelHandler != nil {
			// Multi-handler: logs go to both stderr and OTel
			logger = slog.New(newMultiHandler(logger.Handler(), otelHandler))
		}
	}

	// Pass tracer to NewExecutor
	tracer := tfotel.NewTracer(otelResult.TracerProvider)
	exec := engine.NewExecutor(reg, bus, logger, wf.Sensitive, tracer)

	// ... rest of existing code ...
}
```

**Step 4: Integrate into executeServe**

Same pattern — setup OTel, pass tracer, register metrics:

```go
func executeServe(path string, port int, maxExecs int, selfHosted bool, exportURL, apiKey, exporterName string, otelCfg tfotel.Config) error {
	// Setup OTel
	otelResult, err := tfotel.Setup(context.Background(), otelCfg)
	if err != nil {
		return fmt.Errorf("otel setup: %w", err)
	}
	defer otelResult.Shutdown(context.Background()) //nolint:errcheck

	wf, err := parser.Parse(path)
	// ... existing code ...

	// After metrics.New() in server, register OTel metrics
	// This requires passing the metrics collector to RegisterMetrics
	// Done in server.Run or here after server creation
}
```

**Step 5: Add multiHandler for dual logging**

Add a simple slog multi-handler (writes to both handlers) in `cmd/tailflow/main.go`:

```go
type multiHandler struct {
	handlers []slog.Handler
}

func newMultiHandler(handlers ...slog.Handler) *multiHandler {
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			_ = h.Handle(ctx, r.Clone())
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}
```

**Step 6: Wire OTel metrics in server**

In `executeServe`, after creating the server and before `srv.Run`, register OTel metrics with the server's metrics collector. This requires exposing the collector or passing the `MeterProvider`. Simplest approach: pass via server config.

Add to `server.Config`:
```go
OTelMeterProvider *sdkmetric.MeterProvider // nil = disabled
```

In `server.Run`, after `s.metrics.Start(ctx)`:
```go
if s.config.OTelMeterProvider != nil {
    tfotel.RegisterMetrics(s.config.OTelMeterProvider, s.metrics)
}
```

**Step 7: Update runCmd and serveCmd to pass otelCfg**

Thread `otelEndpoint` and `otelServiceName` through the command chain.

**Step 8: Run the full test suite**

Run: `go test ./... -count=1`
Expected: PASS

**Step 9: Commit**

```bash
make lint && make test
git add cmd/tailflow/main.go internal/server/server.go
git commit -m "feat(otel): add CLI flags and wire OTel into run/serve commands"
```

---

## Task 9: Update all existing NewExecutor call sites in tests

**Files:**
- Modify: All `*_test.go` files in `internal/engine/` that call `NewExecutor`

**Step 1: Find all call sites**

Search for `NewExecutor(` in test files. Each one needs to add the `tracer` parameter.

For all test call sites, pass `tfotel.NewTracer(nil)` and `nil` for businessMetrics — this gives noop behavior, zero overhead, no behavior change.

**Step 2: Update each call site**

Add import `tfotel "github.com/tailflow/tailflow/internal/otel"` and append `tfotel.NewTracer(nil), nil` to each `NewExecutor(` call (tracer + businessMetrics).

**Step 3: Run all tests**

Run: `go test ./... -count=1`
Expected: PASS

**Step 4: Commit**

```bash
make lint && make test
git add internal/engine/
git commit -m "test: update NewExecutor calls to include tracer parameter"
```

---

## Task 10: End-to-end verification

**Files:** None (verification only)

**Step 1: Run full test suite**

Run: `go test ./... -count=1 -race`
Expected: PASS, no race conditions

**Step 2: Build binary**

Run: `go build -o bin/tailflow ./cmd/tailflow`
Expected: builds successfully

**Step 3: Test with no OTel (zero overhead)**

Run: `bin/tailflow run examples/hello.yaml`
Expected: works exactly like before, no OTel output

**Step 4: Test with OTel endpoint (manual)**

Run with a local OTLP collector (e.g., `docker run -p 4318:4318 otel/opentelemetry-collector`):
```bash
bin/tailflow run examples/hello.yaml --otel-endpoint http://localhost:4318
```
Expected: workflow executes, traces exported to collector

**Step 5: Test with env var only**

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 bin/tailflow run examples/hello.yaml
```
Expected: same as above, OTel activated via env var

**Step 6: Final commit**

```bash
git add -A
git commit -m "feat: OpenTelemetry integration — traces, metrics, logs via OTLP/HTTP"
```

---

## Summary of changes

| File | Change |
|---|---|
| `go.mod` | Add OTel dependencies |
| `internal/otel/otel.go` | Provider setup (noop when disabled) |
| `internal/otel/tracing.go` | Workflow/step/retry span helpers |
| `internal/otel/metrics.go` | OTel system metrics from existing collector |
| `internal/otel/business_metrics.go` | Business metrics (counters, histograms) |
| `internal/otel/logging.go` | slog → OTel log bridge |
| `internal/otel/propagation.go` | W3C trace context injection |
| `internal/otel/enrich.go` | Semantic convention enrichment per action type |
| `internal/otel/*_test.go` | Tests for all OTel package code (testify/suite) |
| `internal/engine/engine.go` | Tracer + BusinessMetrics field, spans + metrics in Execute/executeNode/executeWithRetry |
| `internal/engine/sensitive.go` | Add MaskAny helper |
| `internal/action/http.go` | Inject trace context in outgoing requests |
| `internal/server/server.go` | Accept OTelMeterProvider in Config, register metrics |
| `cmd/tailflow/main.go` | Add `--otel-endpoint`/`--otel-service-name` flags, init+shutdown OTel, multi-handler logger |
| `internal/engine/*_test.go` | Update NewExecutor calls with tracer + businessMetrics params |

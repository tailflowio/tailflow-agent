package otel

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type businessFailingMeter struct {
	noop.Meter
	int64CounterCallCount    int
	float64HistCallCount     int
	upDownCounterCallCount   int
	failAtInt64Counter       int
	failAtFloat64Hist        int
	failAtUpDownCounter      int
}

func (m *businessFailingMeter) Int64Counter(
	_ string,
	_ ...metric.Int64CounterOption,
) (metric.Int64Counter, error) {
	m.int64CounterCallCount++

	if m.int64CounterCallCount == m.failAtInt64Counter {
		return nil, errStub
	}

	return noop.Int64Counter{}, nil
}

func (m *businessFailingMeter) Float64Histogram(
	_ string,
	_ ...metric.Float64HistogramOption,
) (metric.Float64Histogram, error) {
	m.float64HistCallCount++

	if m.float64HistCallCount == m.failAtFloat64Hist {
		return nil, errStub
	}

	return noop.Float64Histogram{}, nil
}

func (m *businessFailingMeter) Int64UpDownCounter(
	_ string,
	_ ...metric.Int64UpDownCounterOption,
) (metric.Int64UpDownCounter, error) {
	m.upDownCounterCallCount++

	if m.upDownCounterCallCount == m.failAtUpDownCounter {
		return nil, errStub
	}

	return noop.Int64UpDownCounter{}, nil
}

type BusinessMetricsTestSuite struct {
	suite.Suite
	ctx       context.Context
	reader    *sdkmetric.ManualReader
	mp        *sdkmetric.MeterProvider
	origMeter func(*sdkmetric.MeterProvider) metric.Meter
}

func TestBusinessMetrics(t *testing.T) {
	suite.Run(t, new(BusinessMetricsTestSuite))
}

func (s *BusinessMetricsTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.reader = sdkmetric.NewManualReader()
	s.mp = sdkmetric.NewMeterProvider(sdkmetric.WithReader(s.reader))
	s.origMeter = newMeter
}

func (s *BusinessMetricsTestSuite) TearDownTest() {
	newMeter = s.origMeter
	_ = s.mp.Shutdown(s.ctx)
}

func (s *BusinessMetricsTestSuite) TestNewBusinessMetrics_Success() {
	bm, err := NewBusinessMetrics(s.mp)

	s.Require().NoError(err)
	s.NotNil(bm)
	s.NotNil(bm.workflowExecutionsTotal)
	s.NotNil(bm.workflowDuration)
	s.NotNil(bm.activeExecutions)
	s.NotNil(bm.stepDuration)
	s.NotNil(bm.stepErrorsTotal)
	s.NotNil(bm.stepRetriesTotal)
}

func (s *BusinessMetricsTestSuite) TestNewBusinessMetrics_WhenProviderIsNil() {
	bm, err := NewBusinessMetrics(nil)

	s.NoError(err)
	s.Nil(bm)
}

func (s *BusinessMetricsTestSuite) TestRecordWorkflowExecution_IncrementsCounter() {
	bm, err := NewBusinessMetrics(s.mp)
	s.Require().NoError(err)

	bm.RecordWorkflowStarted(s.ctx, "my-workflow")
	bm.RecordWorkflowCompleted(s.ctx, "my-workflow", "success", 150.0)

	collected := s.collectMetrics()

	s.assertInt64Sum(collected, "tailflow.workflow.executions.total", 1)
	s.assertFloat64Histogram(
		collected, "tailflow.workflow.execution.duration", 150.0,
	)
}

func (s *BusinessMetricsTestSuite) TestRecordStepExecution_IncrementsCounterAndHistogram() {
	bm, err := NewBusinessMetrics(s.mp)
	s.Require().NoError(err)

	bm.RecordStepCompleted(s.ctx, "step-1", "http", "success", 42.0)

	collected := s.collectMetrics()

	s.assertFloat64Histogram(
		collected, "tailflow.step.execution.duration", 42.0,
	)
}

func (s *BusinessMetricsTestSuite) TestRecordStepError_IncrementsErrorCounter() {
	bm, err := NewBusinessMetrics(s.mp)
	s.Require().NoError(err)

	bm.RecordStepError(s.ctx, "step-1", "http", "timeout")

	collected := s.collectMetrics()

	s.assertInt64Sum(collected, "tailflow.step.errors.total", 1)
}

func (s *BusinessMetricsTestSuite) TestRecordStepRetry_IncrementsRetryCounter() {
	bm, err := NewBusinessMetrics(s.mp)
	s.Require().NoError(err)

	bm.RecordStepRetry(s.ctx, "step-1", "http")

	collected := s.collectMetrics()

	s.assertInt64Sum(collected, "tailflow.step.retries.total", 1)
}

func (s *BusinessMetricsTestSuite) TestActiveExecutions_UpDown() {
	bm, err := NewBusinessMetrics(s.mp)
	s.Require().NoError(err)

	bm.RecordWorkflowStarted(s.ctx, "wf-a")
	bm.RecordWorkflowStarted(s.ctx, "wf-b")
	bm.RecordWorkflowCompleted(s.ctx, "wf-a", "success", 100.0)

	collected := s.collectMetrics()

	s.assertInt64Sum(
		collected, "tailflow.workflow.active_executions", 1,
	)
}

func (s *BusinessMetricsTestSuite) TestRecordWorkflowStarted_WhenNilReceiver() {
	var bm *BusinessMetrics

	s.NotPanics(func() { bm.RecordWorkflowStarted(s.ctx, "wf") })
}

func (s *BusinessMetricsTestSuite) TestRecordWorkflowCompleted_WhenNilReceiver() {
	var bm *BusinessMetrics

	s.NotPanics(func() {
		bm.RecordWorkflowCompleted(s.ctx, "wf", "success", 1.0)
	})
}

func (s *BusinessMetricsTestSuite) TestRecordStepCompleted_WhenNilReceiver() {
	var bm *BusinessMetrics

	s.NotPanics(func() {
		bm.RecordStepCompleted(s.ctx, "s1", "http", "ok", 1.0)
	})
}

func (s *BusinessMetricsTestSuite) TestRecordStepError_WhenNilReceiver() {
	var bm *BusinessMetrics

	s.NotPanics(func() { bm.RecordStepError(s.ctx, "s1", "http", "err") })
}

func (s *BusinessMetricsTestSuite) TestRecordStepRetry_WhenNilReceiver() {
	var bm *BusinessMetrics

	s.NotPanics(func() { bm.RecordStepRetry(s.ctx, "s1", "http") })
}

func (s *BusinessMetricsTestSuite) TestNewBusinessMetrics_WhenWorkflowCounterFails() {
	s.assertCreationFailure(1, 0, 0, "failed to create workflow executions counter")
}

func (s *BusinessMetricsTestSuite) TestNewBusinessMetrics_WhenWorkflowDurationFails() {
	s.assertCreationFailure(0, 1, 0, "failed to create workflow duration histogram")
}

func (s *BusinessMetricsTestSuite) TestNewBusinessMetrics_WhenActiveExecutionsFails() {
	s.assertCreationFailure(0, 0, 1, "failed to create active executions counter")
}

func (s *BusinessMetricsTestSuite) TestNewBusinessMetrics_WhenStepDurationFails() {
	s.assertCreationFailure(0, 2, 0, "failed to create step duration histogram")
}

func (s *BusinessMetricsTestSuite) TestNewBusinessMetrics_WhenStepErrorsFails() {
	s.assertCreationFailure(2, 0, 0, "failed to create step errors counter")
}

func (s *BusinessMetricsTestSuite) TestNewBusinessMetrics_WhenStepRetriesFails() {
	s.assertCreationFailure(3, 0, 0, "failed to create step retries counter")
}

func (s *BusinessMetricsTestSuite) assertCreationFailure(
	failAtCounter, failAtHist, failAtUpDown int,
	expectedMsg string,
) {
	s.T().Helper()

	fm := &businessFailingMeter{
		failAtInt64Counter:  failAtCounter,
		failAtFloat64Hist:   failAtHist,
		failAtUpDownCounter: failAtUpDown,
	}

	newMeter = func(_ *sdkmetric.MeterProvider) metric.Meter {
		return fm
	}

	bm, err := NewBusinessMetrics(s.mp)

	s.Nil(bm)
	s.ErrorIs(err, errStub)
	s.ErrorContains(err, expectedMsg)
}

func (s *BusinessMetricsTestSuite) TestDurationMs() {
	start := time.Now().Add(-100 * time.Millisecond)

	d := DurationMs(start)

	s.GreaterOrEqual(d, float64(90))
}

func (s *BusinessMetricsTestSuite) collectMetrics() map[string]metricdata.Metrics {
	s.T().Helper()

	var rm metricdata.ResourceMetrics

	err := s.reader.Collect(s.ctx, &rm)
	s.Require().NoError(err)

	result := make(map[string]metricdata.Metrics)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			result[m.Name] = m
		}
	}

	return result
}

func (s *BusinessMetricsTestSuite) assertInt64Sum(
	collected map[string]metricdata.Metrics,
	name string,
	expected int64,
) {
	s.T().Helper()

	m, ok := collected[name]
	s.Require().True(ok, "metric %s not found", name)

	sum, ok := m.Data.(metricdata.Sum[int64])
	s.Require().True(ok, "metric %s is not Sum[int64]", name)
	s.Require().NotEmpty(sum.DataPoints)

	var total int64

	for _, dp := range sum.DataPoints {
		total += dp.Value
	}

	s.Equal(expected, total)
}

func (s *BusinessMetricsTestSuite) assertFloat64Histogram(
	collected map[string]metricdata.Metrics,
	name string,
	expected float64,
) {
	s.T().Helper()

	m, ok := collected[name]
	s.Require().True(ok, "metric %s not found", name)

	hist, ok := m.Data.(metricdata.Histogram[float64])
	s.Require().True(ok, "metric %s is not Histogram[float64]", name)
	s.Require().NotEmpty(hist.DataPoints)
	s.InDelta(expected, hist.DataPoints[0].Sum, 0.01)
}

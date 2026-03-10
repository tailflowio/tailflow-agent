package otel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tailflow/tailflow/internal/metrics"
)

type fakeSnapshotProvider struct {
	snap metrics.ProcessMetrics
}

func (f *fakeSnapshotProvider) Snapshot() metrics.ProcessMetrics {
	return f.snap
}

type failingMeter struct {
	noop.Meter
	float64CallCount int
	int64CallCount   int
	failAtFloat64    int
	failAtInt64      int
}

func (m *failingMeter) Float64ObservableGauge(
	_ string,
	_ ...metric.Float64ObservableGaugeOption,
) (metric.Float64ObservableGauge, error) {
	m.float64CallCount++

	if m.float64CallCount == m.failAtFloat64 {
		return nil, errStub
	}

	return noop.Float64ObservableGauge{}, nil
}

func (m *failingMeter) Int64ObservableGauge(
	_ string,
	_ ...metric.Int64ObservableGaugeOption,
) (metric.Int64ObservableGauge, error) {
	m.int64CallCount++

	if m.int64CallCount == m.failAtInt64 {
		return nil, errStub
	}

	return noop.Int64ObservableGauge{}, nil
}

type MetricsTestSuite struct {
	suite.Suite
	ctx       context.Context
	origMeter func(*sdkmetric.MeterProvider) metric.Meter
}

func TestMetrics(t *testing.T) {
	suite.Run(t, new(MetricsTestSuite))
}

func (s *MetricsTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.origMeter = newMeter
}

func (s *MetricsTestSuite) TearDownTest() {
	newMeter = s.origMeter
}

func (s *MetricsTestSuite) TestRegisterMetrics_Success() {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	defer func() { _ = mp.Shutdown(s.ctx) }()

	collector := &fakeSnapshotProvider{
		snap: metrics.ProcessMetrics{
			CPUPercent: 42.5,
			RSSKB:      1024,
			Goroutines: 10,
			HeapMB:     256.5,
			NetRxBytes: 5000,
			NetTxBytes: 3000,
			UptimeS:    600,
			Available:  true,
		},
	}

	err := RegisterMetrics(mp, collector)
	s.Require().NoError(err)

	var rm metricdata.ResourceMetrics

	err = reader.Collect(s.ctx, &rm)
	s.Require().NoError(err)
	s.Require().NotEmpty(rm.ScopeMetrics)

	gauges := make(map[string]any)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch data := m.Data.(type) {
			case metricdata.Gauge[float64]:
				s.Require().NotEmpty(data.DataPoints)
				gauges[m.Name] = data.DataPoints[0].Value
			case metricdata.Gauge[int64]:
				s.Require().NotEmpty(data.DataPoints)
				gauges[m.Name] = data.DataPoints[0].Value
			}
		}
	}

	s.InDelta(42.5, gauges["tailflow.cpu.usage"], 0.01)
	s.Equal(int64(1024), gauges["tailflow.memory.rss"])
	s.Equal(int64(10), gauges["tailflow.goroutines"])
	s.InDelta(256.5, gauges["tailflow.heap.alloc"], 0.01)
	s.Equal(int64(5000), gauges["tailflow.network.rx_bytes"])
	s.Equal(int64(3000), gauges["tailflow.network.tx_bytes"])
	s.Equal(int64(600), gauges["tailflow.uptime"])
}

func (s *MetricsTestSuite) TestRegisterMetrics_WhenProviderIsNil() {
	collector := &fakeSnapshotProvider{}

	err := RegisterMetrics(nil, collector)

	s.NoError(err)
}

func (s *MetricsTestSuite) TestRegisterMetrics_WhenCollectorIsNil() {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	defer func() { _ = mp.Shutdown(s.ctx) }()

	err := RegisterMetrics(mp, nil)

	s.NoError(err)
}

func (s *MetricsTestSuite) TestRegisterMetrics_WhenCPUGaugeFails() {
	s.assertFailure(1, 0, "failed to register cpu gauge")
}

func (s *MetricsTestSuite) TestRegisterMetrics_WhenMemoryGaugeFails() {
	s.assertFailure(0, 1, "failed to register memory gauge")
}

func (s *MetricsTestSuite) TestRegisterMetrics_WhenGoroutinesGaugeFails() {
	s.assertFailure(0, 2, "failed to register goroutines gauge")
}

func (s *MetricsTestSuite) TestRegisterMetrics_WhenHeapGaugeFails() {
	s.assertFailure(2, 0, "failed to register heap gauge")
}

func (s *MetricsTestSuite) TestRegisterMetrics_WhenRxBytesGaugeFails() {
	s.assertFailure(0, 3, "failed to register rx_bytes gauge")
}

func (s *MetricsTestSuite) TestRegisterMetrics_WhenTxBytesGaugeFails() {
	s.assertFailure(0, 4, "failed to register tx_bytes gauge")
}

func (s *MetricsTestSuite) TestRegisterMetrics_WhenUptimeGaugeFails() {
	s.assertFailure(0, 5, "failed to register uptime gauge")
}

func (s *MetricsTestSuite) assertFailure(
	failAtFloat64 int,
	failAtInt64 int,
	expectedMsg string,
) {
	s.T().Helper()

	fm := &failingMeter{
		failAtFloat64: failAtFloat64,
		failAtInt64:   failAtInt64,
	}

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	defer func() { _ = mp.Shutdown(s.ctx) }()

	newMeter = func(_ *sdkmetric.MeterProvider) metric.Meter {
		return fm
	}

	collector := &fakeSnapshotProvider{}

	err := RegisterMetrics(mp, collector)

	s.ErrorIs(err, errStub)
	s.ErrorContains(err, expectedMsg)
}

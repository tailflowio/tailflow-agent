package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/tailflow/tailflow/internal/metrics"
)

type SnapshotProvider interface {
	Snapshot() metrics.ProcessMetrics
}

var newMeter = func(mp *sdkmetric.MeterProvider) metric.Meter {
	return mp.Meter("tailflow")
}

func RegisterMetrics(
	mp *sdkmetric.MeterProvider,
	collector SnapshotProvider,
) error {
	if mp == nil || collector == nil {
		return nil
	}

	meter := newMeter(mp)

	err := registerFloat64Gauge(meter, "tailflow.cpu.usage",
		"CPU usage percentage",
		func(s metrics.ProcessMetrics) float64 { return s.CPUPercent },
		collector,
	)
	if err != nil {
		return fmt.Errorf("failed to register cpu gauge: %w", err)
	}

	err = registerInt64Gauge(meter, "tailflow.memory.rss",
		"Resident set size in KB",
		func(s metrics.ProcessMetrics) int64 { return s.RSSKB },
		collector,
	)
	if err != nil {
		return fmt.Errorf("failed to register memory gauge: %w", err)
	}

	err = registerInt64Gauge(meter, "tailflow.goroutines",
		"Number of goroutines",
		func(s metrics.ProcessMetrics) int64 { return int64(s.Goroutines) },
		collector,
	)
	if err != nil {
		return fmt.Errorf("failed to register goroutines gauge: %w", err)
	}

	err = registerFloat64Gauge(meter, "tailflow.heap.alloc",
		"Heap allocation in MB",
		func(s metrics.ProcessMetrics) float64 { return s.HeapMB },
		collector,
	)
	if err != nil {
		return fmt.Errorf("failed to register heap gauge: %w", err)
	}

	err = registerInt64Gauge(meter, "tailflow.network.rx_bytes",
		"Network bytes received",
		func(s metrics.ProcessMetrics) int64 { return s.NetRxBytes },
		collector,
	)
	if err != nil {
		return fmt.Errorf("failed to register rx_bytes gauge: %w", err)
	}

	err = registerInt64Gauge(meter, "tailflow.network.tx_bytes",
		"Network bytes transmitted",
		func(s metrics.ProcessMetrics) int64 { return s.NetTxBytes },
		collector,
	)
	if err != nil {
		return fmt.Errorf("failed to register tx_bytes gauge: %w", err)
	}

	err = registerInt64Gauge(meter, "tailflow.uptime",
		"Uptime in seconds",
		func(s metrics.ProcessMetrics) int64 { return s.UptimeS },
		collector,
	)
	if err != nil {
		return fmt.Errorf("failed to register uptime gauge: %w", err)
	}

	return nil
}

func registerFloat64Gauge(
	meter metric.Meter,
	name string,
	description string,
	extract func(metrics.ProcessMetrics) float64,
	collector SnapshotProvider,
) error {
	_, err := meter.Float64ObservableGauge(name,
		metric.WithDescription(description),
		metric.WithFloat64Callback(
			func(_ context.Context, o metric.Float64Observer) error {
				o.Observe(extract(collector.Snapshot()))
				return nil
			}),
	)

	return err
}

func registerInt64Gauge(
	meter metric.Meter,
	name string,
	description string,
	extract func(metrics.ProcessMetrics) int64,
	collector SnapshotProvider,
) error {
	_, err := meter.Int64ObservableGauge(name,
		metric.WithDescription(description),
		metric.WithInt64Callback(
			func(_ context.Context, o metric.Int64Observer) error {
				o.Observe(extract(collector.Snapshot()))
				return nil
			}),
	)

	return err
}

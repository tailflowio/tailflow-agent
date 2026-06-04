package fx

import (
	"fmt"

	tfotel "github.com/tailflow/tailflow/internal/otel"
	uberfx "go.uber.org/fx"
)

var tfotelNewBusinessMetrics = tfotel.NewBusinessMetrics

type BusinessMetricsIn struct {
	uberfx.In

	Result *tfotel.Result
}

type BusinessMetricsOut struct {
	uberfx.Out

	BusinessMetrics *tfotel.BusinessMetrics
}

func NewBusinessMetrics(in BusinessMetricsIn) (out BusinessMetricsOut, err error) {
	bm, bmErr := tfotelNewBusinessMetrics(in.Result.MeterProvider)
	if bmErr != nil {
		return out, fmt.Errorf("otel business metrics: %w", bmErr)
	}

	out.BusinessMetrics = bm

	return out, nil
}

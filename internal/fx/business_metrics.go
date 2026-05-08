package fx

import (
	"fmt"

	tfotel "github.com/tailflow/tailflow/internal/otel"
	uberfx "go.uber.org/fx"
)

type BusinessMetricsIn struct {
	uberfx.In

	Result *tfotel.Result
}

type BusinessMetricsOut struct {
	uberfx.Out

	BusinessMetrics *tfotel.BusinessMetrics
}

func NewBusinessMetrics(in BusinessMetricsIn) (out BusinessMetricsOut, err error) {
	bm, bmErr := tfotel.NewBusinessMetrics(in.Result.MeterProvider)
	if bmErr != nil {
		return out, fmt.Errorf("otel business metrics: %w", bmErr)
	}

	out.BusinessMetrics = bm

	return out, nil
}

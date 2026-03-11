package otel

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func InjectTraceContext(req *http.Request) {
	otel.GetTextMapPropagator().Inject(
		req.Context(), propagation.HeaderCarrier(req.Header),
	)
}

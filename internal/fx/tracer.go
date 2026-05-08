package fx

import (
	tfotel "github.com/tailflow/tailflow/internal/otel"
	uberfx "go.uber.org/fx"
)

type TracerIn struct {
	uberfx.In

	Result *tfotel.Result
}

type TracerOut struct {
	uberfx.Out

	Tracer *tfotel.Tracer
}

func NewTracer(in TracerIn) TracerOut {
	return TracerOut{Tracer: tfotel.NewTracer(in.Result.TracerProvider)}
}

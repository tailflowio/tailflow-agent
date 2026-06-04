package fx

import (
	"context"
	"fmt"
	"time"

	tfotel "github.com/tailflow/tailflow/internal/otel"
	uberfx "go.uber.org/fx"
)

var tfotelSetup = tfotel.Setup

type OTelIn struct {
	uberfx.In

	Config    Config
	Lifecycle uberfx.Lifecycle
}

type OTelOut struct {
	uberfx.Out

	Result *tfotel.Result
}

func NewOTel(in OTelIn) (out OTelOut, err error) {
	res, setupErr := tfotelSetup(context.Background(), in.Config.OTel)
	if setupErr != nil {
		return out, fmt.Errorf("otel setup: %w", setupErr)
	}

	in.Lifecycle.Append(uberfx.Hook{
		OnStop: func(ctx context.Context) error {
			flushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			_ = res.ForceFlush(flushCtx)

			return res.Shutdown(flushCtx)
		},
	})

	out.Result = res

	return out, nil
}

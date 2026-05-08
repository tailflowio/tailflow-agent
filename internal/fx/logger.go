package fx

import (
	"log/slog"

	tfotel "github.com/tailflow/tailflow/internal/otel"
	uberfx "go.uber.org/fx"
)

type LoggerIn struct {
	uberfx.In

	Config Config
	Result *tfotel.Result
}

type LoggerOut struct {
	uberfx.Out

	Logger *slog.Logger
}

func NewLogger(in LoggerIn) LoggerOut {
	return LoggerOut{Logger: tfotel.NewSlogLogger(in.Config.LogLevel, in.Result)}
}

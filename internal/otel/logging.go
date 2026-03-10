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

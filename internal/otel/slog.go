package otel

import (
	"context"
	"log/slog"
	"os"
)

// NewSlogLogger builds a slog.Logger that writes to stderr (text format) and,
// when an OTel LoggerProvider is configured on the Result, also forwards log
// records to the OTel pipeline. A nil Result yields a stderr-only logger.
func NewSlogLogger(level slog.Level, result *Result) *slog.Logger {
	baseHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})

	if result == nil {
		return slog.New(baseHandler)
	}

	otelHandler := NewLogHandler(result.LoggerProvider)
	if otelHandler == nil {
		return slog.New(baseHandler)
	}

	return slog.New(&multiHandler{handlers: []slog.Handler{baseHandler, otelHandler}})
}

type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}

	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error

	for _, h := range m.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}

		handleErr := h.Handle(ctx, r.Clone())
		if handleErr != nil && firstErr == nil {
			firstErr = handleErr
		}
	}

	return firstErr
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}

	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}

	return &multiHandler{handlers: handlers}
}

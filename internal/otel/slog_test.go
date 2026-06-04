package otel

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// failingHandler is a slog.Handler that always returns an error on Handle.
type failingHandler struct {
	err error
}

func (f *failingHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (f *failingHandler) Handle(_ context.Context, _ slog.Record) error {
	return f.err
}

func (f *failingHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return f
}

func (f *failingHandler) WithGroup(_ string) slog.Handler {
	return f
}

type SlogTestSuite struct {
	suite.Suite
	ctx context.Context
}

func TestSlog(t *testing.T) {
	suite.Run(t, new(SlogTestSuite))
}

func (s *SlogTestSuite) SetupTest() {
	s.ctx = context.Background()
}

func (s *SlogTestSuite) TestNewSlogLogger_WhenResultIsNil() {
	logger := NewSlogLogger(slog.LevelDebug, nil)

	s.NotNil(logger)
}

func (s *SlogTestSuite) TestNewSlogLogger_WhenLoggerProviderIsNil() {
	result := &Result{
		LoggerProvider: nil,
	}

	logger := NewSlogLogger(slog.LevelDebug, result)

	s.NotNil(logger)
}

func (s *SlogTestSuite) TestNewSlogLogger_WhenLoggerProviderIsConfigured() {
	lp := sdklog.NewLoggerProvider()
	result := &Result{
		LoggerProvider: lp,
	}

	logger := NewSlogLogger(slog.LevelDebug, result)

	s.NotNil(logger)

	logger.InfoContext(s.ctx, "test message", "key", "value")
}

func (s *SlogTestSuite) TestMultiHandler_Enabled_WhenOneHandlerIsEnabled() {
	handler := &multiHandler{
		handlers: []slog.Handler{
			slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}),
			slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}),
		},
	}

	s.True(handler.Enabled(s.ctx, slog.LevelDebug))
}

func (s *SlogTestSuite) TestMultiHandler_Enabled_WhenNoHandlerIsEnabled() {
	handler := &multiHandler{
		handlers: []slog.Handler{
			slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}),
			slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}),
		},
	}

	s.False(handler.Enabled(s.ctx, slog.LevelDebug))
}

func (s *SlogTestSuite) TestMultiHandler_Handle_WhenAllHandlersSucceed() {
	handler := &multiHandler{
		handlers: []slog.Handler{
			slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}),
			slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}),
		},
	}

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)

	err := handler.Handle(s.ctx, record)

	s.NoError(err)
}

func (s *SlogTestSuite) TestMultiHandler_Handle_WhenHandlerReturnsError() {
	stubErr := errors.New("handle error")
	handler := &multiHandler{
		handlers: []slog.Handler{
			&failingHandler{err: stubErr},
			slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}),
		},
	}

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)

	err := handler.Handle(s.ctx, record)

	s.ErrorIs(err, stubErr)
}

func (s *SlogTestSuite) TestMultiHandler_Handle_WhenHandlerIsDisabled() {
	handler := &multiHandler{
		handlers: []slog.Handler{
			slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}),
		},
	}

	record := slog.NewRecord(time.Now(), slog.LevelDebug, "test", 0)

	err := handler.Handle(s.ctx, record)

	s.NoError(err)
}

func (s *SlogTestSuite) TestMultiHandler_WithAttrs() {
	base := &multiHandler{
		handlers: []slog.Handler{
			slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}),
		},
	}

	attrs := []slog.Attr{slog.String("key", "value")}
	derived := base.WithAttrs(attrs)

	s.NotNil(derived)
	s.IsType(&multiHandler{}, derived)
}

func (s *SlogTestSuite) TestMultiHandler_WithGroup() {
	base := &multiHandler{
		handlers: []slog.Handler{
			slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}),
		},
	}

	derived := base.WithGroup("my-group")

	s.NotNil(derived)
	s.IsType(&multiHandler{}, derived)
}

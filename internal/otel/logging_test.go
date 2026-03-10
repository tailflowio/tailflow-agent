package otel

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/suite"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

type LoggingTestSuite struct {
	suite.Suite
}

func TestLogging(t *testing.T) {
	suite.Run(t, new(LoggingTestSuite))
}

func (s *LoggingTestSuite) TestNewLogHandler_WhenProviderIsNil() {
	handler := NewLogHandler(nil)
	s.Nil(handler)
}

func (s *LoggingTestSuite) TestNewLogHandler_WhenProviderIsConfigured() {
	provider := sdklog.NewLoggerProvider()
	handler := NewLogHandler(provider)

	s.NotNil(handler)

	logger := slog.New(handler)
	s.NotNil(logger)
}

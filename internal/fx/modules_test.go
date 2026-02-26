package fx

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/action"
)

type ModulesTestSuite struct {
	suite.Suite
}

func TestModules(t *testing.T) { suite.Run(t, new(ModulesTestSuite)) }

func (s *ModulesTestSuite) SetupTest() {
	// required by convention
}

func (s *ModulesTestSuite) TestProvideRegistry() {
	reg := provideRegistry()
	s.NotNil(reg)

	// Type assertion to confirm the concrete type.
	var _ *action.Registry = reg
}

func (s *ModulesTestSuite) TestProvideLogger() {
	logger := provideLogger()
	s.NotNil(logger)

	// Type assertion to confirm the concrete type.
	var _ *slog.Logger = logger
}

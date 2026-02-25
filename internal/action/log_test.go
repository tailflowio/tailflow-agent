package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type LogActionTestSuite struct {
	suite.Suite
}

func TestLogAction(t *testing.T) {
	suite.Run(t, new(LogActionTestSuite))
}

func (s *LogActionTestSuite) SetupTest() {}

func (s *LogActionTestSuite) TestExecute() {
	a := NewLogAction()
	ctx := newTestContext(map[string]any{
		"message": "Hello, World!",
		"level":   "info",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	outMap := out.(map[string]any)
	s.Equal("Hello, World!", outMap["message"])
	s.Equal("info", outMap["level"])
}

func (s *LogActionTestSuite) TestValidateMissingMessage() {
	a := NewLogAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
}

func (s *LogActionTestSuite) TestDefaultLevel() {
	a := NewLogAction()
	ctx := newTestContext(map[string]any{"message": "test"})
	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("info", out.(map[string]any)["level"])
}

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

func (s *LogActionTestSuite) TestExecute_InfoLevel() {
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

func (s *LogActionTestSuite) TestValidateOK() {
	a := NewLogAction()
	err := a.Validate(newTestContext(map[string]any{"message": "hello"}))
	s.NoError(err)
}

func (s *LogActionTestSuite) TestDebugLevel() {
	a := NewLogAction()
	ctx := newTestContext(map[string]any{"message": "debug msg", "level": "debug"})
	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("debug", out.(map[string]any)["level"])
}

func (s *LogActionTestSuite) TestWarnLevel() {
	a := NewLogAction()
	ctx := newTestContext(map[string]any{"message": "warn msg", "level": "warn"})
	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("warn", out.(map[string]any)["level"])
}

func (s *LogActionTestSuite) TestErrorLevel() {
	a := NewLogAction()
	ctx := newTestContext(map[string]any{"message": "error msg", "level": "error"})
	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("error", out.(map[string]any)["level"])
}

func (s *LogActionTestSuite) TestUnknownLevel() {
	a := NewLogAction()
	ctx := newTestContext(map[string]any{"message": "custom msg", "level": "custom"})
	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("custom", out.(map[string]any)["level"])
}

func (s *LogActionTestSuite) TestStreamEmitsLog() {
	a := NewLogAction()
	ctx := newTestContext(map[string]any{"message": "streaming", "stream": true})

	var emitted string
	ctx.EmitLog = func(msg string) { emitted = msg }

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("streaming", emitted)
	s.Equal("streaming", out.(map[string]any)["message"])
}

func (s *LogActionTestSuite) TestStreamFalseUsesLogger() {
	a := NewLogAction()
	ctx := newTestContext(map[string]any{"message": "not streaming", "stream": false})

	var emitted bool
	ctx.EmitLog = func(_ string) { emitted = true }

	_, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.False(emitted)
}

func (s *LogActionTestSuite) TestStreamNonBoolIgnored() {
	a := NewLogAction()
	ctx := newTestContext(map[string]any{"message": "not streaming", "stream": "yes"})

	var emitted bool
	ctx.EmitLog = func(_ string) { emitted = true }

	_, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.False(emitted)
}

package action

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type DelayActionTestSuite struct {
	suite.Suite
}

func TestDelayAction(t *testing.T) {
	suite.Run(t, new(DelayActionTestSuite))
}

func (s *DelayActionTestSuite) SetupTest() {}

func (s *DelayActionTestSuite) TestExecute() {
	a := NewDelayAction()
	ctx := newTestContext(map[string]any{
		"duration": "10ms",
	})

	start := time.Now()
	out, err := a.Execute(ctx)
	elapsed := time.Since(start)

	s.Require().NoError(err)
	s.GreaterOrEqual(elapsed, 10*time.Millisecond)
	s.Equal("10ms", out.(map[string]any)["waited"])
}

func (s *DelayActionTestSuite) TestCancelledContext() {
	a := NewDelayAction()
	cancelCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	ctx := newTestContext(map[string]any{
		"duration": "10s",
	})
	ctx.Context = cancelCtx

	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *DelayActionTestSuite) TestInvalidDuration() {
	a := NewDelayAction()
	ctx := newTestContext(map[string]any{
		"duration": "notaduration",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *DelayActionTestSuite) TestValidateMissingDuration() {
	a := NewDelayAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
}

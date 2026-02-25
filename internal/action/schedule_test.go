package action

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/runtime"
)

type ScheduleActionTestSuite struct {
	suite.Suite
}

func TestScheduleAction(t *testing.T) {
	suite.Run(t, new(ScheduleActionTestSuite))
}

func (s *ScheduleActionTestSuite) SetupTest() {}

func (s *ScheduleActionTestSuite) TestValidateDelay() {
	a := &ScheduleAction{}
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"delay": "30m"},
	}
	s.NoError(a.Validate(ctx))
}

func (s *ScheduleActionTestSuite) TestValidateAt() {
	a := &ScheduleAction{}
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"at": "2025-01-15T08:00:00Z"},
	}
	s.NoError(a.Validate(ctx))
}

func (s *ScheduleActionTestSuite) TestValidateBoth() {
	a := &ScheduleAction{}
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"delay": "30m", "at": "2025-01-15T08:00:00Z"},
	}
	s.ErrorContains(a.Validate(ctx), "cannot specify both")
}

func (s *ScheduleActionTestSuite) TestValidateNeither() {
	a := &ScheduleAction{}
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{},
	}
	s.ErrorContains(a.Validate(ctx), "must specify")
}

func (s *ScheduleActionTestSuite) TestValidateInvalidDelay() {
	a := &ScheduleAction{}
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"delay": "invalid"},
	}
	s.ErrorContains(a.Validate(ctx), "invalid delay")
}

func (s *ScheduleActionTestSuite) TestValidateInvalidAt() {
	a := &ScheduleAction{}
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"at": "not-a-date"},
	}
	s.ErrorContains(a.Validate(ctx), "invalid 'at'")
}

func (s *ScheduleActionTestSuite) TestExecuteNoServices() {
	a := &ScheduleAction{}
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"delay": "1s"},
	}
	_, err := a.Execute(ctx)
	s.ErrorContains(err, "server mode")
}

func (s *ScheduleActionTestSuite) TestExecuteWithDelay() {
	a := &ScheduleAction{}

	var scheduledDelay time.Duration
	var scheduledParams map[string]any

	ctx := &ActionContext{
		Context: context.Background(),
		Config: map[string]any{
			"delay":  "30m",
			"params": map[string]any{"key": "value"},
		},
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: &runtime.ActionServices{
			ScheduleExecution: func(delay time.Duration, params map[string]any) (string, error) {
				scheduledDelay = delay
				scheduledParams = params
				return "test-exec-id", nil
			},
		},
	}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	outMap := out.(map[string]any)
	s.Equal("test-exec-id", outMap["execution_id"])
	s.NotEmpty(outMap["scheduled_at"])
	s.Equal(30*time.Minute, scheduledDelay)
	s.Equal("value", scheduledParams["key"])
}

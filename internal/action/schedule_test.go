package action

import (
	"context"
	"fmt"
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

func (s *ScheduleActionTestSuite) TestComputeDelayAtFuture() {
	futureTime := time.Now().Add(2 * time.Hour).Format(time.RFC3339)
	d := computeDelay(map[string]any{"at": futureTime})
	// Should be roughly 2 hours (allow some slack for test execution)
	s.InDelta(2*time.Hour, d, float64(5*time.Second))
}

func (s *ScheduleActionTestSuite) TestComputeDelayAtPast() {
	pastTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	d := computeDelay(map[string]any{"at": pastTime})
	s.Equal(time.Duration(0), d)
}

func (s *ScheduleActionTestSuite) TestComputeDelayNoConfig() {
	d := computeDelay(map[string]any{})
	s.Equal(time.Duration(0), d)
}

func (s *ScheduleActionTestSuite) TestExtractParamsNone() {
	params := extractParams(map[string]any{})
	s.Nil(params)
}

func (s *ScheduleActionTestSuite) TestExtractParamsNonMap() {
	params := extractParams(map[string]any{"params": "not-a-map"})
	s.Nil(params)
}

func (s *ScheduleActionTestSuite) TestValidateNonStringDelay() {
	a := &ScheduleAction{}
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"delay": 123},
	}
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "must be a string")
}

func (s *ScheduleActionTestSuite) TestValidateNonStringAt() {
	a := &ScheduleAction{}
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"at": 123},
	}
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "must be an RFC3339")
}

func (s *ScheduleActionTestSuite) TestExecuteScheduleError() {
	a := NewScheduleAction()

	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"delay": "1m"},
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: &runtime.ActionServices{
			ScheduleExecution: func(delay time.Duration, params map[string]any) (string, error) {
				return "", fmt.Errorf("storage full")
			},
		},
	}

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "storage full")
}

func (s *ScheduleActionTestSuite) TestExecuteWithAt() {
	a := NewScheduleAction()

	futureTime := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	var scheduledDelay time.Duration

	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"at": futureTime},
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: &runtime.ActionServices{
			ScheduleExecution: func(delay time.Duration, params map[string]any) (string, error) {
				scheduledDelay = delay
				return "sched-id", nil
			},
		},
	}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal("sched-id", out.(map[string]any)["execution_id"])
	s.InDelta(1*time.Hour, scheduledDelay, float64(5*time.Second))
}

func (s *ScheduleActionTestSuite) TestExecuteNilScheduleExecution() {
	a := NewScheduleAction()

	ctx := &ActionContext{
		Context:  context.Background(),
		Config:   map[string]any{"delay": "1m"},
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: &runtime.ActionServices{},
	}

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "server mode")
}

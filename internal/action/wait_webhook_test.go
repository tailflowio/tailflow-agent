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

type WaitWebhookActionTestSuite struct {
	suite.Suite
}

func TestWaitWebhookAction(t *testing.T) {
	suite.Run(t, new(WaitWebhookActionTestSuite))
}

func (s *WaitWebhookActionTestSuite) SetupTest() {}

func (s *WaitWebhookActionTestSuite) TestValidateMissingPath() {
	act := NewWaitWebhookAction()
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{},
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}
	err := act.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "path")
}

func (s *WaitWebhookActionTestSuite) TestValidateNoServices() {
	act := NewWaitWebhookAction()
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"path": "/callback"},
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}
	err := act.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *WaitWebhookActionTestSuite) TestExecuteReceives() {
	act := NewWaitWebhookAction()

	ch := make(chan runtime.WaitRequest, 1)
	services := &runtime.ActionServices{
		WaitWebhookRegister: func(executionID, stepID, path string, ctx context.Context) (<-chan runtime.WaitRequest, func()) {
			return ch, func() {}
		},
	}

	execCtx := runtime.NewExecutionContext("exec-1", "test-wf", nil, nil)
	execCtx.Services = services

	actCtx := &ActionContext{
		Context:  context.Background(),
		Config:   map[string]any{"path": "/callback", "timeout": "2s"},
		ExecCtx:  execCtx,
		StepID:   "wait-step",
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: services,
	}

	// Send a request into the buffered channel before Execute blocks
	ch <- runtime.WaitRequest{
		Method: "POST",
		Path:   "/callback",
		Body:   map[string]any{"data": "test"},
	}

	output, err := act.Execute(actCtx)
	s.NoError(err)
	s.NotNil(output)

	result := output.(map[string]any)
	s.Equal("POST", result["method"])
	s.Equal("/callback", result["path"])
}

func (s *WaitWebhookActionTestSuite) TestExecuteTimeout() {
	act := NewWaitWebhookAction()

	services := &runtime.ActionServices{
		WaitWebhookRegister: func(executionID, stepID, path string, ctx context.Context) (<-chan runtime.WaitRequest, func()) {
			return make(chan runtime.WaitRequest), func() {}
		},
	}

	execCtx := runtime.NewExecutionContext("exec-1", "test-wf", nil, nil)
	execCtx.Services = services

	actCtx := &ActionContext{
		Context:  context.Background(),
		Config:   map[string]any{"path": "/callback", "timeout": "100ms"},
		ExecCtx:  execCtx,
		StepID:   "wait-step",
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: services,
	}

	_, err := act.Execute(actCtx)
	s.Error(err)
	s.Contains(err.Error(), "timeout")
}

func (s *WaitWebhookActionTestSuite) TestValidateOK() {
	act := NewWaitWebhookAction()
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"path": "/callback"},
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: &runtime.ActionServices{
			WaitWebhookRegister: func(executionID, stepID, path string, ctx context.Context) (<-chan runtime.WaitRequest, func()) {
				return make(chan runtime.WaitRequest), func() {}
			},
		},
	}
	err := act.Validate(ctx)
	s.NoError(err)
}

func (s *WaitWebhookActionTestSuite) TestValidateNilRegister() {
	act := NewWaitWebhookAction()
	ctx := &ActionContext{
		Context:  context.Background(),
		Config:   map[string]any{"path": "/callback"},
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: &runtime.ActionServices{},
	}
	err := act.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *WaitWebhookActionTestSuite) TestExecuteInvalidTimeout() {
	act := NewWaitWebhookAction()

	services := &runtime.ActionServices{
		WaitWebhookRegister: func(executionID, stepID, path string, ctx context.Context) (<-chan runtime.WaitRequest, func()) {
			return make(chan runtime.WaitRequest), func() {}
		},
	}

	execCtx := runtime.NewExecutionContext("exec-1", "test-wf", nil, nil)
	execCtx.Services = services

	actCtx := &ActionContext{
		Context:  context.Background(),
		Config:   map[string]any{"path": "/callback", "timeout": "not-a-duration"},
		ExecCtx:  execCtx,
		StepID:   "wait-step",
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: services,
	}

	_, err := act.Execute(actCtx)
	s.Error(err)
	s.Contains(err.Error(), "invalid timeout")
}

func (s *WaitWebhookActionTestSuite) TestExecuteContextCancelled() {
	act := NewWaitWebhookAction()

	services := &runtime.ActionServices{
		WaitWebhookRegister: func(executionID, stepID, path string, ctx context.Context) (<-chan runtime.WaitRequest, func()) {
			return make(chan runtime.WaitRequest), func() {}
		},
	}

	execCtx := runtime.NewExecutionContext("exec-1", "test-wf", nil, nil)
	execCtx.Services = services

	bgCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	actCtx := &ActionContext{
		Context:  bgCtx,
		Config:   map[string]any{"path": "/callback", "timeout": "10s"},
		ExecCtx:  execCtx,
		StepID:   "wait-step",
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: services,
	}

	_, err := act.Execute(actCtx)
	s.Error(err)
}

func (s *WaitWebhookActionTestSuite) TestExecuteWithEmitWaiting() {
	act := NewWaitWebhookAction()

	ch := make(chan runtime.WaitRequest, 1)
	var waitingCalled bool

	services := &runtime.ActionServices{
		WaitWebhookRegister: func(executionID, stepID, path string, ctx context.Context) (<-chan runtime.WaitRequest, func()) {
			return ch, func() {}
		},
		EmitWaiting: func(execID, stepID, waitType string, details map[string]any) {
			waitingCalled = true
		},
	}

	execCtx := runtime.NewExecutionContext("exec-1", "test-wf", nil, nil)
	execCtx.Services = services

	actCtx := &ActionContext{
		Context:  context.Background(),
		Config:   map[string]any{"path": "/callback", "timeout": "2s"},
		ExecCtx:  execCtx,
		StepID:   "wait-step",
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: services,
	}

	ch <- runtime.WaitRequest{
		Method: "GET",
		Path:   "/callback",
	}

	_, err := act.Execute(actCtx)
	s.NoError(err)
	s.True(waitingCalled)
}

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

type WaitRabbitMQActionTestSuite struct {
	suite.Suite
}

func TestWaitRabbitMQAction(t *testing.T) {
	suite.Run(t, new(WaitRabbitMQActionTestSuite))
}

func (s *WaitRabbitMQActionTestSuite) SetupTest() {}

func (s *WaitRabbitMQActionTestSuite) TestValidateMissingURL() {
	act := NewWaitRabbitMQAction()
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"queue": "q"},
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}
	err := act.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "url")
}

func (s *WaitRabbitMQActionTestSuite) TestValidateMissingQueue() {
	act := NewWaitRabbitMQAction()
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"url": "amqp://localhost"},
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}
	err := act.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "queue")
}

func (s *WaitRabbitMQActionTestSuite) TestValidateNoServices() {
	act := NewWaitRabbitMQAction()
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"url": "amqp://localhost", "queue": "q"},
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}
	err := act.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *WaitRabbitMQActionTestSuite) TestValidateNilRegister() {
	act := NewWaitRabbitMQAction()
	ctx := &ActionContext{
		Context:  context.Background(),
		Config:   map[string]any{"url": "amqp://localhost", "queue": "q"},
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: &runtime.ActionServices{},
	}
	err := act.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *WaitRabbitMQActionTestSuite) TestValidateOK() {
	act := NewWaitRabbitMQAction()
	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{"url": "amqp://localhost", "queue": "q"},
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: &runtime.ActionServices{
			WaitRabbitMQRegister: func(url, queue, matchField, matchValue string, ctx context.Context) (<-chan map[string]any, func()) {
				return make(chan map[string]any), func() {}
			},
		},
	}
	err := act.Validate(ctx)
	s.NoError(err)
}

func (s *WaitRabbitMQActionTestSuite) TestExecuteReceives() {
	act := NewWaitRabbitMQAction()

	ch := make(chan map[string]any, 1)
	services := &runtime.ActionServices{
		WaitRabbitMQRegister: func(url, queue, matchField, matchValue string, ctx context.Context) (<-chan map[string]any, func()) {
			return ch, func() {}
		},
	}

	execCtx := runtime.NewExecutionContext("exec-1", "test-wf", nil, nil)
	execCtx.Services = services

	actCtx := &ActionContext{
		Context:  context.Background(),
		Config:   map[string]any{"url": "amqp://localhost", "queue": "q", "timeout": "2s"},
		ExecCtx:  execCtx,
		StepID:   "wait-rmq",
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: services,
	}

	// Send a message into the buffered channel before Execute blocks
	ch <- map[string]any{
		"body":         map[string]any{"order_id": "123"},
		"content_type": "application/json",
		"routing_key":  "",
		"message_id":   "msg-1",
		"headers":      map[string]any{},
	}

	output, err := act.Execute(actCtx)
	s.NoError(err)
	s.NotNil(output)

	result := output.(map[string]any)
	s.Equal("application/json", result["content_type"])
}

func (s *WaitRabbitMQActionTestSuite) TestExecuteTimeout() {
	act := NewWaitRabbitMQAction()

	services := &runtime.ActionServices{
		WaitRabbitMQRegister: func(url, queue, matchField, matchValue string, ctx context.Context) (<-chan map[string]any, func()) {
			return make(chan map[string]any), func() {}
		},
	}

	execCtx := runtime.NewExecutionContext("exec-1", "test-wf", nil, nil)
	execCtx.Services = services

	actCtx := &ActionContext{
		Context:  context.Background(),
		Config:   map[string]any{"url": "amqp://localhost", "queue": "q", "timeout": "100ms"},
		ExecCtx:  execCtx,
		StepID:   "wait-rmq",
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: services,
	}

	_, err := act.Execute(actCtx)
	s.Error(err)
	s.Contains(err.Error(), "timeout")
}

func (s *WaitRabbitMQActionTestSuite) TestExecuteChannelClosed() {
	act := NewWaitRabbitMQAction()

	ch := make(chan map[string]any, 1)
	close(ch) // simulate connection failure

	services := &runtime.ActionServices{
		WaitRabbitMQRegister: func(url, queue, matchField, matchValue string, ctx context.Context) (<-chan map[string]any, func()) {
			return ch, func() {}
		},
	}

	execCtx := runtime.NewExecutionContext("exec-1", "test-wf", nil, nil)
	execCtx.Services = services

	actCtx := &ActionContext{
		Context:  context.Background(),
		Config:   map[string]any{"url": "amqp://localhost", "queue": "q", "timeout": "2s"},
		ExecCtx:  execCtx,
		StepID:   "wait-rmq",
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: services,
	}

	_, err := act.Execute(actCtx)
	s.Error(err)
	s.Contains(err.Error(), "connection failed")
}

func (s *WaitRabbitMQActionTestSuite) TestExecuteContextCancelled() {
	act := NewWaitRabbitMQAction()

	services := &runtime.ActionServices{
		WaitRabbitMQRegister: func(url, queue, matchField, matchValue string, ctx context.Context) (<-chan map[string]any, func()) {
			return make(chan map[string]any), func() {}
		},
	}

	execCtx := runtime.NewExecutionContext("exec-1", "test-wf", nil, nil)
	execCtx.Services = services

	bgCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	actCtx := &ActionContext{
		Context:  bgCtx,
		Config:   map[string]any{"url": "amqp://localhost", "queue": "q", "timeout": "10s"},
		ExecCtx:  execCtx,
		StepID:   "wait-rmq",
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Services: services,
	}

	_, err := act.Execute(actCtx)
	s.Error(err)
}

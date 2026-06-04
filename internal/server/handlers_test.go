package server

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/store"
)

type HandlersTestSuite struct {
	suite.Suite
}

func TestHandlers(t *testing.T) {
	suite.Run(t, new(HandlersTestSuite))
}

func (s *HandlersTestSuite) SetupTest() {} // required by convention

func newTestServer(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "test-workflow"
description: "A test workflow"
tags: ["test"]
params:
  - name: env
    type: string
    default: "staging"
stages:
  - name: default
steps:
  - id: greet
    action: log
    stage: default
    title: "Log greeting"
    config:
      message: "Hello from test"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	bus := event.NewBus()
	t.Cleanup(bus.Close)

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := engine.NewExecutor(reg, bus, logger, nil, nil, nil)

	return New(Config{
		Port:           0,
		Executor:       exec,
		Workflow:       wf,
		ExecutionStore: store.NewExecutionStore(10),
		EventBus:       bus,
		Logger:         logger,
	})
}

func newTestServerCron(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "cron-workflow"
description: "A cron workflow"
trigger:
  schedule:
    cron: "*/5 * * * *"
stages:
  - name: default
steps:
  - id: greet
    action: log
    stage: default
    title: "Log greeting"
    config:
      message: "Hello from cron"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	bus := event.NewBus()
	t.Cleanup(bus.Close)

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := engine.NewExecutor(reg, bus, logger, nil, nil, nil)

	return New(Config{
		Port:           0,
		Executor:       exec,
		Workflow:       wf,
		ExecutionStore: store.NewExecutionStore(10),
		EventBus:       bus,
		Logger:         logger,
	})
}

func newTestServerHTTPTrigger(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "http-workflow"
description: "HTTP triggered workflow"
trigger:
  http:
    method: POST
    path: /submit
stages:
  - name: default
steps:
  - id: echo
    action: log
    stage: default
    title: "Echo"
    config:
      message: "trigger received"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	bus := event.NewBus()
	t.Cleanup(bus.Close)

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := engine.NewExecutor(reg, bus, logger, nil, nil, nil)

	return New(Config{
		Port:           0,
		Executor:       exec,
		Workflow:       wf,
		ExecutionStore: store.NewExecutionStore(10),
		EventBus:       bus,
		Logger:         logger,
	})
}

func newTestServerAsyncHTTPTrigger(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "async-http-workflow"
description: "Async HTTP triggered workflow"
trigger:
  http:
    method: POST
    path: /async-submit
    async: true
stages:
  - name: default
steps:
  - id: echo
    action: log
    stage: default
    title: "Echo"
    config:
      message: "async trigger received"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	bus := event.NewBus()
	t.Cleanup(bus.Close)

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := engine.NewExecutor(reg, bus, logger, nil, nil, nil)

	return New(Config{
		Port:           0,
		Executor:       exec,
		Workflow:       wf,
		ExecutionStore: store.NewExecutionStore(10),
		EventBus:       bus,
		Logger:         logger,
	})
}

func newTestServerWebhookTrigger(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "webhook-workflow"
description: "Webhook triggered workflow"
trigger:
  webhook:
    path: /hook
stages:
  - name: default
steps:
  - id: echo
    action: log
    stage: default
    title: "Echo"
    config:
      message: "webhook received"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	bus := event.NewBus()
	t.Cleanup(bus.Close)

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := engine.NewExecutor(reg, bus, logger, nil, nil, nil)

	return New(Config{
		Port:           0,
		Executor:       exec,
		Workflow:       wf,
		ExecutionStore: store.NewExecutionStore(10),
		EventBus:       bus,
		Logger:         logger,
	})
}

func newTestServerGraph(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "graph-workflow"
description: "Graph test"
stages:
  - name: default
steps:
  - id: step_a
    action: log
    stage: default
    config:
      message: "a"
  - id: step_b
    action: log
    stage: default
    title: "Step B"
    depends_on: [step_a]
    when: 'steps.step_a.status == "success"'
    config:
      message: "b"
    goto:
      target: step_a
      when: 'steps.step_b.output.retry == true'
      max_iterations: 3
  - id: step_loop
    action: loop
    stage: default
    title: "Loop Step"
    depends_on: [step_a]
    config:
      items: [1, 2, 3]
      actions:
        - action: log
          title: "Log in loop"
        - action: set
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	bus := event.NewBus()
	t.Cleanup(bus.Close)

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := engine.NewExecutor(reg, bus, logger, nil, nil, nil)

	return New(Config{
		Port:           0,
		Executor:       exec,
		Workflow:       wf,
		ExecutionStore: store.NewExecutionStore(10),
		EventBus:       bus,
		Logger:         logger,
	})
}

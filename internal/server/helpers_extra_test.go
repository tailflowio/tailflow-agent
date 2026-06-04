package server

import (
	"log/slog"
	"os"
	"testing"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/store"
)

// newTestServerWithErrStore builds a Server that uses an errExecutionStore so
// tests can inject store errors without touching production code.
func newTestServerWithErrStore(t *testing.T, es *errExecutionStore) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "test-workflow"
stages:
  - name: default
steps:
  - id: greet
    action: log
    stage: default
    config:
      message: "hello"
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
		ExecutionStore: es,
		EventBus:       bus,
		Logger:         logger,
	})
}

// newTestServerConvergent builds a workflow with a diamond dependency graph.
func newTestServerConvergent(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "convergent-workflow"
stages:
  - name: default
steps:
  - id: step_start
    action: log
    stage: default
    config:
      message: "start"
  - id: step_a
    action: log
    stage: default
    depends_on: [step_start]
    config:
      message: "a"
  - id: step_b
    action: log
    stage: default
    depends_on: [step_start]
    config:
      message: "b"
  - id: step_end
    action: log
    stage: default
    depends_on: [step_a, step_b]
    config:
      message: "end"
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

// newTestServerDeepTree builds a workflow with 3 levels of nesting.
func newTestServerDeepTree(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "deep-tree-workflow"
stages:
  - name: default
steps:
  - id: root
    action: log
    stage: default
    config:
      message: "root"
  - id: child_a
    action: log
    stage: default
    depends_on: [root]
    config:
      message: "child a"
  - id: child_b
    action: log
    stage: default
    depends_on: [root]
    config:
      message: "child b"
  - id: grandchild
    action: log
    stage: default
    depends_on: [child_a]
    config:
      message: "grandchild"
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

// newTestServerWithRawFile builds a server that has a FilePath set.
func newTestServerWithRawFile(t *testing.T, filePath string) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "raw-workflow"
stages:
  - name: default
steps:
  - id: greet
    action: log
    stage: default
    config:
      message: "hello"
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
		FilePath:       filePath,
	})
}

// newTestServerWithRecoverer builds a server with a custom Recoverer.
func newTestServerWithRecoverer(t *testing.T, recoverer export.ExecutionRecoverer) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "recover-workflow"
recovery: true
stages:
  - name: default
steps:
  - id: greet
    action: log
    stage: default
    config:
      message: "hello"
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
		Recoverer:      recoverer,
	})
}

// newTestServerWithCustomStore creates a server with the given store.
func newTestServerWithCustomStore(t *testing.T, st *store.MemoryExecutionStore) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "test-workflow"
stages:
  - name: default
steps:
  - id: greet
    action: log
    stage: default
    config:
      message: "hello"
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
		ExecutionStore: st,
		EventBus:       bus,
		Logger:         logger,
	})
}

package fx

import (
	"log/slog"
	"os"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
	uberfx "go.uber.org/fx"
)

// CoreModule provides parser, DAG builder, expression evaluator, and executor.
var CoreModule = uberfx.Module("core",
	uberfx.Provide(
		runtime.NewExprEvaluator,
		engine.NewExecutor,
		provideLogger,
	),
)

// ActionModule provides the action registry with built-in actions.
var ActionModule = uberfx.Module("actions",
	uberfx.Provide(provideRegistry),
)

// EventModule provides the event bus.
var EventModule = uberfx.Module("event",
	uberfx.Provide(event.NewBus),
)

func provideRegistry() *action.Registry {
	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	return reg
}

func provideLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

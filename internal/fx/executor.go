package fx

import (
	"log/slog"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	tfotel "github.com/tailflow/tailflow/internal/otel"
	"github.com/tailflow/tailflow/internal/parser"
	uberfx "go.uber.org/fx"
)

type ExecutorIn struct {
	uberfx.In

	BusinessMetrics *tfotel.BusinessMetrics
	EventBus        *event.Bus
	Logger          *slog.Logger
	Registry        *action.Registry
	Tracer          *tfotel.Tracer
	Workflow        *parser.Workflow
}

type ExecutorOut struct {
	uberfx.Out

	Executor *engine.Executor
}

func NewExecutor(in ExecutorIn) ExecutorOut {
	exec := engine.NewExecutor(
		in.Registry, in.EventBus, in.Logger,
		in.Workflow.Sensitive, in.Tracer, in.BusinessMetrics,
	)

	return ExecutorOut{Executor: exec}
}

package fx

import (
	"log/slog"

	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/server"
	"github.com/tailflow/tailflow/internal/store"
	uberfx "go.uber.org/fx"
)

type ServerIn struct {
	uberfx.In

	Claimer        export.IdempotencyClaimer
	Config         Config
	EventBus       *event.Bus
	Executor       *engine.Executor
	ExecutionStore store.ExecutionStore
	Exporter       export.EventExporter
	Logger         *slog.Logger
	Recoverer      export.ExecutionRecoverer
	Workflow       *parser.Workflow
}

type ServerOut struct {
	uberfx.Out

	Server *server.Server
}

func NewServer(in ServerIn) ServerOut {
	srv := server.New(server.Config{
		Port:           in.Config.Port,
		Executor:       in.Executor,
		Workflow:       in.Workflow,
		FilePath:       in.Config.WorkflowPath,
		EditorEnabled:  in.Config.Editor,
		ExecutionStore: in.ExecutionStore,
		EventBus:       in.EventBus,
		Logger:         in.Logger,
		Version:        in.Config.Version,
		Exporter:       in.Exporter,
		Claimer:        in.Claimer,
		Recoverer:      in.Recoverer,
	})

	return ServerOut{Server: srv}
}

package fx

import (
	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/store"
	"github.com/tailflow/tailflow/internal/store/mariadb"
	uberfx "go.uber.org/fx"
)

// ExportPortsIn / ExportPortsOut centralise the three export.* ports the
// server depends on. The agent runs self-hosted mono-instance: the Claimer and
// the Recoverer are store-backed (memory map or MariaDB), while the Exporter
// stays noop in v1. The Recoverer is only armed when workflow recovery is
// enabled; otherwise it stays noop. The server owns the exporter
// Start/Shutdown lifecycle, so no fx hook is registered here.
type ExportPortsIn struct {
	uberfx.In

	Workflow *parser.Workflow
	Store    store.ExecutionStore
}

type ExportPortsOut struct {
	uberfx.Out

	Claimer   export.IdempotencyClaimer
	Exporter  export.EventExporter
	Recoverer export.ExecutionRecoverer
}

// NewExportPorts selects the idempotency claimer matching the workflow's
// persistence backend. The MariaDB claimer INSERTs the execution row itself,
// so prepareTriggerExecution's Add tolerates the resulting duplicate-key on
// the same id (see mariadb.Store.Add).
func NewExportPorts(in ExportPortsIn) ExportPortsOut {
	return ExportPortsOut{
		Claimer:   selectClaimer(in),
		Exporter:  export.NewNoopExporter(),
		Recoverer: selectRecoverer(in),
	}
}

func selectRecoverer(in ExportPortsIn) export.ExecutionRecoverer {
	if !in.Workflow.Recovery {
		return export.NewNoopRecoverer()
	}

	switch s := in.Store.(type) {
	case *mariadb.Store:
		return s

	case *store.MemoryExecutionStore:
		return store.NewMemoryRecoverer(s)

	default:
		return export.NewNoopRecoverer()
	}
}

func selectClaimer(in ExportPortsIn) export.IdempotencyClaimer {
	cfg := in.Workflow.Persistence

	switch {
	case cfg == nil, cfg.Type == "", cfg.Type == parser.PersistenceMemory:
		return export.NewMemoryClaimer()

	case cfg.Type == parser.PersistenceMariaDB:
		mariaStore, ok := in.Store.(*mariadb.Store)
		if !ok {
			return export.NewMemoryClaimer()
		}

		return mariaStore

	default:
		return export.NewNoopClaimer()
	}
}

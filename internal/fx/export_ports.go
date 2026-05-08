package fx

import (
	"github.com/tailflow/tailflow/internal/export"
	uberfx "go.uber.org/fx"
)

// ExportPortsIn / ExportPortsOut centralise the three export.* ports the
// server depends on. The agent runs self-hosted only — every port is wired
// to its noop implementation. The server takes ownership of the
// Start/Shutdown lifecycle on the exporter, so no fx hook is registered
// here.
type ExportPortsIn struct {
	uberfx.In
}

type ExportPortsOut struct {
	uberfx.Out

	Claimer   export.IdempotencyClaimer
	Exporter  export.EventExporter
	Recoverer export.ExecutionRecoverer
}

func NewExportPorts(_ ExportPortsIn) ExportPortsOut {
	return ExportPortsOut{
		Claimer:   export.NewNoopClaimer(),
		Exporter:  export.NewNoopExporter(),
		Recoverer: export.NewNoopRecoverer(),
	}
}

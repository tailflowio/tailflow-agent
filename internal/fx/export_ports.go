package fx

import (
	"log/slog"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/export/saas"
	"github.com/tailflow/tailflow/internal/parser"
	uberfx "go.uber.org/fx"
)

// ExportPortsIn / ExportPortsOut centralise the SaaS-vs-noop branching for the
// three export.* ports. The server takes ownership of the Start/Shutdown
// lifecycle on the exporter, so no fx hook is registered here.
type ExportPortsIn struct {
	uberfx.In

	Config   Config
	EventBus *event.Bus
	Logger   *slog.Logger
	Workflow *parser.Workflow
}

type ExportPortsOut struct {
	uberfx.Out

	Claimer   export.IdempotencyClaimer
	Exporter  export.EventExporter
	Recoverer export.ExecutionRecoverer
}

func NewExportPorts(in ExportPortsIn) ExportPortsOut {
	if in.Config.ExportURL == "" {
		return ExportPortsOut{
			Claimer:   export.NewNoopClaimer(),
			Exporter:  export.NewNoopExporter(),
			Recoverer: export.NewNoopRecoverer(),
		}
	}

	exporter := saas.NewExporter(saas.Config{
		ExportURL:           in.Config.ExportURL,
		APIKey:              in.Config.APIKey,
		AgentName:           in.Config.ExporterName,
		EventBus:            in.EventBus,
		Logger:              in.Logger,
		WorkflowName:        in.Workflow.Name,
		WorkflowDescription: in.Workflow.Description,
		WorkflowTags:        in.Workflow.Tags,
		TriggerType:         resolveTriggerType(in.Workflow),
		StepsCount:          len(in.Workflow.Steps),
		Version:             in.Config.Version,
		Revision:            in.Workflow.Revision,
	})

	return ExportPortsOut{
		Claimer:   saas.NewClaimClient(in.Config.ExportURL, in.Config.APIKey),
		Exporter:  exporter,
		Recoverer: saas.NewRecoveryClient(in.Config.ExportURL, in.Config.APIKey),
	}
}

// resolveTriggerType mirrors the helper used elsewhere in the agent: it
// derives the trigger kind tag attached to SaaS exporter handshakes from the
// workflow's trigger config. Returns "" when no trigger is configured.
func resolveTriggerType(wf *parser.Workflow) string {
	t := wf.Trigger
	if t == nil {
		return ""
	}

	switch {
	case t.HTTP != nil:
		return "http"
	case t.Webhook != nil:
		return "webhook"
	case t.Schedule != nil:
		return "schedule"
	case t.RabbitMQ != nil:
		return "rabbitmq"
	}

	return ""
}

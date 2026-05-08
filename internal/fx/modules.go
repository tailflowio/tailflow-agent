// Package fx wires the agent's dependencies via uber-fx. The ServeModule and
// RunApp helpers replace the manual assembly that previously lived in
// cmd/tailflow/main.executeServe — providers register OnStop lifecycle hooks
// for OTel, the event bus, and other resources that need clean teardown.
package fx

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/export/saas"
	tfotel "github.com/tailflow/tailflow/internal/otel"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/server"
	"github.com/tailflow/tailflow/internal/store"
	uberfx "go.uber.org/fx"
)

// Config carries every input the serve fx app needs. It is supplied to the
// app via uberfx.Supply and consumed by the providers below.
type Config struct {
	WorkflowPath string
	Port         int
	MaxExecs     int
	SelfHosted   bool
	Editor       bool
	ExportURL    string
	APIKey       string
	ExporterName string
	Version      string
	OTel         tfotel.Config
	LogLevel     slog.Level
}

// ServeModule bundles every provider required to run `tailflow serve`.
var ServeModule = uberfx.Module("serve",
	uberfx.Provide(
		NewOTel,
		NewTracer,
		NewBusinessMetrics,
		NewLogger,
		NewWorkflow,
		NewEventBus,
		NewActionRegistry,
		NewExecutor,
		NewExecutionStore,
		NewExportPorts,
		NewServer,
	),
)

// RunApp builds the serve fx app, starts it, runs the server until ctx is
// cancelled, then tears everything down via fx OnStop hooks. The caller is
// expected to wire ctx to the desired signal handlers (SIGINT / SIGTERM).
func RunApp(ctx context.Context, cfg Config) error {
	var srv *server.Server

	app := uberfx.New(
		uberfx.NopLogger,
		uberfx.Supply(cfg),
		ServeModule,
		uberfx.Populate(&srv),
	)

	err := app.Err()
	if err != nil {
		return fmt.Errorf("fx wiring: %w", err)
	}

	startCtx, startCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startCancel()

	err = app.Start(startCtx)
	if err != nil {
		return fmt.Errorf("fx start: %w", err)
	}

	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = app.Stop(stopCtx)
	}()

	return srv.Run(ctx)
}

// --- OTel ---

type OTelIn struct {
	uberfx.In

	Config    Config
	Lifecycle uberfx.Lifecycle
}

type OTelOut struct {
	uberfx.Out

	Result *tfotel.Result
}

func NewOTel(in OTelIn) (out OTelOut, err error) {
	res, setupErr := tfotel.Setup(context.Background(), in.Config.OTel)
	if setupErr != nil {
		return out, fmt.Errorf("otel setup: %w", setupErr)
	}

	in.Lifecycle.Append(uberfx.Hook{
		OnStop: func(ctx context.Context) error {
			flushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			_ = res.ForceFlush(flushCtx)

			return res.Shutdown(flushCtx)
		},
	})

	out.Result = res

	return out, nil
}

// --- Tracer ---

type TracerIn struct {
	uberfx.In

	Result *tfotel.Result
}

type TracerOut struct {
	uberfx.Out

	Tracer *tfotel.Tracer
}

func NewTracer(in TracerIn) TracerOut {
	return TracerOut{Tracer: tfotel.NewTracer(in.Result.TracerProvider)}
}

// --- BusinessMetrics ---

type BusinessMetricsIn struct {
	uberfx.In

	Result *tfotel.Result
}

type BusinessMetricsOut struct {
	uberfx.Out

	BusinessMetrics *tfotel.BusinessMetrics
}

func NewBusinessMetrics(in BusinessMetricsIn) (out BusinessMetricsOut, err error) {
	bm, bmErr := tfotel.NewBusinessMetrics(in.Result.MeterProvider)
	if bmErr != nil {
		return out, fmt.Errorf("otel business metrics: %w", bmErr)
	}

	out.BusinessMetrics = bm

	return out, nil
}

// --- Logger ---

type LoggerIn struct {
	uberfx.In

	Config Config
	Result *tfotel.Result
}

type LoggerOut struct {
	uberfx.Out

	Logger *slog.Logger
}

func NewLogger(in LoggerIn) LoggerOut {
	return LoggerOut{Logger: tfotel.NewSlogLogger(in.Config.LogLevel, in.Result)}
}

// --- Workflow ---

type WorkflowIn struct {
	uberfx.In

	Config Config
}

type WorkflowOut struct {
	uberfx.Out

	Workflow *parser.Workflow
}

func NewWorkflow(in WorkflowIn) (out WorkflowOut, err error) {
	wf, parseErr := parser.Parse(in.Config.WorkflowPath)
	if parseErr != nil {
		return out, fmt.Errorf("parse workflow: %w", parseErr)
	}

	out.Workflow = wf

	return out, nil
}

// --- EventBus ---

type EventBusIn struct {
	uberfx.In

	Lifecycle uberfx.Lifecycle
}

type EventBusOut struct {
	uberfx.Out

	EventBus *event.Bus
}

func NewEventBus(in EventBusIn) EventBusOut {
	bus := event.NewBus()

	in.Lifecycle.Append(uberfx.Hook{
		OnStop: func(_ context.Context) error {
			bus.Close()
			return nil
		},
	})

	return EventBusOut{EventBus: bus}
}

// --- ActionRegistry ---

type ActionRegistryIn struct {
	uberfx.In

	Config Config
}

type ActionRegistryOut struct {
	uberfx.Out

	Registry *action.Registry
}

func NewActionRegistry(in ActionRegistryIn) ActionRegistryOut {
	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	if !in.Config.SelfHosted {
		reg.SetAllowlist(saasAllowedActions(reg.Names()))
	}

	return ActionRegistryOut{Registry: reg}
}

// --- Executor ---

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

// --- ExecutionStore ---

type ExecutionStoreIn struct {
	uberfx.In

	Config Config
}

type ExecutionStoreOut struct {
	uberfx.Out

	Store store.ExecutionStore
}

func NewExecutionStore(in ExecutionStoreIn) ExecutionStoreOut {
	return ExecutionStoreOut{Store: store.NewExecutionStore(in.Config.MaxExecs)}
}

// --- ExportPorts ---

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

// --- Server ---

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
		ExporterName:   in.Config.ExporterName,
		Version:        in.Config.Version,
		Exporter:       in.Exporter,
		Claimer:        in.Claimer,
		Recoverer:      in.Recoverer,
	})

	return ServerOut{Server: srv}
}

// --- helpers ---

// saasAllowedActions filters out actions that should not be runnable on a
// hosted SaaS deployment (raw exec / js / file IO).
func saasAllowedActions(all []string) []string {
	blocked := map[string]bool{
		"js":         true,
		"exec":       true,
		"file.read":  true,
		"file.write": true,
	}

	allowed := make([]string, 0, len(all))

	for _, name := range all {
		if blocked[name] {
			continue
		}

		allowed = append(allowed, name)
	}

	return allowed
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

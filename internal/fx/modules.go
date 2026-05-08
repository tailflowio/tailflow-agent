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
		provideOTel,
		provideTracer,
		provideBusinessMetrics,
		provideLogger,
		provideWorkflow,
		provideEventBus,
		provideRegistry,
		provideExecutor,
		provideExecutionStore,
		provideExporter,
		provideClaimer,
		provideRecoverer,
		provideServer,
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

func provideOTel(lc uberfx.Lifecycle, cfg Config) (*tfotel.Result, error) {
	res, err := tfotel.Setup(context.Background(), cfg.OTel)
	if err != nil {
		return nil, fmt.Errorf("otel setup: %w", err)
	}

	lc.Append(uberfx.Hook{
		OnStop: func(ctx context.Context) error {
			flushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			_ = res.ForceFlush(flushCtx)

			return res.Shutdown(flushCtx)
		},
	})

	return res, nil
}

func provideTracer(res *tfotel.Result) *tfotel.Tracer {
	return tfotel.NewTracer(res.TracerProvider)
}

func provideBusinessMetrics(res *tfotel.Result) (*tfotel.BusinessMetrics, error) {
	bm, err := tfotel.NewBusinessMetrics(res.MeterProvider)
	if err != nil {
		return nil, fmt.Errorf("otel business metrics: %w", err)
	}

	return bm, nil
}

func provideLogger(cfg Config, res *tfotel.Result) *slog.Logger {
	return tfotel.NewSlogLogger(cfg.LogLevel, res)
}

func provideWorkflow(cfg Config) (*parser.Workflow, error) {
	wf, err := parser.Parse(cfg.WorkflowPath)
	if err != nil {
		return nil, fmt.Errorf("parse workflow: %w", err)
	}

	return wf, nil
}

func provideEventBus(lc uberfx.Lifecycle) *event.Bus {
	bus := event.NewBus()

	lc.Append(uberfx.Hook{
		OnStop: func(_ context.Context) error {
			bus.Close()
			return nil
		},
	})

	return bus
}

func provideRegistry(cfg Config) *action.Registry {
	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	if !cfg.SelfHosted {
		reg.SetAllowlist(saasAllowedActions(reg.Names()))
	}

	return reg
}

func provideExecutor(
	reg *action.Registry, bus *event.Bus, logger *slog.Logger,
	wf *parser.Workflow, tracer *tfotel.Tracer, bm *tfotel.BusinessMetrics,
) *engine.Executor {
	return engine.NewExecutor(reg, bus, logger, wf.Sensitive, tracer, bm)
}

func provideExecutionStore(cfg Config) store.ExecutionStore {
	return store.NewExecutionStore(cfg.MaxExecs)
}

// provideExporter returns the SaaS exporter when ExportURL is set, otherwise a
// noop. The server owns the Start/Shutdown lifecycle on this exporter (see
// internal/server.shutdownServices), so no fx hook is registered here.
func provideExporter(cfg Config, bus *event.Bus, wf *parser.Workflow, logger *slog.Logger) export.EventExporter {
	if cfg.ExportURL == "" {
		return export.NewNoopExporter()
	}

	return saas.NewExporter(saas.Config{
		ExportURL:           cfg.ExportURL,
		APIKey:              cfg.APIKey,
		AgentName:           cfg.ExporterName,
		EventBus:            bus,
		Logger:              logger,
		WorkflowName:        wf.Name,
		WorkflowDescription: wf.Description,
		WorkflowTags:        wf.Tags,
		TriggerType:         resolveTriggerType(wf),
		StepsCount:          len(wf.Steps),
		Version:             cfg.Version,
		Revision:            wf.Revision,
	})
}

func provideClaimer(cfg Config) export.IdempotencyClaimer {
	if cfg.ExportURL == "" {
		return export.NewNoopClaimer()
	}

	return saas.NewClaimClient(cfg.ExportURL, cfg.APIKey)
}

func provideRecoverer(cfg Config) export.ExecutionRecoverer {
	if cfg.ExportURL == "" {
		return export.NewNoopRecoverer()
	}

	return saas.NewRecoveryClient(cfg.ExportURL, cfg.APIKey)
}

func provideServer(
	cfg Config,
	exec *engine.Executor, wf *parser.Workflow, logger *slog.Logger,
	execStore store.ExecutionStore, bus *event.Bus,
	exporter export.EventExporter, claimer export.IdempotencyClaimer,
	recoverer export.ExecutionRecoverer,
) *server.Server {
	return server.New(server.Config{
		Port:           cfg.Port,
		Executor:       exec,
		Workflow:       wf,
		FilePath:       cfg.WorkflowPath,
		EditorEnabled:  cfg.Editor,
		ExecutionStore: execStore,
		EventBus:       bus,
		Logger:         logger,
		ExporterName:   cfg.ExporterName,
		Version:        cfg.Version,
		Exporter:       exporter,
		Claimer:        claimer,
		Recoverer:      recoverer,
	})
}

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

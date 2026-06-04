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

	tfotel "github.com/tailflow/tailflow/internal/otel"
	"github.com/tailflow/tailflow/internal/server"
	uberfx "go.uber.org/fx"
)

var appStartFunc = func(app *uberfx.App, ctx context.Context) error {
	return app.Start(ctx)
}

// Config carries every input the serve fx app needs. It is supplied to the
// app via uberfx.Supply and consumed by the providers below.
type Config struct {
	WorkflowPath string
	Port         int
	MaxExecs     int
	// Unsafe disables the default action allowlist when true. Secure by
	// default: when false (the default) the allowlist is active and dangerous
	// actions (exec, js, file.*) are blocked.
	Unsafe   bool
	Editor   bool
	Version  string
	OTel     tfotel.Config
	LogLevel slog.Level
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

	err = appStartFunc(app, startCtx) //nolint:contextcheck // intentional: fx start uses a fresh context independent of the run context
	if err != nil {
		return fmt.Errorf("fx start: %w", err)
	}

	defer func() { //nolint:contextcheck // intentional: shutdown uses a fresh context because the run context is already cancelled
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()

		_ = app.Stop(stopCtx)
	}()

	return srv.Run(ctx)
}

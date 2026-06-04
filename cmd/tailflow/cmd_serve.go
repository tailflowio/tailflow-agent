package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	agentfx "github.com/tailflow/tailflow/internal/fx"
	tfotel "github.com/tailflow/tailflow/internal/otel"
	"github.com/tailflow/tailflow/internal/parser"
)

func serveCmd(otelEndpoint, otelServiceName *string) *cobra.Command {
	var (
		port     int
		maxExecs int
		unsafe   bool
		editor   bool
	)

	cmd := &cobra.Command{
		Use:   "serve <workflow.yaml>",
		Short: "Start the web server for a single workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			otelCfg := resolveOTelConfig(otelEndpoint, otelServiceName)

			return executeServe(args[0], port, maxExecs, unsafe, editor, otelCfg)
		},
	}
	cmd.Flags().IntVarP(&port, "port", "P", 8080, "Server port")
	cmd.Flags().IntVar(&maxExecs, "max-executions", 100, "Max executions to keep in memory")
	cmd.Flags().BoolVar(&unsafe, "unsafe", false, "Allow exec/js/file.* actions (disables the default allowlist; trusted hosts only)")
	cmd.Flags().BoolVar(&editor, "editor", false, "Enable workflow editor: persist YAML changes via PUT /api/workflow/raw")

	return cmd
}

func executeServe(path string, port, maxExecs int, unsafe, editor bool, otelCfg tfotel.Config) error {
	// Validate the workflow at the CLI boundary so a content error surfaces as
	// a clear, comprehensive message instead of an opaque fx dependency-graph
	// wiring dump.
	_, parseErr := parser.Parse(path)
	if parseErr != nil {
		renderWorkflowErrors(path, parseErr)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return agentfx.RunApp(ctx, agentfx.Config{
		WorkflowPath: path,
		Port:         port,
		MaxExecs:     maxExecs,
		Unsafe:       unsafe,
		Editor:       editor,
		Version:      version,
		OTel:         otelCfg,
		LogLevel:     slog.LevelInfo,
	})
}

// renderWorkflowErrors prints every workflow validation error on its own line
// so the operator sees all problems at once rather than an fx wiring dump.
func renderWorkflowErrors(path string, err error) {
	r := &cliRenderer{}

	fmt.Printf("  %s %s\n", r.c("31", "✗"), r.c("31", fmt.Sprintf("Cannot start %q — the workflow is invalid:", path)))

	for _, message := range parser.Messages(err) {
		fmt.Printf("      %s\n", r.c("31", "• "+message))
	}
}

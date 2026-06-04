package main

import (
	"context"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	agentfx "github.com/tailflow/tailflow/internal/fx"
	tfotel "github.com/tailflow/tailflow/internal/otel"
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
	cmd.Flags().BoolVar(&unsafe, "unsafe", false, "Disable the default action allowlist: allow exec, js and file.* actions (use only on trusted self-hosted instances)")
	cmd.Flags().BoolVar(&editor, "editor", false, "Enable workflow editor: persist YAML changes via PUT /api/workflow/raw")

	return cmd
}

func executeServe(path string, port, maxExecs int, unsafe, editor bool, otelCfg tfotel.Config) error {
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

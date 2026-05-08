package main

import (
	"context"
	"errors"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	agentfx "github.com/tailflow/tailflow/internal/fx"
	tfotel "github.com/tailflow/tailflow/internal/otel"
)

func serveCmd(exporterURL, exporterKey, exporterName, otelEndpoint, otelServiceName *string) *cobra.Command {
	var (
		port       int
		maxExecs   int
		selfHosted bool
		editor     bool
	)

	cmd := &cobra.Command{
		Use:   "serve <workflow.yaml>",
		Short: "Start the web server for a single workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := flagOrEnv(*exporterURL, "TAILFLOW_EXPORTER_URL")
			key := flagOrEnv(*exporterKey, "TAILFLOW_EXPORTER_KEY")
			name := flagOrEnv(*exporterName, "TAILFLOW_EXPORTER_NAME")

			if url != "" && name == "" {
				return errors.New("--exporter-name (or TAILFLOW_EXPORTER_NAME) is required when exporter is enabled")
			}

			otelCfg := resolveOTelConfig(otelEndpoint, otelServiceName)

			return executeServe(args[0], port, maxExecs, selfHosted, editor, url, key, name, otelCfg)
		},
	}
	cmd.Flags().IntVarP(&port, "port", "P", 8080, "Server port")
	cmd.Flags().IntVar(&maxExecs, "max-executions", 100, "Max executions to keep in memory")
	cmd.Flags().BoolVar(&selfHosted, "selfhosted", false, "Enable all actions (exec, js, file.*) for self-hosted deployments")
	cmd.Flags().BoolVar(&editor, "editor", false, "Enable workflow editor: persist YAML changes via PUT /api/workflow/raw")

	return cmd
}

func executeServe(
	path string, port int, maxExecs int, selfHosted, editor bool,
	exportURL, apiKey, exporterName string, otelCfg tfotel.Config,
) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return agentfx.RunApp(ctx, agentfx.Config{
		WorkflowPath: path,
		Port:         port,
		MaxExecs:     maxExecs,
		SelfHosted:   selfHosted,
		Editor:       editor,
		ExportURL:    exportURL,
		APIKey:       apiKey,
		ExporterName: exporterName,
		Version:      version,
		OTel:         otelCfg,
		LogLevel:     slog.LevelInfo,
	})
}

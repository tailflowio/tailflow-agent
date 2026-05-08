package main

import (
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	_ = godotenv.Load()

	var (
		noColorFlag     bool
		exporterURL     string
		exporterKey     string
		exporterName    string
		otelEndpoint    string
		otelServiceName string
	)

	rootCmd := &cobra.Command{
		Use:          "tailflow",
		Short:        "TailFlow - Workflow Engine",
		Version:      version,
		SilenceUsage: true,
	}
	rootCmd.PersistentFlags().BoolVar(&noColorFlag, "no-color", false, "Disable colour output")
	rootCmd.PersistentFlags().StringVar(&exporterURL, "exporter-url", "", "SaaS endpoint URL for event export (env: TAILFLOW_EXPORTER_URL)")
	rootCmd.PersistentFlags().StringVar(&exporterKey, "exporter-key", "", "API key for SaaS authentication (env: TAILFLOW_EXPORTER_KEY)")
	rootCmd.PersistentFlags().StringVar(&exporterName, "exporter-name", "", "Unique agent name (env: TAILFLOW_EXPORTER_NAME)")
	rootCmd.PersistentFlags().StringVar(&otelEndpoint, "otel-endpoint", "", "OTLP/HTTP endpoint (env: OTEL_EXPORTER_OTLP_ENDPOINT)")
	rootCmd.PersistentFlags().StringVar(&otelServiceName, "otel-service-name", "", "Service name (env: OTEL_SERVICE_NAME, default: tailflow)")

	rootCmd.AddCommand(runCmd(&noColorFlag, &exporterURL, &exporterKey, &exporterName, &otelEndpoint, &otelServiceName))
	rootCmd.AddCommand(validateCmd(&noColorFlag))
	rootCmd.AddCommand(serveCmd(&exporterURL, &exporterKey, &exporterName, &otelEndpoint, &otelServiceName))
	rootCmd.AddCommand(testCmd(&noColorFlag))

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

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
	rootCmd.PersistentFlags().StringVar(&otelEndpoint, "otel-endpoint", "", "OTLP/HTTP endpoint (env: OTEL_EXPORTER_OTLP_ENDPOINT)")
	rootCmd.PersistentFlags().StringVar(&otelServiceName, "otel-service-name", "", "Service name (env: OTEL_SERVICE_NAME, default: tailflow)")

	rootCmd.AddCommand(runCmd(&noColorFlag, &otelEndpoint, &otelServiceName))
	rootCmd.AddCommand(validateCmd(&noColorFlag))
	rootCmd.AddCommand(serveCmd(&otelEndpoint, &otelServiceName))
	rootCmd.AddCommand(testCmd(&noColorFlag))

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

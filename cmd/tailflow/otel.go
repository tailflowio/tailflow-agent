package main

import (
	"context"
	"log"
	"os"
	"time"

	tfotel "github.com/tailflow/tailflow/internal/otel"
)

func shutdownOTel(result *tfotel.Result) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	flushErr := result.ForceFlush(ctx)
	if flushErr != nil {
		log.Printf("[otel] flush error: %v", flushErr)
	}

	shutdownErr := result.Shutdown(ctx)
	if shutdownErr != nil {
		log.Printf("[otel] shutdown error: %v", shutdownErr)
	}
}

func resolveOTelConfig(endpoint, serviceName *string) tfotel.Config {
	ep := flagOrEnv(*endpoint, "OTEL_EXPORTER_OTLP_ENDPOINT")
	sn := flagOrEnv(*serviceName, "OTEL_SERVICE_NAME")

	return tfotel.Config{
		Endpoint:    ep,
		ServiceName: sn,
		Debug:       os.Getenv("OTEL_DEBUG") != "",
	}
}

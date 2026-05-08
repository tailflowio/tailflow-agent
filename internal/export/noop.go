package export

import "context"

// NewNoopExporter returns an EventExporter that drops all events. Used by
// self-hosted deployments without a SaaS endpoint configured.
func NewNoopExporter() EventExporter { return noopExporter{} }

// NewNoopClaimer returns an IdempotencyClaimer that always grants the claim
// (Claimed=true). Matches the fail-open semantics used by the SaaS client
// when the remote endpoint is unreachable: a missing dedup authority must
// never block legitimate executions.
func NewNoopClaimer() IdempotencyClaimer { return noopClaimer{} }

// NewNoopRecoverer returns an ExecutionRecoverer that yields no executions.
func NewNoopRecoverer() ExecutionRecoverer { return noopRecoverer{} }

type noopExporter struct{}

func (noopExporter) Start(context.Context) {}
func (noopExporter) Shutdown()              {}

type noopClaimer struct{}

func (noopClaimer) ClaimExecution(context.Context, string, string, string) (*ClaimResult, error) {
	return &ClaimResult{Claimed: true}, nil
}

type noopRecoverer struct{}

func (noopRecoverer) RecoverExecutions(context.Context, string) ([]RecoveredExecution, error) {
	return nil, nil
}

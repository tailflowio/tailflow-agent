package export

import "context"

// NewNoopExporter returns an EventExporter that drops all events. Default in
// v1, where no event sink is wired.
func NewNoopExporter() EventExporter { return noopExporter{} }

// NewNoopClaimer returns an IdempotencyClaimer that always grants the claim
// (Claimed=true). Fail-open by design: a missing dedup authority must never
// block legitimate executions. Acceptable only where no durable dedup exists;
// real deduplication relies on a UNIQUE constraint in a store-backed claimer.
func NewNoopClaimer() IdempotencyClaimer { return noopClaimer{} }

// NewNoopRecoverer returns an ExecutionRecoverer that yields no executions.
func NewNoopRecoverer() ExecutionRecoverer { return noopRecoverer{} }

type noopExporter struct{}

func (noopExporter) Start(context.Context) {}
func (noopExporter) Shutdown()             {}

type noopClaimer struct{}

func (noopClaimer) ClaimExecution(context.Context, string, string, string) (*ClaimResult, error) {
	return &ClaimResult{Claimed: true}, nil
}

type noopRecoverer struct{}

func (noopRecoverer) RecoverExecutions(context.Context, string) ([]RecoveredExecution, error) {
	return nil, nil
}

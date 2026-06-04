// Package export defines a stable local seam for streaming agent events and
// resolving cross-execution concerns (idempotency, recovery) against a
// store-backed adapter.
//
// In v1 the agent ships with no-op implementations of every port (see
// noop.go). The interfaces and DTOs are kept as a structural seam so a
// store-backed adapter can plug in without re-shaping the agent core.
package export

import (
	"context"
	"encoding/json"

	"github.com/tailflow/tailflow/internal/runtime"
)

// EventExporter streams agent events to a sink. The v1 implementation is a
// no-op (see noop.go).
type EventExporter interface {
	// Start spawns the export workers. Implementations must return promptly;
	// long-running work belongs in goroutines tied to ctx.
	Start(ctx context.Context)
	// Shutdown blocks until the export workers have drained.
	Shutdown()
}

// IdempotencyClaimer decides whether an inbound trigger (identified by
// idempotencyKey) may start a fresh execution or must be deduplicated against
// an existing one. Deduplication is enforced locally via a UNIQUE constraint
// on the backing store, not via a distributed lock.
type IdempotencyClaimer interface {
	ClaimExecution(ctx context.Context, executionID, workflowName, idempotencyKey string) (*ClaimResult, error)
}

// ExecutionRecoverer scans the backing store at boot for executions left in a
// non-terminal state by a previous agent run, so the agent can resume them on
// startup. The scan is lock-free.
type ExecutionRecoverer interface {
	RecoverExecutions(ctx context.Context, agentID string) ([]RecoveredExecution, error)
}

// ClaimResult is the response from an idempotency claim attempt.
//
// Claimed=true means the caller should proceed with a fresh execution.
// Claimed=false means an existing execution matched the idempotency key and
// the caller should short-circuit using ExistingExecutionID/ExistingStatus.
type ClaimResult struct {
	Claimed             bool   `json:"claimed"`
	ExistingExecutionID string `json:"execution_id,omitempty"`
	ExistingStatus      string `json:"status,omitempty"`
}

// RecoveredExecution describes an execution to resume on agent startup.
//
// The Steps field intentionally references runtime.StepResult: this is a
// shared DTO between the agent core and the store-backed recovery adapter, and
// decoupling them through map[string]any would push the parsing burden onto
// the server. The trade-off was accepted at the time of the export-port
// refactor.
type RecoveredExecution struct {
	ExecutionID  string                         `json:"execution_id"`
	WorkflowName string                         `json:"workflow_name"`
	Status       string                         `json:"status"`
	RawParams    string                         `json:"params,omitempty"`
	RawSteps     json.RawMessage                `json:"steps,omitempty"`
	Params       map[string]any                 `json:"-"`
	Steps        map[string]*runtime.StepResult `json:"-"`
}

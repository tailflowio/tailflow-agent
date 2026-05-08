// Package export defines the boundary between the agent and the SaaS backend.
//
// The agent depends only on the interfaces and DTOs declared here. Concrete
// implementations live in subpackages (e.g. internal/export/saas) and are
// selected by main.go based on runtime configuration. A no-op implementation
// (see noop.go) is used by self-hosted deployments without a SaaS endpoint.
package export

import (
	"context"
	"encoding/json"

	"github.com/tailflow/tailflow/internal/runtime"
)

// EventExporter streams agent events to a remote sink.
type EventExporter interface {
	// Start spawns the export workers. Implementations must return promptly;
	// long-running work belongs in goroutines tied to ctx.
	Start(ctx context.Context)
	// Shutdown blocks until the export workers have drained.
	Shutdown()
}

// IdempotencyClaimer asks a remote authority whether an inbound trigger
// (identified by idempotencyKey) is allowed to start a fresh execution or
// must be deduplicated against an existing one.
type IdempotencyClaimer interface {
	ClaimExecution(ctx context.Context, executionID, workflowName, idempotencyKey string) (*ClaimResult, error)
}

// ExecutionRecoverer fetches executions left in a non-terminal state on a
// previous agent run, so the agent can resume them on startup.
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
// shared DTO between the agent and the SaaS, and decoupling them through
// map[string]any would push the parsing burden onto the server. The
// trade-off was accepted at the time of the export-port refactor.
type RecoveredExecution struct {
	ExecutionID  string                         `json:"execution_id"`
	WorkflowName string                         `json:"workflow_name"`
	Status       string                         `json:"status"`
	RawParams    string                         `json:"params,omitempty"`
	RawSteps     json.RawMessage                `json:"steps,omitempty"`
	Params       map[string]any                 `json:"-"`
	Steps        map[string]*runtime.StepResult `json:"-"`
}

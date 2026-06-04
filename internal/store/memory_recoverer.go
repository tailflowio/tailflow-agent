package store

import (
	"context"

	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/runtime"
)

var _ export.ExecutionRecoverer = (*MemoryRecoverer)(nil)

// executionLister is a subset of ExecutionStore used by MemoryRecoverer.
// Kept unexported: external callers always pass *MemoryExecutionStore.
type executionLister interface {
	List(ctx context.Context) ([]*Execution, error)
}

// MemoryRecoverer scans a MemoryExecutionStore for executions left in a
// non-terminal state. The agent is self-hosted mono-instance, so the scan is
// lock-free and the agentID argument is ignored.
type MemoryRecoverer struct {
	store executionLister
}

// NewMemoryRecoverer returns a MemoryRecoverer backed by the given store.
func NewMemoryRecoverer(s *MemoryExecutionStore) *MemoryRecoverer {
	return &MemoryRecoverer{store: s}
}

// RecoverExecutions returns the running and waiting executions held by the
// backing store, mapped to export.RecoveredExecution. The agentID is ignored
// because the agent is mono-instance.
func (r *MemoryRecoverer) RecoverExecutions(
	ctx context.Context,
	_ string,
) ([]export.RecoveredExecution, error) {
	execs, err := r.store.List(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]export.RecoveredExecution, 0, len(execs))

	for _, exec := range execs {
		if exec.Status != runtime.StatusRunning && exec.Status != runtime.StatusWaiting {
			continue
		}

		out = append(out, export.RecoveredExecution{
			ExecutionID:  exec.ID,
			WorkflowName: exec.WorkflowName,
			Status:       exec.Status,
			Params:       exec.Params,
			Steps:        exec.Steps,
		})
	}

	return out, nil
}

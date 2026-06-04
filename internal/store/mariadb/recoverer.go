package mariadb

import (
	"context"
	"fmt"

	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/runtime"
)

var _ export.ExecutionRecoverer = (*Store)(nil)

// RecoverExecutions returns the running and waiting executions persisted by a
// previous agent run, oldest first, so the server can resume them on startup.
// The scan is lock-free (no GET_LOCK): the agent is self-hosted mono-instance,
// so the agentID argument is ignored. The projection mirrors scanExecution's
// eight columns exactly; created_seq drives the ordering only and is never
// selected.
func (s *Store) RecoverExecutions(
	ctx context.Context,
	_ string,
) ([]export.RecoveredExecution, error) {
	q := fmt.Sprintf(`
		SELECT
			id,
			workflow_name,
			status,
			params,
			steps,
			started_at,
			finished_at,
			error_msg
		FROM %sexecutions
		WHERE status IN (?,?)
		ORDER BY created_seq ASC`, s.prefix)

	rows, err := s.db.QueryContext(ctx, q,
		runtime.StatusRunning,
		runtime.StatusWaiting,
	)
	if err != nil {
		return nil, fmt.Errorf("mariadb recover: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var out []export.RecoveredExecution

	for rows.Next() {
		exec, scanErr := scanExecution(rows.Scan)
		if scanErr != nil {
			return nil, fmt.Errorf("mariadb recover: scan: %w", scanErr)
		}

		out = append(out, export.RecoveredExecution{
			ExecutionID:  exec.ID,
			WorkflowName: exec.WorkflowName,
			Status:       exec.Status,
			Params:       exec.Params,
			Steps:        exec.Steps,
		})
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("mariadb recover: rows: %w", err)
	}

	return out, nil
}

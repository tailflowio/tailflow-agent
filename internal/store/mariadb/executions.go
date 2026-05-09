package mariadb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

// Add inserts a new execution. Steps and Params are JSON-encoded.
func (s *Store) Add(ctx context.Context, exec *store.Execution) error {
	params, err := encodeJSON(exec.Params)
	if err != nil {
		return fmt.Errorf("mariadb add: encode params: %w", err)
	}

	steps, err := encodeJSON(exec.Steps)
	if err != nil {
		return fmt.Errorf("mariadb add: encode steps: %w", err)
	}

	q := fmt.Sprintf(`INSERT INTO %sexecutions
        (id, workflow_name, status, params, steps, started_at, finished_at, error_msg)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, s.prefix)

	_, err = s.db.ExecContext(ctx, q,
		exec.ID, exec.WorkflowName, exec.Status,
		nullableJSON(params), nullableJSON(steps),
		exec.StartedAt, nullableTime(exec.FinishedAt), nullableString(exec.Error),
	)
	if err != nil {
		return fmt.Errorf("mariadb add: %w", err)
	}

	return nil
}

// Get returns the stored execution. Wraps store.ErrNotFound when absent.
func (s *Store) Get(ctx context.Context, id string) (*store.Execution, error) {
	q := fmt.Sprintf(`SELECT id, workflow_name, status, params, steps,
        started_at, finished_at, error_msg
        FROM %sexecutions WHERE id = ?`, s.prefix)

	row := s.db.QueryRowContext(ctx, q, id)

	exec, err := scanExecution(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", store.ErrNotFound, id)
	}

	if err != nil {
		return nil, fmt.Errorf("mariadb get: %w", err)
	}

	return exec, nil
}

// Update overwrites the stored execution with the given snapshot.
func (s *Store) Update(ctx context.Context, exec *store.Execution) error {
	params, err := encodeJSON(exec.Params)
	if err != nil {
		return fmt.Errorf("mariadb update: encode params: %w", err)
	}

	steps, err := encodeJSON(exec.Steps)
	if err != nil {
		return fmt.Errorf("mariadb update: encode steps: %w", err)
	}

	q := fmt.Sprintf(`UPDATE %sexecutions SET
        workflow_name = ?, status = ?, params = ?, steps = ?,
        started_at = ?, finished_at = ?, error_msg = ?
        WHERE id = ?`, s.prefix)

	_, err = s.db.ExecContext(ctx, q,
		exec.WorkflowName, exec.Status,
		nullableJSON(params), nullableJSON(steps),
		exec.StartedAt, nullableTime(exec.FinishedAt), nullableString(exec.Error),
		exec.ID,
	)
	if err != nil {
		return fmt.Errorf("mariadb update: %w", err)
	}

	return nil
}

// UpdateExecution loads, mutates and re-saves an execution within a single
// transaction with row-level locking, mirroring the in-memory store's
// serialised semantics for concurrent captureEvents/finalizeExecution calls.
func (s *Store) UpdateExecution(ctx context.Context, id string, fn func(exec *store.Execution)) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mariadb update: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	exec, err := s.lockExecution(ctx, tx, id)
	if errors.Is(err, sql.ErrNoRows) {
		// Missing execution → no-op, matches in-memory contract.
		return nil
	}

	if err != nil {
		return err
	}

	fn(exec)

	err = s.writeExecutionTx(ctx, tx, exec)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("mariadb update: commit: %w", err)
	}

	return nil
}

// UpdateStep mutates one step within an execution under a row-level lock,
// then derives the execution-level status from step states (running vs
// waiting) for active executions, mirroring MemoryExecutionStore.
func (s *Store) UpdateStep(ctx context.Context, executionID, stepID string, fn func(step *runtime.StepResult)) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mariadb update step: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	exec, err := s.lockExecution(ctx, tx, executionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}

	if err != nil {
		return err
	}

	if exec.Steps == nil {
		exec.Steps = make(map[string]*runtime.StepResult)
	}

	if exec.Steps[stepID] == nil {
		exec.Steps[stepID] = &runtime.StepResult{}
	}

	fn(exec.Steps[stepID])
	deriveExecutionStatus(exec)

	err = s.writeExecutionTx(ctx, tx, exec)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("mariadb update step: commit: %w", err)
	}

	return nil
}

// List returns every stored execution, newest first.
func (s *Store) List(ctx context.Context) ([]*store.Execution, error) {
	q := fmt.Sprintf(`SELECT id, workflow_name, status, params, steps,
        started_at, finished_at, error_msg
        FROM %sexecutions ORDER BY created_seq DESC`, s.prefix)

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("mariadb list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*store.Execution

	for rows.Next() {
		exec, scanErr := scanExecution(rows.Scan)
		if scanErr != nil {
			return nil, fmt.Errorf("mariadb list: scan: %w", scanErr)
		}

		out = append(out, exec)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("mariadb list: rows: %w", err)
	}

	return out, nil
}

// Count returns the number of stored executions.
func (s *Store) Count(ctx context.Context) (int, error) {
	q := fmt.Sprintf(`SELECT COUNT(*) FROM %sexecutions`, s.prefix)

	var n int

	err := s.db.QueryRowContext(ctx, q).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("mariadb count: %w", err)
	}

	return n, nil
}

// lockExecution selects an execution row FOR UPDATE inside an open
// transaction, decoding it into a *store.Execution.
func (s *Store) lockExecution(ctx context.Context, tx *sql.Tx, id string) (*store.Execution, error) {
	q := fmt.Sprintf(`SELECT id, workflow_name, status, params, steps,
        started_at, finished_at, error_msg
        FROM %sexecutions WHERE id = ? FOR UPDATE`, s.prefix)

	row := tx.QueryRowContext(ctx, q, id)

	exec, err := scanExecution(row.Scan)
	if err != nil {
		return nil, err
	}

	return exec, nil
}

// writeExecutionTx writes back a mutated execution within an open transaction.
func (s *Store) writeExecutionTx(ctx context.Context, tx *sql.Tx, exec *store.Execution) error {
	params, err := encodeJSON(exec.Params)
	if err != nil {
		return fmt.Errorf("mariadb write tx: encode params: %w", err)
	}

	steps, err := encodeJSON(exec.Steps)
	if err != nil {
		return fmt.Errorf("mariadb write tx: encode steps: %w", err)
	}

	q := fmt.Sprintf(`UPDATE %sexecutions SET
        workflow_name = ?, status = ?, params = ?, steps = ?,
        started_at = ?, finished_at = ?, error_msg = ?
        WHERE id = ?`, s.prefix)

	_, err = tx.ExecContext(ctx, q,
		exec.WorkflowName, exec.Status,
		nullableJSON(params), nullableJSON(steps),
		exec.StartedAt, nullableTime(exec.FinishedAt), nullableString(exec.Error),
		exec.ID,
	)
	if err != nil {
		return fmt.Errorf("mariadb write tx: %w", err)
	}

	return nil
}

// deriveExecutionStatus mirrors MemoryExecutionStore's auto-derivation:
// a running execution becomes "running" if any step is running, otherwise
// "waiting" if any step is waiting. Terminal statuses are left untouched.
func deriveExecutionStatus(exec *store.Execution) {
	if exec.Status != runtime.StatusRunning && exec.Status != runtime.StatusWaiting {
		return
	}

	for _, step := range exec.Steps {
		if step.Status == runtime.StatusRunning {
			exec.Status = runtime.StatusRunning
			return
		}
	}

	for _, step := range exec.Steps {
		if step.Status == runtime.StatusWaiting {
			exec.Status = runtime.StatusWaiting
			return
		}
	}
}

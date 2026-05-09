package clickhouse

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

// executionColumns lists the columns returned by SELECT and matches the
// argument order expected by scanExecution.
const executionColumns = `id, workflow_name, status, params, steps,
        started_at, finished_at, error_msg`

// Add inserts a new execution row. ClickHouse's ReplacingMergeTree merges
// rows by primary key (id) keeping only the highest version, so subsequent
// updates simply insert a new row with a larger version timestamp.
func (s *Store) Add(ctx context.Context, exec *store.Execution) error {
	return s.insertExecution(ctx, exec)
}

// Get returns the latest snapshot for an execution. Uses FINAL so the
// background merge state is collapsed at read time. For self-hosted
// volumes this is fine; switch to argMax aggregation if FINAL becomes hot.
func (s *Store) Get(ctx context.Context, id string) (*store.Execution, error) {
	q := fmt.Sprintf(`SELECT %s FROM %sexecutions FINAL WHERE id = ?`, executionColumns, s.prefix)

	row := s.db.QueryRowContext(ctx, q, id)

	exec, err := scanExecution(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", store.ErrNotFound, id)
	}

	if err != nil {
		return nil, fmt.Errorf("clickhouse get: %w", err)
	}

	return exec, nil
}

// Update inserts a new version of the execution. The ReplacingMergeTree
// engine collapses to the highest version on merge.
func (s *Store) Update(ctx context.Context, exec *store.Execution) error {
	return s.insertExecution(ctx, exec)
}

// UpdateExecution loads the latest version, applies fn, and inserts a new
// version. Missing executions are silently ignored to match the in-memory
// contract used by the SSE capture loop.
func (s *Store) UpdateExecution(ctx context.Context, id string, fn func(exec *store.Execution)) error {
	exec, err := s.Get(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}

	if err != nil {
		return err
	}

	fn(exec)

	return s.insertExecution(ctx, exec)
}

// UpdateStep mutates one step within the execution and inserts a new
// version. Status auto-derivation matches MemoryExecutionStore.
func (s *Store) UpdateStep(ctx context.Context, executionID, stepID string, fn func(step *runtime.StepResult)) error {
	exec, err := s.Get(ctx, executionID)
	if errors.Is(err, store.ErrNotFound) {
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

	return s.insertExecution(ctx, exec)
}

// List returns every execution, newest first.
func (s *Store) List(ctx context.Context) ([]*store.Execution, error) {
	q := fmt.Sprintf(`SELECT %s FROM %sexecutions FINAL ORDER BY started_at DESC`, executionColumns, s.prefix)

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("clickhouse list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*store.Execution

	for rows.Next() {
		exec, scanErr := scanExecution(rows.Scan)
		if scanErr != nil {
			return nil, fmt.Errorf("clickhouse list: scan: %w", scanErr)
		}

		out = append(out, exec)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("clickhouse list: rows: %w", err)
	}

	return out, nil
}

// Count returns the number of distinct executions. Wraps the FINAL select
// because we count after dedup.
func (s *Store) Count(ctx context.Context) (int, error) {
	q := fmt.Sprintf(`SELECT count() FROM %sexecutions FINAL`, s.prefix)

	var n int

	err := s.db.QueryRowContext(ctx, q).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("clickhouse count: %w", err)
	}

	return n, nil
}

// insertExecution writes a new version of the execution. version=now64(6)
// so the most recent INSERT wins on ReplacingMergeTree merges.
func (s *Store) insertExecution(ctx context.Context, exec *store.Execution) error {
	params, err := encodeJSON(exec.Params)
	if err != nil {
		return fmt.Errorf("clickhouse insert: encode params: %w", err)
	}

	steps, err := encodeJSON(exec.Steps)
	if err != nil {
		return fmt.Errorf("clickhouse insert: encode steps: %w", err)
	}

	q := fmt.Sprintf(`INSERT INTO %sexecutions
        (id, workflow_name, status, params, steps, started_at, finished_at, error_msg, version)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, s.prefix)

	_, err = s.db.ExecContext(ctx, q,
		exec.ID, exec.WorkflowName, exec.Status,
		params, steps,
		exec.StartedAt, nullableTime(exec.FinishedAt), exec.Error,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("clickhouse insert: %w", err)
	}

	return nil
}

// deriveExecutionStatus mirrors MemoryExecutionStore's auto-derivation.
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

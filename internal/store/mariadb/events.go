package mariadb

import (
	"context"
	"fmt"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/store"
)

// AppendEvent inserts a new event row.
func (s *Store) AppendEvent(ctx context.Context, executionID string, ev event.Event) error {
	data, err := encodeJSON(ev.Data)
	if err != nil {
		return fmt.Errorf("mariadb append event: encode data: %w", err)
	}

	q := fmt.Sprintf(`INSERT INTO %sevents
        (execution_id, event_type, step_id, message, data, timestamp)
        VALUES (?, ?, ?, ?, ?, ?)`, s.prefix)

	_, err = s.db.ExecContext(ctx, q,
		executionID, string(ev.Type),
		nullableString(ev.StepID), nullableString(ev.Message),
		nullableJSON(data), ev.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("mariadb append event: %w", err)
	}

	return nil
}

// GetEvents returns the full event log for executionID.
func (s *Store) GetEvents(ctx context.Context, executionID string) ([]event.Event, error) {
	events, _, err := s.queryEvents(ctx, executionID, 0, 0)
	return events, err
}

// GetEventsPaginated returns a slice of events plus the total count.
func (s *Store) GetEventsPaginated(ctx context.Context, executionID string, offset, limit int) ([]event.Event, int, error) {
	return s.queryEvents(ctx, executionID, offset, limit)
}

// queryEvents implements both GetEvents and GetEventsPaginated. limit == 0
// returns every row.
func (s *Store) queryEvents(ctx context.Context, executionID string, offset, limit int) ([]event.Event, int, error) {
	if limit < 0 {
		limit = 0
	}

	if offset < 0 {
		offset = 0
	}

	totalQ := fmt.Sprintf(`SELECT COUNT(*) FROM %sevents WHERE execution_id = ?`, s.prefix)

	var total int

	err := s.db.QueryRowContext(ctx, totalQ, executionID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("mariadb events count: %w", err)
	}

	if total == 0 {
		return nil, 0, nil
	}

	q := fmt.Sprintf(`SELECT execution_id, event_type, step_id, message, data, timestamp
        FROM %sevents WHERE execution_id = ? ORDER BY seq ASC`, s.prefix)

	args := []any{executionID}

	if limit > 0 {
		q += " LIMIT ? OFFSET ?"

		args = append(args, limit, offset)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("mariadb events: %w", err)
	}

	defer func() { _ = rows.Close() }()

	out := make([]event.Event, 0, total)

	for rows.Next() {
		ev, scanErr := scanEvent(rows.Scan)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("mariadb events: scan: %w", scanErr)
		}

		out = append(out, ev)
	}

	err = rows.Err()
	if err != nil {
		return nil, 0, fmt.Errorf("mariadb events: rows: %w", err)
	}

	return out, total, nil
}

// IncrStepExecCount uses INSERT ... ON DUPLICATE KEY UPDATE to upsert the
// step's lifetime counter without a read-modify-write race.
func (s *Store) IncrStepExecCount(ctx context.Context, stepID string) error {
	q := fmt.Sprintf(`INSERT INTO %sstep_exec_counts (step_id, cnt) VALUES (?, 1)
        ON DUPLICATE KEY UPDATE cnt = cnt + 1`, s.prefix)

	_, err := s.db.ExecContext(ctx, q, stepID)
	if err != nil {
		return fmt.Errorf("mariadb incr step count: %w", err)
	}

	return nil
}

// StepExecCounts returns a snapshot of every step's execution counter.
func (s *Store) StepExecCounts(ctx context.Context) (map[string]int, error) {
	q := fmt.Sprintf(`SELECT step_id, cnt FROM %sstep_exec_counts`, s.prefix)

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("mariadb step counts: %w", err)
	}

	defer func() { _ = rows.Close() }()

	out := map[string]int{}

	for rows.Next() {
		var (
			stepID string
			cnt    int
		)

		scanErr := rows.Scan(&stepID, &cnt)
		if scanErr != nil {
			return nil, fmt.Errorf("mariadb step counts: scan: %w", scanErr)
		}

		out[stepID] = cnt
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("mariadb step counts: rows: %w", err)
	}

	return out, nil
}

// GetStepMetrics computes metrics for a single step. The aggregation is
// performed in Go after a List() because the canonical Steps map is stored
// as a JSON column; switching to a normalized step_results table is left as
// a follow-up optimisation when scale demands it.
func (s *Store) GetStepMetrics(ctx context.Context, stepID string) (*store.StepMetrics, error) {
	all, err := s.GetAllStepMetrics(ctx)
	if err != nil {
		return nil, err
	}

	m, ok := all[stepID]
	if !ok {
		return nil, nil
	}

	return m, nil
}

// GetAllStepMetrics computes per-step metrics over all stored executions.
func (s *Store) GetAllStepMetrics(ctx context.Context) (map[string]*store.StepMetrics, error) {
	execs, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	return store.ComputeStepMetrics(execs), nil
}

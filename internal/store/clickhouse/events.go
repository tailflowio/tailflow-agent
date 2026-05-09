package clickhouse

import (
	"context"
	"fmt"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/store"
)

// AppendEvent inserts a new event row. seq is generated client-side from
// the wall-clock nanoseconds because ClickHouse has no AUTO_INCREMENT.
func (s *Store) AppendEvent(ctx context.Context, executionID string, ev event.Event) error {
	data, err := encodeJSON(ev.Data)
	if err != nil {
		return fmt.Errorf("clickhouse append event: encode data: %w", err)
	}

	q := fmt.Sprintf(`INSERT INTO %sevents
        (seq, execution_id, event_type, step_id, message, data, timestamp)
        VALUES (?, ?, ?, ?, ?, ?, ?)`, s.prefix)

	_, err = s.db.ExecContext(ctx, q,
		nextSeq(), executionID, string(ev.Type),
		ev.StepID, ev.Message, data, ev.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("clickhouse append event: %w", err)
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

// queryEvents implements both GetEvents and GetEventsPaginated.
func (s *Store) queryEvents(ctx context.Context, executionID string, offset, limit int) ([]event.Event, int, error) {
	if limit < 0 {
		limit = 0
	}

	if offset < 0 {
		offset = 0
	}

	totalQ := fmt.Sprintf(`SELECT count() FROM %sevents WHERE execution_id = ?`, s.prefix)

	var total int

	err := s.db.QueryRowContext(ctx, totalQ, executionID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("clickhouse events count: %w", err)
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
		return nil, 0, fmt.Errorf("clickhouse events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]event.Event, 0, total)

	for rows.Next() {
		ev, scanErr := scanEvent(rows.Scan)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("clickhouse events: scan: %w", scanErr)
		}

		out = append(out, ev)
	}

	err = rows.Err()
	if err != nil {
		return nil, 0, fmt.Errorf("clickhouse events: rows: %w", err)
	}

	return out, total, nil
}

// IncrStepExecCount inserts a +1 row. SummingMergeTree merges these into
// a single row per step_id with the cumulative count, so concurrent
// inserts never lose updates.
func (s *Store) IncrStepExecCount(ctx context.Context, stepID string) error {
	q := fmt.Sprintf(`INSERT INTO %sstep_exec_counts (step_id, cnt) VALUES (?, 1)`, s.prefix)

	_, err := s.db.ExecContext(ctx, q, stepID)
	if err != nil {
		return fmt.Errorf("clickhouse incr step count: %w", err)
	}

	return nil
}

// StepExecCounts aggregates the SummingMergeTree rows on read so callers
// see the correct total even before background merges run.
func (s *Store) StepExecCounts(ctx context.Context) (map[string]int, error) {
	q := fmt.Sprintf(`SELECT step_id, sum(cnt) AS total
        FROM %sstep_exec_counts GROUP BY step_id`, s.prefix)

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("clickhouse step counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]int{}

	for rows.Next() {
		var (
			stepID string
			total  int
		)

		scanErr := rows.Scan(&stepID, &total)
		if scanErr != nil {
			return nil, fmt.Errorf("clickhouse step counts: scan: %w", scanErr)
		}

		out[stepID] = total
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("clickhouse step counts: rows: %w", err)
	}

	return out, nil
}

// GetStepMetrics computes metrics for a single step by aggregating List().
// See the package-level note: optimisable with a normalized step_results
// table when scale demands it.
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

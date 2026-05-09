package clickhouse

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2" // database/sql driver registration

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/store"
)

// Store is the ClickHouse-backed implementation of store.ExecutionStore.
// Safe for concurrent use; *sql.DB owns the connection pool.
type Store struct {
	db     *sql.DB
	prefix string
}

// New opens a *sql.DB to the given DSN, runs the idempotent schema migration
// and returns a ready-to-use Store. The DSN supports ${VAR} interpolation
// so workflow YAML can reference env-stored secrets.
//
// DSN format follows clickhouse-go/v2:
//
//	clickhouse://user:pass@host:9000/database?dial_timeout=...&secure=true
func New(ctx context.Context, dsn, tablePrefix string) (*Store, error) {
	if tablePrefix == "" {
		tablePrefix = DefaultTablePrefix
	}

	err := validatePrefix(tablePrefix)
	if err != nil {
		return nil, err
	}

	expanded := os.ExpandEnv(dsn)

	db, err := sql.Open("clickhouse", expanded)
	if err != nil {
		return nil, fmt.Errorf("clickhouse open: %w", err)
	}

	err = db.PingContext(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("clickhouse ping: %w", err)
	}

	s := &Store{db: db, prefix: tablePrefix}

	err = migrate(ctx, db, tablePrefix)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	return s, nil
}

// NewWithDB wraps an already-open *sql.DB. Used by tests with sqlmock; in
// production callers go through New which also runs the migration.
func NewWithDB(db *sql.DB, tablePrefix string) (*Store, error) {
	if tablePrefix == "" {
		tablePrefix = DefaultTablePrefix
	}

	err := validatePrefix(tablePrefix)
	if err != nil {
		return nil, err
	}

	return &Store{db: db, prefix: tablePrefix}, nil
}

// Close releases the underlying database connections.
func (s *Store) Close() error {
	return s.db.Close()
}

// scanExecution decodes a row from the executions table.
func scanExecution(scan func(...any) error) (*store.Execution, error) {
	var (
		exec       store.Execution
		params     string
		steps      string
		finishedAt sql.NullTime
		errMsg     string
	)

	err := scan(
		&exec.ID, &exec.WorkflowName, &exec.Status,
		&params, &steps,
		&exec.StartedAt, &finishedAt, &errMsg,
	)
	if err != nil {
		return nil, err
	}

	if params != "" {
		decodeErr := json.Unmarshal([]byte(params), &exec.Params)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode params: %w", decodeErr)
		}
	}

	if steps != "" {
		decodeErr := json.Unmarshal([]byte(steps), &exec.Steps)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode steps: %w", decodeErr)
		}
	}

	if finishedAt.Valid {
		t := finishedAt.Time
		exec.FinishedAt = &t
	}

	exec.Error = errMsg

	return &exec, nil
}

func scanEvent(scan func(...any) error) (event.Event, error) {
	var (
		ev      event.Event
		stepID  string
		message string
		data    string
		ts      time.Time
		etype   string
	)

	err := scan(&ev.ExecutionID, &etype, &stepID, &message, &data, &ts)
	if err != nil {
		return event.Event{}, err
	}

	ev.Type = event.EventType(etype)
	ev.Timestamp = ts
	ev.StepID = stepID
	ev.Message = message

	if data != "" {
		decodeErr := json.Unmarshal([]byte(data), &ev.Data)
		if decodeErr != nil {
			return event.Event{}, fmt.Errorf("decode event data: %w", decodeErr)
		}
	}

	return ev, nil
}

// encodeJSON marshals v, returning "" when v is nil, a typed-nil map/slice
// or empty. ClickHouse stores all JSON columns as String, so the empty
// string is the natural absent-value marker.
func encodeJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}

	rv := reflect.ValueOf(v)

	switch rv.Kind() {
	case reflect.Map, reflect.Slice:
		if rv.IsNil() || rv.Len() == 0 {
			return "", nil
		}
	}

	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

// nullableTime returns sql.NullTime; ClickHouse Nullable(DateTime64) maps to
// time.Time (zero) or NULL.
func nullableTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: *t, Valid: true}
}

// nextSeq returns a per-process monotonic sequence number for inserting
// events. ClickHouse has no AUTO_INCREMENT; using time.Now().UnixNano() is
// monotonic enough for a single-process agent and orders correctly within
// the events MergeTree.
func nextSeq() uint64 {
	return uint64(time.Now().UnixNano())
}

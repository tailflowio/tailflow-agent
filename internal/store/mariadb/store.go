package mariadb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	_ "github.com/go-sql-driver/mysql" // database/sql driver registration

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/store"
)

// Store is the MariaDB / MySQL backed implementation of store.ExecutionStore.
// It is safe for concurrent use; *sql.DB handles its own connection pool.
type Store struct {
	db     *sql.DB
	prefix string
}

// New opens a *sql.DB to the given DSN, runs the idempotent schema migration
// and returns a ready-to-use Store. The DSN supports ${VAR} interpolation
// against the process environment.
func New(ctx context.Context, dsn, tablePrefix string) (*Store, error) {
	if tablePrefix == "" {
		tablePrefix = DefaultTablePrefix
	}

	err := validatePrefix(tablePrefix)
	if err != nil {
		return nil, err
	}

	expanded := os.ExpandEnv(dsn)

	db, err := sql.Open("mysql", expanded)
	if err != nil {
		return nil, fmt.Errorf("mariadb open: %w", err)
	}

	err = db.PingContext(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("mariadb ping: %w", err)
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

// scanExecution decodes a row produced by a SELECT against the executions
// table into a *store.Execution. The signature accepts a Scan function so
// it can be reused for both *sql.Row and *sql.Rows.
func scanExecution(scan func(...any) error) (*store.Execution, error) {
	var (
		exec       store.Execution
		params     sql.NullString
		steps      sql.NullString
		finishedAt sql.NullTime
		errMsg     sql.NullString
	)

	err := scan(
		&exec.ID, &exec.WorkflowName, &exec.Status,
		&params, &steps,
		&exec.StartedAt, &finishedAt, &errMsg,
	)
	if err != nil {
		return nil, err
	}

	if params.Valid {
		decodeErr := json.Unmarshal([]byte(params.String), &exec.Params)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode params: %w", decodeErr)
		}
	}

	if steps.Valid {
		decodeErr := json.Unmarshal([]byte(steps.String), &exec.Steps)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode steps: %w", decodeErr)
		}
	}

	if finishedAt.Valid {
		t := finishedAt.Time
		exec.FinishedAt = &t
	}

	if errMsg.Valid {
		exec.Error = errMsg.String
	}

	return &exec, nil
}

func scanEvent(scan func(...any) error) (event.Event, error) {
	var (
		ev      event.Event
		stepID  sql.NullString
		message sql.NullString
		data    sql.NullString
		ts      time.Time
		etype   string
	)

	err := scan(&ev.ExecutionID, &etype, &stepID, &message, &data, &ts)
	if err != nil {
		return event.Event{}, err
	}

	ev.Type = event.EventType(etype)
	ev.Timestamp = ts

	if stepID.Valid {
		ev.StepID = stepID.String
	}

	if message.Valid {
		ev.Message = message.String
	}

	if data.Valid {
		decodeErr := json.Unmarshal([]byte(data.String), &ev.Data)
		if decodeErr != nil {
			return event.Event{}, fmt.Errorf("decode event data: %w", decodeErr)
		}
	}

	return ev, nil
}

// encodeJSON marshals v to JSON. nil values, including typed-nil maps and
// slices, and empty maps/slices are returned as a SQL NULL marker.
func encodeJSON(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}

	rv := reflect.ValueOf(v)

	isCollection := rv.Kind() == reflect.Map || rv.Kind() == reflect.Slice
	if isCollection && (rv.IsNil() || rv.Len() == 0) {
		return nil, nil
	}

	return json.Marshal(v)
}

func nullableJSON(b []byte) any {
	if b == nil {
		return nil
	}

	return string(b)
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}

	return *t
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}

	return s
}

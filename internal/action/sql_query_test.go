package action

import (
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	fakeruntime "github.com/tailflow/tailflow/internal/fake/fakeruntime"
	"github.com/tailflow/tailflow/internal/runtime"
)

// mockSQLRows implements sqlRows for unit-testing readSQLRows.
type mockSQLRows struct {
	columnsFn func() ([]string, error)
	nextFn    func() bool
	scanFn    func(dest ...any) error
	errFn     func() error
	closeFn   func() error
}

func (m *mockSQLRows) Columns() ([]string, error) { return m.columnsFn() }
func (m *mockSQLRows) Next() bool                  { return m.nextFn() }
func (m *mockSQLRows) Scan(dest ...any) error       { return m.scanFn(dest...) }
func (m *mockSQLRows) Err() error                   { return m.errFn() }
func (m *mockSQLRows) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

// helper that provides all services (Locker, DBPool, TxRegistry) using fakes
func newTestContextWithAllServices(t testing.TB, config map[string]any) *ActionContext {
	services := &runtime.ActionServices{
		Locker:     fakeruntime.NewLocker(t),
		DBPool:     fakeruntime.NewDBPool(t),
		TxRegistry: fakeruntime.NewTxRegistry(t),
	}
	return &ActionContext{
		Context:  newTestContext(config).Context,
		Config:   config,
		ExecCtx:  runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:   "test-step",
		Logger:   newTestContext(config).Logger,
		Services: services,
	}
}

type SQLQueryTestSuite struct {
	suite.Suite
}

func TestSQLQuery(t *testing.T) {
	suite.Run(t, new(SQLQueryTestSuite))
}

func (s *SQLQueryTestSuite) SetupTest() {}

func (s *SQLQueryTestSuite) TestQueryMissingQuery() {
	a := NewSQLQueryAction()
	locker := fakeruntime.NewLocker(s.T())
	ctx := newTestContextWithServices(map[string]any{"dsn": "postgres://localhost/db"}, locker)
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "query")
}

func (s *SQLQueryTestSuite) TestQueryMissingDSNAndTx() {
	a := NewSQLQueryAction()
	locker := fakeruntime.NewLocker(s.T())
	ctx := newTestContextWithServices(map[string]any{"query": "SELECT 1"}, locker)
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "dsn")
}

func (s *SQLQueryTestSuite) TestQueryNoServices() {
	a := NewSQLQueryAction()
	ctx := newTestContext(map[string]any{"dsn": "postgres://localhost/db", "query": "SELECT 1"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *SQLQueryTestSuite) TestQueryWithTxValid() {
	a := NewSQLQueryAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{"tx": "my_tx", "query": "SELECT 1"})
	err := a.Validate(ctx)
	s.NoError(err)
}

func (s *SQLQueryTestSuite) TestQueryWithTxNoServices() {
	a := NewSQLQueryAction()
	ctx := newTestContext(map[string]any{
		"tx":    "my_tx",
		"query": "SELECT 1",
	})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *SQLQueryTestSuite) TestQueryValidateOKWithDSN() {
	a := NewSQLQueryAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{
		"dsn":   "postgres://localhost/db",
		"query": "SELECT 1",
	})
	err := a.Validate(ctx)
	s.NoError(err)
}

func (s *SQLQueryTestSuite) TestQueryExecute() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow(1, "Alice").
		AddRow(2, "Bob")
	mockDB.ExpectQuery("SELECT").WillReturnRows(rows)

	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "test-dsn").Return(db, nil)

	ctx := newTestContext(map[string]any{
		"dsn":   "test-dsn",
		"query": "SELECT id, name FROM users",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}

	a := NewSQLQueryAction()
	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(2, m["count"])
	resultRows := m["rows"].([]map[string]any)
	s.Len(resultRows, 2)
	s.Equal(int64(1), resultRows[0]["id"])
	s.Equal("Alice", resultRows[0]["name"])
	s.Equal(int64(2), resultRows[1]["id"])
	s.Equal("Bob", resultRows[1]["name"])
}

func (s *SQLQueryTestSuite) TestQueryExecuteEmptyResult() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "name"})
	mockDB.ExpectQuery("SELECT").WillReturnRows(rows)

	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "test-dsn").Return(db, nil)

	ctx := newTestContext(map[string]any{
		"dsn":   "test-dsn",
		"query": "SELECT id, name FROM users WHERE 1=0",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}

	a := NewSQLQueryAction()
	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(0, m["count"])
	s.Len(m["rows"].([]map[string]any), 0)
}

func (s *SQLQueryTestSuite) TestQueryExecuteDBPoolError() {
	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "bad-dsn").Return(nil, fmt.Errorf("connection refused"))

	ctx := newTestContext(map[string]any{
		"dsn":   "bad-dsn",
		"query": "SELECT 1",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}

	a := NewSQLQueryAction()
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "connection refused")
}

func (s *SQLQueryTestSuite) TestQueryExecuteQueryError() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mockDB.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("syntax error"))

	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "test-dsn").Return(db, nil)

	ctx := newTestContext(map[string]any{
		"dsn":   "test-dsn",
		"query": "SELECT invalid",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}

	a := NewSQLQueryAction()
	_, err = a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "syntax error")
}

func (s *SQLQueryTestSuite) TestQueryExecuteWithParams() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id"}).AddRow(1)
	mockDB.ExpectQuery("SELECT").WithArgs("Alice").WillReturnRows(rows)

	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "test-dsn").Return(db, nil)

	ctx := newTestContext(map[string]any{
		"dsn":    "test-dsn",
		"query":  "SELECT id FROM users WHERE name = ?",
		"params": []any{"Alice"},
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}

	a := NewSQLQueryAction()
	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(1, m["count"])
}

func (s *SQLQueryTestSuite) TestQueryExecuteWithTx() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mockDB.ExpectBegin()
	tx, err := db.Begin()
	s.Require().NoError(err)

	txReg := fakeruntime.NewTxRegistry(s.T())
	txReg.EXPECT().Get("my_tx").Return(tx, nil)

	rows := sqlmock.NewRows([]string{"id"}).AddRow(42)
	mockDB.ExpectQuery("SELECT").WillReturnRows(rows)

	ctx := newTestContext(map[string]any{
		"tx":    "my_tx",
		"query": "SELECT id FROM users",
	})
	ctx.Services = &runtime.ActionServices{
		TxRegistry: txReg,
	}

	a := NewSQLQueryAction()
	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(1, m["count"])
}

func (s *SQLQueryTestSuite) TestQueryExecuteWithTxError() {
	txReg := fakeruntime.NewTxRegistry(s.T())
	txReg.EXPECT().Get("bad_tx").Return(nil, fmt.Errorf("tx not found"))

	ctx := newTestContext(map[string]any{
		"tx":    "bad_tx",
		"query": "SELECT 1",
	})
	ctx.Services = &runtime.ActionServices{
		TxRegistry: txReg,
	}

	a := NewSQLQueryAction()
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "tx not found")
}

func (s *SQLQueryTestSuite) TestQueryExecuteWithByteColumns() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"data"}).AddRow([]byte("binary data"))
	mockDB.ExpectQuery("SELECT").WillReturnRows(rows)

	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "test-dsn").Return(db, nil)

	ctx := newTestContext(map[string]any{
		"dsn":   "test-dsn",
		"query": "SELECT data FROM blobs",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}

	a := NewSQLQueryAction()
	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	resultRows := m["rows"].([]map[string]any)
	// []byte should be converted to string
	s.Equal("binary data", resultRows[0]["data"])
}

func (s *SQLQueryTestSuite) TestQueryExecuteColumnsError() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id"}).AddRow(1).RowError(0, fmt.Errorf("row error"))
	mockDB.ExpectQuery("SELECT").WillReturnRows(rows)

	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "test-dsn").Return(db, nil)

	ctx := newTestContext(map[string]any{
		"dsn":   "test-dsn",
		"query": "SELECT id FROM users",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}

	a := NewSQLQueryAction()
	_, err = a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "sql.query")
}

func (s *SQLQueryTestSuite) TestReadSQLRows_ColumnsError() {
	rows := &mockSQLRows{
		columnsFn: func() ([]string, error) {
			return nil, errors.New("columns exploded")
		},
		nextFn: func() bool { return false },
		errFn:  func() error { return nil },
	}

	_, err := readSQLRows(rows)
	s.Error(err)
	s.Contains(err.Error(), "columns exploded")
}

func (s *SQLQueryTestSuite) TestReadSQLRows_ScanError() {
	callCount := 0
	rows := &mockSQLRows{
		columnsFn: func() ([]string, error) {
			return []string{"id", "name"}, nil
		},
		nextFn: func() bool {
			callCount++
			return callCount == 1
		},
		scanFn: func(dest ...any) error {
			return errors.New("scan failed")
		},
		errFn: func() error { return nil },
	}

	_, err := readSQLRows(rows)
	s.Error(err)
	s.Contains(err.Error(), "scan failed")
}

func (s *SQLQueryTestSuite) TestToSliceNil() {
	result := toSlice(nil)
	s.Nil(result)
}

func (s *SQLQueryTestSuite) TestToSliceAnySlice() {
	input := []any{1, "two", 3.0}
	result := toSlice(input)
	s.Equal(input, result)
}

func (s *SQLQueryTestSuite) TestToSliceStringSlice() {
	input := []string{"a", "b", "c"}
	result := toSlice(input)
	s.Equal([]any{"a", "b", "c"}, result)
}

func (s *SQLQueryTestSuite) TestToSliceSingleValue() {
	result := toSlice("hello")
	s.Equal([]any{"hello"}, result)
}

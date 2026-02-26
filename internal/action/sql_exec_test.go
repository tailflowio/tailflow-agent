package action

import (
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	fakeruntime "github.com/tailflow/tailflow/internal/fake/fakeruntime"
	"github.com/tailflow/tailflow/internal/runtime"
)

type SQLExecTestSuite struct {
	suite.Suite
}

func TestSQLExec(t *testing.T) {
	suite.Run(t, new(SQLExecTestSuite))
}

func (s *SQLExecTestSuite) SetupTest() {}

func (s *SQLExecTestSuite) TestExecMissingQuery() {
	a := NewSQLExecAction()
	locker := fakeruntime.NewLocker(s.T())
	ctx := newTestContextWithServices(map[string]any{"dsn": "postgres://localhost/db"}, locker)
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "query")
}

func (s *SQLExecTestSuite) TestExecMissingDSNAndTx() {
	a := NewSQLExecAction()
	locker := fakeruntime.NewLocker(s.T())
	ctx := newTestContextWithServices(map[string]any{"query": "DELETE FROM t"}, locker)
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "dsn")
}

func (s *SQLExecTestSuite) TestExecNoServices() {
	a := NewSQLExecAction()
	ctx := newTestContext(map[string]any{"dsn": "postgres://localhost/db", "query": "DELETE FROM t"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *SQLExecTestSuite) TestExecWithTxValid() {
	a := NewSQLExecAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{"tx": "my_tx", "query": "DELETE FROM t"})
	err := a.Validate(ctx)
	s.NoError(err)
}

func (s *SQLExecTestSuite) TestExecWithTxNoServices() {
	a := NewSQLExecAction()
	ctx := newTestContext(map[string]any{
		"tx":    "my_tx",
		"query": "DELETE FROM t",
	})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *SQLExecTestSuite) TestExecValidateOKWithDSN() {
	a := NewSQLExecAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{
		"dsn":   "postgres://localhost/db",
		"query": "DELETE FROM t",
	})
	err := a.Validate(ctx)
	s.NoError(err)
}

func (s *SQLExecTestSuite) TestExecExecute() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mockDB.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(10, 1))

	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "test-dsn").Return(db, nil)

	ctx := newTestContext(map[string]any{
		"dsn":   "test-dsn",
		"query": "INSERT INTO users (name) VALUES (?)",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}

	a := NewSQLExecAction()
	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(int64(1), m["affected"])
	s.Equal(int64(10), m["last_insert_id"])
}

func (s *SQLExecTestSuite) TestExecExecuteDBPoolError() {
	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "bad-dsn").Return(nil, fmt.Errorf("connection refused"))

	ctx := newTestContext(map[string]any{
		"dsn":   "bad-dsn",
		"query": "DELETE FROM users",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}

	a := NewSQLExecAction()
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "connection refused")
}

func (s *SQLExecTestSuite) TestExecExecuteExecError() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mockDB.ExpectExec("DELETE").WillReturnError(fmt.Errorf("constraint violation"))

	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "test-dsn").Return(db, nil)

	ctx := newTestContext(map[string]any{
		"dsn":   "test-dsn",
		"query": "DELETE FROM users WHERE id = ?",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}

	a := NewSQLExecAction()
	_, err = a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "constraint violation")
}

func (s *SQLExecTestSuite) TestExecExecuteWithTx() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mockDB.ExpectBegin()
	tx, err := db.Begin()
	s.Require().NoError(err)

	txReg := fakeruntime.NewTxRegistry(s.T())
	txReg.EXPECT().Get("my_tx").Return(tx, nil)

	mockDB.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(5, 1))

	ctx := newTestContext(map[string]any{
		"tx":    "my_tx",
		"query": "INSERT INTO users (name) VALUES (?)",
	})
	ctx.Services = &runtime.ActionServices{
		TxRegistry: txReg,
	}

	a := NewSQLExecAction()
	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(int64(1), m["affected"])
}

func (s *SQLExecTestSuite) TestExecExecuteWithTxError() {
	txReg := fakeruntime.NewTxRegistry(s.T())
	txReg.EXPECT().Get("bad_tx").Return(nil, fmt.Errorf("tx not found"))

	ctx := newTestContext(map[string]any{
		"tx":    "bad_tx",
		"query": "DELETE FROM users",
	})
	ctx.Services = &runtime.ActionServices{
		TxRegistry: txReg,
	}

	a := NewSQLExecAction()
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "tx not found")
}

func (s *SQLExecTestSuite) TestExecExecuteWithParams() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mockDB.ExpectExec("UPDATE").WithArgs("Bob", 1).WillReturnResult(sqlmock.NewResult(0, 1))

	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "test-dsn").Return(db, nil)

	ctx := newTestContext(map[string]any{
		"dsn":    "test-dsn",
		"query":  "UPDATE users SET name = ? WHERE id = ?",
		"params": []any{"Bob", 1},
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}

	a := NewSQLExecAction()
	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(int64(1), m["affected"])
}

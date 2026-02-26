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

type SQLBeginTestSuite struct {
	suite.Suite
}

func TestSQLBegin(t *testing.T) {
	suite.Run(t, new(SQLBeginTestSuite))
}

func (s *SQLBeginTestSuite) SetupTest() {}

func (s *SQLBeginTestSuite) TestBeginMissingDSN() {
	a := NewSQLBeginAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{"name": "tx1"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "dsn")
}

func (s *SQLBeginTestSuite) TestBeginMissingName() {
	a := NewSQLBeginAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{"dsn": "postgres://localhost/db"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "name")
}

func (s *SQLBeginTestSuite) TestBeginNoServices() {
	a := NewSQLBeginAction()
	ctx := newTestContext(map[string]any{"dsn": "postgres://localhost/db", "name": "tx1"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *SQLBeginTestSuite) TestBeginValidateOK() {
	a := NewSQLBeginAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{
		"dsn":  "postgres://localhost/db",
		"name": "tx1",
	})
	err := a.Validate(ctx)
	s.NoError(err)
}

func (s *SQLBeginTestSuite) TestBeginValidateNoTxRegistry() {
	a := NewSQLBeginAction()
	dbPool := fakeruntime.NewDBPool(s.T())
	ctx := newTestContext(map[string]any{
		"dsn":  "postgres://localhost/db",
		"name": "tx1",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *SQLBeginTestSuite) TestBeginExecute() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mockDB.ExpectBegin()
	tx, err := db.Begin()
	s.Require().NoError(err)

	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "test-dsn").Return(db, nil)

	txReg := fakeruntime.NewTxRegistry(s.T())
	txReg.EXPECT().Begin(mock.Anything, db, "tx1").Return(tx, nil)

	ctx := newTestContext(map[string]any{
		"dsn":  "test-dsn",
		"name": "tx1",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool:     dbPool,
		TxRegistry: txReg,
	}

	a := NewSQLBeginAction()
	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal("tx1", m["name"])
	s.Equal(true, m["started"])
}

func (s *SQLBeginTestSuite) TestBeginExecuteDBPoolError() {
	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "bad-dsn").Return(nil, fmt.Errorf("connection refused"))

	txReg := fakeruntime.NewTxRegistry(s.T())

	ctx := newTestContext(map[string]any{
		"dsn":  "bad-dsn",
		"name": "tx1",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool:     dbPool,
		TxRegistry: txReg,
	}

	a := NewSQLBeginAction()
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "connection refused")
}

func (s *SQLBeginTestSuite) TestBeginExecuteTxRegistryError() {
	db, _, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "test-dsn").Return(db, nil)

	txReg := fakeruntime.NewTxRegistry(s.T())
	txReg.EXPECT().Begin(mock.Anything, db, "tx1").Return(nil, fmt.Errorf("tx already exists"))

	ctx := newTestContext(map[string]any{
		"dsn":  "test-dsn",
		"name": "tx1",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool:     dbPool,
		TxRegistry: txReg,
	}

	a := NewSQLBeginAction()
	_, err = a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "tx already exists")
}

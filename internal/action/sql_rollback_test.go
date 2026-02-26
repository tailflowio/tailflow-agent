package action

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	fakeruntime "github.com/tailflow/tailflow/internal/fake/fakeruntime"
	"github.com/tailflow/tailflow/internal/runtime"
)

type SQLRollbackTestSuite struct {
	suite.Suite
}

func TestSQLRollback(t *testing.T) {
	suite.Run(t, new(SQLRollbackTestSuite))
}

func (s *SQLRollbackTestSuite) SetupTest() {}

func (s *SQLRollbackTestSuite) TestRollbackMissingName() {
	a := NewSQLRollbackAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "name")
}

func (s *SQLRollbackTestSuite) TestRollbackNoServices() {
	a := NewSQLRollbackAction()
	ctx := newTestContext(map[string]any{"name": "tx1"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *SQLRollbackTestSuite) TestRollbackValidateOK() {
	a := NewSQLRollbackAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{
		"name": "tx1",
	})
	err := a.Validate(ctx)
	s.NoError(err)
}

func (s *SQLRollbackTestSuite) TestRollbackExecute() {
	txReg := fakeruntime.NewTxRegistry(s.T())
	txReg.EXPECT().Rollback("tx1").Return(nil)

	ctx := newTestContext(map[string]any{
		"name": "tx1",
	})
	ctx.Services = &runtime.ActionServices{
		TxRegistry: txReg,
	}

	a := NewSQLRollbackAction()
	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal("tx1", m["name"])
	s.Equal(true, m["rolled_back"])
}

func (s *SQLRollbackTestSuite) TestRollbackExecuteError() {
	txReg := fakeruntime.NewTxRegistry(s.T())
	txReg.EXPECT().Rollback("bad_tx").Return(fmt.Errorf("tx not found"))

	ctx := newTestContext(map[string]any{
		"name": "bad_tx",
	})
	ctx.Services = &runtime.ActionServices{
		TxRegistry: txReg,
	}

	a := NewSQLRollbackAction()
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "tx not found")
}

package action

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	fakeruntime "github.com/tailflow/tailflow/internal/fake/fakeruntime"
	"github.com/tailflow/tailflow/internal/runtime"
)

type SQLCommitTestSuite struct {
	suite.Suite
}

func TestSQLCommit(t *testing.T) {
	suite.Run(t, new(SQLCommitTestSuite))
}

func (s *SQLCommitTestSuite) SetupTest() {}

func (s *SQLCommitTestSuite) TestCommitMissingName() {
	a := NewSQLCommitAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "name")
}

func (s *SQLCommitTestSuite) TestCommitNoServices() {
	a := NewSQLCommitAction()
	ctx := newTestContext(map[string]any{"name": "tx1"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

func (s *SQLCommitTestSuite) TestCommitValidateOK() {
	a := NewSQLCommitAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{
		"name": "tx1",
	})
	err := a.Validate(ctx)
	s.NoError(err)
}

func (s *SQLCommitTestSuite) TestCommitExecute() {
	txReg := fakeruntime.NewTxRegistry(s.T())
	txReg.EXPECT().Commit("tx1").Return(nil)

	ctx := newTestContext(map[string]any{
		"name": "tx1",
	})
	ctx.Services = &runtime.ActionServices{
		TxRegistry: txReg,
	}

	a := NewSQLCommitAction()
	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal("tx1", m["name"])
	s.Equal(true, m["committed"])
}

func (s *SQLCommitTestSuite) TestCommitExecuteError() {
	txReg := fakeruntime.NewTxRegistry(s.T())
	txReg.EXPECT().Commit("bad_tx").Return(fmt.Errorf("tx not found"))

	ctx := newTestContext(map[string]any{
		"name": "bad_tx",
	})
	ctx.Services = &runtime.ActionServices{
		TxRegistry: txReg,
	}

	a := NewSQLCommitAction()
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "tx not found")
}

package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/runtime"
	fakeruntime "github.com/tailflow/tailflow/internal/fake/fakeruntime"
)

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

type SQLActionTestSuite struct {
	suite.Suite
}

func TestSQLAction(t *testing.T) {
	suite.Run(t, new(SQLActionTestSuite))
}

func (s *SQLActionTestSuite) SetupTest() {}

// --- sql.query validation ---

func (s *SQLActionTestSuite) TestQueryMissingQuery() {
	a := NewSQLQueryAction()
	locker := fakeruntime.NewLocker(s.T())
	ctx := newTestContextWithServices(map[string]any{"dsn": "postgres://localhost/db"}, locker)
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "query")
}

func (s *SQLActionTestSuite) TestQueryMissingDSNAndTx() {
	a := NewSQLQueryAction()
	locker := fakeruntime.NewLocker(s.T())
	ctx := newTestContextWithServices(map[string]any{"query": "SELECT 1"}, locker)
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "dsn")
}

func (s *SQLActionTestSuite) TestQueryNoServices() {
	a := NewSQLQueryAction()
	ctx := newTestContext(map[string]any{"dsn": "postgres://localhost/db", "query": "SELECT 1"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

// --- sql.exec validation ---

func (s *SQLActionTestSuite) TestExecMissingQuery() {
	a := NewSQLExecAction()
	locker := fakeruntime.NewLocker(s.T())
	ctx := newTestContextWithServices(map[string]any{"dsn": "postgres://localhost/db"}, locker)
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "query")
}

func (s *SQLActionTestSuite) TestExecMissingDSNAndTx() {
	a := NewSQLExecAction()
	locker := fakeruntime.NewLocker(s.T())
	ctx := newTestContextWithServices(map[string]any{"query": "DELETE FROM t"}, locker)
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "dsn")
}

func (s *SQLActionTestSuite) TestExecNoServices() {
	a := NewSQLExecAction()
	ctx := newTestContext(map[string]any{"dsn": "postgres://localhost/db", "query": "DELETE FROM t"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

// --- sql.begin validation ---

func (s *SQLActionTestSuite) TestBeginMissingDSN() {
	a := NewSQLBeginAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{"name": "tx1"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "dsn")
}

func (s *SQLActionTestSuite) TestBeginMissingName() {
	a := NewSQLBeginAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{"dsn": "postgres://localhost/db"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "name")
}

func (s *SQLActionTestSuite) TestBeginNoServices() {
	a := NewSQLBeginAction()
	ctx := newTestContext(map[string]any{"dsn": "postgres://localhost/db", "name": "tx1"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

// --- sql.commit validation ---

func (s *SQLActionTestSuite) TestCommitMissingName() {
	a := NewSQLCommitAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "name")
}

func (s *SQLActionTestSuite) TestCommitNoServices() {
	a := NewSQLCommitAction()
	ctx := newTestContext(map[string]any{"name": "tx1"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

// --- sql.rollback validation ---

func (s *SQLActionTestSuite) TestRollbackMissingName() {
	a := NewSQLRollbackAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "name")
}

func (s *SQLActionTestSuite) TestRollbackNoServices() {
	a := NewSQLRollbackAction()
	ctx := newTestContext(map[string]any{"name": "tx1"})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "serve")
}

// --- sql.query / sql.exec with tx (validation only) ---

func (s *SQLActionTestSuite) TestQueryWithTxValid() {
	a := NewSQLQueryAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{"tx": "my_tx", "query": "SELECT 1"})
	err := a.Validate(ctx)
	s.NoError(err)
}

func (s *SQLActionTestSuite) TestExecWithTxValid() {
	a := NewSQLExecAction()
	ctx := newTestContextWithAllServices(s.T(), map[string]any{"tx": "my_tx", "query": "DELETE FROM t"})
	err := a.Validate(ctx)
	s.NoError(err)
}

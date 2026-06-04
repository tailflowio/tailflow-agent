package fx

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/store"
	"github.com/tailflow/tailflow/internal/store/mariadb"
	uberfx "go.uber.org/fx"
)

type ExecutionStoreTestSuite struct {
	suite.Suite
}

func TestExecutionStoreTestSuite(t *testing.T) {
	suite.Run(t, new(ExecutionStoreTestSuite))
}

func (s *ExecutionStoreTestSuite) TestNewExecutionStore_DefaultsToMemoryWithCLICap() {
	out, err := NewExecutionStore(ExecutionStoreIn{
		Config:   Config{MaxExecs: 42},
		Workflow: &parser.Workflow{},
	})

	s.Require().NoError(err)
	s.Require().NotNil(out.Store)

	mem, ok := out.Store.(*store.MemoryExecutionStore)
	s.Require().True(ok, "default backend should be in-memory")
	_ = mem
}

func (s *ExecutionStoreTestSuite) TestNewExecutionStore_ExplicitMemoryType() {
	out, err := NewExecutionStore(ExecutionStoreIn{
		Config: Config{MaxExecs: 50},
		Workflow: &parser.Workflow{
			Persistence: &parser.Persistence{Type: parser.PersistenceMemory},
		},
	})

	s.Require().NoError(err)
	s.Require().NotNil(out.Store)
}

func (s *ExecutionStoreTestSuite) TestNewExecutionStore_MemoryMaxExecutionsOverridesCLIFlag() {
	out, err := NewExecutionStore(ExecutionStoreIn{
		Config: Config{MaxExecs: 100},
		Workflow: &parser.Workflow{
			Persistence: &parser.Persistence{
				Type:   parser.PersistenceMemory,
				Memory: &parser.MemoryPersistence{MaxExecutions: 7},
			},
		},
	})

	s.Require().NoError(err)
	mem, ok := out.Store.(*store.MemoryExecutionStore)
	s.Require().True(ok)
	// the YAML override (7) wins over the CLI flag (100)
	s.Equal(7, memoryCapacityFor(mem))
}

func (s *ExecutionStoreTestSuite) TestNewExecutionStore_MariaDBPropagatesConnectError() {
	// Bogus DSN points at a port nothing listens on; mariadb.New must fail
	// fast (Ping) and surface the error through the fx provider.
	_, err := NewExecutionStore(ExecutionStoreIn{
		Config: Config{MaxExecs: 100},
		Workflow: &parser.Workflow{
			Persistence: &parser.Persistence{
				Type:    parser.PersistenceMariaDB,
				MariaDB: &parser.MariaDBPersistence{DSN: "tailflow:nope@tcp(127.0.0.1:1)/tailflow?timeout=200ms"},
			},
		},
	})

	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb")
}

func (s *ExecutionStoreTestSuite) TestNewExecutionStore_ClickHouseTypeRejected() {
	_, err := NewExecutionStore(ExecutionStoreIn{
		Config:   Config{MaxExecs: 100},
		Workflow: &parser.Workflow{Persistence: &parser.Persistence{Type: "clickhouse"}},
	})

	s.Require().Error(err)
	s.Contains(err.Error(), "unknown backend")
}

func (s *ExecutionStoreTestSuite) TestNewExecutionStore_UnknownBackendRejected() {
	_, err := NewExecutionStore(ExecutionStoreIn{
		Config: Config{MaxExecs: 100},
		Workflow: &parser.Workflow{
			Persistence: &parser.Persistence{Type: "redis"},
		},
	})

	s.Require().Error(err)
	s.Contains(err.Error(), "unknown backend")
}

func (s *ExecutionStoreTestSuite) TestNewMariaDBStore_SuccessWithLifecycleHook() {
	db, _, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	s.Require().NoError(err)

	defer func() { _ = db.Close() }()

	st, stErr := mariadb.NewWithDB(db, "tf_")
	s.Require().NoError(stErr)

	original := mariaDBNewFn
	mariaDBNewFn = func(_ context.Context, _, _ string) (*mariadb.Store, error) {
		return st, nil
	}

	defer func() { mariaDBNewFn = original }()

	stopCalled := false

	lc := &fakeLifecycle{
		onAppend: func(hook uberfx.Hook) {
			if hook.OnStop != nil {
				stopCalled = true
				_ = hook.OnStop(context.Background())
			}
		},
	}

	out, execErr := newMariaDBStore(ExecutionStoreIn{
		Config:    Config{MaxExecs: 10},
		Lifecycle: lc,
		Workflow: &parser.Workflow{
			Persistence: &parser.Persistence{
				Type:    parser.PersistenceMariaDB,
				MariaDB: &parser.MariaDBPersistence{DSN: "fake"},
			},
		},
	})

	s.Require().NoError(execErr)
	s.Same(st, out.Store)
	s.True(stopCalled, "lifecycle OnStop hook must be registered and called")
}

func (s *ExecutionStoreTestSuite) TestNewMariaDBStore_SuccessWithNilLifecycle() {
	db, _, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	s.Require().NoError(err)

	defer func() { _ = db.Close() }()

	st, stErr := mariadb.NewWithDB(db, "tf_")
	s.Require().NoError(stErr)

	original := mariaDBNewFn
	mariaDBNewFn = func(_ context.Context, _, _ string) (*mariadb.Store, error) {
		return st, nil
	}

	defer func() { mariaDBNewFn = original }()

	out, execErr := newMariaDBStore(ExecutionStoreIn{
		Config:    Config{MaxExecs: 10},
		Lifecycle: nil,
		Workflow: &parser.Workflow{
			Persistence: &parser.Persistence{
				Type:    parser.PersistenceMariaDB,
				MariaDB: &parser.MariaDBPersistence{DSN: "fake"},
			},
		},
	})

	s.Require().NoError(execErr)
	s.Same(st, out.Store)
}

// fakeLifecycle is a minimal uberfx.Lifecycle stub for tests that need to
// inspect registered hooks without starting a full fx app.
type fakeLifecycle struct {
	onAppend func(hook uberfx.Hook)
}

func (f *fakeLifecycle) Append(hook uberfx.Hook) {
	if f.onAppend != nil {
		f.onAppend(hook)
	}
}

// memoryCapacityFor exposes the unexported capacity field for assertion only.
func memoryCapacityFor(s *store.MemoryExecutionStore) int {
	return s.Capacity()
}

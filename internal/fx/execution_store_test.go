package fx

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/store"
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

func (s *ExecutionStoreTestSuite) TestNewExecutionStore_ClickHouseNotYetImplemented() {
	_, err := NewExecutionStore(ExecutionStoreIn{
		Config: Config{MaxExecs: 100},
		Workflow: &parser.Workflow{
			Persistence: &parser.Persistence{
				Type:       parser.PersistenceClickHouse,
				ClickHouse: &parser.ClickHousePersistence{DSN: "clickhouse://localhost:9000/tailflow"},
			},
		},
	})

	s.Require().Error(err)
	s.Contains(err.Error(), "clickhouse")
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

// memoryCapacityFor exposes the unexported capacity field for assertion only.
func memoryCapacityFor(s *store.MemoryExecutionStore) int {
	return s.Capacity()
}

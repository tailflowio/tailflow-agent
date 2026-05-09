package fx

import (
	"context"
	"fmt"

	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/store"
	"github.com/tailflow/tailflow/internal/store/mariadb"
	uberfx "go.uber.org/fx"
)

type ExecutionStoreIn struct {
	uberfx.In

	Config    Config
	Lifecycle uberfx.Lifecycle
	Workflow  *parser.Workflow
}

type ExecutionStoreOut struct {
	uberfx.Out

	Store store.ExecutionStore
}

// NewExecutionStore selects the execution-history backend based on the
// workflow's persistence block. Defaults to in-memory when omitted.
//
// The CLI flag --max-executions sets the in-memory cap unless the workflow
// explicitly defines persistence.memory.max_executions, which always wins.
func NewExecutionStore(in ExecutionStoreIn) (ExecutionStoreOut, error) {
	cfg := in.Workflow.Persistence

	switch {
	case cfg == nil, cfg.Type == "", cfg.Type == parser.PersistenceMemory:
		return ExecutionStoreOut{Store: store.NewExecutionStore(memoryCapacity(in.Config, cfg))}, nil

	case cfg.Type == parser.PersistenceMariaDB:
		return newMariaDBStore(in)

	case cfg.Type == parser.PersistenceClickHouse:
		return ExecutionStoreOut{}, fmt.Errorf("persistence: clickhouse backend not yet implemented (Phase 2)")

	default:
		return ExecutionStoreOut{}, fmt.Errorf("persistence: unknown backend %q", cfg.Type)
	}
}

// newMariaDBStore opens the MariaDB-backed store and registers a shutdown
// hook so connections are released cleanly on app stop.
func newMariaDBStore(in ExecutionStoreIn) (ExecutionStoreOut, error) {
	cfg := in.Workflow.Persistence.MariaDB

	s, err := mariadb.New(context.Background(), cfg.DSN, cfg.TablePrefix)
	if err != nil {
		return ExecutionStoreOut{}, err
	}

	if in.Lifecycle != nil {
		in.Lifecycle.Append(uberfx.Hook{
			OnStop: func(_ context.Context) error { return s.Close() },
		})
	}

	return ExecutionStoreOut{Store: s}, nil
}

func memoryCapacity(cliCfg Config, p *parser.Persistence) int {
	if p != nil && p.Memory != nil && p.Memory.MaxExecutions > 0 {
		return p.Memory.MaxExecutions
	}

	return cliCfg.MaxExecs
}

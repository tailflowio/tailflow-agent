package fx

import (
	"fmt"

	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/store"
	uberfx "go.uber.org/fx"
)

type ExecutionStoreIn struct {
	uberfx.In

	Config   Config
	Workflow *parser.Workflow
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
		return ExecutionStoreOut{}, fmt.Errorf("persistence: mariadb backend not yet implemented (Phase 2)")

	case cfg.Type == parser.PersistenceClickHouse:
		return ExecutionStoreOut{}, fmt.Errorf("persistence: clickhouse backend not yet implemented (Phase 2)")

	default:
		return ExecutionStoreOut{}, fmt.Errorf("persistence: unknown backend %q", cfg.Type)
	}
}

func memoryCapacity(cliCfg Config, p *parser.Persistence) int {
	if p != nil && p.Memory != nil && p.Memory.MaxExecutions > 0 {
		return p.Memory.MaxExecutions
	}

	return cliCfg.MaxExecs
}

package fx

import (
	"github.com/tailflow/tailflow/internal/store"
	uberfx "go.uber.org/fx"
)

type ExecutionStoreIn struct {
	uberfx.In

	Config Config
}

type ExecutionStoreOut struct {
	uberfx.Out

	Store store.ExecutionStore
}

func NewExecutionStore(in ExecutionStoreIn) ExecutionStoreOut {
	return ExecutionStoreOut{Store: store.NewExecutionStore(in.Config.MaxExecs)}
}

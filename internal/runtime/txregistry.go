package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

type MemoryTxRegistry struct {
	mu  sync.Mutex
	txs map[string]*sql.Tx
}

func NewMemoryTxRegistry() *MemoryTxRegistry {
	return &MemoryTxRegistry{
		txs: make(map[string]*sql.Tx),
	}
}

func (r *MemoryTxRegistry) Begin(ctx context.Context, db *sql.DB, name string) (*sql.Tx, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.txs[name]; ok {
		return nil, fmt.Errorf("txregistry: transaction %q already exists", name)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("txregistry: begin %q: %w", name, err)
	}

	r.txs[name] = tx

	return tx, nil
}

func (r *MemoryTxRegistry) Get(name string) (*sql.Tx, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, ok := r.txs[name]
	if !ok {
		return nil, fmt.Errorf("txregistry: transaction %q not found", name)
	}

	return tx, nil
}

func (r *MemoryTxRegistry) Commit(name string) error {
	r.mu.Lock()

	tx, ok := r.txs[name]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("txregistry: transaction %q not found", name)
	}

	delete(r.txs, name)
	r.mu.Unlock()

	return tx.Commit()
}

func (r *MemoryTxRegistry) Rollback(name string) error {
	r.mu.Lock()

	tx, ok := r.txs[name]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("txregistry: transaction %q not found", name)
	}

	delete(r.txs, name)
	r.mu.Unlock()

	return tx.Rollback()
}

func (r *MemoryTxRegistry) RollbackAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, tx := range r.txs {
		_ = tx.Rollback()

		delete(r.txs, name)
	}
}

package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type MemoryDBPool struct {
	mu   sync.RWMutex
	pool map[string]*sql.DB
}

func NewMemoryDBPool() *MemoryDBPool {
	return &MemoryDBPool{
		pool: make(map[string]*sql.DB),
	}
}

func detectDriver(dsn string) (string, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return "pgx", nil
	}

	if strings.Contains(dsn, "@tcp(") {
		return "mysql", nil
	}

	return "", errors.New("dbpool: cannot detect driver for DSN (expected postgres:// or @tcp(...))")
}

func (p *MemoryDBPool) Get(ctx context.Context, dsn string) (*sql.DB, error) {
	// Fast path: read lock
	p.mu.RLock()
	db, ok := p.pool[dsn]
	p.mu.RUnlock()

	if ok {
		return db, nil
	}

	// Slow path: write lock, double-check
	p.mu.Lock()
	defer p.mu.Unlock()

	if cachedDB, ok := p.pool[dsn]; ok {
		return cachedDB, nil
	}

	driver, err := detectDriver(dsn)
	if err != nil {
		return nil, err
	}

	db, err = sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("dbpool: open %s: %w", driver, err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("dbpool: ping %s: %w", driver, err)
	}

	p.pool[dsn] = db

	return db, nil
}

func (p *MemoryDBPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var firstErr error
	for dsn, db := range p.pool {
		err := db.Close()
		if err != nil && firstErr == nil {
			firstErr = err
		}

		delete(p.pool, dsn)
	}

	return firstErr
}

package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// sqlOpenFn wraps sql.Open for testing.
var sqlOpenFn = sql.Open

// testHookAfterRLockMiss is called in Get after the RLock fast-path misses and before acquiring the write lock.
// It is nil in production; tests can set it to inject concurrent state between the two lock phases.
var testHookAfterRLockMiss func()

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

	if strings.HasPrefix(dsn, "mysql://") {
		return "mysql", nil
	}

	return "", errors.New("dbpool: cannot detect driver for DSN (expected postgres://, mysql:// or @tcp(...))")
}

func mysqlURLToDSN(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("dbpool: invalid mysql URL: %w", err)
	}

	host := u.Hostname()

	port := u.Port()
	if port == "" {
		port = "3306"
	}

	var userInfo string
	if u.User != nil {
		userInfo = u.User.String() + "@"
	}

	dbName := strings.TrimPrefix(u.Path, "/")
	query := u.RawQuery

	dsn := fmt.Sprintf("%stcp(%s:%s)/%s", userInfo, host, port, dbName)
	if query != "" {
		dsn += "?" + query
	}

	return dsn, nil
}

func (p *MemoryDBPool) Get(ctx context.Context, dsn string) (*sql.DB, error) {
	// Fast path: read lock
	p.mu.RLock()
	db, ok := p.pool[dsn]
	p.mu.RUnlock()

	if ok {
		return db, nil
	}

	if testHookAfterRLockMiss != nil {
		testHookAfterRLockMiss()
	}

	// Slow path: write lock, double-check
	p.mu.Lock()
	defer p.mu.Unlock()

	cachedDB, ok := p.pool[dsn]
	if ok {
		return cachedDB, nil
	}

	db, err := openAndPing(ctx, dsn)
	if err != nil {
		return nil, err
	}

	p.pool[dsn] = db

	return db, nil
}

func openAndPing(ctx context.Context, dsn string) (*sql.DB, error) {
	driver, err := detectDriver(dsn)
	if err != nil {
		return nil, err
	}

	openDSN := dsn
	if driver == "mysql" && strings.HasPrefix(dsn, "mysql://") {
		openDSN, err = mysqlURLToDSN(dsn)
		if err != nil {
			return nil, err
		}
	}

	db, err := sqlOpenFn(driver, openDSN)
	if err != nil {
		return nil, fmt.Errorf("dbpool: open %s: %w", driver, err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	err = db.PingContext(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("dbpool: ping %s: %w", driver, err)
	}

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

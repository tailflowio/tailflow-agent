package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/suite"
)

// ---- dbpool.go coverage: Get and Close ----

type DBPoolCoverageTestSuite struct {
	suite.Suite
}

func TestDBPoolCoverage(t *testing.T) {
	suite.Run(t, new(DBPoolCoverageTestSuite))
}

func (s *DBPoolCoverageTestSuite) SetupTest() {}

// TestGet_CacheHitFastPath covers the RLock fast path in Get when a DB
// is already cached.
func (s *DBPoolCoverageTestSuite) TestGet_CacheHitFastPath() {
	pool := NewMemoryDBPool()

	db, _, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	pool.mu.Lock()
	pool.pool["test-dsn"] = db
	pool.mu.Unlock()

	got, err := pool.Get(context.Background(), "test-dsn")
	s.NoError(err)
	s.Equal(db, got)
}

// TestGet_DoubleCheckAfterWriteLock covers the slow path double-check in Get
// when another goroutine inserts the DB between the RLock miss and the Lock acquire.
func (s *DBPoolCoverageTestSuite) TestGet_DoubleCheckAfterWriteLock() {
	pool := NewMemoryDBPool()

	db, _, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	origHook := testHookAfterRLockMiss
	defer func() { testHookAfterRLockMiss = origHook }()

	testHookAfterRLockMiss = func() {
		pool.mu.Lock()
		pool.pool["postgres://user:pass@localhost:5432/testdb"] = db
		pool.mu.Unlock()
	}

	got, err := pool.Get(context.Background(), "postgres://user:pass@localhost:5432/testdb")
	s.NoError(err)
	s.Equal(db, got)
}

// TestGet_UnknownDriver covers the branch where the DSN doesn't match any known driver.
func (s *DBPoolCoverageTestSuite) TestGet_UnknownDriver() {
	pool := NewMemoryDBPool()

	_, err := pool.Get(context.Background(), "unknown://localhost")
	s.Error(err)
	s.Contains(err.Error(), "cannot detect driver")
}

// TestGet_SQLOpenError covers the branch where sql.Open fails.
func (s *DBPoolCoverageTestSuite) TestGet_SQLOpenError() {
	pool := NewMemoryDBPool()

	origOpen := sqlOpenFn
	defer func() { sqlOpenFn = origOpen }()

	sqlOpenFn = func(driver, dsn string) (*sql.DB, error) {
		return nil, fmt.Errorf("open failed")
	}

	_, err := pool.Get(context.Background(), "postgres://user:pass@localhost:5432/db")
	s.Error(err)
	s.Contains(err.Error(), "open")
}

// TestGet_PingError covers the branch where db.PingContext fails.
func (s *DBPoolCoverageTestSuite) TestGet_PingError() {
	pool := NewMemoryDBPool()

	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	s.Require().NoError(err)

	mock.ExpectPing().WillReturnError(fmt.Errorf("ping failed"))
	mock.ExpectClose()

	origOpen := sqlOpenFn
	defer func() { sqlOpenFn = origOpen }()

	sqlOpenFn = func(driver, dsn string) (*sql.DB, error) {
		return db, nil
	}

	_, err = pool.Get(context.Background(), "postgres://user:pass@localhost:5432/db")
	s.Error(err)
	s.Contains(err.Error(), "ping")
}

// TestGet_Success covers the full happy path of Get creating a new connection.
func (s *DBPoolCoverageTestSuite) TestGet_Success() {
	pool := NewMemoryDBPool()

	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	s.Require().NoError(err)

	mock.ExpectPing()

	origOpen := sqlOpenFn
	defer func() { sqlOpenFn = origOpen }()

	sqlOpenFn = func(driver, dsn string) (*sql.DB, error) {
		return db, nil
	}

	got, err := pool.Get(context.Background(), "postgres://user:pass@localhost:5432/db")
	s.NoError(err)
	s.NotNil(got)

	// Second call should hit cache
	got2, err := pool.Get(context.Background(), "postgres://user:pass@localhost:5432/db")
	s.NoError(err)
	s.Equal(got, got2)
}

// TestClose_WithConnections covers the Close method when the pool has DB connections.
func (s *DBPoolCoverageTestSuite) TestClose_WithConnections() {
	pool := NewMemoryDBPool()

	db1, mock1, err := sqlmock.New()
	s.Require().NoError(err)
	mock1.ExpectClose()

	db2, mock2, err := sqlmock.New()
	s.Require().NoError(err)
	mock2.ExpectClose()

	pool.mu.Lock()
	pool.pool["dsn1"] = db1
	pool.pool["dsn2"] = db2
	pool.mu.Unlock()

	err = pool.Close()
	s.NoError(err)

	pool.mu.RLock()
	s.Len(pool.pool, 0)
	pool.mu.RUnlock()
}

// TestClose_FirstDBCloseReturnsError covers the branch where the first
// db.Close() call returns an error (firstErr is set).
func (s *DBPoolCoverageTestSuite) TestClose_FirstDBCloseReturnsError() {
	pool := NewMemoryDBPool()

	db, mock, err := sqlmock.New()
	s.Require().NoError(err)

	// ExpectClose with error makes db.Close() return an error
	mock.ExpectClose().WillReturnError(fmt.Errorf("close error"))

	pool.mu.Lock()
	pool.pool["dsn1"] = db
	pool.mu.Unlock()

	err = pool.Close()
	s.Error(err)
	s.Contains(err.Error(), "close error")

	pool.mu.RLock()
	s.Len(pool.pool, 0)
	pool.mu.RUnlock()
}

// ---- kv_memory.go coverage: Close method ----

type KVMemoryCoverageTestSuite struct {
	suite.Suite
}

func TestKVMemoryCoverage(t *testing.T) {
	suite.Run(t, new(KVMemoryCoverageTestSuite))
}

func (s *KVMemoryCoverageTestSuite) SetupTest() {}

// TestClose covers the Close method of MemoryKVStore.
func (s *KVMemoryCoverageTestSuite) TestClose() {
	store := NewMemoryKVStore()
	store.Set(context.Background(), "key", "value", 0)

	err := store.Close()
	s.NoError(err)
}

// ---- txregistry.go coverage: RollbackAll error logging ----

type TxRegistryCoverageTestSuite struct {
	suite.Suite
}

func TestTxRegistryCoverage(t *testing.T) {
	suite.Run(t, new(TxRegistryCoverageTestSuite))
}

func (s *TxRegistryCoverageTestSuite) SetupTest() {}

// TestRollbackAll_WithRollbackError covers the branch where tx.Rollback()
// returns an error during RollbackAll, which triggers a logger.Warn.
func (s *TxRegistryCoverageTestSuite) TestRollbackAll_WithRollbackError() {
	db, mock, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectRollback().WillReturnError(fmt.Errorf("rollback failed"))

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := NewMemoryTxRegistry(logger)

	_, err = r.Begin(context.Background(), db, "tx1")
	s.NoError(err)

	// RollbackAll should not panic even when rollback fails
	s.NotPanics(func() {
		r.RollbackAll()
	})

	// tx1 should be removed despite the rollback error
	_, err = r.Get("tx1")
	s.Error(err)
	s.Contains(err.Error(), "not found")

	s.NoError(mock.ExpectationsWereMet())
}

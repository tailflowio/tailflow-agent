package runtime

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/suite"
)

type MemoryTxRegistryTestSuite struct {
	suite.Suite
}

func TestMemoryTxRegistry(t *testing.T) {
	suite.Run(t, new(MemoryTxRegistryTestSuite))
}

func (s *MemoryTxRegistryTestSuite) SetupTest() {
	// required by convention
}

func (s *MemoryTxRegistryTestSuite) TestGetUnknown() {
	r := NewMemoryTxRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := r.Get("nonexistent")
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

func (s *MemoryTxRegistryTestSuite) TestCommitUnknown() {
	r := NewMemoryTxRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := r.Commit("nonexistent")
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

func (s *MemoryTxRegistryTestSuite) TestRollbackUnknown() {
	r := NewMemoryTxRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := r.Rollback("nonexistent")
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

func (s *MemoryTxRegistryTestSuite) TestRollbackAllEmpty() {
	r := NewMemoryTxRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	// Should not panic on empty registry
	s.NotPanics(func() {
		r.RollbackAll()
	}, "RollbackAll on empty registry should not panic")
}

func (s *MemoryTxRegistryTestSuite) TestBeginAndGet() {
	db, mock, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mock.ExpectBegin()

	r := NewMemoryTxRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	tx, err := r.Begin(context.Background(), db, "tx1")
	s.NoError(err)
	s.NotNil(tx)

	// Get should return the same tx
	got, err := r.Get("tx1")
	s.NoError(err)
	s.Equal(tx, got)

	s.NoError(mock.ExpectationsWereMet())
}

func (s *MemoryTxRegistryTestSuite) TestBeginDuplicate() {
	db, mock, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mock.ExpectBegin()

	r := NewMemoryTxRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err = r.Begin(context.Background(), db, "tx1")
	s.NoError(err)

	// Second begin with same name should fail
	_, err = r.Begin(context.Background(), db, "tx1")
	s.Error(err)
	s.Contains(err.Error(), "already exists")

	s.NoError(mock.ExpectationsWereMet())
}

func (s *MemoryTxRegistryTestSuite) TestBeginError() {
	db, mock, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mock.ExpectBegin().WillReturnError(fmt.Errorf("begin failed"))

	r := NewMemoryTxRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err = r.Begin(context.Background(), db, "tx1")
	s.Error(err)
	s.Contains(err.Error(), "begin")

	s.NoError(mock.ExpectationsWereMet())
}

func (s *MemoryTxRegistryTestSuite) TestCommitSuccess() {
	db, mock, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()

	r := NewMemoryTxRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err = r.Begin(context.Background(), db, "tx1")
	s.NoError(err)

	err = r.Commit("tx1")
	s.NoError(err)

	// Should be removed after commit
	_, err = r.Get("tx1")
	s.Error(err)
	s.Contains(err.Error(), "not found")

	s.NoError(mock.ExpectationsWereMet())
}

func (s *MemoryTxRegistryTestSuite) TestRollbackSuccess() {
	db, mock, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()

	r := NewMemoryTxRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err = r.Begin(context.Background(), db, "tx1")
	s.NoError(err)

	err = r.Rollback("tx1")
	s.NoError(err)

	// Should be removed after rollback
	_, err = r.Get("tx1")
	s.Error(err)
	s.Contains(err.Error(), "not found")

	s.NoError(mock.ExpectationsWereMet())
}

func (s *MemoryTxRegistryTestSuite) TestRollbackAllWithTxs() {
	db, mock, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectBegin()
	mock.ExpectRollback()
	mock.ExpectRollback()

	r := NewMemoryTxRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, _ = r.Begin(context.Background(), db, "tx1")
	_, _ = r.Begin(context.Background(), db, "tx2")

	r.RollbackAll()

	_, err = r.Get("tx1")
	s.Error(err)
	_, err = r.Get("tx2")
	s.Error(err)

	s.NoError(mock.ExpectationsWereMet())
}

package mariadb

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/suite"
)

type NewCloseTestSuite struct {
	suite.Suite

	ctx context.Context
}

func TestNewClose(t *testing.T) {
	suite.Run(t, new(NewCloseTestSuite))
}

func (s *NewCloseTestSuite) SetupTest() {
	s.ctx = context.Background()
}

// newMigrateMock creates a sqlmock configured for successful ping + migrate.
func newMigrateMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(
		sqlmock.MonitorPingsOption(true),
		sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp),
	)
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}

	return db, mock
}

// expectMigrateSuccess sets up all expectations for a successful migrate call.
func expectMigrateSuccess(mock sqlmock.Sqlmock, prefix string) {
	mock.ExpectPing()

	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS " + prefix + "executions")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS " + prefix + "events")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS " + prefix + "step_exec_counts")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec(regexp.QuoteMeta("ALTER TABLE " + prefix + "executions ADD COLUMN idempotency_key")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec(regexp.QuoteMeta("ADD UNIQUE KEY uk_" + prefix + "exec_idem")).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

func (s *NewCloseTestSuite) TestNew_SuccessWithExplicitPrefix() {
	db, mock := newMigrateMock(s.T())
	expectMigrateSuccess(mock, "tf_")

	original := sqlOpen
	sqlOpen = func(_ string, _ string) (*sql.DB, error) { return db, nil }
	defer func() { sqlOpen = original }()

	st, err := New(s.ctx, "mysql://ignored", "tf_")
	s.Require().NoError(err)
	s.Require().NotNil(st)
	s.Equal("tf_", st.prefix)

	s.NoError(mock.ExpectationsWereMet())
}

func (s *NewCloseTestSuite) TestNew_EmptyPrefixUsesDefault() {
	db, mock := newMigrateMock(s.T())
	expectMigrateSuccess(mock, DefaultTablePrefix)

	original := sqlOpen
	sqlOpen = func(_ string, _ string) (*sql.DB, error) { return db, nil }
	defer func() { sqlOpen = original }()

	st, err := New(s.ctx, "mysql://ignored", "")
	s.Require().NoError(err)
	s.Require().NotNil(st)
	s.Equal(DefaultTablePrefix, st.prefix)

	s.NoError(mock.ExpectationsWereMet())
}

func (s *NewCloseTestSuite) TestNew_InvalidPrefixReturnsError() {
	original := sqlOpen
	sqlOpen = func(_ string, _ string) (*sql.DB, error) {
		s.Fail("sqlOpen must not be called when prefix is invalid")
		return nil, nil
	}
	defer func() { sqlOpen = original }()

	_, err := New(s.ctx, "mysql://ignored", "bad-prefix!")
	s.Require().Error(err)
	s.Contains(err.Error(), "invalid table_prefix")
}

func (s *NewCloseTestSuite) TestNew_SqlOpenErrorReturns() {
	original := sqlOpen
	sqlOpen = func(_ string, _ string) (*sql.DB, error) {
		return nil, errors.New("driver registration failed")
	}
	defer func() { sqlOpen = original }()

	_, err := New(s.ctx, "mysql://ignored", "tf_")
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb open")
}

func (s *NewCloseTestSuite) TestNew_PingErrorReturns() {
	db, mock := newMigrateMock(s.T())
	mock.ExpectPing().WillReturnError(errors.New("connection refused"))

	original := sqlOpen
	sqlOpen = func(_ string, _ string) (*sql.DB, error) { return db, nil }
	defer func() { sqlOpen = original }()

	_, err := New(s.ctx, "mysql://ignored", "tf_")
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb ping")

	s.NoError(mock.ExpectationsWereMet())
}

func (s *NewCloseTestSuite) TestNew_MigrateErrorReturns() {
	db, mock := newMigrateMock(s.T())
	mock.ExpectPing()

	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS tf_executions")).
		WillReturnError(errors.New("disk full"))

	original := sqlOpen
	sqlOpen = func(_ string, _ string) (*sql.DB, error) { return db, nil }
	defer func() { sqlOpen = original }()

	_, err := New(s.ctx, "mysql://ignored", "tf_")
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb migrate")

	s.NoError(mock.ExpectationsWereMet())
}

func (s *NewCloseTestSuite) TestClose_ClosesDB() {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	s.Require().NoError(err)

	st, err := NewWithDB(db, "tf_")
	s.Require().NoError(err)

	mock.ExpectClose()

	closeErr := st.Close()
	s.Require().NoError(closeErr)
	s.NoError(mock.ExpectationsWereMet())
}

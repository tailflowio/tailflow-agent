package mariadb

import (
	"errors"
	"regexp"

	"github.com/DATA-DOG/go-sqlmock"
	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/tailflow/tailflow/internal/runtime"
)

func (s *StoreTestSuite) TestClaimExecution_FreshKeyClaims() {
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_executions")).
		WithArgs("exec-1", "wf", runtime.StatusRunning, sqlmock.AnyArg(), "key-1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := s.st.ClaimExecution(s.ctx, "exec-1", "wf", "key-1")
	s.Require().NoError(err)
	s.True(result.Claimed)
	s.Empty(result.ExistingExecutionID)
}

func (s *StoreTestSuite) TestClaimExecution_DuplicateReturnsExisting() {
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_executions")).
		WithArgs("exec-2", "wf", runtime.StatusRunning, sqlmock.AnyArg(), "key-1").
		WillReturnError(&mysqldriver.MySQLError{Number: 1062})

	rows := sqlmock.NewRows([]string{"id", "status"}).
		AddRow("exec-1", runtime.StatusRunning)

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, status")).
		WithArgs("wf", "key-1").
		WillReturnRows(rows)

	result, err := s.st.ClaimExecution(s.ctx, "exec-2", "wf", "key-1")
	s.Require().NoError(err)
	s.False(result.Claimed)
	s.Equal("exec-1", result.ExistingExecutionID)
	s.Equal(runtime.StatusRunning, result.ExistingStatus)
}

func (s *StoreTestSuite) TestClaimExecution_DuplicateSelectErrorPropagates() {
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_executions")).
		WithArgs("exec-2", "wf", runtime.StatusRunning, sqlmock.AnyArg(), "key-1").
		WillReturnError(&mysqldriver.MySQLError{Number: 1062})

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, status")).
		WithArgs("wf", "key-1").
		WillReturnError(errors.New("connection lost"))

	_, err := s.st.ClaimExecution(s.ctx, "exec-2", "wf", "key-1")
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb claim: lookup")
}

func (s *StoreTestSuite) TestClaimExecution_OtherErrorPropagates() {
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_executions")).
		WithArgs("exec-1", "wf", runtime.StatusRunning, sqlmock.AnyArg(), "key-1").
		WillReturnError(errors.New("connection lost"))

	_, err := s.st.ClaimExecution(s.ctx, "exec-1", "wf", "key-1")
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb claim")
}

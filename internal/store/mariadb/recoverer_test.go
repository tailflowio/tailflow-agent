package mariadb

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/suite"

	"github.com/tailflow/tailflow/internal/runtime"
)

var errMockQuery = errors.New("connection lost")

type RecovererTestSuite struct {
	suite.Suite

	ctx  context.Context
	db   *sql.DB
	mock sqlmock.Sqlmock
	st   *Store
}

func TestRecoverer(t *testing.T) {
	suite.Run(t, new(RecovererTestSuite))
}

func (s *RecovererTestSuite) SetupTest() {
	s.ctx = context.Background()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	s.Require().NoError(err)

	s.db = db
	s.mock = mock

	st, err := NewWithDB(db, testPrefix)
	s.Require().NoError(err)
	s.st = st
}

func (s *RecovererTestSuite) TearDownTest() {
	s.NoError(s.mock.ExpectationsWereMet())
	_ = s.db.Close()
}

const recoverProjectionRegexp = `SELECT\s+id,\s*workflow_name,\s*status,\s*params,\s*steps,\s*` +
	`started_at,\s*finished_at,\s*error_msg\s+FROM tf_executions\s+` +
	`WHERE status IN \(\?,\?\)\s+ORDER BY created_seq ASC`

func (s *RecovererTestSuite) TestRecoverExecutions_ReturnsNonTerminal() {
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).
		AddRow("exec-running", "wf", runtime.StatusRunning, nil, nil, now, nil, nil).
		AddRow("exec-waiting", "wf", runtime.StatusWaiting, nil, nil, now, nil, nil)

	s.mock.ExpectQuery(recoverProjectionRegexp).
		WithArgs(runtime.StatusRunning, runtime.StatusWaiting).
		WillReturnRows(rows)

	recovered, err := s.st.RecoverExecutions(s.ctx, "ignored-agent")
	s.Require().NoError(err)
	s.Require().Len(recovered, 2)
	s.Equal("exec-running", recovered[0].ExecutionID)
	s.Equal(runtime.StatusRunning, recovered[0].Status)
	s.Equal("exec-waiting", recovered[1].ExecutionID)
	s.Equal(runtime.StatusWaiting, recovered[1].Status)
}

func (s *RecovererTestSuite) TestRecoverExecutions_MapsParamsAndSteps() {
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow(
		"exec-running", "wf", runtime.StatusRunning,
		`{"env":"prod"}`, `{"step1":{"status":"running"}}`,
		now, nil, nil,
	)

	s.mock.ExpectQuery(recoverProjectionRegexp).
		WithArgs(runtime.StatusRunning, runtime.StatusWaiting).
		WillReturnRows(rows)

	recovered, err := s.st.RecoverExecutions(s.ctx, "")
	s.Require().NoError(err)
	s.Require().Len(recovered, 1)

	rec := recovered[0]
	s.Equal(map[string]any{"env": "prod"}, rec.Params)
	s.Require().Contains(rec.Steps, "step1")
	s.Equal(runtime.StatusRunning, rec.Steps["step1"].Status)
}

func (s *RecovererTestSuite) TestRecoverExecutions_EmptyResult() {
	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	})

	s.mock.ExpectQuery(recoverProjectionRegexp).
		WithArgs(runtime.StatusRunning, runtime.StatusWaiting).
		WillReturnRows(rows)

	recovered, err := s.st.RecoverExecutions(s.ctx, "")
	s.Require().NoError(err)
	s.Empty(recovered)
}

func (s *RecovererTestSuite) TestRecoverExecutions_QueryError() {
	s.mock.ExpectQuery(recoverProjectionRegexp).
		WithArgs(runtime.StatusRunning, runtime.StatusWaiting).
		WillReturnError(errMockQuery)

	_, err := s.st.RecoverExecutions(s.ctx, "")
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb recover")
}

func (s *RecovererTestSuite) TestRecoverExecutions_ScanDecodeError() {
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow(
		"exec-running", "wf", runtime.StatusRunning,
		`{invalid`, nil,
		now, nil, nil,
	)

	s.mock.ExpectQuery(recoverProjectionRegexp).
		WithArgs(runtime.StatusRunning, runtime.StatusWaiting).
		WillReturnRows(rows)

	_, err := s.st.RecoverExecutions(s.ctx, "")
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb recover")
}

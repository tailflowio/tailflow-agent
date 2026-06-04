package mariadb

import (
	"errors"
	"regexp"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

func (s *StoreTestSuite) TestMigrateIdempotencyColumn_ToleratesDuplicateColumn() {
	s.mock.ExpectExec(regexp.QuoteMeta("ALTER TABLE tf_executions ADD COLUMN idempotency_key")).
		WillReturnError(&mysqldriver.MySQLError{Number: mysqlErrDuplicateColumn})

	s.mock.ExpectExec(regexp.QuoteMeta("ADD UNIQUE KEY uk_tf_exec_idem")).
		WillReturnError(&mysqldriver.MySQLError{Number: mysqlErrDuplicateKeyName})

	err := migrateIdempotencyColumn(s.ctx, s.db, testPrefix)
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestMigrateIdempotencyColumn_AddsOnFreshTable() {
	s.mock.ExpectExec(regexp.QuoteMeta("ALTER TABLE tf_executions ADD COLUMN idempotency_key")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	s.mock.ExpectExec(regexp.QuoteMeta("ADD UNIQUE KEY uk_tf_exec_idem")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := migrateIdempotencyColumn(s.ctx, s.db, testPrefix)
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestMigrateIdempotencyColumn_PropagatesOtherError() {
	s.mock.ExpectExec(regexp.QuoteMeta("ALTER TABLE tf_executions ADD COLUMN idempotency_key")).
		WillReturnError(errors.New("connection lost"))

	err := migrateIdempotencyColumn(s.ctx, s.db, testPrefix)
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb migrate")
}

func (s *StoreTestSuite) TestUpdate_WritesBackSnapshot() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	s.mock.ExpectExec(regexp.QuoteMeta("UPDATE tf_executions")).
		WithArgs("wf", runtime.StatusSuccess, sqlmock.AnyArg(), nil, now, nil, nil, "exec-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.st.Update(s.ctx, &store.Execution{
		ID:           "exec-1",
		WorkflowName: "wf",
		Status:       runtime.StatusSuccess,
		Params:       map[string]any{"env": "prod"},
		StartedAt:    now,
	})
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestUpdate_PropagatesDriverError() {
	s.mock.ExpectExec(regexp.QuoteMeta("UPDATE tf_executions")).
		WillReturnError(errors.New("connection lost"))

	err := s.st.Update(s.ctx, &store.Execution{ID: "exec-1", StartedAt: time.Now()})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb update")
}

func (s *StoreTestSuite) TestUpdateStep_DerivesWaitingStatus() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	s.mock.ExpectBegin()

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow("exec-1", "wf", runtime.StatusRunning, nil, nil, now, nil, nil)

	s.mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).
		WithArgs("exec-1").
		WillReturnRows(rows)

	s.mock.ExpectExec(regexp.QuoteMeta("UPDATE tf_executions")).
		WithArgs("wf", runtime.StatusWaiting, nil, sqlmock.AnyArg(), now, nil, nil, "exec-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	err := s.st.UpdateStep(s.ctx, "exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusWaiting
	})
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestUpdateExecution_WriteBackErrorRollsBack() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	s.mock.ExpectBegin()

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow("exec-1", "wf", runtime.StatusRunning, nil, nil, now, nil, nil)

	s.mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).
		WithArgs("exec-1").
		WillReturnRows(rows)

	s.mock.ExpectExec(regexp.QuoteMeta("UPDATE tf_executions")).
		WillReturnError(errors.New("connection lost"))

	s.mock.ExpectRollback()

	err := s.st.UpdateExecution(s.ctx, "exec-1", func(exec *store.Execution) {
		exec.Status = runtime.StatusSuccess
	})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb write tx")
}

func (s *StoreTestSuite) TestGetEventsPaginated_NormalizesNegativeBounds() {
	ts := time.Now().UTC().Truncate(time.Microsecond)

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tf_events")).
		WithArgs("exec-1").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	rows := sqlmock.NewRows([]string{
		"execution_id", "event_type", "step_id", "message", "data", "timestamp",
	}).AddRow("exec-1", string(event.StepStarted), nil, nil, nil, ts)

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT execution_id, event_type")).
		WithArgs("exec-1").
		WillReturnRows(rows)

	events, total, err := s.st.GetEventsPaginated(s.ctx, "exec-1", -5, -10)
	s.Require().NoError(err)
	s.Equal(1, total)
	s.Len(events, 1)
}

func (s *StoreTestSuite) TestStepExecCounts_PropagatesScanError() {
	rows := sqlmock.NewRows([]string{"step_id", "cnt"}).
		AddRow("step1", "not-an-int")

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT step_id, cnt FROM tf_step_exec_counts")).
		WillReturnRows(rows)

	_, err := s.st.StepExecCounts(s.ctx)
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb step counts: scan")
}

func (s *StoreTestSuite) TestGet_PropagatesDecodeError() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow("exec-1", "wf", runtime.StatusRunning, `{invalid-json`, nil, now, nil, nil)

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, workflow_name")).
		WithArgs("exec-1").
		WillReturnRows(rows)

	_, err := s.st.Get(s.ctx, "exec-1")
	s.Require().Error(err)
	s.Contains(err.Error(), "decode params")
}

func (s *StoreTestSuite) TestEncodeJSON_NilAndEmptyYieldNull() {
	out, err := encodeJSON(nil)
	s.Require().NoError(err)
	s.Nil(out)

	var typedNilMap map[string]any

	out, err = encodeJSON(typedNilMap)
	s.Require().NoError(err)
	s.Nil(out)

	out, err = encodeJSON(map[string]any{})
	s.Require().NoError(err)
	s.Nil(out)

	out, err = encodeJSON([]string{})
	s.Require().NoError(err)
	s.Nil(out)
}

func (s *StoreTestSuite) TestEncodeJSON_NonEmptyMarshals() {
	out, err := encodeJSON(map[string]any{"env": "prod"})
	s.Require().NoError(err)
	s.JSONEq(`{"env":"prod"}`, string(out))
}

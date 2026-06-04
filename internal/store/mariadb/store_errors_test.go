package mariadb

import (
	"database/sql"
	"errors"
	"regexp"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

func (s *StoreTestSuite) TestMigrate_RunsSchemaThenAlters() {
	s.mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS tf_executions")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	s.mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS tf_events")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	s.mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS tf_step_exec_counts")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	s.mock.ExpectExec(regexp.QuoteMeta("ALTER TABLE tf_executions ADD COLUMN idempotency_key")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	s.mock.ExpectExec(regexp.QuoteMeta("ADD UNIQUE KEY uk_tf_exec_idem")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := migrate(s.ctx, s.db, testPrefix)
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestMigrate_PropagatesCreateTableError() {
	s.mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS tf_executions")).
		WillReturnError(errors.New("connection lost"))

	err := migrate(s.ctx, s.db, testPrefix)
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb migrate")
}

func (s *StoreTestSuite) TestAdd_PropagatesEncodeError() {
	err := s.st.Add(s.ctx, &store.Execution{
		ID:        "exec-1",
		Params:    map[string]any{"bad": make(chan int)},
		StartedAt: time.Now(),
	})
	s.Require().Error(err)
	s.Contains(err.Error(), "encode params")
}

func (s *StoreTestSuite) TestUpdate_PropagatesEncodeError() {
	err := s.st.Update(s.ctx, &store.Execution{
		ID:        "exec-1",
		Params:    map[string]any{"bad": make(chan int)},
		StartedAt: time.Now(),
	})
	s.Require().Error(err)
	s.Contains(err.Error(), "encode params")
}

func (s *StoreTestSuite) TestCount_PropagatesError() {
	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tf_executions")).
		WillReturnError(errors.New("connection lost"))

	_, err := s.st.Count(s.ctx)
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb count")
}

func (s *StoreTestSuite) TestList_PropagatesQueryError() {
	s.mock.ExpectQuery(regexp.QuoteMeta("ORDER BY created_seq DESC")).
		WillReturnError(errors.New("connection lost"))

	_, err := s.st.List(s.ctx)
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb list")
}

func (s *StoreTestSuite) TestList_PropagatesScanError() {
	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow("exec-1", "wf", runtime.StatusRunning, `{invalid`, nil, time.Now(), nil, nil)

	s.mock.ExpectQuery(regexp.QuoteMeta("ORDER BY created_seq DESC")).
		WillReturnRows(rows)

	_, err := s.st.List(s.ctx)
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb list: scan")
}

func (s *StoreTestSuite) TestAppendEvent_PropagatesError() {
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_events")).
		WillReturnError(errors.New("connection lost"))

	err := s.st.AppendEvent(s.ctx, "exec-1", event.Event{Type: event.StepStarted, Timestamp: time.Now()})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb append event")
}

func (s *StoreTestSuite) TestIncrStepExecCount_PropagatesError() {
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_step_exec_counts")).
		WillReturnError(errors.New("connection lost"))

	err := s.st.IncrStepExecCount(s.ctx, "step1")
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb incr step count")
}

func (s *StoreTestSuite) TestStepExecCounts_PropagatesQueryError() {
	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT step_id, cnt FROM tf_step_exec_counts")).
		WillReturnError(errors.New("connection lost"))

	_, err := s.st.StepExecCounts(s.ctx)
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb step counts")
}

func (s *StoreTestSuite) TestGetEvents_PropagatesCountError() {
	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tf_events")).
		WithArgs("exec-1").
		WillReturnError(errors.New("connection lost"))

	_, err := s.st.GetEvents(s.ctx, "exec-1")
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb events count")
}

func (s *StoreTestSuite) TestGetEvents_PropagatesScanError() {
	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tf_events")).
		WithArgs("exec-1").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	rows := sqlmock.NewRows([]string{
		"execution_id", "event_type", "step_id", "message", "data", "timestamp",
	}).AddRow("exec-1", string(event.StepStarted), nil, nil, `{invalid`, time.Now())

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT execution_id, event_type")).
		WithArgs("exec-1").
		WillReturnRows(rows)

	_, err := s.st.GetEvents(s.ctx, "exec-1")
	s.Require().Error(err)
	s.Contains(err.Error(), "decode event data")
}

func (s *StoreTestSuite) TestGetStepMetrics_ReturnsSingleStep() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow(
		"exec-1", "wf", runtime.StatusSuccess, nil,
		`{"step1":{"status":"success"}}`, now, nil, nil,
	)

	s.mock.ExpectQuery(regexp.QuoteMeta("ORDER BY created_seq DESC")).
		WillReturnRows(rows)

	m, err := s.st.GetStepMetrics(s.ctx, "step1")
	s.Require().NoError(err)
	s.Require().NotNil(m)
	s.Equal(1, m.TotalExecutions)
}

func (s *StoreTestSuite) TestGetStepMetrics_UnknownStepReturnsNil() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow("exec-1", "wf", runtime.StatusSuccess, nil, nil, now, nil, nil)

	s.mock.ExpectQuery(regexp.QuoteMeta("ORDER BY created_seq DESC")).
		WillReturnRows(rows)

	m, err := s.st.GetStepMetrics(s.ctx, "missing")
	s.Require().NoError(err)
	s.Nil(m)
}

func (s *StoreTestSuite) TestGetStepMetrics_PropagatesListError() {
	s.mock.ExpectQuery(regexp.QuoteMeta("ORDER BY created_seq DESC")).
		WillReturnError(errors.New("connection lost"))

	_, err := s.st.GetStepMetrics(s.ctx, "step1")
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb list")
}

func (s *StoreTestSuite) TestUpdateExecution_PropagatesBeginError() {
	s.mock.ExpectBegin().WillReturnError(errors.New("connection lost"))

	err := s.st.UpdateExecution(s.ctx, "exec-1", func(_ *store.Execution) {})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb update: begin")
}

func (s *StoreTestSuite) TestUpdateStep_PropagatesBeginError() {
	s.mock.ExpectBegin().WillReturnError(errors.New("connection lost"))

	err := s.st.UpdateStep(s.ctx, "exec-1", "step1", func(_ *runtime.StepResult) {})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb update step: begin")
}

func (s *StoreTestSuite) TestUpdateStep_NotFoundIsNoop() {
	s.mock.ExpectBegin()

	s.mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	s.mock.ExpectRollback()

	err := s.st.UpdateStep(s.ctx, "missing", "step1", func(_ *runtime.StepResult) {
		s.Fail("callback must not run when row missing")
	})
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestWriteExecutionTx_PropagatesEncodeError() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	s.mock.ExpectBegin()

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow("exec-1", "wf", runtime.StatusRunning, nil, nil, now, nil, nil)

	s.mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).
		WithArgs("exec-1").
		WillReturnRows(rows)

	s.mock.ExpectRollback()

	err := s.st.UpdateExecution(s.ctx, "exec-1", func(exec *store.Execution) {
		exec.Params = map[string]any{"bad": make(chan int)}
	})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb write tx: encode params")
}

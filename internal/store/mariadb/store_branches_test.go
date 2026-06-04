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

// ---- AppendEvent ----

func (s *StoreTestSuite) TestAppendEvent_PropagatesEncodeDataError() {
	err := s.st.AppendEvent(s.ctx, "exec-1", event.Event{
		Type:      event.StepStarted,
		Timestamp: time.Now(),
		Data:      map[string]any{"bad": make(chan int)},
	})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb append event: encode data")
}

// ---- queryEvents ----

func (s *StoreTestSuite) TestGetEvents_PropagatesQuerySelectError() {
	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tf_events")).
		WithArgs("exec-1").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT execution_id, event_type")).
		WithArgs("exec-1").
		WillReturnError(errors.New("connection lost"))

	_, err := s.st.GetEvents(s.ctx, "exec-1")
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb events")
}

func (s *StoreTestSuite) TestGetEvents_PropagatesRowsErrAfterScan() {
	ts := time.Now().UTC().Truncate(time.Microsecond)

	rows := sqlmock.NewRows([]string{
		"execution_id", "event_type", "step_id", "message", "data", "timestamp",
	}).
		AddRow("exec-1", string(event.StepStarted), "step1", nil, nil, ts).
		RowError(0, errors.New("network timeout"))

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tf_events")).
		WithArgs("exec-1").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT execution_id, event_type")).
		WithArgs("exec-1").
		WillReturnRows(rows)

	_, err := s.st.GetEvents(s.ctx, "exec-1")
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb events: rows")
}

// ---- StepExecCounts ----

func (s *StoreTestSuite) TestStepExecCounts_PropagatesRowsErr() {
	rows := sqlmock.NewRows([]string{"step_id", "cnt"}).
		AddRow("step1", 3).
		RowError(0, errors.New("network timeout"))

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT step_id, cnt FROM tf_step_exec_counts")).
		WillReturnRows(rows)

	_, err := s.st.StepExecCounts(s.ctx)
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb step counts: rows")
}

// ---- Add ----

func (s *StoreTestSuite) TestAdd_PropagatesEncodeStepsError() {
	err := s.st.Add(s.ctx, &store.Execution{
		ID:    "exec-1",
		Steps: map[string]*runtime.StepResult{"s1": {Output: make(chan int)}},
		StartedAt: time.Now(),
	})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb add: encode steps")
}

func (s *StoreTestSuite) TestAdd_ToleratesDuplicateEntry() {
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_executions")).
		WillReturnError(&mysqldriver.MySQLError{Number: mysqlErrDuplicateEntry})

	err := s.st.Add(s.ctx, &store.Execution{
		ID:        "exec-dup",
		StartedAt: time.Now(),
	})
	s.Require().NoError(err)
}

// ---- isDuplicateEntryErr ----

func (s *StoreTestSuite) TestIsDuplicateEntryErr_FalseForNonDuplicateMySQL() {
	result := isDuplicateEntryErr(&mysqldriver.MySQLError{Number: 1045})
	s.False(result)
}

func (s *StoreTestSuite) TestIsDuplicateEntryErr_TrueFor1062() {
	result := isDuplicateEntryErr(&mysqldriver.MySQLError{Number: mysqlErrDuplicateEntry})
	s.True(result)
}

// ---- Update ----

func (s *StoreTestSuite) TestUpdate_PropagatesEncodeStepsError() {
	err := s.st.Update(s.ctx, &store.Execution{
		ID:    "exec-1",
		Steps: map[string]*runtime.StepResult{"s1": {Output: make(chan int)}},
		StartedAt: time.Now(),
	})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb update: encode steps")
}

// ---- UpdateExecution ----

func (s *StoreTestSuite) TestUpdateExecution_PropagatesLockError() {
	s.mock.ExpectBegin()

	s.mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).
		WithArgs("exec-1").
		WillReturnError(errors.New("lock timeout"))

	s.mock.ExpectRollback()

	err := s.st.UpdateExecution(s.ctx, "exec-1", func(_ *store.Execution) {})
	s.Require().Error(err)
	s.Contains(err.Error(), "lock timeout")
}

func (s *StoreTestSuite) TestUpdateExecution_PropagatesCommitError() {
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
		WithArgs("wf", runtime.StatusRunning, nil, nil, now, nil, nil, "exec-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit().WillReturnError(errors.New("commit failed"))

	err := s.st.UpdateExecution(s.ctx, "exec-1", func(_ *store.Execution) {})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb update: commit")
}

// ---- UpdateStep ----

func (s *StoreTestSuite) TestUpdateStep_PropagatesLockError() {
	s.mock.ExpectBegin()

	s.mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).
		WithArgs("exec-1").
		WillReturnError(errors.New("lock timeout"))

	s.mock.ExpectRollback()

	err := s.st.UpdateStep(s.ctx, "exec-1", "step1", func(_ *runtime.StepResult) {})
	s.Require().Error(err)
	s.Contains(err.Error(), "lock timeout")
}

func (s *StoreTestSuite) TestUpdateStep_PropagatesWriteBackError() {
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
		WillReturnError(errors.New("write failed"))

	s.mock.ExpectRollback()

	err := s.st.UpdateStep(s.ctx, "exec-1", "step1", func(_ *runtime.StepResult) {})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb write tx")
}

func (s *StoreTestSuite) TestUpdateStep_PropagatesCommitError() {
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
		WithArgs("wf", runtime.StatusRunning, nil, sqlmock.AnyArg(), now, nil, nil, "exec-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit().WillReturnError(errors.New("commit failed"))

	err := s.st.UpdateStep(s.ctx, "exec-1", "step1", func(_ *runtime.StepResult) {})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb update step: commit")
}

// ---- writeExecutionTx ----

func (s *StoreTestSuite) TestWriteExecutionTx_PropagatesEncodeStepsError() {
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
		exec.Steps = map[string]*runtime.StepResult{"s1": {Output: make(chan int)}}
	})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb write tx: encode steps")
}

// ---- List ----

func (s *StoreTestSuite) TestList_PropagatesRowsErr() {
	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).
		AddRow("exec-1", "wf", runtime.StatusSuccess, nil, nil, time.Now(), nil, nil).
		RowError(0, errors.New("network timeout"))

	s.mock.ExpectQuery(regexp.QuoteMeta("ORDER BY created_seq DESC")).
		WillReturnRows(rows)

	_, err := s.st.List(s.ctx)
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb list: rows")
}

// ---- deriveExecutionStatus ----

func (s *StoreTestSuite) TestDeriveExecutionStatus_TerminalStatusIsNoop() {
	exec := &store.Execution{
		Status: runtime.StatusSuccess,
		Steps:  map[string]*runtime.StepResult{"s1": {Status: runtime.StatusRunning}},
	}
	deriveExecutionStatus(exec)
	s.Equal(runtime.StatusSuccess, exec.Status)
}

func (s *StoreTestSuite) TestDeriveExecutionStatus_FailedStatusIsNoop() {
	exec := &store.Execution{
		Status: runtime.StatusFailed,
		Steps:  map[string]*runtime.StepResult{"s1": {Status: runtime.StatusRunning}},
	}
	deriveExecutionStatus(exec)
	s.Equal(runtime.StatusFailed, exec.Status)
}

// ---- RecoverExecutions ----

func (s *RecovererTestSuite) TestRecoverExecutions_PropagatesRowsErr() {
	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).
		AddRow("exec-1", "wf", runtime.StatusRunning, nil, nil, time.Now(), nil, nil).
		RowError(0, errors.New("network timeout"))

	s.mock.ExpectQuery(recoverProjectionRegexp).
		WithArgs(runtime.StatusRunning, runtime.StatusWaiting).
		WillReturnRows(rows)

	_, err := s.st.RecoverExecutions(s.ctx, "ignored")
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb recover: rows")
}

// ---- migrate ----

func (s *StoreTestSuite) TestMigrate_PropagatesIdempotencyColumnError() {
	s.mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS tf_executions")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	s.mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS tf_events")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	s.mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS tf_step_exec_counts")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	s.mock.ExpectExec(regexp.QuoteMeta("ALTER TABLE tf_executions ADD COLUMN idempotency_key")).
		WillReturnError(errors.New("alter failed"))

	err := migrate(s.ctx, s.db, testPrefix)
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb migrate")
}

// ---- scanExecution ----

func (s *StoreTestSuite) TestScanExecution_PropagatesDecodeStepsError() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow("exec-1", "wf", runtime.StatusRunning, nil, `{invalid-steps-json`, now, nil, nil)

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, workflow_name")).
		WithArgs("exec-1").
		WillReturnRows(rows)

	_, err := s.st.Get(s.ctx, "exec-1")
	s.Require().Error(err)
	s.Contains(err.Error(), "decode steps")
}

// ---- scanEvent ----

func (s *StoreTestSuite) TestScanEvent_PropagatesScanTypeFailure() {
	rows := sqlmock.NewRows([]string{
		"execution_id", "event_type", "step_id", "message", "data", "timestamp",
	}).AddRow("exec-1", "bad-type", nil, nil, nil, "not-a-time")

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tf_events")).
		WithArgs("exec-1").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT execution_id, event_type")).
		WithArgs("exec-1").
		WillReturnRows(rows)

	_, err := s.st.GetEvents(s.ctx, "exec-1")
	s.Require().Error(err)
}

func (s *StoreTestSuite) TestScanEvent_SetsMessageWhenValid() {
	ts := time.Now().UTC().Truncate(time.Microsecond)

	rows := sqlmock.NewRows([]string{
		"execution_id", "event_type", "step_id", "message", "data", "timestamp",
	}).AddRow("exec-1", string(event.WorkflowStarted), nil, "hello message", nil, ts)

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tf_events")).
		WithArgs("exec-1").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT execution_id, event_type")).
		WithArgs("exec-1").
		WillReturnRows(rows)

	events, err := s.st.GetEvents(s.ctx, "exec-1")
	s.Require().NoError(err)
	s.Require().Len(events, 1)
	s.Equal("hello message", events[0].Message)
}

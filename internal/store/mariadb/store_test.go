package mariadb

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/suite"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

const testPrefix = "tf_"

type StoreTestSuite struct {
	suite.Suite

	ctx  context.Context
	db   *sql.DB
	mock sqlmock.Sqlmock
	st   *Store
}

func TestStore(t *testing.T) {
	suite.Run(t, new(StoreTestSuite))
}

func (s *StoreTestSuite) SetupTest() {
	s.ctx = context.Background()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	s.Require().NoError(err)

	s.db = db
	s.mock = mock

	st, err := NewWithDB(db, testPrefix)
	s.Require().NoError(err)
	s.st = st
}

func (s *StoreTestSuite) TearDownTest() {
	s.NoError(s.mock.ExpectationsWereMet())
	_ = s.db.Close()
}

func (s *StoreTestSuite) TestValidatePrefix_RejectsSpecialChars() {
	_, err := NewWithDB(s.db, "tf-bad;DROP")
	s.Require().Error(err)
	s.Contains(err.Error(), "invalid table_prefix")
}

func (s *StoreTestSuite) TestValidatePrefix_AcceptsEmpty() {
	st, err := NewWithDB(s.db, "")
	s.Require().NoError(err)
	s.Equal(DefaultTablePrefix, st.prefix)
}

func (s *StoreTestSuite) TestSchemaStatements_ContainsExpectedTables() {
	stmts := schemaStatements("tf_")
	s.Require().Len(stmts, 3)
	s.Contains(stmts[0], "CREATE TABLE IF NOT EXISTS tf_executions")
	s.Contains(stmts[1], "CREATE TABLE IF NOT EXISTS tf_events")
	s.Contains(stmts[2], "CREATE TABLE IF NOT EXISTS tf_step_exec_counts")
}

func (s *StoreTestSuite) TestAdd_InsertsExecution() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_executions")).
		WithArgs("exec-1", "wf", runtime.StatusRunning,
			sqlmock.AnyArg(), nil, now, nil, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := s.st.Add(s.ctx, &store.Execution{
		ID:           "exec-1",
		WorkflowName: "wf",
		Status:       runtime.StatusRunning,
		Params:       map[string]any{"env": "prod"},
		StartedAt:    now,
	})

	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestAdd_PropagatesDriverError() {
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_executions")).
		WillReturnError(errors.New("connection lost"))

	err := s.st.Add(s.ctx, &store.Execution{ID: "exec-1", StartedAt: time.Now()})
	s.Require().Error(err)
	s.Contains(err.Error(), "mariadb add")
}

func (s *StoreTestSuite) TestGet_ReturnsExecution() {
	now := time.Now().UTC().Truncate(time.Microsecond)
	finished := now.Add(2 * time.Second)

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow(
		"exec-1", "wf", runtime.StatusSuccess,
		`{"env":"prod"}`, `{"step1":{"status":"success"}}`,
		now, finished, nil,
	)

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, workflow_name")).
		WithArgs("exec-1").
		WillReturnRows(rows)

	exec, err := s.st.Get(s.ctx, "exec-1")
	s.Require().NoError(err)
	s.Equal("exec-1", exec.ID)
	s.Equal(runtime.StatusSuccess, exec.Status)
	s.Equal("prod", exec.Params["env"])
	s.Require().NotNil(exec.Steps["step1"])
	s.Equal(runtime.StatusSuccess, exec.Steps["step1"].Status)
	s.Require().NotNil(exec.FinishedAt)
}

func (s *StoreTestSuite) TestGet_NotFoundWrapsErrNotFound() {
	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, workflow_name")).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err := s.st.Get(s.ctx, "missing")
	s.Require().Error(err)
	s.True(errors.Is(err, store.ErrNotFound), "should wrap store.ErrNotFound")
}

func (s *StoreTestSuite) TestList_ReturnsNewestFirst() {
	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).
		AddRow("exec-2", "wf", runtime.StatusSuccess, nil, nil, time.Now(), nil, nil).
		AddRow("exec-1", "wf", runtime.StatusFailed, nil, nil, time.Now(), nil, "boom")

	s.mock.ExpectQuery(regexp.QuoteMeta("ORDER BY created_seq DESC")).
		WillReturnRows(rows)

	out, err := s.st.List(s.ctx)
	s.Require().NoError(err)
	s.Len(out, 2)
	s.Equal("exec-2", out[0].ID)
	s.Equal("exec-1", out[1].ID)
	s.Equal("boom", out[1].Error)
}

func (s *StoreTestSuite) TestCount_ReturnsScalar() {
	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tf_executions")).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(7))

	n, err := s.st.Count(s.ctx)
	s.Require().NoError(err)
	s.Equal(7, n)
}

func (s *StoreTestSuite) TestAppendEvent_Inserts() {
	ts := time.Now().UTC().Truncate(time.Microsecond)

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_events")).
		WithArgs("exec-1", string(event.StepStarted), "step1", nil, sqlmock.AnyArg(), ts).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := s.st.AppendEvent(s.ctx, "exec-1", event.Event{
		Type:        event.StepStarted,
		Timestamp:   ts,
		ExecutionID: "exec-1",
		StepID:      "step1",
		Data:        map[string]any{"a": 1},
	})

	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestGetEvents_ReturnsAllRows() {
	ts := time.Now().UTC().Truncate(time.Microsecond)

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tf_events")).
		WithArgs("exec-1").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(2))

	rows := sqlmock.NewRows([]string{
		"execution_id", "event_type", "step_id", "message", "data", "timestamp",
	}).
		AddRow("exec-1", string(event.StepStarted), "step1", nil, nil, ts).
		AddRow("exec-1", string(event.StepCompleted), "step1", nil, nil, ts.Add(time.Second))

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT execution_id, event_type")).
		WithArgs("exec-1").
		WillReturnRows(rows)

	events, err := s.st.GetEvents(s.ctx, "exec-1")
	s.Require().NoError(err)
	s.Require().Len(events, 2)
	s.Equal(event.StepStarted, events[0].Type)
	s.Equal(event.StepCompleted, events[1].Type)
}

func (s *StoreTestSuite) TestGetEvents_EmptyShortCircuitsSecondQuery() {
	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tf_events")).
		WithArgs("exec-empty").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))

	events, err := s.st.GetEvents(s.ctx, "exec-empty")
	s.Require().NoError(err)
	s.Empty(events)
}

func (s *StoreTestSuite) TestGetEventsPaginated_AppliesLimitOffset() {
	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tf_events")).
		WithArgs("exec-1").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(50))

	rows := sqlmock.NewRows([]string{
		"execution_id", "event_type", "step_id", "message", "data", "timestamp",
	}).AddRow("exec-1", string(event.StepStarted), nil, nil, nil, time.Now())

	s.mock.ExpectQuery(regexp.QuoteMeta("LIMIT ? OFFSET ?")).
		WithArgs("exec-1", 10, 5).
		WillReturnRows(rows)

	events, total, err := s.st.GetEventsPaginated(s.ctx, "exec-1", 5, 10)
	s.Require().NoError(err)
	s.Equal(50, total)
	s.Len(events, 1)
}

func (s *StoreTestSuite) TestIncrStepExecCount_UpsertsRow() {
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_step_exec_counts")).
		WithArgs("step1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.st.IncrStepExecCount(s.ctx, "step1")
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestStepExecCounts_ReturnsMap() {
	rows := sqlmock.NewRows([]string{"step_id", "cnt"}).
		AddRow("step1", 5).
		AddRow("step2", 2)

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT step_id, cnt FROM tf_step_exec_counts")).
		WillReturnRows(rows)

	counts, err := s.st.StepExecCounts(s.ctx)
	s.Require().NoError(err)
	s.Equal(5, counts["step1"])
	s.Equal(2, counts["step2"])
}

func (s *StoreTestSuite) TestUpdateExecution_LocksAndWritesBack() {
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
		WithArgs("wf", runtime.StatusSuccess, nil, nil, now, sqlmock.AnyArg(), nil, "exec-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	err := s.st.UpdateExecution(s.ctx, "exec-1", func(exec *store.Execution) {
		exec.Status = runtime.StatusSuccess
		t := now.Add(time.Second)
		exec.FinishedAt = &t
	})
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestUpdateExecution_NotFoundIsNoop() {
	s.mock.ExpectBegin()

	s.mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	s.mock.ExpectRollback()

	err := s.st.UpdateExecution(s.ctx, "missing", func(_ *store.Execution) {
		s.Fail("callback must not run when row missing")
	})
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestUpdateStep_AddsStepAndDerivesStatus() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	s.mock.ExpectBegin()

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow("exec-1", "wf", runtime.StatusWaiting, nil, nil, now, nil, nil)

	s.mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).
		WithArgs("exec-1").
		WillReturnRows(rows)

	// Capture the steps payload for assertion (auto-derive should bump status to running).
	s.mock.ExpectExec(regexp.QuoteMeta("UPDATE tf_executions")).
		WithArgs("wf", runtime.StatusRunning, nil, sqlmock.AnyArg(), now, nil, nil, "exec-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	err := s.st.UpdateStep(s.ctx, "exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusRunning
	})
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestGetAllStepMetrics_AggregatesOverList() {
	now := time.Now().UTC().Truncate(time.Microsecond)
	stepStartedAt := now.Add(-100 * time.Millisecond)
	stepFinishedAt := now

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow(
		"exec-1", "wf", runtime.StatusSuccess, nil,
		`{"step1":{"status":"success","started_at":"`+
			stepStartedAt.Format(time.RFC3339Nano)+`","finished_at":"`+
			stepFinishedAt.Format(time.RFC3339Nano)+`"}}`,
		now, nil, nil,
	)

	s.mock.ExpectQuery(regexp.QuoteMeta("ORDER BY created_seq DESC")).
		WillReturnRows(rows)

	metrics, err := s.st.GetAllStepMetrics(s.ctx)
	s.Require().NoError(err)
	s.Require().NotNil(metrics["step1"])
	s.Equal(1, metrics["step1"].TotalExecutions)
	s.Equal(1, metrics["step1"].SuccessCount)
	s.Equal(0, metrics["step1"].FailureCount)
	s.GreaterOrEqual(metrics["step1"].AvgDurationMs, int64(50))
}

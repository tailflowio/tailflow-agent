package clickhouse

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

func (s *StoreTestSuite) TestValidatePrefix_DefaultsWhenEmpty() {
	st, err := NewWithDB(s.db, "")
	s.Require().NoError(err)
	s.Equal(DefaultTablePrefix, st.prefix)
}

func (s *StoreTestSuite) TestSchemaStatements_ContainsExpectedEngines() {
	stmts := schemaStatements("tf_")
	s.Require().Len(stmts, 3)
	s.Contains(stmts[0], "ReplacingMergeTree(version)")
	s.Contains(stmts[0], "tf_executions")
	s.Contains(stmts[1], "MergeTree")
	s.Contains(stmts[1], "tf_events")
	s.Contains(stmts[2], "SummingMergeTree(cnt)")
	s.Contains(stmts[2], "tf_step_exec_counts")
}

func (s *StoreTestSuite) TestEncodeJSON_NilAndEmptyReturnEmptyString() {
	out, err := encodeJSON(nil)
	s.NoError(err)
	s.Empty(out)

	var typedNilMap map[string]any

	out, err = encodeJSON(typedNilMap)
	s.NoError(err)
	s.Empty(out)

	out, err = encodeJSON(map[string]any{})
	s.NoError(err)
	s.Empty(out)
}

func (s *StoreTestSuite) TestEncodeJSON_NonEmptyMarshals() {
	out, err := encodeJSON(map[string]any{"a": 1})
	s.NoError(err)
	s.JSONEq(`{"a":1}`, out)
}

func (s *StoreTestSuite) TestAdd_InsertsExecution() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_executions")).
		WithArgs("exec-1", "wf", runtime.StatusRunning,
			sqlmock.AnyArg(), "", now, sql.NullTime{}, "", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.st.Add(s.ctx, &store.Execution{
		ID:           "exec-1",
		WorkflowName: "wf",
		Status:       runtime.StatusRunning,
		Params:       map[string]any{"env": "prod"},
		StartedAt:    now,
	})
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestGet_UsesFINAL() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow("exec-1", "wf", runtime.StatusSuccess,
		`{"env":"prod"}`, `{"step1":{"status":"success"}}`,
		now, now, "")

	s.mock.ExpectQuery("FROM tf_executions FINAL WHERE id = ?").
		WithArgs("exec-1").
		WillReturnRows(rows)

	exec, err := s.st.Get(s.ctx, "exec-1")
	s.Require().NoError(err)
	s.Equal("exec-1", exec.ID)
	s.Equal("prod", exec.Params["env"])
	s.Require().NotNil(exec.Steps["step1"])
	s.Equal(runtime.StatusSuccess, exec.Steps["step1"].Status)
}

func (s *StoreTestSuite) TestGet_NotFoundWrapsErrNotFound() {
	s.mock.ExpectQuery("FROM tf_executions FINAL").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err := s.st.Get(s.ctx, "missing")
	s.Require().Error(err)
	s.True(errors.Is(err, store.ErrNotFound))
}

func (s *StoreTestSuite) TestUpdateExecution_ReadModifyInsertNewVersion() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow("exec-1", "wf", runtime.StatusRunning, "", "", now, sql.NullTime{}, "")

	s.mock.ExpectQuery("FROM tf_executions FINAL").
		WithArgs("exec-1").
		WillReturnRows(rows)

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_executions")).
		WithArgs("exec-1", "wf", runtime.StatusSuccess,
			"", "", now, sqlmock.AnyArg(), "", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.st.UpdateExecution(s.ctx, "exec-1", func(e *store.Execution) {
		e.Status = runtime.StatusSuccess
		t := now.Add(time.Second)
		e.FinishedAt = &t
	})
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestUpdateExecution_NotFoundIsNoop() {
	s.mock.ExpectQuery("FROM tf_executions FINAL").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	err := s.st.UpdateExecution(s.ctx, "missing", func(_ *store.Execution) {
		s.Fail("callback must not run when row missing")
	})
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestUpdateStep_AutoDerivesStatus() {
	now := time.Now().UTC().Truncate(time.Microsecond)

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow("exec-1", "wf", runtime.StatusWaiting, "", "", now, sql.NullTime{}, "")

	s.mock.ExpectQuery("FROM tf_executions FINAL").
		WithArgs("exec-1").
		WillReturnRows(rows)

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_executions")).
		WithArgs("exec-1", "wf", runtime.StatusRunning,
			"", sqlmock.AnyArg(), now, sql.NullTime{}, "", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.st.UpdateStep(s.ctx, "exec-1", "step1", func(step *runtime.StepResult) {
		step.Status = runtime.StatusRunning
	})
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestList_NewestFirst() {
	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).
		AddRow("exec-2", "wf", runtime.StatusSuccess, "", "", time.Now(), sql.NullTime{}, "").
		AddRow("exec-1", "wf", runtime.StatusFailed, "", "", time.Now(), sql.NullTime{}, "boom")

	s.mock.ExpectQuery("ORDER BY started_at DESC").
		WillReturnRows(rows)

	out, err := s.st.List(s.ctx)
	s.Require().NoError(err)
	s.Len(out, 2)
	s.Equal("exec-2", out[0].ID)
	s.Equal("boom", out[1].Error)
}

func (s *StoreTestSuite) TestCount_UsesFINAL() {
	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT count() FROM tf_executions FINAL")).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(42))

	n, err := s.st.Count(s.ctx)
	s.Require().NoError(err)
	s.Equal(42, n)
}

func (s *StoreTestSuite) TestAppendEvent_InsertsWithGeneratedSeq() {
	ts := time.Now().UTC().Truncate(time.Microsecond)

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_events")).
		WithArgs(
			sqlmock.AnyArg(), // seq
			"exec-1", string(event.StepStarted),
			"step1", "", sqlmock.AnyArg(), ts,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.st.AppendEvent(s.ctx, "exec-1", event.Event{
		Type:        event.StepStarted,
		Timestamp:   ts,
		ExecutionID: "exec-1",
		StepID:      "step1",
		Data:        map[string]any{"a": 1},
	})
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestGetEvents_OrdersBySeq() {
	ts := time.Now().UTC().Truncate(time.Microsecond)

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT count() FROM tf_events")).
		WithArgs("exec-1").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(2))

	rows := sqlmock.NewRows([]string{
		"execution_id", "event_type", "step_id", "message", "data", "timestamp",
	}).
		AddRow("exec-1", string(event.StepStarted), "step1", "", "", ts).
		AddRow("exec-1", string(event.StepCompleted), "step1", "", "", ts.Add(time.Second))

	s.mock.ExpectQuery("ORDER BY seq ASC").
		WithArgs("exec-1").
		WillReturnRows(rows)

	events, err := s.st.GetEvents(s.ctx, "exec-1")
	s.Require().NoError(err)
	s.Require().Len(events, 2)
	s.Equal(event.StepStarted, events[0].Type)
	s.Equal(event.StepCompleted, events[1].Type)
}

func (s *StoreTestSuite) TestGetEvents_EmptyShortCircuits() {
	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT count() FROM tf_events")).
		WithArgs("exec-empty").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))

	events, err := s.st.GetEvents(s.ctx, "exec-empty")
	s.Require().NoError(err)
	s.Empty(events)
}

func (s *StoreTestSuite) TestGetEventsPaginated_AppliesLimitOffset() {
	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT count() FROM tf_events")).
		WithArgs("exec-1").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(50))

	rows := sqlmock.NewRows([]string{
		"execution_id", "event_type", "step_id", "message", "data", "timestamp",
	}).AddRow("exec-1", string(event.StepStarted), "", "", "", time.Now())

	s.mock.ExpectQuery(regexp.QuoteMeta("LIMIT ? OFFSET ?")).
		WithArgs("exec-1", 10, 5).
		WillReturnRows(rows)

	events, total, err := s.st.GetEventsPaginated(s.ctx, "exec-1", 5, 10)
	s.Require().NoError(err)
	s.Equal(50, total)
	s.Len(events, 1)
}

func (s *StoreTestSuite) TestIncrStepExecCount_InsertsPlusOne() {
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tf_step_exec_counts")).
		WithArgs("step1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.st.IncrStepExecCount(s.ctx, "step1")
	s.Require().NoError(err)
}

func (s *StoreTestSuite) TestStepExecCounts_AggregatesOnRead() {
	rows := sqlmock.NewRows([]string{"step_id", "total"}).
		AddRow("step1", 5).
		AddRow("step2", 2)

	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT step_id, sum(cnt)")).
		WillReturnRows(rows)

	counts, err := s.st.StepExecCounts(s.ctx)
	s.Require().NoError(err)
	s.Equal(5, counts["step1"])
	s.Equal(2, counts["step2"])
}

func (s *StoreTestSuite) TestGetAllStepMetrics_AggregatesOverList() {
	now := time.Now().UTC().Truncate(time.Microsecond)
	stepStartedAt := now.Add(-50 * time.Millisecond)
	stepFinishedAt := now

	rows := sqlmock.NewRows([]string{
		"id", "workflow_name", "status", "params", "steps",
		"started_at", "finished_at", "error_msg",
	}).AddRow(
		"exec-1", "wf", runtime.StatusSuccess, "",
		`{"step1":{"status":"success","started_at":"`+
			stepStartedAt.Format(time.RFC3339Nano)+`","finished_at":"`+
			stepFinishedAt.Format(time.RFC3339Nano)+`"}}`,
		now, sql.NullTime{}, "",
	)

	s.mock.ExpectQuery("ORDER BY started_at DESC").
		WillReturnRows(rows)

	metrics, err := s.st.GetAllStepMetrics(s.ctx)
	s.Require().NoError(err)
	s.Require().NotNil(metrics["step1"])
	s.Equal(1, metrics["step1"].TotalExecutions)
	s.Equal(1, metrics["step1"].SuccessCount)
}

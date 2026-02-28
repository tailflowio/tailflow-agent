//go:build !saas

package action

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	fakeruntime "github.com/tailflow/tailflow/internal/fake/fakeruntime"
	"github.com/tailflow/tailflow/internal/runtime"
)

// ---- exec.go coverage: parseCommandArgs default branch (not string, not []any) ----

type ExecCoverageTestSuite struct {
	suite.Suite
}

func TestExecCoverage(t *testing.T) {
	suite.Run(t, new(ExecCoverageTestSuite))
}

func (s *ExecCoverageTestSuite) SetupTest() {}

// TestParseCommandArgs_DefaultBranch covers the default case in parseCommandArgs
// where config["command"] is neither string nor []any (returns nil, which leads to empty command error).
func (s *ExecCoverageTestSuite) TestParseCommandArgs_DefaultBranch() {
	a := NewExecAction()
	// Bypass validation by setting command to an int (unusual type)
	ctx := newTestContext(map[string]any{
		"command": 12345,
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "empty command")
}

// ---- http.go coverage: checkExpectedStatus with int match ----

type HTTPCoverageTestSuite struct {
	suite.Suite
}

func TestHTTPCoverage(t *testing.T) {
	suite.Run(t, new(HTTPCoverageTestSuite))
}

func (s *HTTPCoverageTestSuite) SetupTest() {}

// TestCheckExpectedStatus_IntMatch covers the int case in checkExpectedStatus
// where the expected status matches the actual status (no error returned).
func (s *HTTPCoverageTestSuite) TestCheckExpectedStatus_IntMatch() {
	ctx := newTestContext(map[string]any{
		"expect": map[string]any{"status": 200},
	})

	err := checkExpectedStatus(ctx, nil, 200)
	s.NoError(err)
}

// TestCheckExpectedStatus_ExpectNotMap covers the branch where expect is not a map.
func (s *HTTPCoverageTestSuite) TestCheckExpectedStatus_ExpectNotMap() {
	ctx := newTestContext(map[string]any{
		"expect": "not-a-map",
	})

	err := checkExpectedStatus(ctx, nil, 200)
	s.NoError(err)
}

// TestCheckExpectedStatus_NoStatusKey covers the branch where expect map
// has no "status" key.
func (s *HTTPCoverageTestSuite) TestCheckExpectedStatus_NoStatusKey() {
	ctx := newTestContext(map[string]any{
		"expect": map[string]any{"other": "val"},
	})

	err := checkExpectedStatus(ctx, nil, 200)
	s.NoError(err)
}

// TestCheckExpectedStatus_UnsupportedType covers the branch where the expected
// status is neither int nor float64 (e.g. string). Should return nil (no error).
func (s *HTTPCoverageTestSuite) TestCheckExpectedStatus_UnsupportedType() {
	ctx := newTestContext(map[string]any{
		"expect": map[string]any{"status": "200"},
	})

	err := checkExpectedStatus(ctx, nil, 200)
	s.NoError(err)
}

func (s *HTTPCoverageTestSuite) TestCheckExpectedStatus_MismatchWithBody() {
	ctx := newTestContext(map[string]any{
		"expect": map[string]any{"status": 200},
	})

	output := map[string]any{"body": map[string]any{"error": "internal server error"}}
	err := checkExpectedStatus(ctx, output, 500)
	s.Error(err)
	s.Contains(err.Error(), "expected status 200, got 500")
	s.Contains(err.Error(), "internal server error")
}

func (s *HTTPCoverageTestSuite) TestTruncateBody_Nil() {
	s.Equal("(empty body)", truncateBody(nil, 512))
}

func (s *HTTPCoverageTestSuite) TestTruncateBody_JSON() {
	body := map[string]any{"error": "bad request"}
	result := truncateBody(body, 512)
	s.Contains(result, "bad request")
}

func (s *HTTPCoverageTestSuite) TestTruncateBody_LongString() {
	long := strings.Repeat("x", 600)
	result := truncateBody(long, 512)
	s.Len(result, 512+len("…"))
	s.True(strings.HasSuffix(result, "…"))
}

func (s *HTTPCoverageTestSuite) TestTruncateBody_UnmarshalableValue() {
	result := truncateBody(func() {}, 512)
	s.NotEmpty(result)
}

// ---- loop.go coverage: setLastIterationVars with empty items ----

type LoopCoverageTestSuite struct {
	suite.Suite
}

func TestLoopCoverage(t *testing.T) {
	suite.Run(t, new(LoopCoverageTestSuite))
}

func (s *LoopCoverageTestSuite) SetupTest() {}

// TestSetLastIterationVars_EmptyItems covers the early return in setLastIterationVars
// when items is empty.
func (s *LoopCoverageTestSuite) TestSetLastIterationVars_EmptyItems() {
	ctx := newTestContext(map[string]any{})

	setLastIterationVars(ctx, []any{}, "item", "index")

	// Should not have set any variables
	_, ok := ctx.ExecCtx.GetVariable("item")
	s.False(ok)
}

// TestSetLastIterationVars_WithItems covers the normal path setting the last item.
func (s *LoopCoverageTestSuite) TestSetLastIterationVars_WithItems() {
	ctx := newTestContext(map[string]any{})

	setLastIterationVars(ctx, []any{"a", "b", "c"}, "elem", "idx")

	v, ok := ctx.ExecCtx.GetVariable("elem")
	s.True(ok)
	s.Equal("c", v)

	idx, ok := ctx.ExecCtx.GetVariable("idx")
	s.True(ok)
	s.Equal(2, idx)
}

// TestEmptyItems_SingleAction covers the branch in executeSingleAction where
// items is empty and setLastIterationVars is called with an empty slice.
func (s *LoopCoverageTestSuite) TestEmptyItems_SingleAction() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items":         []any{},
		"action":        "noop",
		"action_config": map[string]any{},
	})

	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return nil, nil
	}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(0, m["iterations"])
}

// TestEmptyItems_Pipeline covers the branch in executePipeline where
// items is empty and setLastIterationVars is called with an empty slice.
func (s *LoopCoverageTestSuite) TestEmptyItems_Pipeline() {
	a := NewLoopAction()
	ctx := newTestContext(map[string]any{
		"items": []any{},
		"actions": []any{
			map[string]any{"action": "step1", "config": map[string]any{}},
		},
	})

	ctx.RunAction = func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error) {
		return nil, nil
	}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	m := out.(map[string]any)
	s.Equal(0, m["iterations"])
}

// ---- rabbitmq_shovel.go coverage: nackMessage error path ----

type ShovelCoverageTestSuite struct {
	suite.Suite
}

func TestShovelCoverage(t *testing.T) {
	suite.Run(t, new(ShovelCoverageTestSuite))
}

func (s *ShovelCoverageTestSuite) SetupTest() {}

// TestNackMessage_Error covers the error branch in nackMessage when Nack returns an error.
func (s *ShovelCoverageTestSuite) TestNackMessage_Error() {
	acker := &mockShovelAcknowledger{
		nackErr: fmt.Errorf("nack transport error"),
	}

	msg := amqp.Delivery{
		Acknowledger: acker,
	}

	var errs []string
	nackMessage(msg, 0, &errs)

	s.True(acker.nackCalled)
	s.Len(errs, 1)
	s.Contains(errs[0], "nack failed")
	s.Contains(errs[0], "nack transport error")
}

// TestNackMessage_Success covers the success path of nackMessage (no error appended).
func (s *ShovelCoverageTestSuite) TestNackMessage_Success() {
	acker := &mockShovelAcknowledger{}

	msg := amqp.Delivery{
		Acknowledger: acker,
	}

	var errs []string
	nackMessage(msg, 0, &errs)

	s.True(acker.nackCalled)
	s.Len(errs, 0)
}

// ---- sql_exec.go coverage: RowsAffected and LastInsertId errors ----

type SQLExecCoverageTestSuite struct {
	suite.Suite
}

func TestSQLExecCoverage(t *testing.T) {
	suite.Run(t, new(SQLExecCoverageTestSuite))
}

func (s *SQLExecCoverageTestSuite) SetupTest() {}

// TestExecExecute_RowsAffectedError covers the branch where RowsAffected returns an error.
func (s *SQLExecCoverageTestSuite) TestExecExecute_RowsAffectedError() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mockDB.ExpectExec("INSERT").WillReturnResult(sqlmock.NewErrorResult(fmt.Errorf("rows affected error")))

	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "test-dsn").Return(db, nil)

	ctx := newTestContext(map[string]any{
		"dsn":   "test-dsn",
		"query": "INSERT INTO t (x) VALUES (1)",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}

	a := NewSQLExecAction()
	_, err = a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "rows affected")
}

// TestExecExecute_LastInsertIdError covers the branch where LastInsertId returns an error.
func (s *SQLExecCoverageTestSuite) TestExecExecute_LastInsertIdError() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	// Use a custom result that returns an error for LastInsertId but not RowsAffected
	mockDB.ExpectExec("INSERT").WillReturnResult(&mockResult{
		lastInsertIdErr: fmt.Errorf("last insert id error"),
		rowsAffected:    1,
	})

	dbPool := fakeruntime.NewDBPool(s.T())
	dbPool.EXPECT().Get(mock.Anything, "test-dsn").Return(db, nil)

	ctx := newTestContext(map[string]any{
		"dsn":   "test-dsn",
		"query": "INSERT INTO t (x) VALUES (1)",
	})
	ctx.Services = &runtime.ActionServices{
		DBPool: dbPool,
	}

	a := NewSQLExecAction()
	_, err = a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "last insert id")
}

// mockResult implements sql.Result for fine-grained error control.
type mockResult struct {
	lastInsertId    int64
	lastInsertIdErr error
	rowsAffected    int64
	rowsAffectedErr error
}

func (r *mockResult) LastInsertId() (int64, error) {
	return r.lastInsertId, r.lastInsertIdErr
}

func (r *mockResult) RowsAffected() (int64, error) {
	return r.rowsAffected, r.rowsAffectedErr
}

// TestExecExecuteWithTx_ExecError covers the branch where tx.ExecContext fails.
func (s *SQLExecCoverageTestSuite) TestExecExecuteWithTx_ExecError() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mockDB.ExpectBegin()
	tx, err := db.Begin()
	s.Require().NoError(err)

	txReg := fakeruntime.NewTxRegistry(s.T())
	txReg.EXPECT().Get("my_tx").Return(tx, nil)

	mockDB.ExpectExec("DELETE").WillReturnError(fmt.Errorf("exec in tx failed"))

	ctx := newTestContext(map[string]any{
		"tx":    "my_tx",
		"query": "DELETE FROM users WHERE id = 1",
	})
	ctx.Services = &runtime.ActionServices{
		TxRegistry: txReg,
	}

	a := NewSQLExecAction()
	_, err = a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "exec in tx failed")
}

// ---- sql_query.go coverage: tx QueryContext error ----

type SQLQueryCoverageTestSuite struct {
	suite.Suite
}

func TestSQLQueryCoverage(t *testing.T) {
	suite.Run(t, new(SQLQueryCoverageTestSuite))
}

func (s *SQLQueryCoverageTestSuite) SetupTest() {}

// TestExecSQLQuery_TxQueryContextError covers the branch where tx.QueryContext fails.
func (s *SQLQueryCoverageTestSuite) TestExecSQLQuery_TxQueryContextError() {
	db, mockDB, err := sqlmock.New()
	s.Require().NoError(err)
	defer db.Close()

	mockDB.ExpectBegin()
	tx, err := db.Begin()
	s.Require().NoError(err)

	txReg := fakeruntime.NewTxRegistry(s.T())
	txReg.EXPECT().Get("my_tx").Return(tx, nil)

	mockDB.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("query in tx failed"))

	ctx := &ActionContext{
		Context: context.Background(),
		Config: map[string]any{
			"tx":    "my_tx",
			"query": "SELECT 1",
		},
		ExecCtx: runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:  "test-step",
		Logger:  slog.Default(),
		Services: &runtime.ActionServices{
			TxRegistry: txReg,
		},
	}

	a := NewSQLQueryAction()
	_, err = a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "query in tx failed")
}

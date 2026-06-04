package runtime

import (
	"time"
)

func (s *ExprEvaluatorTestSuite) TestBuiltin_UUID() {
	e := NewExprEvaluator()
	result, err := e.Eval("uuid()", map[string]any{})
	s.Require().NoError(err)
	s.Len(result.(string), 36)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Now() {
	e := NewExprEvaluator()
	result, err := e.Eval("now()", map[string]any{})
	s.Require().NoError(err)
	_, parseErr := time.Parse(time.RFC3339, result.(string))
	s.NoError(parseErr)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_NowWithTimezone() {
	e := NewExprEvaluator()
	result, err := e.Eval(`now("America/New_York")`, map[string]any{})
	s.Require().NoError(err)
	s.NotEmpty(result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_NowInvalidTimezone() {
	e := NewExprEvaluator()
	_, err := e.Eval(`now("Invalid/Zone")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_NowNonStringArg() {
	e := NewExprEvaluator()
	_, err := e.Eval("now(123)", map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_FormatDate() {
	e := NewExprEvaluator()
	result, err := e.Eval(`formatDate("2025-01-15T08:00:00Z", "date")`, map[string]any{})
	s.Require().NoError(err)
	s.Equal("2025-01-15", result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_FormatDate_TooFewArgs() {
	e := NewExprEvaluator()
	_, err := e.Eval(`formatDate("2025-01-15T08:00:00Z")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_FormatDate_NonStringArg1() {
	e := NewExprEvaluator()
	_, err := e.Eval(`formatDate(123, "date")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_FormatDate_NonStringArg2() {
	e := NewExprEvaluator()
	_, err := e.Eval(`formatDate("2025-01-15T08:00:00Z", 123)`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_FormatDate_InvalidDate() {
	e := NewExprEvaluator()
	_, err := e.Eval(`formatDate("not-a-date", "date")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_AddDate() {
	e := NewExprEvaluator()
	result, err := e.Eval(`addDate("2025-01-15T08:00:00Z", "1h")`, map[string]any{})
	s.Require().NoError(err)
	s.Contains(result.(string), "09:00:00")
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_AddDate_TooFewArgs() {
	e := NewExprEvaluator()
	_, err := e.Eval(`addDate("2025-01-15T08:00:00Z")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_AddDate_NonStringArg1() {
	e := NewExprEvaluator()
	_, err := e.Eval(`addDate(123, "1h")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_AddDate_NonStringArg2() {
	e := NewExprEvaluator()
	_, err := e.Eval(`addDate("2025-01-15T08:00:00Z", 123)`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_AddDate_InvalidDate() {
	e := NewExprEvaluator()
	_, err := e.Eval(`addDate("bad", "1h")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_AddDate_InvalidDuration() {
	e := NewExprEvaluator()
	_, err := e.Eval(`addDate("2025-01-15T08:00:00Z", "bad")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_DiffDate() {
	e := NewExprEvaluator()
	result, err := e.Eval(`diffDate("2025-01-15T09:00:00Z", "2025-01-15T08:00:00Z")`, map[string]any{})
	s.Require().NoError(err)
	s.Equal(3600.0, result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_DiffDate_TooFewArgs() {
	e := NewExprEvaluator()
	_, err := e.Eval(`diffDate("2025-01-15T08:00:00Z")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_DiffDate_NonStringArg1() {
	e := NewExprEvaluator()
	_, err := e.Eval(`diffDate(123, "2025-01-15T08:00:00Z")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_DiffDate_NonStringArg2() {
	e := NewExprEvaluator()
	_, err := e.Eval(`diffDate("2025-01-15T08:00:00Z", 123)`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_DiffDate_InvalidDate1() {
	e := NewExprEvaluator()
	_, err := e.Eval(`diffDate("bad", "2025-01-15T08:00:00Z")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_DiffDate_InvalidDate2() {
	e := NewExprEvaluator()
	_, err := e.Eval(`diffDate("2025-01-15T08:00:00Z", "bad")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_UnixTime_NoArgs() {
	e := NewExprEvaluator()
	result, err := e.Eval("unixTime()", map[string]any{})
	s.Require().NoError(err)
	s.Greater(result.(int64), int64(0))
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_UnixTime_WithDate() {
	e := NewExprEvaluator()
	result, err := e.Eval(`unixTime("2025-01-15T00:00:00Z")`, map[string]any{})
	s.Require().NoError(err)
	s.Greater(result.(int64), int64(0))
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_UnixTime_NonString() {
	e := NewExprEvaluator()
	_, err := e.Eval("unixTime(123)", map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_UnixTime_InvalidDate() {
	e := NewExprEvaluator()
	_, err := e.Eval(`unixTime("bad")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Tz() {
	e := NewExprEvaluator()
	result, err := e.Eval(`tz("2025-01-15T08:00:00Z", "Europe/Paris")`, map[string]any{})
	s.Require().NoError(err)
	s.Contains(result.(string), "+01:00")
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Tz_TooFewArgs() {
	e := NewExprEvaluator()
	_, err := e.Eval(`tz("2025-01-15T08:00:00Z")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Tz_NonStringArg1() {
	e := NewExprEvaluator()
	_, err := e.Eval(`tz(123, "Europe/Paris")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Tz_NonStringArg2() {
	e := NewExprEvaluator()
	_, err := e.Eval(`tz("2025-01-15T08:00:00Z", 123)`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Tz_InvalidDate() {
	e := NewExprEvaluator()
	_, err := e.Eval(`tz("bad", "Europe/Paris")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Tz_InvalidTimezone() {
	e := NewExprEvaluator()
	_, err := e.Eval(`tz("2025-01-15T08:00:00Z", "Invalid/Zone")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Flag() {
	e := NewExprEvaluator()
	result, err := e.Eval(`flag("--id", "abc")`, map[string]any{})
	s.Require().NoError(err)
	s.Equal("--id abc", result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Flag_Empty() {
	e := NewExprEvaluator()
	result, err := e.Eval(`flag("--id", "")`, map[string]any{})
	s.Require().NoError(err)
	s.Equal("", result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Flag_TooFewArgs() {
	e := NewExprEvaluator()
	_, err := e.Eval(`flag("--id")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Flag_NonStringName() {
	e := NewExprEvaluator()
	_, err := e.Eval(`flag(123, "abc")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Flag_NilValue() {
	e := NewExprEvaluator()
	ctx := map[string]any{"val": nil}
	result, err := e.Eval(`flag("--id", val)`, ctx)
	s.Require().NoError(err)
	s.Equal("", result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Bflag_True() {
	e := NewExprEvaluator()
	result, err := e.Eval(`bflag("-v", true)`, map[string]any{})
	s.Require().NoError(err)
	s.Equal("-v", result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Bflag_False() {
	e := NewExprEvaluator()
	result, err := e.Eval(`bflag("-v", false)`, map[string]any{})
	s.Require().NoError(err)
	s.Equal("", result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Bflag_String_Truthy() {
	e := NewExprEvaluator()
	result, err := e.Eval(`bflag("-v", "yes")`, map[string]any{})
	s.Require().NoError(err)
	s.Equal("-v", result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Bflag_String_False() {
	e := NewExprEvaluator()
	result, err := e.Eval(`bflag("-v", "false")`, map[string]any{})
	s.Require().NoError(err)
	s.Equal("", result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Bflag_String_Zero() {
	e := NewExprEvaluator()
	result, err := e.Eval(`bflag("-v", "0")`, map[string]any{})
	s.Require().NoError(err)
	s.Equal("", result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Bflag_String_Empty() {
	e := NewExprEvaluator()
	result, err := e.Eval(`bflag("-v", "")`, map[string]any{})
	s.Require().NoError(err)
	s.Equal("", result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Bflag_Int() {
	e := NewExprEvaluator()
	result, err := e.Eval(`bflag("-v", 1)`, map[string]any{})
	s.Require().NoError(err)
	s.Equal("-v", result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Bflag_TooFewArgs() {
	e := NewExprEvaluator()
	_, err := e.Eval(`bflag("-v")`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Bflag_NonStringName() {
	e := NewExprEvaluator()
	_, err := e.Eval(`bflag(123, true)`, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Bflag_Nil() {
	e := NewExprEvaluator()
	ctx := map[string]any{"val": nil}
	result, err := e.Eval(`bflag("-v", val)`, ctx)
	s.Require().NoError(err)
	s.Equal("", result)
}

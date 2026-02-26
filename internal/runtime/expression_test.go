package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type ExprEvaluatorTestSuite struct {
	suite.Suite
}

func TestExprEvaluator(t *testing.T) {
	suite.Run(t, new(ExprEvaluatorTestSuite))
}

func (s *ExprEvaluatorTestSuite) SetupTest() {
	// required by convention
}

func (s *ExprEvaluatorTestSuite) TestNewExprEvaluator() {
	e := NewExprEvaluator()
	s.NotNil(e)
}

func (s *ExprEvaluatorTestSuite) TestEval_Arithmetic() {
	e := NewExprEvaluator()
	result, err := e.Eval("2 + 3", map[string]any{})
	s.Require().NoError(err)
	s.Equal(int64(5), result)
}

func (s *ExprEvaluatorTestSuite) TestEval_StringConcat() {
	e := NewExprEvaluator()
	result, err := e.Eval(`"hello" + " " + "world"`, map[string]any{})
	s.Require().NoError(err)
	s.Equal("hello world", result)
}

func (s *ExprEvaluatorTestSuite) TestEval_ContextAccess() {
	e := NewExprEvaluator()
	ctx := map[string]any{
		"params": map[string]any{"name": "Alice"},
	}
	result, err := e.Eval("params.name", ctx)
	s.Require().NoError(err)
	s.Equal("Alice", result)
}

func (s *ExprEvaluatorTestSuite) TestEval_CompileError() {
	e := NewExprEvaluator()
	_, err := e.Eval("invalid @@@ expression", map[string]any{})
	s.Error(err)
	s.Contains(err.Error(), "compile expression")
}

func (s *ExprEvaluatorTestSuite) TestEval_NormalizeInt() {
	e := NewExprEvaluator()
	result, err := e.Eval("1 + 2", map[string]any{})
	s.Require().NoError(err)
	// Should be int64, not int
	s.IsType(int64(0), result)
}

func (s *ExprEvaluatorTestSuite) TestEvalBool_True() {
	e := NewExprEvaluator()
	result, err := e.EvalBool("1 == 1", map[string]any{})
	s.Require().NoError(err)
	s.True(result)
}

func (s *ExprEvaluatorTestSuite) TestEvalBool_False() {
	e := NewExprEvaluator()
	result, err := e.EvalBool("1 == 2", map[string]any{})
	s.Require().NoError(err)
	s.False(result)
}

func (s *ExprEvaluatorTestSuite) TestEvalBool_NonBoolResult() {
	e := NewExprEvaluator()
	_, err := e.EvalBool(`"hello"`, map[string]any{})
	s.Error(err)
	s.Contains(err.Error(), "did not return boolean")
}

func (s *ExprEvaluatorTestSuite) TestEvalBool_Error() {
	e := NewExprEvaluator()
	_, err := e.EvalBool("invalid @@@", map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestResolveTemplate_NoTemplate() {
	e := NewExprEvaluator()
	result, err := e.ResolveTemplate("plain text", map[string]any{})
	s.Require().NoError(err)
	s.Equal("plain text", result)
}

func (s *ExprEvaluatorTestSuite) TestResolveTemplate_Simple() {
	e := NewExprEvaluator()
	ctx := map[string]any{"name": "Alice"}
	result, err := e.ResolveTemplate("Hello {{ name }}!", ctx)
	s.Require().NoError(err)
	s.Equal("Hello Alice!", result)
}

func (s *ExprEvaluatorTestSuite) TestResolveTemplate_Multiple() {
	e := NewExprEvaluator()
	ctx := map[string]any{"first": "John", "last": "Doe"}
	result, err := e.ResolveTemplate("{{ first }} {{ last }}", ctx)
	s.Require().NoError(err)
	s.Equal("John Doe", result)
}

func (s *ExprEvaluatorTestSuite) TestResolveTemplate_Error() {
	e := NewExprEvaluator()
	_, err := e.ResolveTemplate("{{ nonexistent_var }}", map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestResolveConfig_Nil() {
	e := NewExprEvaluator()
	result, err := e.ResolveConfig(nil, map[string]any{})
	s.Require().NoError(err)
	s.Nil(result)
}

func (s *ExprEvaluatorTestSuite) TestResolveConfig_Plain() {
	e := NewExprEvaluator()
	config := map[string]any{
		"message": "hello",
		"count":   42,
	}
	result, err := e.ResolveConfig(config, map[string]any{})
	s.Require().NoError(err)
	s.Equal("hello", result["message"])
	s.Equal(42, result["count"])
}

func (s *ExprEvaluatorTestSuite) TestResolveConfig_WithTemplates() {
	e := NewExprEvaluator()
	config := map[string]any{
		"greeting": "Hello {{ name }}!",
	}
	ctx := map[string]any{"name": "Bob"}
	result, err := e.ResolveConfig(config, ctx)
	s.Require().NoError(err)
	s.Equal("Hello Bob!", result["greeting"])
}

func (s *ExprEvaluatorTestSuite) TestResolveConfig_PureExpression() {
	e := NewExprEvaluator()
	config := map[string]any{
		"items": "{{ list }}",
	}
	ctx := map[string]any{"list": []any{"a", "b", "c"}}
	result, err := e.ResolveConfig(config, ctx)
	s.Require().NoError(err)
	// Pure {{ expr }} should return raw value, not stringified
	items := result["items"].([]any)
	s.Len(items, 3)
}

func (s *ExprEvaluatorTestSuite) TestResolveConfig_NestedMap() {
	e := NewExprEvaluator()
	config := map[string]any{
		"nested": map[string]any{
			"value": "{{ x }}",
		},
	}
	ctx := map[string]any{"x": "resolved"}
	result, err := e.ResolveConfig(config, ctx)
	s.Require().NoError(err)
	nested := result["nested"].(map[string]any)
	s.Equal("resolved", nested["value"])
}

func (s *ExprEvaluatorTestSuite) TestResolveConfig_Slice() {
	e := NewExprEvaluator()
	config := map[string]any{
		"items": []any{"{{ a }}", "{{ b }}"},
	}
	ctx := map[string]any{"a": "first", "b": "second"}
	result, err := e.ResolveConfig(config, ctx)
	s.Require().NoError(err)
	items := result["items"].([]any)
	s.Equal("first", items[0])
	s.Equal("second", items[1])
}

func (s *ExprEvaluatorTestSuite) TestResolveConfig_Error() {
	e := NewExprEvaluator()
	config := map[string]any{
		"bad": "{{ nonexistent }}",
	}
	_, err := e.ResolveConfig(config, map[string]any{})
	s.Error(err)
	s.Contains(err.Error(), "resolve config key")
}

func (s *ExprEvaluatorTestSuite) TestResolveStringValue_Plain() {
	e := NewExprEvaluator()
	result, err := e.resolveStringValue("plain", map[string]any{})
	s.Require().NoError(err)
	s.Equal("plain", result)
}

func (s *ExprEvaluatorTestSuite) TestResolveStringValue_PureTemplate() {
	e := NewExprEvaluator()
	ctx := map[string]any{"items": []any{1, 2, 3}}
	result, err := e.resolveStringValue("{{ items }}", ctx)
	s.Require().NoError(err)
	// Pure template returns raw value
	s.IsType([]any{}, result)
}

func (s *ExprEvaluatorTestSuite) TestResolveStringValue_MixedTemplate() {
	e := NewExprEvaluator()
	ctx := map[string]any{"name": "Alice"}
	result, err := e.resolveStringValue("Hello {{ name }}!", ctx)
	s.Require().NoError(err)
	s.Equal("Hello Alice!", result)
}

func (s *ExprEvaluatorTestSuite) TestNormalizeResult_Int() {
	s.Equal(int64(42), normalizeResult(42))
}

func (s *ExprEvaluatorTestSuite) TestNormalizeResult_String() {
	s.Equal("hello", normalizeResult("hello"))
}

func (s *ExprEvaluatorTestSuite) TestNormalizeResult_Slice() {
	input := []any{1, "two", 3}
	result := normalizeResult(input).([]any)
	s.Equal(int64(1), result[0])
	s.Equal("two", result[1])
	s.Equal(int64(3), result[2])
}

func (s *ExprEvaluatorTestSuite) TestNormalizeResult_Map() {
	input := map[string]any{"count": 5, "name": "test"}
	result := normalizeResult(input).(map[string]any)
	s.Equal(int64(5), result["count"])
	s.Equal("test", result["name"])
}

func (s *ExprEvaluatorTestSuite) TestNormalizeResult_Float() {
	s.Equal(3.14, normalizeResult(3.14))
}

func (s *ExprEvaluatorTestSuite) TestParseDate_RFC3339() {
	t, err := parseDate("2025-01-15T08:00:00Z")
	s.Require().NoError(err)
	s.Equal(2025, t.Year())
	s.Equal(time.January, t.Month())
	s.Equal(15, t.Day())
}

func (s *ExprEvaluatorTestSuite) TestParseDate_DateOnly() {
	t, err := parseDate("2025-01-15")
	s.Require().NoError(err)
	s.Equal(2025, t.Year())
}

func (s *ExprEvaluatorTestSuite) TestParseDate_DateTime() {
	t, err := parseDate("2025-01-15 08:30:00")
	s.Require().NoError(err)
	s.Equal(8, t.Hour())
	s.Equal(30, t.Minute())
}

func (s *ExprEvaluatorTestSuite) TestParseDate_DateTimeT() {
	t, err := parseDate("2025-01-15T08:30:00")
	s.Require().NoError(err)
	s.Equal(8, t.Hour())
}

func (s *ExprEvaluatorTestSuite) TestParseDate_Invalid() {
	_, err := parseDate("not-a-date")
	s.Error(err)
	s.Contains(err.Error(), "cannot parse date")
}

func (s *ExprEvaluatorTestSuite) TestDateFormat_ISO() {
	s.Equal(time.RFC3339, dateFormat("iso"))
	s.Equal(time.RFC3339, dateFormat("ISO"))
	s.Equal(time.RFC3339, dateFormat("RFC3339"))
}

func (s *ExprEvaluatorTestSuite) TestDateFormat_Date() {
	s.Equal("2006-01-02", dateFormat("date"))
}

func (s *ExprEvaluatorTestSuite) TestDateFormat_DateTime() {
	s.Equal("2006-01-02 15:04:05", dateFormat("datetime"))
}

func (s *ExprEvaluatorTestSuite) TestDateFormat_Time() {
	s.Equal("15:04:05", dateFormat("time"))
}

func (s *ExprEvaluatorTestSuite) TestDateFormat_Unix() {
	s.Equal(time.UnixDate, dateFormat("unix"))
}

func (s *ExprEvaluatorTestSuite) TestDateFormat_Custom() {
	s.Equal("2006/01/02", dateFormat("2006/01/02"))
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_UUID() {
	e := NewExprEvaluator()
	result, err := e.Eval("uuid()", map[string]any{})
	s.Require().NoError(err)
	s.Len(result.(string), 36) // UUID format
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Now() {
	e := NewExprEvaluator()
	result, err := e.Eval("now()", map[string]any{})
	s.Require().NoError(err)
	// Should be valid RFC3339
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

func (s *ExprEvaluatorTestSuite) TestResolveValue_NonStringNonMapNonSlice() {
	e := NewExprEvaluator()
	result, err := e.resolveValue(42, map[string]any{})
	s.Require().NoError(err)
	s.Equal(42, result)
}

func (s *ExprEvaluatorTestSuite) TestResolveValue_Bool() {
	e := NewExprEvaluator()
	result, err := e.resolveValue(true, map[string]any{})
	s.Require().NoError(err)
	s.Equal(true, result)
}

func (s *ExprEvaluatorTestSuite) TestResolveValue_SliceError() {
	e := NewExprEvaluator()
	_, err := e.resolveValue([]any{"{{ missing }}"}, map[string]any{})
	s.Error(err)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Flag_NilValue() {
	e := NewExprEvaluator()
	ctx := map[string]any{"val": nil}
	result, err := e.Eval(`flag("--id", val)`, ctx)
	s.Require().NoError(err)
	s.Equal("", result)
}

func (s *ExprEvaluatorTestSuite) TestBuiltin_Bflag_Nil() {
	e := NewExprEvaluator()
	ctx := map[string]any{"val": nil}
	result, err := e.Eval(`bflag("-v", val)`, ctx)
	s.Require().NoError(err)
	s.Equal("", result)
}

func (s *ExprEvaluatorTestSuite) TestParseDate_RFC3339Nano() {
	t, err := parseDate("2025-01-15T08:00:00.123456789Z")
	s.Require().NoError(err)
	s.Equal(2025, t.Year())
}

func (s *ExprEvaluatorTestSuite) TestResolveTemplate_Whitespace() {
	e := NewExprEvaluator()
	ctx := map[string]any{"x": "hello"}
	result, err := e.ResolveTemplate("{{  x  }}", ctx)
	s.Require().NoError(err)
	s.Equal("hello", result)
}

func (s *ExprEvaluatorTestSuite) TestEval_StepsContext() {
	e := NewExprEvaluator()
	ctx := map[string]any{
		"steps": map[string]any{
			"fetch": map[string]any{
				"status": "success",
				"output": map[string]any{
					"data": []any{"a", "b"},
				},
			},
		},
	}
	result, err := e.Eval(`steps.fetch.status == "success"`, ctx)
	s.Require().NoError(err)
	s.Equal(true, result)
}

func (s *ExprEvaluatorTestSuite) TestEval_RuntimeError() {
	e := NewExprEvaluator()
	// Access a field on nil
	ctx := map[string]any{"data": nil}
	_, err := e.Eval("data.field", ctx)
	s.Error(err)
	s.True(strings.Contains(err.Error(), "eval expression") || strings.Contains(err.Error(), "compile expression"))
}

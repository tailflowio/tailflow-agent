package runtime

import (
	"time"
)

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

func (s *ExprEvaluatorTestSuite) TestParseDate_RFC3339Nano() {
	t, err := parseDate("2025-01-15T08:00:00.123456789Z")
	s.Require().NoError(err)
	s.Equal(2025, t.Year())
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

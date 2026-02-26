package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type ExpressionTestSuite struct {
	suite.Suite
}

func TestExpression(t *testing.T) {
	suite.Run(t, new(ExpressionTestSuite))
}

func (s *ExpressionTestSuite) SetupTest() {}

func (s *ExpressionTestSuite) TestEval_BasicExpressions() {
	eval := NewExprEvaluator()
	ctx := map[string]any{
		"params": map[string]any{"env": "staging"},
		"steps": map[string]any{
			"build": map[string]any{"status": "success"},
		},
	}

	tests := []struct {
		name   string
		expr   string
		expect any
	}{
		{"simple string", "'hello'", "hello"},
		{"number", "1 + 2", int64(3)},
		{"param access", "params.env", "staging"},
		{"step status", "steps.build.status", "success"},
		{"comparison", "params.env == 'staging'", true},
		{"boolean", "true", true},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result, err := eval.Eval(tt.expr, ctx)
			s.Require().NoError(err)
			s.Equal(tt.expect, result)
		})
	}
}

func (s *ExpressionTestSuite) TestEvalBool() {
	eval := NewExprEvaluator()
	ctx := map[string]any{
		"params": map[string]any{"dry_run": false},
	}

	result, err := eval.EvalBool("params.dry_run == false", ctx)
	s.Require().NoError(err)
	s.True(result)

	result, err = eval.EvalBool("params.dry_run == true", ctx)
	s.Require().NoError(err)
	s.False(result)
}

func (s *ExpressionTestSuite) TestEvalBool_NonBoolError() {
	eval := NewExprEvaluator()
	_, err := eval.EvalBool("'hello'", nil)
	s.ErrorContains(err, "did not return boolean")
}

func (s *ExpressionTestSuite) TestResolveTemplate() {
	eval := NewExprEvaluator()
	ctx := map[string]any{
		"params": map[string]any{"env": "staging"},
		"steps": map[string]any{
			"deploy": map[string]any{"status": "success"},
		},
	}

	tests := []struct {
		name   string
		tmpl   string
		expect string
	}{
		{"no template", "hello world", "hello world"},
		{"simple", "deploy to {{ params.env }}", "deploy to staging"},
		{"multiple", "{{ steps.deploy.status }} on {{ params.env }}", "success on staging"},
		{"expression", "count: {{ 1 + 2 }}", "count: 3"},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result, err := eval.ResolveTemplate(tt.tmpl, ctx)
			s.Require().NoError(err)
			s.Equal(tt.expect, result)
		})
	}
}

func (s *ExpressionTestSuite) TestResolveConfig() {
	eval := NewExprEvaluator()
	ctx := map[string]any{
		"params": map[string]any{"env": "production"},
	}

	config := map[string]any{
		"url":     "https://{{ params.env }}.example.com",
		"timeout": 30,
		"nested": map[string]any{
			"key": "value-{{ params.env }}",
		},
		"list": []any{"{{ params.env }}", "static"},
	}

	resolved, err := eval.ResolveConfig(config, ctx)
	s.Require().NoError(err)
	s.Equal("https://production.example.com", resolved["url"])
	s.Equal(30, resolved["timeout"])
	s.Equal("value-production", resolved["nested"].(map[string]any)["key"])
	s.Equal("production", resolved["list"].([]any)[0])
	s.Equal("static", resolved["list"].([]any)[1])
}

func (s *ExpressionTestSuite) TestResolveConfig_Nil() {
	eval := NewExprEvaluator()
	result, err := eval.ResolveConfig(nil, nil)
	s.Require().NoError(err)
	s.Nil(result)
}

func (s *ExpressionTestSuite) TestEval_Error() {
	eval := NewExprEvaluator()
	_, err := eval.Eval("invalid syntax ===", nil)
	s.Error(err)
}

func (s *ExpressionTestSuite) TestBuiltinFunctions() {
	eval := NewExprEvaluator()

	s.Run("uuid", func() {
		result, err := eval.Eval("uuid()", nil)
		s.Require().NoError(err)
		str, ok := result.(string)
		s.Require().True(ok)
		s.Len(str, 36) // UUID format
	})

	s.Run("now returns RFC3339", func() {
		result, err := eval.Eval("now()", nil)
		s.Require().NoError(err)
		str := result.(string)
		_, err = time.Parse(time.RFC3339, str)
		s.NoError(err)
	})

	s.Run("now with timezone", func() {
		result, err := eval.Eval(`now("Asia/Tokyo")`, nil)
		s.Require().NoError(err)
		str := result.(string)
		s.Contains(str, "+09:00")
	})

	s.Run("tz converts timezone", func() {
		result, err := eval.Eval(`tz("2026-02-13T12:00:00Z", "Europe/Paris")`, nil)
		s.Require().NoError(err)
		s.Equal("2026-02-13T13:00:00+01:00", result)
	})

	s.Run("formatDate", func() {
		result, err := eval.Eval(`formatDate("2026-02-13T15:04:05Z", "date")`, nil)
		s.Require().NoError(err)
		s.Equal("2026-02-13", result)
	})

	s.Run("addDate", func() {
		result, err := eval.Eval(`addDate("2026-02-13T12:00:00Z", "2h")`, nil)
		s.Require().NoError(err)
		s.Equal("2026-02-13T14:00:00Z", result)
	})

	s.Run("addDate negative", func() {
		result, err := eval.Eval(`addDate("2026-02-13T12:00:00Z", "-30m")`, nil)
		s.Require().NoError(err)
		s.Equal("2026-02-13T11:30:00Z", result)
	})

	s.Run("diffDate in seconds", func() {
		result, err := eval.Eval(`diffDate("2026-02-13T13:00:00Z", "2026-02-13T12:00:00Z")`, nil)
		s.Require().NoError(err)
		s.Equal(3600.0, result)
	})

	s.Run("unixTime", func() {
		result, err := eval.Eval("unixTime()", nil)
		s.Require().NoError(err)
		ts, ok := result.(int64)
		s.Require().True(ok)
		s.InDelta(time.Now().Unix(), ts, 2)
	})
}

func (s *ExpressionTestSuite) TestFunctionChaining() {
	eval := NewExprEvaluator()

	s.Run("formatDate + tz + now", func() {
		result, err := eval.Eval(`formatDate(tz(now(), "Europe/Paris"), "date")`, nil)
		s.Require().NoError(err)
		str := result.(string)
		// Should be a valid date
		_, err = time.Parse("2006-01-02", str)
		s.NoError(err)
	})

	s.Run("addDate + tz", func() {
		result, err := eval.Eval(`tz(addDate("2026-02-13T23:00:00Z", "2h"), "Europe/Paris")`, nil)
		s.Require().NoError(err)
		// 23:00 UTC + 2h = 01:00 UTC next day = 02:00 Paris
		s.Equal("2026-02-14T02:00:00+01:00", result)
	})

	s.Run("nested in template", func() {
		result, err := eval.ResolveTemplate(`Created: {{ formatDate("2026-06-15T10:00:00Z", "date") }}`, nil)
		s.Require().NoError(err)
		s.Equal("Created: 2026-06-15", result)
	})

	s.Run("chained with expr builtins", func() {
		result, err := eval.Eval(`upper(formatDate("2026-02-13T10:00:00Z", "datetime"))`, nil)
		s.Require().NoError(err)
		s.Equal("2026-02-13 10:00:00", result) // upper has no effect on digits, but it compiles
	})

	s.Run("uuid in string concat", func() {
		result, err := eval.Eval(`"order-" + uuid()`, nil)
		s.Require().NoError(err)
		str := result.(string)
		s.True(strings.HasPrefix(str, "order-"))
		s.Len(str, 6+36) // "order-" + UUID
	})
}

func (s *ExpressionTestSuite) TestFlagFunction() {
	eval := NewExprEvaluator()

	s.Run("flag with value", func() {
		result, err := eval.Eval(`flag("--id", "123")`, nil)
		s.Require().NoError(err)
		s.Equal("--id 123", result)
	})

	s.Run("flag with empty string", func() {
		result, err := eval.Eval(`flag("--id", "")`, nil)
		s.Require().NoError(err)
		s.Equal("", result)
	})

	s.Run("flag with param value", func() {
		ctx := map[string]any{"params": map[string]any{"id": "abc"}}
		result, err := eval.Eval(`flag("--id", params.id)`, ctx)
		s.Require().NoError(err)
		s.Equal("--id abc", result)
	})

	s.Run("flag with empty param", func() {
		ctx := map[string]any{"params": map[string]any{"id": ""}}
		result, err := eval.Eval(`flag("--id", params.id)`, ctx)
		s.Require().NoError(err)
		s.Equal("", result)
	})

	s.Run("flag in template with mixed params", func() {
		ctx := map[string]any{"params": map[string]any{"id": "42", "ids": ""}}
		result, err := eval.ResolveTemplate(`mycommand {{ flag("--id", params.id) }} {{ flag("--ids", params.ids) }}`, ctx)
		s.Require().NoError(err)
		s.Equal("mycommand --id 42 ", result)
	})

	s.Run("flag with nil value", func() {
		result, err := eval.Eval(`flag("--id", nil)`, nil)
		s.Require().NoError(err)
		s.Equal("", result)
	})
}

func (s *ExpressionTestSuite) TestBflagFunction() {
	eval := NewExprEvaluator()

	s.Run("bflag true", func() {
		result, err := eval.Eval(`bflag("-v", true)`, nil)
		s.Require().NoError(err)
		s.Equal("-v", result)
	})

	s.Run("bflag false", func() {
		result, err := eval.Eval(`bflag("-v", false)`, nil)
		s.Require().NoError(err)
		s.Equal("", result)
	})

	s.Run("bflag nil", func() {
		result, err := eval.Eval(`bflag("-v", nil)`, nil)
		s.Require().NoError(err)
		s.Equal("", result)
	})

	s.Run("bflag with param", func() {
		ctx := map[string]any{"params": map[string]any{"verbose": true}}
		result, err := eval.Eval(`bflag("-v", params.verbose)`, ctx)
		s.Require().NoError(err)
		s.Equal("-v", result)
	})

	s.Run("bflag string false", func() {
		result, err := eval.Eval(`bflag("-v", "false")`, nil)
		s.Require().NoError(err)
		s.Equal("", result)
	})

	s.Run("bflag in template combo", func() {
		ctx := map[string]any{"params": map[string]any{"verbose": true, "id": "42", "dry": false}}
		result, err := eval.ResolveTemplate(
			`mycmd {{ bflag("-v", params.verbose) }} {{ flag("--id", params.id) }} {{ bflag("--dry-run", params.dry) }}`, ctx)
		s.Require().NoError(err)
		s.Equal("mycmd -v --id 42 ", result)
	})
}

package runtime

import (
	"strings"
)

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
	ctx := map[string]any{"data": nil}
	_, err := e.Eval("data.field", ctx)
	s.Error(err)
	s.True(strings.Contains(err.Error(), "eval expression") || strings.Contains(err.Error(), "compile expression"))
}

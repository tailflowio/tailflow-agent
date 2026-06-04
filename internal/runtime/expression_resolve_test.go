package runtime

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

func (s *ExprEvaluatorTestSuite) TestResolveTemplate_Whitespace() {
	e := NewExprEvaluator()
	ctx := map[string]any{"x": "hello"}
	result, err := e.ResolveTemplate("{{  x  }}", ctx)
	s.Require().NoError(err)
	s.Equal("hello", result)
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
	s.IsType([]any{}, result)
}

func (s *ExprEvaluatorTestSuite) TestResolveStringValue_MixedTemplate() {
	e := NewExprEvaluator()
	ctx := map[string]any{"name": "Alice"}
	result, err := e.resolveStringValue("Hello {{ name }}!", ctx)
	s.Require().NoError(err)
	s.Equal("Hello Alice!", result)
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

package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ArrayActionValidateTestSuite struct {
	suite.Suite
}

func TestArrayActionValidate(t *testing.T) {
	suite.Run(t, new(ArrayActionValidateTestSuite))
}

func (s *ArrayActionValidateTestSuite) SetupTest() {}

func (s *ArrayActionValidateTestSuite) TestSortValidateOK() {
	a := NewArraySortAction()
	err := a.Validate(newTestContext(map[string]any{"input": []any{}, "field": "name"}))
	s.NoError(err)
}

func (s *ArrayActionValidateTestSuite) TestFilterValidateOK() {
	a := NewArrayFilterAction()
	err := a.Validate(newTestContext(map[string]any{"input": []any{}, "condition": "true"}))
	s.NoError(err)
}

func (s *ArrayActionValidateTestSuite) TestFilterValidateMissingInput() {
	a := NewArrayFilterAction()
	err := a.Validate(newTestContext(map[string]any{"condition": "true"}))
	s.Error(err)
	s.Contains(err.Error(), "input")
}

func (s *ArrayActionValidateTestSuite) TestMapValidateOK() {
	a := NewArrayMapAction()
	err := a.Validate(newTestContext(map[string]any{"input": []any{}, "expression": "item"}))
	s.NoError(err)
}

func (s *ArrayActionValidateTestSuite) TestMapValidateMissingInput() {
	a := NewArrayMapAction()
	err := a.Validate(newTestContext(map[string]any{"expression": "item"}))
	s.Error(err)
	s.Contains(err.Error(), "input")
}

func (s *ArrayActionValidateTestSuite) TestUniqValidateOK() {
	a := NewArrayUniqAction()
	err := a.Validate(newTestContext(map[string]any{"input": []any{}, "field": "id"}))
	s.NoError(err)
}

func (s *ArrayActionValidateTestSuite) TestUniqValidateMissingInput() {
	a := NewArrayUniqAction()
	err := a.Validate(newTestContext(map[string]any{"field": "id"}))
	s.Error(err)
	s.Contains(err.Error(), "input")
}

func (s *ArrayActionValidateTestSuite) TestPickValidateOK() {
	a := NewArrayPickAction()
	err := a.Validate(newTestContext(map[string]any{"input": []any{}, "fields": []any{"name"}}))
	s.NoError(err)
}

func (s *ArrayActionValidateTestSuite) TestPickValidateMissingInput() {
	a := NewArrayPickAction()
	err := a.Validate(newTestContext(map[string]any{"fields": []any{"name"}}))
	s.Error(err)
	s.Contains(err.Error(), "input")
}

func (s *ArrayActionValidateTestSuite) TestConcatValidateOK() {
	a := NewArrayConcatAction()
	err := a.Validate(newTestContext(map[string]any{"arrays": []any{}}))
	s.NoError(err)
}

func (s *ArrayActionValidateTestSuite) TestSortInputNotArray() {
	a := NewArraySortAction()
	ctx := newTestContext(map[string]any{
		"input": "not-an-array",
		"field": "name",
	})
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "must be an array")
}

func (s *ArrayActionValidateTestSuite) TestFilterInputNotArray() {
	a := NewArrayFilterAction()
	ctx := newTestContext(map[string]any{
		"input":     "not-an-array",
		"condition": "true",
	})
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "must be an array")
}

func (s *ArrayActionValidateTestSuite) TestMapInputNotArray() {
	a := NewArrayMapAction()
	ctx := newTestContext(map[string]any{
		"input":      "not-an-array",
		"expression": "item",
	})
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "must be an array")
}

func (s *ArrayActionValidateTestSuite) TestUniqInputNotArray() {
	a := NewArrayUniqAction()
	ctx := newTestContext(map[string]any{
		"input": "not-an-array",
		"field": "id",
	})
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "must be an array")
}

func (s *ArrayActionValidateTestSuite) TestPickInputNotArray() {
	a := NewArrayPickAction()
	ctx := newTestContext(map[string]any{
		"input":  "not-an-array",
		"fields": []any{"name"},
	})
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "must be an array")
}

func (s *ArrayActionValidateTestSuite) TestConcatNotArrayOfArrays() {
	a := NewArrayConcatAction()
	ctx := newTestContext(map[string]any{
		"arrays": "not-an-array",
	})
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "must be an array")
}

func (s *ArrayActionValidateTestSuite) TestSortNonMapItems() {
	a := NewArraySortAction()
	ctx := newTestContext(map[string]any{
		"input": []any{"c", "a", "b"},
		"field": "nonexistent",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.NotNil(out)
}

func (s *ArrayActionValidateTestSuite) TestFilterExpressionError() {
	a := NewArrayFilterAction()
	ctx := newTestContext(map[string]any{
		"input":     []any{map[string]any{"x": 1}},
		"condition": "item.nonexistent.deep > 0",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *ArrayActionValidateTestSuite) TestMapExpressionError() {
	a := NewArrayMapAction()
	ctx := newTestContext(map[string]any{
		"input":      []any{"a"},
		"expression": "item.deep.nested.value",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *ArrayActionValidateTestSuite) TestPickNonObjectItems() {
	a := NewArrayPickAction()
	ctx := newTestContext(map[string]any{
		"input":  []any{"not-a-map", 42},
		"fields": []any{"name"},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	items := out.([]any)
	s.Equal("not-a-map", items[0])
	s.Equal(42, items[1])
}

func (s *ArrayActionValidateTestSuite) TestSortMissingInput() {
	a := NewArraySortAction()
	ctx := newTestContext(map[string]any{
		"field": "name",
	})
	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "missing 'input'")
}

func (s *ArrayActionValidateTestSuite) TestCompareEqualValues() {
	a := NewArraySortAction()
	ctx := newTestContext(map[string]any{
		"input": []any{
			map[string]any{"val": 1.0},
			map[string]any{"val": 1.0},
			map[string]any{"val": 2.0},
		},
		"field": "val",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	items := out.([]any)
	s.Len(items, 3)
	s.Equal(1.0, items[0].(map[string]any)["val"])
}

func (s *ArrayActionValidateTestSuite) TestSortStringComparison() {
	a := NewArraySortAction()
	ctx := newTestContext(map[string]any{
		"input": []any{
			map[string]any{"val": true},
			map[string]any{"val": false},
		},
		"field": "val",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.NotNil(out)
}

func (s *ArrayActionValidateTestSuite) TestToAnySlice_NonSliceType() {
	result, ok := toAnySlice("not-a-slice")
	s.False(ok)
	s.Nil(result)
}

func (s *ArrayActionValidateTestSuite) TestToAnySlice_IntType() {
	result, ok := toAnySlice(42)
	s.False(ok)
	s.Nil(result)
}

func (s *ArrayActionValidateTestSuite) TestToAnySlice_TypedStringSlice() {
	input := []string{"a", "b", "c"}
	result, ok := toAnySlice(input)
	s.True(ok)
	s.Require().Len(result, 3)
	s.Equal("a", result[0])
	s.Equal("b", result[1])
	s.Equal("c", result[2])
}

func (s *ArrayActionValidateTestSuite) TestToAnySlice_TypedIntSlice() {
	input := []int{1, 2, 3}
	result, ok := toAnySlice(input)
	s.True(ok)
	s.Require().Len(result, 3)
	s.Equal(1, result[0])
	s.Equal(2, result[1])
	s.Equal(3, result[2])
}

func (s *ArrayActionValidateTestSuite) TestToAnySlice_Nil() {
	result, ok := toAnySlice(nil)
	s.False(ok)
	s.Nil(result)
}

func (s *ArrayActionValidateTestSuite) TestToAnySlice_EmptyTypedSlice() {
	input := []string{}
	result, ok := toAnySlice(input)
	s.True(ok)
	s.Require().Len(result, 0)
}

func (s *ArrayActionValidateTestSuite) TestToAnySlice_MapType() {
	result, ok := toAnySlice(map[string]string{"a": "b"})
	s.False(ok)
	s.Nil(result)
}

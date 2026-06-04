package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ArrayActionTestSuite struct {
	suite.Suite
}

func TestArrayAction(t *testing.T) {
	suite.Run(t, new(ArrayActionTestSuite))
}

func (s *ArrayActionTestSuite) SetupTest() {}

func (s *ArrayActionTestSuite) TestSortAsc() {
	a := NewArraySortAction()
	ctx := newTestContext(map[string]any{
		"input": []any{
			map[string]any{"name": "Charlie"},
			map[string]any{"name": "Alice"},
			map[string]any{"name": "Bob"},
		},
		"field":     "name",
		"direction": "asc",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	items := out.([]any)
	s.Require().Len(items, 3)
	s.Equal("Alice", items[0].(map[string]any)["name"])
	s.Equal("Bob", items[1].(map[string]any)["name"])
	s.Equal("Charlie", items[2].(map[string]any)["name"])
}

func (s *ArrayActionTestSuite) TestSortDesc() {
	a := NewArraySortAction()
	ctx := newTestContext(map[string]any{
		"input": []any{
			map[string]any{"name": "Alice"},
			map[string]any{"name": "Charlie"},
			map[string]any{"name": "Bob"},
		},
		"field":     "name",
		"direction": "desc",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	items := out.([]any)
	s.Require().Len(items, 3)
	s.Equal("Charlie", items[0].(map[string]any)["name"])
	s.Equal("Bob", items[1].(map[string]any)["name"])
	s.Equal("Alice", items[2].(map[string]any)["name"])
}

func (s *ArrayActionTestSuite) TestSortNumeric() {
	a := NewArraySortAction()
	ctx := newTestContext(map[string]any{
		"input": []any{
			map[string]any{"price": 30.0},
			map[string]any{"price": 10.0},
			map[string]any{"price": 20.0},
		},
		"field": "price",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	items := out.([]any)
	s.Require().Len(items, 3)
	s.Equal(10.0, items[0].(map[string]any)["price"])
	s.Equal(20.0, items[1].(map[string]any)["price"])
	s.Equal(30.0, items[2].(map[string]any)["price"])
}

func (s *ArrayActionTestSuite) TestSortValidateMissingInput() {
	a := NewArraySortAction()
	err := a.Validate(newTestContext(map[string]any{"field": "name"}))
	s.Error(err)
}

func (s *ArrayActionTestSuite) TestSortValidateMissingField() {
	a := NewArraySortAction()
	err := a.Validate(newTestContext(map[string]any{"input": []any{}}))
	s.Error(err)
}

func (s *ArrayActionTestSuite) TestFilterMatch() {
	a := NewArrayFilterAction()
	ctx := newTestContext(map[string]any{
		"input": []any{
			map[string]any{"name": "Alice", "active": true},
			map[string]any{"name": "Bob", "active": false},
			map[string]any{"name": "Charlie", "active": true},
		},
		"condition": "item.active == true",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	items := out.([]any)
	s.Require().Len(items, 2)
	s.Equal("Alice", items[0].(map[string]any)["name"])
	s.Equal("Charlie", items[1].(map[string]any)["name"])
}

func (s *ArrayActionTestSuite) TestFilterEmpty() {
	a := NewArrayFilterAction()
	ctx := newTestContext(map[string]any{
		"input": []any{
			map[string]any{"name": "Alice", "age": 15.0},
			map[string]any{"name": "Bob", "age": 17.0},
		},
		"condition": "item.age > 18",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	items := out.([]any)
	s.Len(items, 0)
}

func (s *ArrayActionTestSuite) TestFilterValidateMissingCondition() {
	a := NewArrayFilterAction()
	err := a.Validate(newTestContext(map[string]any{"input": []any{}}))
	s.Error(err)
}

func (s *ArrayActionTestSuite) TestMapTransform() {
	a := NewArrayMapAction()
	ctx := newTestContext(map[string]any{
		"input": []any{
			map[string]any{"first": "Alice", "last": "Smith"},
			map[string]any{"first": "Bob", "last": "Jones"},
		},
		"expression": "({ name: item.first + ' ' + item.last })",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	items := out.([]any)
	s.Require().Len(items, 2)
	s.Equal("Alice Smith", items[0].(map[string]any)["name"])
	s.Equal("Bob Jones", items[1].(map[string]any)["name"])
}

func (s *ArrayActionTestSuite) TestMapWithIndex() {
	a := NewArrayMapAction()
	ctx := newTestContext(map[string]any{
		"input":      []any{"a", "b", "c"},
		"expression": "index",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	items := out.([]any)
	s.Require().Len(items, 3)
	s.Equal(int64(0), items[0])
	s.Equal(int64(1), items[1])
	s.Equal(int64(2), items[2])
}

func (s *ArrayActionTestSuite) TestMapValidateMissingExpression() {
	a := NewArrayMapAction()
	err := a.Validate(newTestContext(map[string]any{"input": []any{}}))
	s.Error(err)
}

func (s *ArrayActionTestSuite) TestUniqDedup() {
	a := NewArrayUniqAction()
	ctx := newTestContext(map[string]any{
		"input": []any{
			map[string]any{"email": "a@b.com", "name": "Alice"},
			map[string]any{"email": "c@d.com", "name": "Bob"},
			map[string]any{"email": "a@b.com", "name": "Alice Dup"},
		},
		"field": "email",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	items := out.([]any)
	s.Require().Len(items, 2)
	s.Equal("Alice", items[0].(map[string]any)["name"])
	s.Equal("Bob", items[1].(map[string]any)["name"])
}

func (s *ArrayActionTestSuite) TestUniqEmptyInput() {
	a := NewArrayUniqAction()
	ctx := newTestContext(map[string]any{
		"input": []any{},
		"field": "id",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Len(out.([]any), 0)
}

func (s *ArrayActionTestSuite) TestUniqValidateMissingField() {
	a := NewArrayUniqAction()
	err := a.Validate(newTestContext(map[string]any{"input": []any{}}))
	s.Error(err)
}

func (s *ArrayActionTestSuite) TestPickFields() {
	a := NewArrayPickAction()
	ctx := newTestContext(map[string]any{
		"input": []any{
			map[string]any{"name": "Alice", "email": "a@b.com", "age": 30},
			map[string]any{"name": "Bob", "email": "c@d.com", "age": 25},
		},
		"fields": []any{"name", "email"},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	items := out.([]any)
	s.Require().Len(items, 2)

	first := items[0].(map[string]any)
	s.Equal("Alice", first["name"])
	s.Equal("a@b.com", first["email"])
	s.Nil(first["age"])
}

func (s *ArrayActionTestSuite) TestPickMissingField() {
	a := NewArrayPickAction()
	ctx := newTestContext(map[string]any{
		"input": []any{
			map[string]any{"name": "Alice"},
		},
		"fields": []any{"name", "nonexistent"},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	items := out.([]any)
	first := items[0].(map[string]any)
	s.Equal("Alice", first["name"])
	_, exists := first["nonexistent"]
	s.False(exists)
}

func (s *ArrayActionTestSuite) TestPickValidateMissingFields() {
	a := NewArrayPickAction()
	err := a.Validate(newTestContext(map[string]any{"input": []any{}}))
	s.Error(err)
}

func (s *ArrayActionTestSuite) TestPickInvalidFieldsType() {
	a := NewArrayPickAction()
	ctx := newTestContext(map[string]any{
		"input":  []any{},
		"fields": "name",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *ArrayActionTestSuite) TestConcatBasic() {
	a := NewArrayConcatAction()
	ctx := newTestContext(map[string]any{
		"arrays": []any{
			[]any{1, 2},
			[]any{3, 4},
			[]any{5},
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	items := out.([]any)
	s.Equal([]any{1, 2, 3, 4, 5}, items)
}

func (s *ArrayActionTestSuite) TestConcatEmpty() {
	a := NewArrayConcatAction()
	ctx := newTestContext(map[string]any{
		"arrays": []any{},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Len(out.([]any), 0)
}

func (s *ArrayActionTestSuite) TestConcatInvalidItem() {
	a := NewArrayConcatAction()
	ctx := newTestContext(map[string]any{
		"arrays": []any{
			[]any{1},
			"not an array",
		},
	})

	_, err := a.Execute(ctx)
	s.ErrorContains(err, "not an array")
}

func (s *ArrayActionTestSuite) TestConcatValidateMissingArrays() {
	a := NewArrayConcatAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
}


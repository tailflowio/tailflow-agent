package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type TableActionTestSuite struct {
	suite.Suite
}

func TestTableAction(t *testing.T) {
	suite.Run(t, new(TableActionTestSuite))
}

func (s *TableActionTestSuite) TestValidate_MissingColumns() {
	a := NewTableAction()
	ctx := newTestContext(map[string]any{
		"items": []any{},
	})

	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "columns")
}

func (s *TableActionTestSuite) TestValidate_OK() {
	a := NewTableAction()
	ctx := newTestContext(map[string]any{
		"columns": []any{
			map[string]any{"header": "Name", "field": "name"},
		},
	})

	err := a.Validate(ctx)
	s.NoError(err)
}

func (s *TableActionTestSuite) TestExecute_BasicTable() {
	a := NewTableAction()
	ctx := newTestContext(map[string]any{
		"items": []any{
			map[string]any{"name": "alice", "age": 30},
			map[string]any{"name": "bob", "age": 25},
		},
		"columns": []any{
			map[string]any{"header": "Name", "field": "name"},
			map[string]any{"header": "Age", "field": "age"},
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	outMap := out.(map[string]any)
	s.Equal(2, outMap["count"])
	s.Contains(outMap["table"], "alice")
	s.Contains(outMap["table"], "bob")
	s.Contains(outMap["table"], "Name")
	s.Contains(outMap["table"], "Age")
}

func (s *TableActionTestSuite) TestExecute_NestedField() {
	a := NewTableAction()
	ctx := newTestContext(map[string]any{
		"items": []any{
			map[string]any{
				"item": map[string]any{"path": "my-project"},
				"message": "exit status 128",
			},
		},
		"columns": []any{
			map[string]any{"header": "Project", "field": "item.path"},
			map[string]any{"header": "Error", "field": "message"},
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	outMap := out.(map[string]any)
	s.Contains(outMap["table"], "my-project")
	s.Contains(outMap["table"], "exit status 128")
}

func (s *TableActionTestSuite) TestExecute_EmptyItems() {
	a := NewTableAction()
	ctx := newTestContext(map[string]any{
		"columns": []any{
			map[string]any{"header": "Name", "field": "name"},
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	outMap := out.(map[string]any)
	s.Equal(0, outMap["count"])
	s.Contains(outMap["table"], "Name")
}

func (s *TableActionTestSuite) TestExecute_WithTitle() {
	a := NewTableAction()
	ctx := newTestContext(map[string]any{
		"title": "Results",
		"items": []any{
			map[string]any{"x": "ok"},
		},
		"columns": []any{
			map[string]any{"header": "Status", "field": "x"},
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	outMap := out.(map[string]any)
	s.True(len(outMap["table"].(string)) > 0)
	s.Contains(outMap["table"], "Results")
}

func (s *TableActionTestSuite) TestExecute_HeaderDefaultsToField() {
	a := NewTableAction()
	ctx := newTestContext(map[string]any{
		"items": []any{
			map[string]any{"name": "test"},
		},
		"columns": []any{
			map[string]any{"field": "name"},
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	outMap := out.(map[string]any)
	s.Contains(outMap["table"], "name")
	s.Contains(outMap["table"], "test")
}

func (s *TableActionTestSuite) TestExecute_WithEmitPrint() {
	a := NewTableAction()

	var logged []string

	ctx := newTestContext(map[string]any{
		"items": []any{
			map[string]any{"x": "val"},
		},
		"columns": []any{
			map[string]any{"header": "Col", "field": "x"},
		},
	})
	ctx.EmitPrint = func(msg string) {
		logged = append(logged, msg)
	}

	_, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.True(len(logged) > 0)
}

func (s *TableActionTestSuite) TestExecute_InvalidColumnsReturnsError() {
	a := NewTableAction()
	ctx := newTestContext(map[string]any{
		"items":   []any{},
		"columns": "not-an-array",
	})

	out, err := a.Execute(ctx)
	s.Nil(out)
	s.Error(err)
	s.Contains(err.Error(), "table:")
}

func (s *TableActionTestSuite) TestSortRows_NoSortConfig() {
	rows := [][]string{{"b"}, {"a"}}
	columns := []tableColumn{{Header: "Name", Field: "name"}}
	config := map[string]any{}

	sortRows(rows, columns, config)

	// Order is unchanged because no sort key was provided.
	s.Equal("b", rows[0][0])
	s.Equal("a", rows[1][0])
}

func (s *TableActionTestSuite) TestSortRows_SortFieldNotInColumns() {
	rows := [][]string{{"b"}, {"a"}}
	columns := []tableColumn{{Header: "Name", Field: "name"}}
	config := map[string]any{"sort": "nonexistent"}

	sortRows(rows, columns, config)

	// Order is unchanged because the sort field does not match any column.
	s.Equal("b", rows[0][0])
	s.Equal("a", rows[1][0])
}

func (s *TableActionTestSuite) TestSortRows_SortByField() {
	rows := [][]string{
		{"charlie", "3"},
		{"alice", "1"},
		{"bob", "2"},
	}
	columns := []tableColumn{
		{Header: "Name", Field: "name"},
		{Header: "ID", Field: "id"},
	}
	config := map[string]any{"sort": "name"}

	sortRows(rows, columns, config)

	s.Equal("alice", rows[0][0])
	s.Equal("bob", rows[1][0])
	s.Equal("charlie", rows[2][0])
}

func (s *TableActionTestSuite) TestSortRows_SortByHeader() {
	rows := [][]string{
		{"charlie"},
		{"alice"},
	}
	columns := []tableColumn{
		{Header: "DisplayName", Field: "name"},
	}
	config := map[string]any{"sort": "DisplayName"}

	sortRows(rows, columns, config)

	s.Equal("alice", rows[0][0])
	s.Equal("charlie", rows[1][0])
}

func (s *TableActionTestSuite) TestSortRows_SortConfigNotString() {
	rows := [][]string{{"b"}, {"a"}}
	columns := []tableColumn{{Header: "Name", Field: "name"}}
	config := map[string]any{"sort": 123}

	sortRows(rows, columns, config)

	// Order is unchanged because sort value is not a string.
	s.Equal("b", rows[0][0])
	s.Equal("a", rows[1][0])
}

func (s *TableActionTestSuite) TestExecute_WithSort() {
	a := NewTableAction()
	ctx := newTestContext(map[string]any{
		"items": []any{
			map[string]any{"name": "charlie"},
			map[string]any{"name": "alice"},
			map[string]any{"name": "bob"},
		},
		"columns": []any{
			map[string]any{"header": "Name", "field": "name"},
		},
		"sort": "name",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	table := out.(map[string]any)["table"].(string)
	idxAlice := len(table)
	idxBob := len(table)
	idxCharlie := len(table)

	for i := range len(table) {
		if i+5 <= len(table) && table[i:i+5] == "alice" {
			idxAlice = i
		}
		if i+3 <= len(table) && table[i:i+3] == "bob" {
			idxBob = i
		}
		if i+7 <= len(table) && table[i:i+7] == "charlie" {
			idxCharlie = i
		}
	}

	s.Less(idxAlice, idxBob)
	s.Less(idxBob, idxCharlie)
}

func (s *TableActionTestSuite) TestParseColumns_NotAnArray() {
	_, err := parseColumns("not-an-array")
	s.Error(err)
	s.Contains(err.Error(), "'columns' must be an array")
}

func (s *TableActionTestSuite) TestParseColumns_EmptyArray() {
	_, err := parseColumns([]any{})
	s.Error(err)
	s.Contains(err.Error(), "'columns' must not be empty")
}

func (s *TableActionTestSuite) TestParseColumns_ElementNotObject() {
	_, err := parseColumns([]any{"not-a-map"})
	s.Error(err)
	s.Contains(err.Error(), "columns[0]: must be an object")
}

func (s *TableActionTestSuite) TestParseColumns_MissingField() {
	_, err := parseColumns([]any{
		map[string]any{"header": "Name"},
	})
	s.Error(err)
	s.Contains(err.Error(), "columns[0]: missing 'field'")
}

func (s *TableActionTestSuite) TestParseColumns_EmptyFieldString() {
	_, err := parseColumns([]any{
		map[string]any{"field": ""},
	})
	s.Error(err)
	s.Contains(err.Error(), "missing 'field'")
}

func (s *TableActionTestSuite) TestParseColumns_FieldNotString() {
	_, err := parseColumns([]any{
		map[string]any{"field": 123},
	})
	s.Error(err)
	s.Contains(err.Error(), "missing 'field'")
}

func (s *TableActionTestSuite) TestParseColumns_Valid() {
	cols, err := parseColumns([]any{
		map[string]any{"header": "Name", "field": "name"},
		map[string]any{"field": "age"},
	})
	s.NoError(err)
	s.Len(cols, 2)
	s.Equal("Name", cols[0].Header)
	s.Equal("name", cols[0].Field)
	// When header is missing, it defaults to the field name.
	s.Equal("age", cols[1].Header)
	s.Equal("age", cols[1].Field)
}

func (s *TableActionTestSuite) TestResolveField_SimpleKey() {
	obj := map[string]any{"name": "alice"}
	result := resolveField(obj, "name")
	s.Equal("alice", result)
}

func (s *TableActionTestSuite) TestResolveField_NestedKey() {
	obj := map[string]any{
		"a": map[string]any{
			"b": "deep",
		},
	}
	result := resolveField(obj, "a.b")
	s.Equal("deep", result)
}

func (s *TableActionTestSuite) TestResolveField_NonMapObject() {
	result := resolveField("a plain string", "field")
	s.Equal("", result)
}

func (s *TableActionTestSuite) TestResolveField_NilValue() {
	obj := map[string]any{"name": nil}
	result := resolveField(obj, "name")
	s.Equal("", result)
}

func (s *TableActionTestSuite) TestResolveField_MissingKey() {
	obj := map[string]any{"name": "alice"}
	result := resolveField(obj, "missing")
	s.Equal("", result)
}

func (s *TableActionTestSuite) TestResolveField_IntermediateNonMap() {
	obj := map[string]any{"a": "not-a-map"}
	result := resolveField(obj, "a.b")
	s.Equal("", result)
}

func (s *TableActionTestSuite) TestResolveField_IntermediateNil() {
	obj := map[string]any{"a": nil}
	result := resolveField(obj, "a.b")
	s.Equal("", result)
}

func (s *TableActionTestSuite) TestSanitizeCell_PlainString() {
	result := sanitizeCell("hello")
	s.Equal("hello", result)
}

func (s *TableActionTestSuite) TestSanitizeCell_LeadingTrailingWhitespace() {
	result := sanitizeCell("  hello  ")
	s.Equal("hello", result)
}

func (s *TableActionTestSuite) TestSanitizeCell_Newline() {
	result := sanitizeCell("first line\nsecond line")
	s.Equal("first line", result)
}

func (s *TableActionTestSuite) TestSanitizeCell_NewlineWithWhitespace() {
	result := sanitizeCell("  first line  \n  second line  ")
	s.Equal("first line", result)
}

func (s *TableActionTestSuite) TestSanitizeCell_OnlyNewline() {
	result := sanitizeCell("\n")
	s.Equal("", result)
}

func (s *TableActionTestSuite) TestSanitizeCell_EmptyString() {
	result := sanitizeCell("")
	s.Equal("", result)
}

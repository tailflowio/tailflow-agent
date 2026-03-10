package action

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// TableAction formats an array of items as an ASCII table and logs it.
type TableAction struct{}

func NewTableAction() Action { return &TableAction{} }

func (a *TableAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["columns"]
	if !ok {
		return errors.New("table action requires 'columns' in config")
	}

	return nil
}

type tableColumn struct {
	Header string
	Field  string
}

func (a *TableAction) Execute(ctx *ActionContext) (any, error) {
	columns, err := parseColumns(ctx.Config["columns"])
	if err != nil {
		return nil, fmt.Errorf("table: %w", err)
	}

	items, _ := toAnySlice(ctx.Config["items"])

	rows := buildRows(items, columns)
	sortRows(rows, columns, ctx.Config)

	widths := computeColumnWidths(columns, rows)
	table := renderTable(columns, rows, widths, items, ctx.Config)

	emitTableLog(ctx, table)
	ctx.Logger.Info(table)

	return map[string]any{
		"table": table,
		"count": len(items),
	}, nil
}

func buildRows(items []any, columns []tableColumn) [][]string {
	rows := make([][]string, len(items))
	for i, item := range items {
		row := make([]string, len(columns))
		for j, col := range columns {
			raw := fmt.Sprintf("%v", resolveField(item, col.Field))
			row[j] = sanitizeCell(raw)
		}

		rows[i] = row
	}

	return rows
}

func sortRows(rows [][]string, columns []tableColumn, config map[string]any) {
	sortField, ok := config["sort"].(string)
	if !ok {
		return
	}

	sortIdx := -1

	for j, col := range columns {
		if col.Field == sortField || col.Header == sortField {
			sortIdx = j
			break
		}
	}

	if sortIdx < 0 {
		return
	}

	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i][sortIdx] < rows[j][sortIdx]
	})
}

func computeColumnWidths(columns []tableColumn, rows [][]string) []int {
	widths := make([]int, len(columns))
	for j, col := range columns {
		widths[j] = len(col.Header)
	}

	for _, row := range rows {
		for j, cell := range row {
			if len(cell) > widths[j] {
				widths[j] = len(cell)
			}
		}
	}

	return widths
}

func renderTable(columns []tableColumn, rows [][]string, widths []int, items []any, config map[string]any) string {
	var sb strings.Builder

	title, _ := config["title"].(string)
	if title != "" {
		sb.WriteString(title)
		sb.WriteByte('\n')
	}

	writeSeparator(&sb, widths)
	writeRow(&sb, widths, headerStrings(columns))
	writeSeparator(&sb, widths)

	for _, row := range rows {
		writeRow(&sb, widths, row)
	}

	if len(rows) > 0 {
		writeSeparator(&sb, widths)
	}

	if len(items) == 0 {
		writeRow(&sb, widths, emptyRow(len(columns), "\u2014"))
		writeSeparator(&sb, widths)
	}

	return sb.String()
}

func emitTableLog(ctx *ActionContext, table string) {
	if ctx.EmitPrint == nil {
		return
	}

	for _, line := range strings.Split(strings.TrimRight(table, "\n"), "\n") {
		ctx.EmitPrint(line)
	}
}

func parseColumns(raw any) ([]tableColumn, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, errors.New("'columns' must be an array")
	}

	if len(arr) == 0 {
		return nil, errors.New("'columns' must not be empty")
	}

	cols := make([]tableColumn, 0, len(arr))

	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("columns[%d]: must be an object", i)
		}

		field, _ := m["field"].(string)
		if field == "" {
			return nil, fmt.Errorf("columns[%d]: missing 'field'", i)
		}

		header, _ := m["header"].(string)
		if header == "" {
			header = field
		}

		cols = append(cols, tableColumn{Header: header, Field: field})
	}

	return cols, nil
}

func resolveField(obj any, field string) any {
	parts := strings.Split(field, ".")
	current := obj

	for _, p := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}

		current = m[p]
	}

	if current == nil {
		return ""
	}

	return current
}

func writeSeparator(sb *strings.Builder, widths []int) {
	sb.WriteByte('+')

	for _, w := range widths {
		sb.WriteString(strings.Repeat("-", w+2))
		sb.WriteByte('+')
	}

	sb.WriteByte('\n')
}

func writeRow(sb *strings.Builder, widths []int, cells []string) {
	sb.WriteByte('|')

	for j, cell := range cells {
		sb.WriteByte(' ')
		sb.WriteString(cell)
		sb.WriteString(strings.Repeat(" ", widths[j]-len(cell)))
		sb.WriteString(" |")
	}

	sb.WriteByte('\n')
}

func headerStrings(cols []tableColumn) []string {
	h := make([]string, len(cols))
	for i, c := range cols {
		h[i] = c.Header
	}

	return h
}

func emptyRow(n int, fill string) []string {
	row := make([]string, n)
	for i := range row {
		row[i] = fill
	}

	return row
}

func sanitizeCell(s string) string {
	s = strings.TrimSpace(s)

	idx := strings.IndexByte(s, '\n')
	if idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}

	return s
}

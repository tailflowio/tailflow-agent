package action

import (
	"database/sql"
	"errors"
	"fmt"
)

// SQLQueryAction executes a SQL SELECT query and returns rows.
type SQLQueryAction struct{}

func NewSQLQueryAction() Action { return &SQLQueryAction{} }

func (a *SQLQueryAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["query"]; !ok {
		return errors.New("sql.query action requires 'query' in config")
	}
	// dsn is required unless tx is provided
	_, hasDSN := ctx.Config["dsn"]
	_, hasTx := ctx.Config["tx"]

	if !hasDSN && !hasTx {
		return errors.New("sql.query action requires 'dsn' or 'tx' in config")
	}

	if hasTx {
		if ctx.Services == nil || ctx.Services.TxRegistry == nil {
			return errors.New("sql.query with 'tx' requires 'tailflow serve' (server mode)")
		}
	} else {
		if ctx.Services == nil || ctx.Services.DBPool == nil {
			return errors.New("sql.query action requires 'tailflow serve' (server mode)")
		}
	}

	return nil
}

func (a *SQLQueryAction) Execute(ctx *ActionContext) (any, error) {
	query := fmt.Sprintf("%v", ctx.Config["query"])
	params := toSlice(ctx.Config["params"])

	var rows *sql.Rows
	var err error

	if txName, ok := ctx.Config["tx"]; ok {
		tx, txErr := ctx.Services.TxRegistry.Get(fmt.Sprintf("%v", txName))
		if txErr != nil {
			return nil, fmt.Errorf("sql.query: %w", txErr)
		}

		rows, err = tx.QueryContext(ctx, query, params...)
	} else {
		dsn := fmt.Sprintf("%v", ctx.Config["dsn"])

		db, dbErr := ctx.Services.DBPool.Get(ctx, dsn)
		if dbErr != nil {
			return nil, fmt.Errorf("sql.query: %w", dbErr)
		}

		rows, err = db.QueryContext(ctx, query, params...)
	}

	if err != nil {
		return nil, fmt.Errorf("sql.query: %w", err)
	}

	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("sql.query: columns: %w", err)
	}

	var result []map[string]any

	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))

		for i := range values {
			ptrs[i] = &values[i]
		}

		err := rows.Scan(ptrs...)
		if err != nil {
			return nil, fmt.Errorf("sql.query: scan: %w", err)
		}

		row := make(map[string]any, len(cols))

		for i, col := range cols {
			v := values[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}

			row[col] = v
		}

		result = append(result, row)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("sql.query: iterate: %w", err)
	}

	if result == nil {
		result = []map[string]any{}
	}

	return map[string]any{
		"rows":  result,
		"count": len(result),
	}, nil
}

// toSlice converts a config value to a []any slice for SQL parameters.
func toSlice(v any) []any {
	if v == nil {
		return nil
	}

	switch s := v.(type) {
	case []any:
		return s
	case []string:
		out := make([]any, len(s))
		for i, val := range s {
			out[i] = val
		}

		return out
	default:
		return []any{v}
	}
}

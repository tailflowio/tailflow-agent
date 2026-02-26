package action

import (
	"database/sql"
	"errors"
	"fmt"
)

type SQLExecAction struct{}

func NewSQLExecAction() Action { return &SQLExecAction{} }

func (a *SQLExecAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["query"]; !ok {
		return errors.New("sql.exec action requires 'query' in config")
	}

	_, hasDSN := ctx.Config["dsn"]
	_, hasTx := ctx.Config["tx"]

	if !hasDSN && !hasTx {
		return errors.New("sql.exec action requires 'dsn' or 'tx' in config")
	}

	if hasTx {
		if ctx.Services == nil || ctx.Services.TxRegistry == nil {
			return errors.New("sql.exec with 'tx' requires 'tailflow serve' (server mode)")
		}
	} else {
		if ctx.Services == nil || ctx.Services.DBPool == nil {
			return errors.New("sql.exec action requires 'tailflow serve' (server mode)")
		}
	}

	return nil
}

func (a *SQLExecAction) Execute(ctx *ActionContext) (any, error) {
	query := fmt.Sprintf("%v", ctx.Config["query"])
	params := toSlice(ctx.Config["params"])

	var res sql.Result
	var err error

	if txName, ok := ctx.Config["tx"]; ok {
		tx, txErr := ctx.Services.TxRegistry.Get(fmt.Sprintf("%v", txName))
		if txErr != nil {
			return nil, fmt.Errorf("sql.exec: %w", txErr)
		}

		res, err = tx.ExecContext(ctx, query, params...)
	} else {
		dsn := fmt.Sprintf("%v", ctx.Config["dsn"])

		db, dbErr := ctx.Services.DBPool.Get(ctx, dsn)
		if dbErr != nil {
			return nil, fmt.Errorf("sql.exec: %w", dbErr)
		}

		res, err = db.ExecContext(ctx, query, params...)
	}

	if err != nil {
		return nil, fmt.Errorf("sql.exec: %w", err)
	}

	affected, affectedErr := res.RowsAffected()
	if affectedErr != nil {
		return nil, fmt.Errorf("sql.exec: rows affected: %w", affectedErr)
	}

	lastID, lastIDErr := res.LastInsertId()
	if lastIDErr != nil {
		return nil, fmt.Errorf("sql.exec: last insert id: %w", lastIDErr)
	}

	return map[string]any{
		"affected":       affected,
		"last_insert_id": lastID,
	}, nil
}

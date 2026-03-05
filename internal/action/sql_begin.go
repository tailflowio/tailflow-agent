package action

import (
	"errors"
	"fmt"
)

// SQLBeginAction starts a new SQL transaction.
type SQLBeginAction struct{}

func NewSQLBeginAction() Action { return &SQLBeginAction{} }

func (a *SQLBeginAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["dsn"]
	if !ok {
		return errors.New("sql.begin action requires 'dsn' in config")
	}

	_, ok = ctx.Config["name"]
	if !ok {
		return errors.New("sql.begin action requires 'name' in config")
	}

	if ctx.Services == nil || ctx.Services.DBPool == nil {
		return errors.New("sql.begin action requires 'tailflow serve' (server mode)")
	}

	if ctx.Services.TxRegistry == nil {
		return errors.New("sql.begin action requires 'tailflow serve' (server mode)")
	}

	return nil
}

func (a *SQLBeginAction) Execute(ctx *ActionContext) (any, error) {
	dsn := fmt.Sprintf("%v", ctx.Config["dsn"])
	name := fmt.Sprintf("%v", ctx.Config["name"])

	db, err := ctx.Services.DBPool.Get(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.begin: %w", err)
	}

	_, err = ctx.Services.TxRegistry.Begin(ctx, db, name)
	if err != nil {
		return nil, fmt.Errorf("sql.begin: %w", err)
	}

	return map[string]any{
		"name":    name,
		"started": true,
	}, nil
}

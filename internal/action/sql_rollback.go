package action

import (
	"errors"
	"fmt"
)

// SQLRollbackAction rolls back a named SQL transaction.
type SQLRollbackAction struct{}

func NewSQLRollbackAction() Action { return &SQLRollbackAction{} }

func (a *SQLRollbackAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["name"]
	if !ok {
		return errors.New("sql.rollback action requires 'name' in config")
	}

	if ctx.Services == nil || ctx.Services.TxRegistry == nil {
		return errors.New("sql.rollback action requires 'tailflow serve' (server mode)")
	}

	return nil
}

func (a *SQLRollbackAction) Execute(ctx *ActionContext) (any, error) {
	name := fmt.Sprintf("%v", ctx.Config["name"])

	err := ctx.Services.TxRegistry.Rollback(name)
	if err != nil {
		return nil, fmt.Errorf("sql.rollback: %w", err)
	}

	return map[string]any{
		"name":        name,
		"rolled_back": true,
	}, nil
}

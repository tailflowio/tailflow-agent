package action

import (
	"errors"
	"fmt"
)

// SQLCommitAction commits a named SQL transaction.
type SQLCommitAction struct{}

func NewSQLCommitAction() Action { return &SQLCommitAction{} }

func (a *SQLCommitAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["name"]; !ok {
		return errors.New("sql.commit action requires 'name' in config")
	}

	if ctx.Services == nil || ctx.Services.TxRegistry == nil {
		return errors.New("sql.commit action requires 'tailflow serve' (server mode)")
	}

	return nil
}

func (a *SQLCommitAction) Execute(ctx *ActionContext) (any, error) {
	name := fmt.Sprintf("%v", ctx.Config["name"])

	err := ctx.Services.TxRegistry.Commit(name)
	if err != nil {
		return nil, fmt.Errorf("sql.commit: %w", err)
	}

	return map[string]any{
		"name":      name,
		"committed": true,
	}, nil
}

package action

import (
	"errors"
	"fmt"
)

// UnlockAction releases a distributed lock.
type UnlockAction struct{}

func NewUnlockAction() Action { return &UnlockAction{} }

func (a *UnlockAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["key"]
	if !ok {
		return errors.New("unlock action requires 'key' in config")
	}

	if ctx.Services == nil || ctx.Services.Locker == nil {
		return errors.New("unlock action requires 'tailflow serve' (server mode)")
	}

	return nil
}

func (a *UnlockAction) Execute(ctx *ActionContext) (any, error) {
	key := fmt.Sprintf("%v", ctx.Config["key"])

	err := ctx.Services.Locker.Unlock(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("unlock: %w", err)
	}

	return map[string]any{
		"released": true,
		"key":      key,
	}, nil
}

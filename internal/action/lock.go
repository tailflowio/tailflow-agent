package action

import (
	"errors"
	"fmt"
	"time"
)

// LockAction acquires a distributed lock.
type LockAction struct{}

func NewLockAction() Action { return &LockAction{} }

func (a *LockAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["key"]; !ok {
		return errors.New("lock action requires 'key' in config")
	}

	if ctx.Services == nil || ctx.Services.Locker == nil {
		return errors.New("lock action requires 'tailflow serve' (server mode)")
	}

	return nil
}

func (a *LockAction) Execute(ctx *ActionContext) (any, error) {
	key := fmt.Sprintf("%v", ctx.Config["key"])

	timeout := 30 * time.Second

	if t, ok := ctx.Config["timeout"]; ok {
		if d, err := time.ParseDuration(fmt.Sprintf("%v", t)); err == nil {
			timeout = d
		}
	}

	err := ctx.Services.Locker.Lock(ctx, key, timeout)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}

	return map[string]any{
		"acquired": true,
		"key":      key,
	}, nil
}

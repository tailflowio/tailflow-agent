package action

import (
	"errors"
	"fmt"
	"time"
)

type KVGetAction struct{}

func NewKVGetAction() Action { return &KVGetAction{} }

func (a *KVGetAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["key"]; !ok {
		return errors.New("kv.get action requires 'key' in config")
	}

	if ctx.Services == nil || ctx.Services.KVStore == nil {
		return errors.New("kv.get action requires 'tailflow serve' (server mode)")
	}

	return nil
}

func (a *KVGetAction) Execute(ctx *ActionContext) (any, error) {
	key := fmt.Sprintf("%v", ctx.Config["key"])

	value, found := ctx.Services.KVStore.Get(ctx, key)

	return map[string]any{
		"key":   key,
		"value": value,
		"found": found,
	}, nil
}

type KVSetAction struct{}

func NewKVSetAction() Action { return &KVSetAction{} }

func (a *KVSetAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["key"]; !ok {
		return errors.New("kv.set action requires 'key' in config")
	}

	if _, ok := ctx.Config["value"]; !ok {
		return errors.New("kv.set action requires 'value' in config")
	}

	if ctx.Services == nil || ctx.Services.KVStore == nil {
		return errors.New("kv.set action requires 'tailflow serve' (server mode)")
	}

	return nil
}

func (a *KVSetAction) Execute(ctx *ActionContext) (any, error) {
	key := fmt.Sprintf("%v", ctx.Config["key"])
	value := ctx.Config["value"]

	var ttl time.Duration

	if t, ok := ctx.Config["ttl"]; ok {
		if d, err := time.ParseDuration(fmt.Sprintf("%v", t)); err == nil {
			ttl = d
		}
	}

	ctx.Services.KVStore.Set(ctx, key, value, ttl)

	result := map[string]any{
		"key":    key,
		"stored": true,
	}
	if ttl > 0 {
		result["ttl"] = ttl.String()
	}

	return result, nil
}

// KVDeleteAction deletes a key from the KV store.
type KVDeleteAction struct{}

func NewKVDeleteAction() Action { return &KVDeleteAction{} }

func (a *KVDeleteAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["key"]; !ok {
		return errors.New("kv.delete action requires 'key' in config")
	}

	if ctx.Services == nil || ctx.Services.KVStore == nil {
		return errors.New("kv.delete action requires 'tailflow serve' (server mode)")
	}

	return nil
}

func (a *KVDeleteAction) Execute(ctx *ActionContext) (any, error) {
	key := fmt.Sprintf("%v", ctx.Config["key"])

	deleted, err := ctx.Services.KVStore.Delete(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("kv.delete: %w", err)
	}

	return map[string]any{
		"key":     key,
		"deleted": deleted,
	}, nil
}

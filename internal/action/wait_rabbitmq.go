package action

import (
	"errors"
	"fmt"
	"time"
)

type WaitRabbitMQAction struct{}

func NewWaitRabbitMQAction() Action { return &WaitRabbitMQAction{} }

func (a *WaitRabbitMQAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["url"]
	if !ok {
		return errors.New("wait.rabbitmq requires 'url' in config")
	}

	_, ok = ctx.Config["queue"]
	if !ok {
		return errors.New("wait.rabbitmq requires 'queue' in config")
	}

	if ctx.Services == nil || ctx.Services.WaitRabbitMQRegister == nil {
		return errors.New("wait.rabbitmq requires 'tailflow serve' (server mode)")
	}

	return nil
}

type waitRabbitMQConfig struct {
	url        string
	queue      string
	matchField string
	matchValue string
	timeout    time.Duration
}

func parseWaitRabbitMQConfig(ctx *ActionContext) (waitRabbitMQConfig, error) {
	cfg := waitRabbitMQConfig{
		url:     fmt.Sprintf("%v", ctx.Config["url"]),
		queue:   fmt.Sprintf("%v", ctx.Config["queue"]),
		timeout: 5 * time.Minute,
	}

	v, ok := ctx.Config["match"]
	if ok {
		cfg.matchField = fmt.Sprintf("%v", v)
	}

	v, ok = ctx.Config["match_value"]
	if ok {
		cfg.matchValue = fmt.Sprintf("%v", v)
	}

	t, ok := ctx.Config["timeout"]
	if ok {
		dur, err := time.ParseDuration(fmt.Sprintf("%v", t))
		if err != nil {
			return cfg, fmt.Errorf("wait.rabbitmq: invalid timeout %q: %w", t, err)
		}

		cfg.timeout = dur
	}

	return cfg, nil
}

func (a *WaitRabbitMQAction) Execute(ctx *ActionContext) (any, error) {
	cfg, err := parseWaitRabbitMQConfig(ctx)
	if err != nil {
		return nil, err
	}

	ch, cleanup := ctx.Services.WaitRabbitMQRegister(cfg.url, cfg.queue, cfg.matchField, cfg.matchValue, ctx)
	defer cleanup()

	emitWaitingSignal(ctx, cfg)

	ctx.Logger.Info("waiting for message", "queue", cfg.queue, "match", cfg.matchField, "timeout", cfg.timeout)

	return awaitMessage(ctx, ch, cfg)
}

func emitWaitingSignal(ctx *ActionContext, cfg waitRabbitMQConfig) {
	if ctx.Services.EmitWaiting == nil {
		return
	}

	details := map[string]any{
		"queue":   cfg.queue,
		"timeout": cfg.timeout.String(),
	}
	if cfg.matchField != "" {
		details["match"] = cfg.matchField
		details["match_value"] = cfg.matchValue
	}

	ctx.Services.EmitWaiting(ctx.ExecCtx.ExecutionID, ctx.StepID, "rabbitmq", details)
}

func awaitMessage(ctx *ActionContext, ch <-chan map[string]any, cfg waitRabbitMQConfig) (any, error) {
	select {
	case msg, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("wait.rabbitmq: connection failed for queue %s", cfg.queue)
		}

		return msg, nil

	case <-time.After(cfg.timeout):
		return nil, fmt.Errorf("wait.rabbitmq: timeout after %s waiting on queue %s", cfg.timeout, cfg.queue)

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

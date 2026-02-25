package action

import (
	"errors"
	"fmt"
	"time"
)

// WaitRabbitMQAction blocks until a matching message is received from a RabbitMQ queue.
// Uses the server-side RabbitMQWaitManager for shared connections and consumers.
type WaitRabbitMQAction struct{}

func NewWaitRabbitMQAction() Action { return &WaitRabbitMQAction{} }

func (a *WaitRabbitMQAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["url"]; !ok {
		return errors.New("wait.rabbitmq requires 'url' in config")
	}

	if _, ok := ctx.Config["queue"]; !ok {
		return errors.New("wait.rabbitmq requires 'queue' in config")
	}

	if ctx.Services == nil || ctx.Services.WaitRabbitMQRegister == nil {
		return errors.New("wait.rabbitmq requires 'tailflow serve' (server mode)")
	}

	return nil
}

func (a *WaitRabbitMQAction) Execute(ctx *ActionContext) (any, error) {
	url := fmt.Sprintf("%v", ctx.Config["url"])
	queue := fmt.Sprintf("%v", ctx.Config["queue"])

	var matchField, matchValue string
	if v, ok := ctx.Config["match"]; ok {
		matchField = fmt.Sprintf("%v", v)
	}
	if v, ok := ctx.Config["match_value"]; ok {
		matchValue = fmt.Sprintf("%v", v)
	}

	// Parse optional timeout (default: 5m)
	timeout := 5 * time.Minute

	if t, ok := ctx.Config["timeout"]; ok {
		dur, err := time.ParseDuration(fmt.Sprintf("%v", t))
		if err != nil {
			return nil, fmt.Errorf("wait.rabbitmq: invalid timeout %q: %w", t, err)
		}

		timeout = dur
	}

	ch, cleanup := ctx.Services.WaitRabbitMQRegister(url, queue, matchField, matchValue, ctx)
	defer cleanup()

	executionID := ctx.ExecCtx.ExecutionID
	stepID := ctx.StepID

	// Signal that this step is now blocking on an external event
	if ctx.Services.EmitWaiting != nil {
		details := map[string]any{
			"queue":   queue,
			"timeout": timeout.String(),
		}
		if matchField != "" {
			details["match"] = matchField
			details["match_value"] = matchValue
		}
		ctx.Services.EmitWaiting(executionID, stepID, "rabbitmq", details)
	}

	ctx.Logger.Info("waiting for message", "queue", queue, "match", matchField, "timeout", timeout)

	select {
	case msg, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("wait.rabbitmq: connection failed for queue %s", queue)
		}

		return msg, nil

	case <-time.After(timeout):
		return nil, fmt.Errorf("wait.rabbitmq: timeout after %s waiting on queue %s", timeout, queue)

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

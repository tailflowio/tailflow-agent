package action

import (
	"errors"
	"fmt"
	"time"
)

// WaitWebhookAction blocks until a webhook request is received on the specified path.
type WaitWebhookAction struct{}

func NewWaitWebhookAction() Action { return &WaitWebhookAction{} }

func (a *WaitWebhookAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["path"]
	if !ok {
		return errors.New("wait.webhook requires 'path' in config")
	}

	if ctx.Services == nil || ctx.Services.WaitWebhookRegister == nil {
		return errors.New("wait.webhook requires 'tailflow serve' (server mode)")
	}

	return nil
}

func (a *WaitWebhookAction) Execute(ctx *ActionContext) (any, error) {
	path := fmt.Sprintf("%v", ctx.Config["path"])

	// Parse optional timeout (default: 5m)
	timeout := 5 * time.Minute

	t, ok := ctx.Config["timeout"]
	if ok {
		dur, err := time.ParseDuration(fmt.Sprintf("%v", t))
		if err != nil {
			return nil, fmt.Errorf("wait.webhook: invalid timeout %q: %w", t, err)
		}

		timeout = dur
	}

	executionID := ctx.ExecCtx.ExecutionID
	stepID := ctx.StepID

	ch, cleanup := ctx.Services.WaitWebhookRegister(executionID, stepID, path, ctx)
	defer cleanup()

	// Signal that this step is now blocking on an external event
	if ctx.Services.EmitWaiting != nil {
		ctx.Services.EmitWaiting(executionID, stepID, "webhook", map[string]any{
			"path":    path,
			"timeout": timeout.String(),
		})
	}

	ctx.Logger.Info("waiting for webhook", "path", path, "timeout", timeout)

	select {
	case req := <-ch:
		return map[string]any{
			"method":  req.Method,
			"path":    req.Path,
			"headers": req.Headers,
			"query":   req.Query,
			"body":    req.Body,
		}, nil

	case <-time.After(timeout):
		return nil, fmt.Errorf("wait.webhook: timeout after %s waiting for %s", timeout, path)

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

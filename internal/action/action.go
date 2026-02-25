package action

import (
	"context"
	"log/slog"

	"github.com/tailflow/tailflow/internal/runtime"
)

type Action interface {
	Validate(ctx *ActionContext) error
	Execute(ctx *ActionContext) (output any, err error)
}

type ActionContext struct {
	context.Context
	Config   map[string]any
	ExecCtx  *runtime.ExecutionContext
	StepID   string
	Logger   *slog.Logger
	Services *runtime.ActionServices
	EmitLog  func(msg string) // optional: emit a live log line during execution

	// RunAction creates, resolves, and executes an action with extra template variables.
	// loopVars are merged into the template context under the "loop" key.
	RunAction func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error)
}

type ActionFactory func() Action

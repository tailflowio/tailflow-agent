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
	Config    map[string]any
	ExecCtx   *runtime.ExecutionContext
	StepID    string
	Logger    *slog.Logger
	Services  *runtime.ActionServices
	EmitLog   func(msg string)
	EmitPrint func(msg string)
	EmitGroup func(key string)
	RunAction func(actionName string, rawConfig map[string]any, loopVars map[string]any) (any, error)
}

type ActionFactory func() Action

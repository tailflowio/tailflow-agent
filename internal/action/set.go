package action

import "errors"

type SetAction struct{}

func NewSetAction() Action { return &SetAction{} }

func (a *SetAction) Validate(ctx *ActionContext) error {
	if len(ctx.Config) == 0 {
		return errors.New("set action requires at least one key-value pair in config")
	}

	return nil
}

func (a *SetAction) Execute(ctx *ActionContext) (any, error) {
	for k, v := range ctx.Config {
		ctx.ExecCtx.SetVariable(k, v)
	}

	return ctx.Config, nil
}

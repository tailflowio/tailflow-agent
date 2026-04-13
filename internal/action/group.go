package action

import (
	"errors"
	"fmt"
)

type GroupAction struct{}

func NewGroupAction() Action { return &GroupAction{} }

func (a *GroupAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["key"]
	if !ok {
		return errors.New("group action requires 'key' in config")
	}

	return nil
}

func (a *GroupAction) Execute(ctx *ActionContext) (any, error) {
	key := fmt.Sprintf("%v", ctx.Config["key"])

	if ctx.EmitGroup != nil {
		ctx.EmitGroup(key)
	}

	return map[string]any{"key": key}, nil
}

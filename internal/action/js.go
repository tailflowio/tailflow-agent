//go:build !saas

package action

import (
	"errors"
	"fmt"

	"github.com/dop251/goja"
)

// JSAction executes inline JavaScript via goja.
type JSAction struct{}

func NewJSAction() Action { return &JSAction{} }

func (a *JSAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["script"]; !ok {
		return errors.New("js action requires 'script' in config")
	}

	return nil
}

func (a *JSAction) Execute(ctx *ActionContext) (any, error) {
	script := fmt.Sprintf("%v", ctx.Config["script"])

	vm := goja.New()

	// Expose context to JS
	ctxMap := ctx.ExecCtx.ToMap()
	for k, v := range ctxMap {
		err := vm.Set(k, v)
		if err != nil {
			return nil, fmt.Errorf("js: set %q: %w", k, err)
		}
	}

	// Expose ctx.get and ctx.set helpers
	ctxObj := vm.NewObject()
	_ = ctxObj.Set("get", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()

		v, ok := ctx.ExecCtx.GetVariable(key)
		if !ok {
			return goja.Undefined()
		}

		return vm.ToValue(v)
	})
	_ = ctxObj.Set("set", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		val := call.Argument(1).Export()
		ctx.ExecCtx.SetVariable(key, val)

		return goja.Undefined()
	})
	_ = vm.Set("ctx", ctxObj)

	// Wrap in a function to support return statements
	wrapped := fmt.Sprintf("(function() { %s })()", script)

	val, err := vm.RunString(wrapped)
	if err != nil {
		return nil, fmt.Errorf("js: %w", err)
	}

	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, nil
	}

	return val.Export(), nil
}

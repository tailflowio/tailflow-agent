//go:build !saas

package action

import (
	"errors"
	"fmt"

	"github.com/dop251/goja"
)

// gojaVMSet wraps vm.Set for testing.
var gojaVMSet = func(vm *goja.Runtime, name string, value any) error { return vm.Set(name, value) }

// gojaObjSet wraps goja.Object.Set for testing.
var gojaObjSet = func(obj *goja.Object, name string, value any) error { return obj.Set(name, value) }

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
		err := gojaVMSet(vm, k, v)
		if err != nil {
			return nil, fmt.Errorf("js: set %q: %w", k, err)
		}
	}

	// Expose ctx.get and ctx.set helpers
	ctxObj := vm.NewObject()

	err := gojaObjSet(ctxObj, "get", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()

		v, ok := ctx.ExecCtx.GetVariable(key)
		if !ok {
			return goja.Undefined()
		}

		return vm.ToValue(v)
	})
	if err != nil {
		return nil, fmt.Errorf("js: set ctx.get: %w", err)
	}

	err = gojaObjSet(ctxObj, "set", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		val := call.Argument(1).Export()
		ctx.ExecCtx.SetVariable(key, val)

		return goja.Undefined()
	})
	if err != nil {
		return nil, fmt.Errorf("js: set ctx.set: %w", err)
	}

	err = gojaVMSet(vm, "ctx", ctxObj)
	if err != nil {
		return nil, fmt.Errorf("js: set ctx: %w", err)
	}

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

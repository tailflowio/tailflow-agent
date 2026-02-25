package action

import (
	"errors"
	"fmt"

	"github.com/tailflow/tailflow/internal/runtime"
)

// ConditionAction evaluates an if/then/else condition.
type ConditionAction struct{}

func NewConditionAction() Action { return &ConditionAction{} }

func (a *ConditionAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["if"]; !ok {
		return errors.New("condition action requires 'if' in config")
	}

	return nil
}

func (a *ConditionAction) Execute(ctx *ActionContext) (any, error) {
	expr := fmt.Sprintf("%v", ctx.Config["if"])
	eval := runtime.NewExprEvaluator()

	result, err := eval.EvalBool(expr, ctx.ExecCtx.ToMap())
	if err != nil {
		return nil, fmt.Errorf("condition: eval 'if': %w", err)
	}

	branch, configKey := "then", "then"
	if !result {
		branch, configKey = "else", "else"
	}

	output := map[string]any{
		"condition": expr,
		"result":    result,
		"branch":    branch,
	}

	if val, ok := ctx.Config[configKey]; ok {
		output["value"] = val
	}

	return output, nil
}

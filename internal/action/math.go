package action

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

type MathAction struct{}

func NewMathAction() Action { return &MathAction{} }

func (a *MathAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["input"]
	if !ok {
		return errors.New("math action requires 'input' in config")
	}

	_, ok = ctx.Config["operations"]
	if !ok {
		return errors.New("math action requires 'operations' in config")
	}

	return nil
}

func (a *MathAction) Execute(ctx *ActionContext) (any, error) {
	values, err := extractNumericValues(ctx)
	if err != nil {
		return nil, err
	}

	ops, err := parseOperations(ctx)
	if err != nil {
		return nil, err
	}

	return computeOperations(values, ops)
}

func extractNumericValues(ctx *ActionContext) ([]float64, error) {
	raw, ok := ctx.Config["input"]
	if !ok {
		return nil, errors.New("math: missing 'input'")
	}

	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("math: 'input' must be an array, got %T", raw)
	}

	field, _ := ctx.Config["field"].(string)

	return toFloat64Slice(arr, field)
}

func toFloat64Slice(arr []any, field string) ([]float64, error) {
	values := make([]float64, 0, len(arr))

	for _, item := range arr {
		var v any
		if field != "" {
			v = fieldValue(item, field)
		} else {
			v = item
		}

		f, ok := toFloat64(v)
		if !ok {
			continue
		}

		values = append(values, f)
	}

	return values, nil
}

func parseOperations(ctx *ActionContext) ([]string, error) {
	rawOps, ok := ctx.Config["operations"].([]any)
	if !ok {
		return nil, errors.New("math: 'operations' must be an array")
	}

	ops := make([]string, len(rawOps))
	for i, o := range rawOps {
		ops[i] = fmt.Sprintf("%v", o)
	}

	return ops, nil
}

func computeOperations(values []float64, ops []string) (map[string]any, error) {
	result := make(map[string]any, len(ops))

	for _, op := range ops {
		v, err := computeSingleOp(values, op)
		if err != nil {
			return nil, err
		}

		result[op] = v
	}

	return result, nil
}

func computeSingleOp(values []float64, op string) (any, error) {
	switch op {
	case "count":
		return float64(len(values)), nil
	case "sum":
		return sumFloat64(values), nil
	case "min":
		return minFloat64(values), nil
	case "max":
		return maxFloat64(values), nil
	case "avg":
		return avgFloat64(values), nil
	default:
		return nil, fmt.Errorf("math: unknown operation %q", op)
	}
}

func sumFloat64(values []float64) float64 {
	var s float64
	for _, v := range values {
		s += v
	}

	return s
}

func minFloat64(values []float64) any {
	if len(values) == 0 {
		return nil
	}

	m := math.Inf(1)
	for _, v := range values {
		if v < m {
			m = v
		}
	}

	return m
}

func maxFloat64(values []float64) any {
	if len(values) == 0 {
		return nil
	}

	m := math.Inf(-1)
	for _, v := range values {
		if v > m {
			m = v
		}
	}

	return m
}

func avgFloat64(values []float64) any {
	if len(values) == 0 {
		return nil
	}

	return sumFloat64(values) / float64(len(values))
}

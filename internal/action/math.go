package action

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

// toFloat64 converts int, float64, or string to float64.
// Returns the value and true on success, 0 and false on failure.
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

// MathAction computes aggregate operations on a numeric array.
type MathAction struct{}

func NewMathAction() Action { return &MathAction{} }

func (a *MathAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["input"]; !ok {
		return errors.New("math action requires 'input' in config")
	}

	if _, ok := ctx.Config["operations"]; !ok {
		return errors.New("math action requires 'operations' in config")
	}

	return nil
}

func (a *MathAction) Execute(ctx *ActionContext) (any, error) {
	raw, ok := ctx.Config["input"]
	if !ok {
		return nil, errors.New("math: missing 'input'")
	}

	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("math: 'input' must be an array, got %T", raw)
	}

	field, _ := ctx.Config["field"].(string)

	// Extract numeric values.
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

	// Parse requested operations.
	rawOps, ok := ctx.Config["operations"].([]any)
	if !ok {
		return nil, errors.New("math: 'operations' must be an array")
	}

	ops := make([]string, len(rawOps))
	for i, o := range rawOps {
		ops[i] = fmt.Sprintf("%v", o)
	}

	result := make(map[string]any, len(ops))

	for _, op := range ops {
		switch op {
		case "count":
			result["count"] = float64(len(values))
		case "sum":
			var s float64
			for _, v := range values {
				s += v
			}

			result["sum"] = s
		case "min":
			if len(values) == 0 {
				result["min"] = nil
			} else {
				m := math.Inf(1)
				for _, v := range values {
					if v < m {
						m = v
					}
				}

				result["min"] = m
			}
		case "max":
			if len(values) == 0 {
				result["max"] = nil
			} else {
				m := math.Inf(-1)
				for _, v := range values {
					if v > m {
						m = v
					}
				}

				result["max"] = m
			}
		case "avg":
			if len(values) == 0 {
				result["avg"] = nil
			} else {
				var s float64
				for _, v := range values {
					s += v
				}

				result["avg"] = s / float64(len(values))
			}
		default:
			return nil, fmt.Errorf("math: unknown operation %q", op)
		}
	}

	return result, nil
}

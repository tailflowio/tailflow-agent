package action

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/tailflow/tailflow/internal/runtime"
)

func toAnySlice(v any) ([]any, bool) {
	arr, ok := v.([]any)
	if ok {
		return arr, true
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, false
	}

	result := make([]any, rv.Len())
	for i := range rv.Len() {
		result[i] = rv.Index(i).Interface()
	}

	return result, true
}

func arrayInput(config map[string]any) ([]any, error) {
	raw, ok := config["input"]
	if !ok {
		return nil, errors.New("missing 'input' in config")
	}

	arr, ok := toAnySlice(raw)
	if !ok {
		return nil, fmt.Errorf("'input' must be an array, got %T", raw)
	}

	return arr, nil
}

func fieldValue(item any, field string) any {
	m, ok := item.(map[string]any)
	if !ok {
		return nil
	}

	return m[field]
}

func compareValues(a, b any) int {
	af, aOK := toFloat64(a)
	bf, bOK := toFloat64(b)

	if aOK && bOK {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		default:
			return 0
		}
	}

	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)

	return strings.Compare(as, bs)
}

type ArraySortAction struct{}

func NewArraySortAction() Action { return &ArraySortAction{} }

func (a *ArraySortAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["input"]
	if !ok {
		return errors.New("array.sort requires 'input' in config")
	}

	_, ok = ctx.Config["field"]
	if !ok {
		return errors.New("array.sort requires 'field' in config")
	}

	return nil
}

func (a *ArraySortAction) Execute(ctx *ActionContext) (any, error) {
	items, err := arrayInput(ctx.Config)
	if err != nil {
		return nil, fmt.Errorf("array.sort: %w", err)
	}

	field := fmt.Sprintf("%v", ctx.Config["field"])

	direction := "asc"

	d, ok := ctx.Config["direction"]
	if ok {
		direction = fmt.Sprintf("%v", d)
	}

	// Copy to avoid mutating the original.
	sorted := make([]any, len(items))
	copy(sorted, items)

	sort.SliceStable(sorted, func(i, j int) bool {
		vi := fieldValue(sorted[i], field)
		vj := fieldValue(sorted[j], field)
		cmp := compareValues(vi, vj)

		if direction == "desc" {
			return cmp > 0
		}

		return cmp < 0
	})

	return sorted, nil
}

type ArrayFilterAction struct{}

func NewArrayFilterAction() Action { return &ArrayFilterAction{} }

func (a *ArrayFilterAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["input"]
	if !ok {
		return errors.New("array.filter requires 'input' in config")
	}

	_, ok = ctx.Config["condition"]
	if !ok {
		return errors.New("array.filter requires 'condition' in config")
	}

	return nil
}

func (a *ArrayFilterAction) Execute(ctx *ActionContext) (any, error) {
	items, err := arrayInput(ctx.Config)
	if err != nil {
		return nil, fmt.Errorf("array.filter: %w", err)
	}

	condition := fmt.Sprintf("%v", ctx.Config["condition"])
	eval := runtime.NewExprEvaluator()

	result := make([]any, 0, len(items))

	for _, item := range items {
		exprCtx := map[string]any{"item": item}

		match, evalErr := eval.EvalBool(condition, exprCtx)
		if evalErr != nil {
			return nil, fmt.Errorf("array.filter: %w", evalErr)
		}

		if match {
			result = append(result, item)
		}
	}

	return result, nil
}

type ArrayMapAction struct{}

func NewArrayMapAction() Action { return &ArrayMapAction{} }

func (a *ArrayMapAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["input"]
	if !ok {
		return errors.New("array.map requires 'input' in config")
	}

	_, ok = ctx.Config["expression"]
	if !ok {
		return errors.New("array.map requires 'expression' in config")
	}

	return nil
}

func (a *ArrayMapAction) Execute(ctx *ActionContext) (any, error) {
	items, err := arrayInput(ctx.Config)
	if err != nil {
		return nil, fmt.Errorf("array.map: %w", err)
	}

	expression := fmt.Sprintf("%v", ctx.Config["expression"])
	eval := runtime.NewExprEvaluator()

	result := make([]any, len(items))

	for i, item := range items {
		exprCtx := map[string]any{"item": item, "index": i}

		val, evalErr := eval.Eval(expression, exprCtx)
		if evalErr != nil {
			return nil, fmt.Errorf("array.map: item %d: %w", i, evalErr)
		}

		result[i] = val
	}

	return result, nil
}

type ArrayUniqAction struct{}

func NewArrayUniqAction() Action { return &ArrayUniqAction{} }

func (a *ArrayUniqAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["input"]
	if !ok {
		return errors.New("array.uniq requires 'input' in config")
	}

	_, ok = ctx.Config["field"]
	if !ok {
		return errors.New("array.uniq requires 'field' in config")
	}

	return nil
}

func (a *ArrayUniqAction) Execute(ctx *ActionContext) (any, error) {
	items, err := arrayInput(ctx.Config)
	if err != nil {
		return nil, fmt.Errorf("array.uniq: %w", err)
	}

	field := fmt.Sprintf("%v", ctx.Config["field"])
	seen := make(map[string]bool)
	result := make([]any, 0, len(items))

	for _, item := range items {
		key := fmt.Sprintf("%v", fieldValue(item, field))
		if seen[key] {
			continue
		}

		seen[key] = true

		result = append(result, item)
	}

	if len(result) == 0 {
		result = []any{}
	}

	return result, nil
}

type ArrayPickAction struct{}

func NewArrayPickAction() Action { return &ArrayPickAction{} }

func (a *ArrayPickAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["input"]
	if !ok {
		return errors.New("array.pick requires 'input' in config")
	}

	_, ok = ctx.Config["fields"]
	if !ok {
		return errors.New("array.pick requires 'fields' in config")
	}

	return nil
}

func (a *ArrayPickAction) Execute(ctx *ActionContext) (any, error) {
	items, err := arrayInput(ctx.Config)
	if err != nil {
		return nil, fmt.Errorf("array.pick: %w", err)
	}

	rawFields, ok := ctx.Config["fields"].([]any)
	if !ok {
		return nil, errors.New("array.pick: 'fields' must be an array")
	}

	fields := make([]string, len(rawFields))
	for i, f := range rawFields {
		fields[i] = fmt.Sprintf("%v", f)
	}

	result := make([]any, len(items))

	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			result[i] = item
			continue
		}

		picked := make(map[string]any, len(fields))

		for _, f := range fields {
			v, exists := m[f]
			if exists {
				picked[f] = v
			}
		}

		result[i] = picked
	}

	return result, nil
}

type ArrayConcatAction struct{}

func NewArrayConcatAction() Action { return &ArrayConcatAction{} }

func (a *ArrayConcatAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["arrays"]
	if !ok {
		return errors.New("array.concat requires 'arrays' in config")
	}

	return nil
}

func (a *ArrayConcatAction) Execute(ctx *ActionContext) (any, error) {
	raw, ok := ctx.Config["arrays"].([]any)
	if !ok {
		return nil, errors.New("array.concat: 'arrays' must be an array of arrays")
	}

	result := make([]any, 0, len(raw))

	for i, item := range raw {
		arr, ok := item.([]any)
		if !ok {
			return nil, fmt.Errorf("array.concat: item %d is not an array (got %T)", i, item)
		}

		result = append(result, arr...)
	}

	return result, nil
}

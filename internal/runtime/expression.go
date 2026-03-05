package runtime

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/google/uuid"
)

var templateRegex = regexp.MustCompile(`\{\{\s*(.+?)\s*\}\}`)

type ExprEvaluator struct{}

func NewExprEvaluator() *ExprEvaluator {
	return &ExprEvaluator{}
}

func builtinOptions() []expr.Option {
	opts := make([]expr.Option, 0, 8)
	opts = append(opts, identityOptions()...)
	opts = append(opts, cliOptions()...)
	opts = append(opts, dateOptions()...)

	return opts
}

func identityOptions() []expr.Option {
	return []expr.Option{
		expr.Function("uuid", func(params ...any) (any, error) {
			return uuid.New().String(), nil
		}),
	}
}

func cliOptions() []expr.Option {
	return []expr.Option{
		// flag("--id", value) → "--id value" if value is non-empty, "" otherwise
		expr.Function("flag", func(params ...any) (any, error) {
			if len(params) < 2 {
				return nil, errors.New("flag requires 2 arguments: flag name, value")
			}

			name, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("flag: first argument must be a string, got %T", params[0])
			}

			val := fmt.Sprintf("%v", params[1])
			if val == "" || val == "<nil>" {
				return "", nil
			}

			return name + " " + val, nil
		}),

		// bflag("-v", true) → "-v" if truthy, "" otherwise
		expr.Function("bflag", func(params ...any) (any, error) {
			if len(params) < 2 {
				return nil, errors.New("bflag requires 2 arguments: flag name, bool")
			}

			name, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("bflag: first argument must be a string, got %T", params[0])
			}

			switch v := params[1].(type) {
			case bool:
				if v {
					return name, nil
				}
			case string:
				if v != "" && v != "false" && v != "0" {
					return name, nil
				}
			case nil:
			default:
				return name, nil
			}

			return "", nil
		}),
	}
}

func dateOptions() []expr.Option {
	opts := make([]expr.Option, 0, 8)
	opts = append(opts, dateNowOptions()...)
	opts = append(opts, dateFormatOptions()...)
	opts = append(opts, dateArithOptions()...)

	return opts
}

func dateNowOptions() []expr.Option {
	return []expr.Option{
		expr.Function("now", evalNow),
		expr.Function("tz", evalTz),
	}
}

func evalNow(params ...any) (any, error) {
	t := time.Now()

	if len(params) > 0 {
		tz, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("now: argument must be a timezone string, got %T", params[0])
		}

		loc, err := time.LoadLocation(tz)
		if err != nil {
			return nil, fmt.Errorf("now: invalid timezone %q: %w", tz, err)
		}

		t = t.In(loc)
	} else {
		t = t.UTC()
	}

	return t.Format(time.RFC3339), nil
}

func evalTz(params ...any) (any, error) {
	if len(params) < 2 {
		return nil, errors.New("tz requires 2 arguments: date string, timezone")
	}

	dateStr, ok := params[0].(string)
	if !ok {
		return nil, fmt.Errorf("tz: first argument must be a string, got %T", params[0])
	}

	tzStr, ok := params[1].(string)
	if !ok {
		return nil, fmt.Errorf("tz: second argument must be a timezone string, got %T", params[1])
	}

	t, err := parseDate(dateStr)
	if err != nil {
		return nil, fmt.Errorf("tz: %w", err)
	}

	loc, err := time.LoadLocation(tzStr)
	if err != nil {
		return nil, fmt.Errorf("tz: invalid timezone %q: %w", tzStr, err)
	}

	return t.In(loc).Format(time.RFC3339), nil
}

func dateFormatOptions() []expr.Option {
	return []expr.Option{
		expr.Function("formatDate", func(params ...any) (any, error) {
			if len(params) < 2 {
				return nil, errors.New("formatDate requires 2 arguments: date string, format")
			}

			dateStr, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("formatDate: first argument must be a string, got %T", params[0])
			}

			format, ok := params[1].(string)
			if !ok {
				return nil, fmt.Errorf("formatDate: second argument must be a string, got %T", params[1])
			}

			t, err := parseDate(dateStr)
			if err != nil {
				return nil, fmt.Errorf("formatDate: %w", err)
			}

			return t.Format(dateFormat(format)), nil
		}),
		expr.Function("unixTime", func(params ...any) (any, error) {
			if len(params) == 0 {
				return time.Now().Unix(), nil
			}

			dateStr, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("unixTime: argument must be a string, got %T", params[0])
			}

			t, err := parseDate(dateStr)
			if err != nil {
				return nil, fmt.Errorf("unixTime: %w", err)
			}

			return t.Unix(), nil
		}),
	}
}

func dateArithOptions() []expr.Option {
	return []expr.Option{
		expr.Function("addDate", evalAddDate),
		expr.Function("diffDate", evalDiffDate),
	}
}

func evalAddDate(params ...any) (any, error) {
	if len(params) < 2 {
		return nil, errors.New("addDate requires 2 arguments: date string, duration string")
	}

	dateStr, ok := params[0].(string)
	if !ok {
		return nil, fmt.Errorf("addDate: first argument must be a string, got %T", params[0])
	}

	durStr, ok := params[1].(string)
	if !ok {
		return nil, fmt.Errorf("addDate: second argument must be a string, got %T", params[1])
	}

	t, err := parseDate(dateStr)
	if err != nil {
		return nil, fmt.Errorf("addDate: %w", err)
	}

	d, err := time.ParseDuration(durStr)
	if err != nil {
		return nil, fmt.Errorf("addDate: invalid duration %q: %w", durStr, err)
	}

	return t.Add(d).Format(time.RFC3339), nil
}

func evalDiffDate(params ...any) (any, error) {
	if len(params) < 2 {
		return nil, errors.New("diffDate requires 2 arguments: date1, date2")
	}

	d1, ok := params[0].(string)
	if !ok {
		return nil, fmt.Errorf("diffDate: first argument must be a string, got %T", params[0])
	}

	d2, ok := params[1].(string)
	if !ok {
		return nil, fmt.Errorf("diffDate: second argument must be a string, got %T", params[1])
	}

	t1, err := parseDate(d1)
	if err != nil {
		return nil, fmt.Errorf("diffDate: %w", err)
	}

	t2, err := parseDate(d2)
	if err != nil {
		return nil, fmt.Errorf("diffDate: %w", err)
	}

	return t1.Sub(t2).Seconds(), nil
}

func parseDate(s string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("cannot parse date %q", s)
}

func dateFormat(f string) string {
	switch f {
	case "iso", "ISO", "RFC3339":
		return time.RFC3339
	case "date":
		return "2006-01-02"
	case "datetime":
		return "2006-01-02 15:04:05"
	case "time":
		return "15:04:05"
	case "unix":
		return time.UnixDate
	default:
		return f // allow raw Go layout
	}
}

func (e *ExprEvaluator) Eval(expression string, ctx map[string]any) (any, error) {
	opts := append(builtinOptions(), expr.Env(ctx))

	program, err := expr.Compile(expression, opts...)
	if err != nil {
		return nil, fmt.Errorf("compile expression %q: %w", expression, err)
	}

	result, err := expr.Run(program, ctx)
	if err != nil {
		return nil, fmt.Errorf("eval expression %q: %w", expression, err)
	}

	return normalizeResult(result), nil
}

func normalizeResult(v any) any {
	switch val := v.(type) {
	case int:
		return int64(val)
	case []any:
		for i, item := range val {
			val[i] = normalizeResult(item)
		}

		return val
	case map[string]any:
		for k, item := range val {
			val[k] = normalizeResult(item)
		}

		return val
	default:
		return v
	}
}

func (e *ExprEvaluator) EvalBool(expression string, ctx map[string]any) (bool, error) {
	result, err := e.Eval(expression, ctx)
	if err != nil {
		return false, err
	}

	switch v := result.(type) {
	case bool:
		return v, nil
	default:
		return false, fmt.Errorf("expression %q did not return boolean, got %T", expression, result)
	}
}

func (e *ExprEvaluator) ResolveTemplate(tmpl string, ctx map[string]any) (string, error) {
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil
	}

	var lastErr error
	result := templateRegex.ReplaceAllStringFunc(tmpl, func(match string) string {
		expression := templateRegex.FindStringSubmatch(match)[1]

		val, err := e.Eval(expression, ctx)
		if err != nil {
			lastErr = err
			return match
		}

		return fmt.Sprintf("%v", val)
	})

	if lastErr != nil {
		return "", lastErr
	}

	return result, nil
}

func (e *ExprEvaluator) ResolveConfig(config map[string]any, ctx map[string]any) (map[string]any, error) {
	if config == nil {
		return nil, nil
	}

	resolved := make(map[string]any, len(config))

	for k, v := range config {
		rv, err := e.resolveValue(v, ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve config key %q: %w", k, err)
		}

		resolved[k] = rv
	}

	return resolved, nil
}

var pureTemplateRegex = regexp.MustCompile(`^\{\{\s*(.+?)\s*\}\}$`)

func (e *ExprEvaluator) resolveStringValue(s string, ctx map[string]any) (any, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}

	m := pureTemplateRegex.FindStringSubmatch(s)
	if m != nil {
		return e.Eval(m[1], ctx)
	}

	return e.ResolveTemplate(s, ctx)
}

func (e *ExprEvaluator) resolveValue(v any, ctx map[string]any) (any, error) {
	switch val := v.(type) {
	case string:
		return e.resolveStringValue(val, ctx)
	case map[string]any:
		return e.ResolveConfig(val, ctx)
	case []any:
		result := make([]any, len(val))

		for i, item := range val {
			rv, err := e.resolveValue(item, ctx)
			if err != nil {
				return nil, err
			}

			result[i] = rv
		}

		return result, nil
	default:
		return v, nil
	}
}

package action

import (
	"errors"
	"fmt"
	"regexp"
)

// StringMatchAllAction extracts all regex matches from a string (deduplicated).
type StringMatchAllAction struct{}

func NewStringMatchAllAction() Action { return &StringMatchAllAction{} }

func (a *StringMatchAllAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["input"]; !ok {
		return errors.New("string.match_all action requires 'input' in config")
	}

	if _, ok := ctx.Config["pattern"]; !ok {
		return errors.New("string.match_all action requires 'pattern' in config")
	}

	pattern := fmt.Sprintf("%v", ctx.Config["pattern"])
	if _, err := regexp.Compile(pattern); err != nil {
		return fmt.Errorf("string.match_all: invalid pattern %q: %w", pattern, err)
	}

	return nil
}

func (a *StringMatchAllAction) Execute(ctx *ActionContext) (any, error) {
	input := fmt.Sprintf("%v", ctx.Config["input"])
	pattern := fmt.Sprintf("%v", ctx.Config["pattern"])

	re := regexp.MustCompile(pattern)

	seen := make(map[string]bool)
	var results []any

	if re.NumSubexp() > 0 {
		// If pattern has capture groups, return first capture group
		for _, m := range re.FindAllStringSubmatch(input, -1) {
			val := m[1]

			if seen[val] {
				continue
			}

			seen[val] = true

			results = append(results, val)
		}
	} else {
		for _, m := range re.FindAllString(input, -1) {
			if seen[m] {
				continue
			}

			seen[m] = true

			results = append(results, m)
		}
	}

	if results == nil {
		results = []any{}
	}

	return map[string]any{
		"matches": results,
	}, nil
}

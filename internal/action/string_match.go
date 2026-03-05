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
	_, ok := ctx.Config["input"]
	if !ok {
		return errors.New("string.match_all action requires 'input' in config")
	}

	_, ok = ctx.Config["pattern"]
	if !ok {
		return errors.New("string.match_all action requires 'pattern' in config")
	}

	pattern := fmt.Sprintf("%v", ctx.Config["pattern"])

	_, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("string.match_all: invalid pattern %q: %w", pattern, err)
	}

	return nil
}

func (a *StringMatchAllAction) Execute(ctx *ActionContext) (any, error) {
	input := fmt.Sprintf("%v", ctx.Config["input"])
	pattern := fmt.Sprintf("%v", ctx.Config["pattern"])

	re := regexp.MustCompile(pattern)

	seen := make(map[string]bool)

	if re.NumSubexp() > 0 {
		matches := re.FindAllStringSubmatch(input, -1)
		results := make([]any, 0, len(matches))

		for _, m := range matches {
			val := m[1]

			if seen[val] {
				continue
			}

			seen[val] = true

			results = append(results, val)
		}

		return map[string]any{
			"matches": results,
		}, nil
	}

	matches := re.FindAllString(input, -1)
	results := make([]any, 0, len(matches))

	for _, m := range matches {
		if seen[m] {
			continue
		}

		seen[m] = true

		results = append(results, m)
	}

	return map[string]any{
		"matches": results,
	}, nil
}

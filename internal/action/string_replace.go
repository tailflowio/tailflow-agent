package action

import (
	"errors"
	"fmt"
	"regexp"
)

// StringReplaceAction replaces occurrences in a string, with optional regex support.
type StringReplaceAction struct{}

func NewStringReplaceAction() Action { return &StringReplaceAction{} }

func (a *StringReplaceAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["input"]
	if !ok {
		return errors.New("string.replace action requires 'input' in config")
	}

	_, ok = ctx.Config["pattern"]
	if !ok {
		return errors.New("string.replace action requires 'pattern' in config")
	}

	// Validate regex patterns at config time
	patterns, ok := ctx.Config["pattern"].([]any)
	if ok {
		return validatePatternList(patterns)
	}

	pattern := fmt.Sprintf("%v", ctx.Config["pattern"])

	_, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("string.replace: invalid pattern %q: %w", pattern, err)
	}

	return nil
}

func (a *StringReplaceAction) Execute(ctx *ActionContext) (any, error) {
	input := fmt.Sprintf("%v", ctx.Config["input"])

	replacement := ""

	r, ok := ctx.Config["replacement"]
	if ok {
		replacement = fmt.Sprintf("%v", r)
	}

	result := input

	// Support single pattern or array of patterns
	patterns, ok := ctx.Config["pattern"].([]any)
	if ok {
		for _, p := range patterns {
			re := regexp.MustCompile(fmt.Sprintf("%v", p))
			result = re.ReplaceAllString(result, replacement)
		}

		return map[string]any{"result": result}, nil
	}

	pattern := fmt.Sprintf("%v", ctx.Config["pattern"])
	re := regexp.MustCompile(pattern)
	result = re.ReplaceAllString(result, replacement)

	return map[string]any{"result": result}, nil
}

func validatePatternList(patterns []any) error {
	for _, p := range patterns {
		_, err := regexp.Compile(fmt.Sprintf("%v", p))
		if err != nil {
			return fmt.Errorf("string.replace: invalid pattern %q: %w", p, err)
		}
	}

	return nil
}

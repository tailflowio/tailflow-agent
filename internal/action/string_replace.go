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
	if _, ok := ctx.Config["input"]; !ok {
		return errors.New("string.replace action requires 'input' in config")
	}

	if _, ok := ctx.Config["pattern"]; !ok {
		return errors.New("string.replace action requires 'pattern' in config")
	}

	// Validate regex patterns at config time
	if patterns, ok := ctx.Config["pattern"].([]any); ok {
		for _, p := range patterns {
			if _, err := regexp.Compile(fmt.Sprintf("%v", p)); err != nil {
				return fmt.Errorf("string.replace: invalid pattern %q: %w", p, err)
			}
		}
	} else {
		pattern := fmt.Sprintf("%v", ctx.Config["pattern"])
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("string.replace: invalid pattern %q: %w", pattern, err)
		}
	}

	return nil
}

func (a *StringReplaceAction) Execute(ctx *ActionContext) (any, error) {
	input := fmt.Sprintf("%v", ctx.Config["input"])

	replacement := ""
	if r, ok := ctx.Config["replacement"]; ok {
		replacement = fmt.Sprintf("%v", r)
	}

	result := input

	// Support single pattern or array of patterns
	if patterns, ok := ctx.Config["pattern"].([]any); ok {
		for _, p := range patterns {
			re := regexp.MustCompile(fmt.Sprintf("%v", p))
			result = re.ReplaceAllString(result, replacement)
		}
	} else {
		pattern := fmt.Sprintf("%v", ctx.Config["pattern"])
		re := regexp.MustCompile(pattern)
		result = re.ReplaceAllString(result, replacement)
	}

	return map[string]any{
		"result": result,
	}, nil
}

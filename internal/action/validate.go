package action

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

// ValidateAction validates data fields against rules using go-playground/validator.
type ValidateAction struct{}

func NewValidateAction() Action { return &ValidateAction{} }

func (a *ValidateAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["rules"]
	if !ok {
		return errors.New("validate action requires 'rules' in config")
	}

	return nil
}

func (a *ValidateAction) Execute(ctx *ActionContext) (any, error) {
	rulesRaw := ctx.Config["rules"]

	rules, ok := rulesRaw.(map[string]any)
	if !ok {
		return map[string]any{"valid": false, "errors": []any{}}, nil
	}

	data := extractValidationData(ctx)

	validationErrs := runValidationRules(rules, data)

	result := map[string]any{
		"valid":  len(validationErrs) == 0,
		"errors": validationErrs,
	}

	if len(validationErrs) == 0 {
		return result, nil
	}

	fail, ok := ctx.Config["fail_on_error"].(bool)
	if !ok || !fail {
		return result, nil
	}

	return result, buildValidationError(validationErrs)
}

func extractValidationData(ctx *ActionContext) map[string]any {
	d, ok := ctx.Config["data"]
	if !ok {
		return nil
	}

	dm, ok := d.(map[string]any)
	if !ok {
		return nil
	}

	return dm
}

func runValidationRules(rules map[string]any, data map[string]any) []map[string]any {
	v := validator.New()
	var validationErrs []map[string]any

	for field, ruleRaw := range rules {
		rule := fmt.Sprintf("%v", ruleRaw)
		value := data[field]

		err := v.Var(value, rule)
		if err == nil {
			continue
		}

		var ve validator.ValidationErrors
		if !errors.As(err, &ve) {
			continue
		}

		for _, e := range ve {
			validationErrs = append(validationErrs, map[string]any{
				"field":   field,
				"tag":     e.Tag(),
				"value":   value,
				"message": fmt.Sprintf("field '%s' failed on '%s' validation", field, e.Tag()),
			})
		}
	}

	return validationErrs
}

func buildValidationError(validationErrs []map[string]any) error {
	msgs := make([]string, len(validationErrs))
	for i, e := range validationErrs {
		msgs[i] = fmt.Sprintf("%v", e["message"])
	}

	return fmt.Errorf("validation failed: %s", strings.Join(msgs, "; "))
}

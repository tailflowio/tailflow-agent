package action

import (
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
)

// ValidateAction validates data fields against rules using go-playground/validator.
type ValidateAction struct{}

func NewValidateAction() Action { return &ValidateAction{} }

func (a *ValidateAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["rules"]; !ok {
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

	// data can be a map or nil
	var data map[string]any

	if d, ok := ctx.Config["data"]; ok {
		if dm, ok := d.(map[string]any); ok {
			data = dm
		}
	}

	v := validator.New()
	var validationErrs []map[string]any

	for field, ruleRaw := range rules {
		rule := fmt.Sprintf("%v", ruleRaw)
		value := data[field]

		err := v.Var(value, rule)
		if err != nil {
			var ve validator.ValidationErrors
			if errors.As(err, &ve) {
				for _, e := range ve {
					validationErrs = append(validationErrs, map[string]any{
						"field":   field,
						"tag":     e.Tag(),
						"value":   value,
						"message": fmt.Sprintf("field '%s' failed on '%s' validation", field, e.Tag()),
					})
				}
			}
		}
	}

	// Always return nil error — validation failure is not a step failure
	return map[string]any{
		"valid":  len(validationErrs) == 0,
		"errors": validationErrs,
	}, nil
}

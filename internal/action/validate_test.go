package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ValidateActionTestSuite struct {
	suite.Suite
}

func TestValidateAction(t *testing.T) {
	suite.Run(t, new(ValidateActionTestSuite))
}

func (s *ValidateActionTestSuite) SetupTest() {}

func (s *ValidateActionTestSuite) TestMissingRules() {
	a := NewValidateAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
	s.Contains(err.Error(), "rules")
}

func (s *ValidateActionTestSuite) TestAllValid() {
	a := NewValidateAction()
	ctx := newTestContext(map[string]any{
		"data": map[string]any{
			"email": "test@example.com",
			"name":  "Alice",
		},
		"rules": map[string]any{
			"email": "required,email",
			"name":  "required,min=2",
		},
	})

	err := a.Validate(ctx)
	s.Require().NoError(err)

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	result := out.(map[string]any)
	s.True(result["valid"].(bool))
}

func (s *ValidateActionTestSuite) TestValidationFailures() {
	a := NewValidateAction()
	ctx := newTestContext(map[string]any{
		"data": map[string]any{
			"email": "not-an-email",
			"name":  "A",
		},
		"rules": map[string]any{
			"email": "required,email",
			"name":  "required,min=2",
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err) // validation failure is not a step error

	result := out.(map[string]any)
	s.False(result["valid"].(bool))

	errors := result["errors"].([]map[string]any)
	s.GreaterOrEqual(len(errors), 2)
}

func (s *ValidateActionTestSuite) TestMissingData() {
	a := NewValidateAction()
	ctx := newTestContext(map[string]any{
		"rules": map[string]any{
			"email": "required",
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	result := out.(map[string]any)
	s.False(result["valid"].(bool))
}

func (s *ValidateActionTestSuite) TestValidateOK() {
	a := NewValidateAction()
	err := a.Validate(newTestContext(map[string]any{
		"rules": map[string]any{"email": "required"},
	}))
	s.NoError(err)
}

func (s *ValidateActionTestSuite) TestRulesNotMap() {
	a := NewValidateAction()
	ctx := newTestContext(map[string]any{
		"rules": "not-a-map",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	result := out.(map[string]any)
	s.False(result["valid"].(bool))
	s.Empty(result["errors"])
}

func (s *ValidateActionTestSuite) TestDataNotMap() {
	a := NewValidateAction()
	ctx := newTestContext(map[string]any{
		"rules": map[string]any{"email": "required"},
		"data":  "not-a-map",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	result := out.(map[string]any)
	s.False(result["valid"].(bool))
}

func (s *ValidateActionTestSuite) TestFailOnError_Fails() {
	a := NewValidateAction()
	ctx := newTestContext(map[string]any{
		"rules":         map[string]any{"email": "required"},
		"fail_on_error": true,
	})

	out, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "validation failed")

	result := out.(map[string]any)
	s.False(result["valid"].(bool))
}

func (s *ValidateActionTestSuite) TestFailOnError_Passes() {
	a := NewValidateAction()
	ctx := newTestContext(map[string]any{
		"data":          map[string]any{"email": "test@example.com"},
		"rules":         map[string]any{"email": "required"},
		"fail_on_error": true,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	result := out.(map[string]any)
	s.True(result["valid"].(bool))
}

// TestRunValidationRules_AllFieldsPass covers runValidationRules: multiple fields all pass validation (err == nil continue branch).
func (s *ValidateActionTestSuite) TestRunValidationRules_AllFieldsPass() {
	rules := map[string]any{
		"email": "required,email",
		"name":  "required",
	}
	data := map[string]any{
		"email": "user@example.com",
		"name":  "Alice",
	}

	errs := runValidationRules(rules, data)
	s.Empty(errs)
}

// TestRunValidationRules_NilDataMap covers runValidationRules: nil data map - field lookups return zero values, triggering.
// validation failures for "required" rules.
func (s *ValidateActionTestSuite) TestRunValidationRules_NilDataMap() {
	rules := map[string]any{
		"name": "required",
	}

	errs := runValidationRules(rules, nil)
	s.Require().Len(errs, 1)
	s.Equal("name", errs[0]["field"])
	s.Equal("required", errs[0]["tag"])
}

// TestRunValidationRules_MultipleFieldErrors covers runValidationRules: multiple rules with multiple validation errors.
func (s *ValidateActionTestSuite) TestRunValidationRules_MultipleFieldErrors() {
	rules := map[string]any{
		"email": "required,email",
		"age":   "required",
	}
	data := map[string]any{
		"email": "not-valid",
	}

	errs := runValidationRules(rules, data)
	// "email" fails on "email" tag, "age" is nil and fails on "required"
	s.GreaterOrEqual(len(errs), 2)
}

// TestRunValidationRules_FieldPresent_NoError covers runValidationRules: value is present and valid so err == nil, continue is taken.
func (s *ValidateActionTestSuite) TestRunValidationRules_FieldPresent_NoError() {
	rules := map[string]any{
		"count": "min=0",
	}
	data := map[string]any{
		"count": 10,
	}

	errs := runValidationRules(rules, data)
	s.Empty(errs)
}

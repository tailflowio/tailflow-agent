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

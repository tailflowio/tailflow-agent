package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type TemplateActionTestSuite struct {
	suite.Suite
}

func TestTemplateAction(t *testing.T) {
	suite.Run(t, new(TemplateActionTestSuite))
}

func (s *TemplateActionTestSuite) SetupTest() {}

func (s *TemplateActionTestSuite) TestExecute_InterpolatesData() {
	a := NewTemplateAction()
	ctx := newTestContext(map[string]any{
		"template": "Hello, {{ .name }}!",
		"data": map[string]any{
			"name": "Alice",
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	outMap, ok := out.(map[string]any)
	s.Require().True(ok)
	s.Equal("Hello, Alice!", outMap["result"])
}

func (s *TemplateActionTestSuite) TestWithContext() {
	a := NewTemplateAction()
	ctx := newTestContext(map[string]any{
		"template": "Env: {{ index .params \"env\" }}",
	})
	ctx.ExecCtx.Params = map[string]any{"env": "prod"}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	outMap, ok := out.(map[string]any)
	s.Require().True(ok)
	s.Equal("Env: prod", outMap["result"])
}

func (s *TemplateActionTestSuite) TestInvalidTemplate() {
	a := NewTemplateAction()
	ctx := newTestContext(map[string]any{
		"template": "{{ .invalid",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *TemplateActionTestSuite) TestValidateMissingTemplate() {
	a := NewTemplateAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
}

func (s *TemplateActionTestSuite) TestValidateOK() {
	a := NewTemplateAction()
	err := a.Validate(newTestContext(map[string]any{"template": "hello"}))
	s.NoError(err)
}

func (s *TemplateActionTestSuite) TestExecuteError() {
	a := NewTemplateAction()
	ctx := newTestContext(map[string]any{
		// Call a function that doesn't exist - this will cause a template execution error
		"template": "{{ call .nonexistent }}",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "template")
}

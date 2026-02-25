package action

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"
)

// TemplateAction renders Go text/template.
type TemplateAction struct{}

func NewTemplateAction() Action { return &TemplateAction{} }

func (a *TemplateAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["template"]; !ok {
		return errors.New("template action requires 'template' in config")
	}

	return nil
}

func (a *TemplateAction) Execute(ctx *ActionContext) (any, error) {
	tmplStr := fmt.Sprintf("%v", ctx.Config["template"])

	tmpl, err := template.New("tmpl").Parse(tmplStr)
	if err != nil {
		return nil, fmt.Errorf("template: parse: %w", err)
	}

	data := ctx.ExecCtx.ToMap()

	if d, ok := ctx.Config["data"]; ok {
		if dMap, ok := d.(map[string]any); ok {
			for k, v := range dMap {
				data[k] = v
			}
		}
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, data)
	if err != nil {
		return nil, fmt.Errorf("template: execute: %w", err)
	}

	return map[string]any{
		"result": buf.String(),
	}, nil
}

package action

import (
	"fmt"
)

// ResponseAction defines the HTTP response for trigger-based workflows.
type ResponseAction struct{}

func NewResponseAction() Action { return &ResponseAction{} }

func (a *ResponseAction) Validate(ctx *ActionContext) error {
	return nil
}

func (a *ResponseAction) Execute(ctx *ActionContext) (any, error) {
	status := 200

	if s, ok := ctx.Config["status"]; ok {
		switch sv := s.(type) {
		case int:
			status = sv
		case float64:
			status = int(sv)
		}
	}

	body := ctx.Config["body"]
	headers := map[string]string{}

	if h, ok := ctx.Config["headers"]; ok {
		if hm, ok := h.(map[string]any); ok {
			for k, v := range hm {
				headers[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	return map[string]any{
		"status":  status,
		"body":    body,
		"headers": headers,
	}, nil
}

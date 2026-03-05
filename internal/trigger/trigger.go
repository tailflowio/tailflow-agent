package trigger

import (
	"fmt"

	"github.com/tailflow/tailflow/internal/parser"
)

type Route struct {
	WorkflowName string
	Method       string
	Path         string
	TriggerType  string // "http" or "webhook"
	Secret       string // For webhook HMAC validation
	Filter       string // JS expression for webhook filtering
}

func ResolveRoutes(workflows []*parser.Workflow) []Route {
	routes := make([]Route, 0, len(workflows))

	for _, wf := range workflows {
		if wf.Trigger == nil {
			continue
		}

		if wf.Trigger.HTTP != nil {
			routes = append(routes, Route{
				WorkflowName: wf.Name,
				Method:       wf.Trigger.HTTP.Method,
				Path:         wf.Trigger.HTTP.Path,
				TriggerType:  "http",
			})
		}

		if wf.Trigger.Webhook != nil {
			routes = append(routes, Route{
				WorkflowName: wf.Name,
				Method:       "POST",
				Path:         wf.Trigger.Webhook.Path,
				TriggerType:  "webhook",
				Secret:       wf.Trigger.Webhook.Secret,
				Filter:       wf.Trigger.Webhook.Filter,
			})
		}
	}

	return routes
}

func MatchRoute(routes []Route, method, path string) (*Route, error) {
	for _, r := range routes {
		if r.Path == path {
			if r.TriggerType == "webhook" || r.Method == method {
				return &r, nil
			}
		}
	}

	return nil, fmt.Errorf("no route matching %s %s", method, path)
}

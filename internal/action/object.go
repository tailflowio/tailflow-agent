package action

import (
	"errors"
	"fmt"
)

// ObjectAction constructs or merges objects.
//
// Mode "keys" (default): every config key except "mode" becomes an output key.
//
//	config:
//	  name: "{{ steps.fetch.output.first_name }}"
//	  email: "{{ steps.fetch.output.email }}"
//
// Mode "merge": merges an array of objects in order (last wins).
//
//	config:
//	  mode: merge
//	  objects:
//	    - "{{ steps.a.output }}"
//	    - "{{ steps.b.output }}"
//	    - { extra: "value" }
type ObjectAction struct{}

func NewObjectAction() Action { return &ObjectAction{} }

func (a *ObjectAction) Validate(ctx *ActionContext) error {
	mode, _ := ctx.Config["mode"].(string)

	if mode == "merge" {
		if _, ok := ctx.Config["objects"]; !ok {
			return errors.New("object action in merge mode requires 'objects' in config")
		}
	}

	return nil
}

func (a *ObjectAction) Execute(ctx *ActionContext) (any, error) {
	mode, _ := ctx.Config["mode"].(string)

	if mode == "merge" {
		return a.executeMerge(ctx)
	}

	return a.executeKeys(ctx)
}

func (a *ObjectAction) executeKeys(ctx *ActionContext) (any, error) {
	result := make(map[string]any, len(ctx.Config))

	for k, v := range ctx.Config {
		if k == "mode" {
			continue
		}

		result[k] = v
	}

	return result, nil
}

func (a *ObjectAction) executeMerge(ctx *ActionContext) (any, error) {
	raw, ok := ctx.Config["objects"].([]any)
	if !ok {
		return nil, errors.New("object: 'objects' must be an array")
	}

	result := make(map[string]any)

	for i, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("object: merge item %d is not an object (got %T)", i, item)
		}

		for k, v := range m {
			result[k] = v
		}
	}

	return result, nil
}

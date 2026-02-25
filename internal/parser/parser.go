package parser

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func Parse(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workflow file: %w", err)
	}

	return ParseBytes(data)
}

func ParseBytes(data []byte) (*Workflow, error) {
	var w Workflow
	err := yaml.Unmarshal(data, &w)
	if err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}

	err = Validate(&w)
	if err != nil {
		return nil, err
	}

	return &w, nil
}

func Validate(w *Workflow) error {
	if w.Version == "" {
		return errors.New("validation: version is required")
	}

	if w.Version != "2.0" {
		return fmt.Errorf("validation: unsupported version %q, expected \"2.0\"", w.Version)
	}

	if w.Name == "" {
		return errors.New("validation: name is required")
	}

	if len(w.Steps) == 0 {
		return errors.New("validation: at least one step is required")
	}

	ids := make(map[string]bool, len(w.Steps))

	for i, s := range w.Steps {
		if s.ID == "" {
			return fmt.Errorf("validation: step[%d] must have an id", i)
		}

		if ids[s.ID] {
			return fmt.Errorf("validation: duplicate step id %q", s.ID)
		}

		ids[s.ID] = true

		if s.Action == "" {
			return fmt.Errorf("validation: step %q must have an action", s.ID)
		}
	}

	// Validate error_policy
	for _, s := range w.Steps {
		if s.ErrorPolicy != "" && s.ErrorPolicy != "stop" && s.ErrorPolicy != "continue" && s.ErrorPolicy != "ignore" {
			return fmt.Errorf("validation: step %q has invalid error_policy %q", s.ID, s.ErrorPolicy)
		}
	}

	// Check depends_on references exist
	for _, s := range w.Steps {
		for _, dep := range s.DependsOn {
			if !ids[dep] {
				return fmt.Errorf("validation: step %q depends_on unknown step %q", s.ID, dep)
			}

			if dep == s.ID {
				return fmt.Errorf("validation: step %q cannot depend on itself", s.ID)
			}
		}
	}

	// Validate params
	for i, p := range w.Params {
		if p.Name == "" {
			return fmt.Errorf("validation: param[%d] must have a name", i)
		}

		if p.Type == "" {
			return fmt.Errorf("validation: param %q must have a type", p.Name)
		}

		switch p.Type {
		case "string", "bool", "int", "float":
		default:
			return fmt.Errorf("validation: param %q has unsupported type %q", p.Name, p.Type)
		}
	}

	// Validate trigger
	if w.Trigger != nil {
		triggerCount := 0
		if w.Trigger.HTTP != nil {
			triggerCount++
		}

		if w.Trigger.Webhook != nil {
			triggerCount++
		}

		if w.Trigger.Schedule != nil {
			triggerCount++
		}

		if w.Trigger.RabbitMQ != nil {
			triggerCount++
		}

		if triggerCount > 1 {
			return errors.New("validation: workflow can only have one trigger (http, webhook, schedule, or rabbitmq)")
		}

		if w.Trigger.HTTP != nil {
			if w.Trigger.HTTP.Path == "" {
				return errors.New("validation: http trigger must have a path")
			}

			if w.Trigger.HTTP.Method == "" {
				return errors.New("validation: http trigger must have a method")
			}
		}

		if w.Trigger.Webhook != nil {
			if w.Trigger.Webhook.Path == "" {
				return errors.New("validation: webhook trigger must have a path")
			}
		}

		if w.Trigger.Schedule != nil {
			if w.Trigger.Schedule.Cron == "" {
				return errors.New("validation: schedule trigger must have a cron expression")
			}
		}

		if w.Trigger.RabbitMQ != nil {
			if w.Trigger.RabbitMQ.URL == "" {
				return errors.New("validation: rabbitmq trigger must have a url")
			}

			if w.Trigger.RabbitMQ.Queue == "" {
				return errors.New("validation: rabbitmq trigger must have a queue")
			}
		}
	}

	// Validate on_error sub-steps
	for _, s := range w.Steps {
		for j, es := range s.OnError {
			if es.ID == "" {
				return fmt.Errorf("validation: step %q on_error[%d] must have an id", s.ID, j)
			}

			if es.Action == "" {
				return fmt.Errorf("validation: step %q on_error[%d] %q must have an action", s.ID, j, es.ID)
			}
		}
	}

	// Validate workflow-level on_error steps
	for j, es := range w.OnError {
		if es.ID == "" {
			return fmt.Errorf("validation: workflow on_error[%d] must have an id", j)
		}

		if es.Action == "" {
			return fmt.Errorf("validation: workflow on_error[%d] %q must have an action", j, es.ID)
		}
	}

	return nil
}

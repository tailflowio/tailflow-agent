package parser

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// lineErr formats a validation message, appending the 1-based source line
// when it is known (line > 0).
func lineErr(line int, msg string) error {
	if line > 0 {
		return fmt.Errorf("validation: %s (line %d)", msg, line)
	}

	return fmt.Errorf("validation: %s", msg)
}

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
	var errs []error

	if w.Version == "" {
		errs = append(errs, errors.New("validation: version is required"))
	}

	if w.Version != "" && w.Version != "2.0" {
		errs = append(errs, fmt.Errorf("validation: unsupported version %q, expected \"2.0\"", w.Version))
	}

	if w.Name == "" {
		errs = append(errs, errors.New("validation: name is required"))
	}

	if len(w.Steps) == 0 {
		errs = append(errs, errors.New("validation: at least one step is required"))
	}

	for _, fn := range []func(*Workflow) []error{
		validateStages,
		validateSteps,
		validateParams,
		validateTrigger,
		validateOnError,
		validateTesting,
		validatePersistence,
	} {
		errs = append(errs, fn(w)...)
	}

	return errors.Join(errs...)
}

// Messages flattens a validation error (typically an errors.Join result)
// into its individual messages. It returns nil for a nil error and a
// single-element slice for a non-joined error.
func Messages(err error) []string {
	if err == nil {
		return nil
	}

	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []string{err.Error()}
	}

	wrapped := joined.Unwrap()
	messages := make([]string, 0, len(wrapped))

	for _, e := range wrapped {
		messages = append(messages, e.Error())
	}

	return messages
}

func validatePersistence(w *Workflow) []error {
	if w.Persistence == nil {
		return nil
	}

	p := w.Persistence

	switch p.Type {
	case "", PersistenceMemory:
		if p.MariaDB != nil {
			return []error{fmt.Errorf("validation: persistence.type %q must not declare other backend sub-blocks", p.Type)}
		}

		if p.Memory != nil && p.Memory.MaxExecutions < 0 {
			return []error{errors.New("validation: persistence.memory.max_executions must be >= 0")}
		}

		return nil

	case PersistenceMariaDB:
		if p.MariaDB == nil {
			return []error{errors.New("validation: persistence.type \"mariadb\" requires a persistence.mariadb sub-block")}
		}

		if p.MariaDB.DSN == "" {
			return []error{errors.New("validation: persistence.mariadb.dsn is required")}
		}

		if p.Memory != nil {
			return []error{errors.New("validation: persistence.type \"mariadb\" must not declare other backend sub-blocks")}
		}

		return nil

	default:
		return []error{fmt.Errorf("validation: persistence.type %q is not supported (memory, mariadb)", p.Type)}
	}
}

func validateStages(w *Workflow) []error {
	var errs []error

	if len(w.Stages) == 0 {
		errs = append(errs, errors.New("validation: at least one stage is required"))
	}

	stageNames := make(map[string]bool, len(w.Stages))

	for i, s := range w.Stages {
		if s.Name == "" {
			errs = append(errs, lineErr(s.Line, fmt.Sprintf("stage[%d] must have a name", i)))
			continue
		}

		if stageNames[s.Name] {
			errs = append(errs, lineErr(s.Line, fmt.Sprintf("duplicate stage name %q", s.Name)))
			continue
		}

		stageNames[s.Name] = true
	}

	for _, s := range w.Steps {
		if s.Stage == "" {
			errs = append(errs, lineErr(s.Line, fmt.Sprintf("step %q must have a stage", s.ID)))
			continue
		}

		if !stageNames[s.Stage] {
			errs = append(errs, lineErr(s.Line, fmt.Sprintf("step %q references unknown stage %q", s.ID, s.Stage)))
		}
	}

	return errs
}

func validateSteps(w *Workflow) []error {
	var errs []error

	ids := make(map[string]bool, len(w.Steps))

	for i, s := range w.Steps {
		if s.ID == "" {
			errs = append(errs, lineErr(s.Line, fmt.Sprintf("step[%d] must have an id", i)))
			continue
		}

		if ids[s.ID] {
			errs = append(errs, lineErr(s.Line, fmt.Sprintf("duplicate step id %q", s.ID)))
			continue
		}

		ids[s.ID] = true

		if s.Action == "" {
			errs = append(errs, lineErr(s.Line, fmt.Sprintf("step %q must have an action", s.ID)))
		}

		if s.ErrorPolicy != "" && s.ErrorPolicy != "stop" && s.ErrorPolicy != "continue" && s.ErrorPolicy != "ignore" {
			errs = append(errs, lineErr(s.Line, fmt.Sprintf("step %q has invalid error_policy %q", s.ID, s.ErrorPolicy)))
		}

		if s.OnRecovery != "" && s.OnRecovery != "retry" && s.OnRecovery != "skip" && s.OnRecovery != "fail" {
			errs = append(errs, lineErr(s.Line, fmt.Sprintf("step %q: on_recovery must be retry, skip, or fail", s.ID)))
		}
	}

	for _, s := range w.Steps {
		for _, dep := range s.DependsOn {
			if dep == s.ID {
				errs = append(errs, lineErr(s.Line, fmt.Sprintf("step %q cannot depend on itself", s.ID)))
				continue
			}

			if !ids[dep] {
				errs = append(errs, lineErr(s.Line, fmt.Sprintf("step %q depends_on unknown step %q", s.ID, dep)))
			}
		}
	}

	return errs
}

func validateParams(w *Workflow) []error {
	var errs []error

	for i, p := range w.Params {
		if p.Name == "" {
			errs = append(errs, lineErr(p.Line, fmt.Sprintf("param[%d] must have a name", i)))
			continue
		}

		if p.Type == "" {
			errs = append(errs, lineErr(p.Line, fmt.Sprintf("param %q must have a type", p.Name)))
			continue
		}

		switch p.Type {
		case "string", "bool", "int", "float":
		default:
			errs = append(errs, lineErr(p.Line, fmt.Sprintf("param %q has unsupported type %q", p.Name, p.Type)))
		}
	}

	return errs
}

func boolToInt(b bool) int {
	if b {
		return 1
	}

	return 0
}

func validateTrigger(w *Workflow) []error {
	if w.Trigger == nil {
		return nil
	}

	var errs []error

	t := w.Trigger
	count := boolToInt(t.HTTP != nil) + boolToInt(t.Webhook != nil) +
		boolToInt(t.Schedule != nil) + boolToInt(t.RabbitMQ != nil)

	if count > 1 {
		errs = append(errs, errors.New("validation: workflow can only have one trigger (http, webhook, schedule, or rabbitmq)"))
	}

	if t.HTTP != nil {
		if t.HTTP.Path == "" {
			errs = append(errs, errors.New("validation: http trigger must have a path"))
		}

		if t.HTTP.Method == "" {
			errs = append(errs, errors.New("validation: http trigger must have a method"))
		}
	}

	if t.Webhook != nil && t.Webhook.Path == "" {
		errs = append(errs, errors.New("validation: webhook trigger must have a path"))
	}

	if t.Schedule != nil && t.Schedule.Cron == "" {
		errs = append(errs, errors.New("validation: schedule trigger must have a cron expression"))
	}

	if t.RabbitMQ != nil {
		if t.RabbitMQ.URL == "" {
			errs = append(errs, errors.New("validation: rabbitmq trigger must have a url"))
		}

		if t.RabbitMQ.Queue == "" {
			errs = append(errs, errors.New("validation: rabbitmq trigger must have a queue"))
		}
	}

	return errs
}

func validateTesting(w *Workflow) []error {
	var errs []error

	for _, s := range w.Steps {
		names := make(map[string]bool, len(s.Testing))

		for i, tc := range s.Testing {
			if tc.Name == "" {
				errs = append(errs, lineErr(s.Line, fmt.Sprintf("step %q testing[%d] must have a name", s.ID, i)))
				continue
			}

			if names[tc.Name] {
				errs = append(errs, lineErr(s.Line, fmt.Sprintf("step %q has duplicate test case name %q", s.ID, tc.Name)))
				continue
			}

			names[tc.Name] = true

			if tc.Output != nil && tc.Error != nil {
				errs = append(errs, lineErr(s.Line, fmt.Sprintf("step %q test case %q cannot have both output and error", s.ID, tc.Name)))
			}

			if tc.Expect != nil && tc.Expect.Status != "" {
				switch tc.Expect.Status {
				case "success", "failed", "skipped":
				default:
					errs = append(errs, lineErr(s.Line, fmt.Sprintf("step %q test case %q has invalid expect status %q", s.ID, tc.Name, tc.Expect.Status)))
				}
			}
		}
	}

	return errs
}

func validateOnError(w *Workflow) []error {
	var errs []error

	for _, s := range w.Steps {
		for j, es := range s.OnError {
			if es.ID == "" {
				errs = append(errs, fmt.Errorf("validation: step %q on_error[%d] must have an id", s.ID, j))
				continue
			}

			if es.Action == "" {
				errs = append(errs, fmt.Errorf("validation: step %q on_error[%d] %q must have an action", s.ID, j, es.ID))
			}
		}
	}

	for j, es := range w.OnError {
		if es.ID == "" {
			errs = append(errs, fmt.Errorf("validation: workflow on_error[%d] must have an id", j))
			continue
		}

		if es.Action == "" {
			errs = append(errs, fmt.Errorf("validation: workflow on_error[%d] %q must have an action", j, es.ID))
		}
	}

	return errs
}

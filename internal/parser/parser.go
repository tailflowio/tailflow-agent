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

	for _, fn := range []func(*Workflow) error{
		validateStages,
		validateSteps,
		validateParams,
		validateTrigger,
		validateOnError,
		validateTesting,
		validatePersistence,
	} {
		err := fn(w)
		if err != nil {
			return err
		}
	}

	return nil
}

func validatePersistence(w *Workflow) error {
	if w.Persistence == nil {
		return nil
	}

	p := w.Persistence

	switch p.Type {
	case "", PersistenceMemory:
		if p.MariaDB != nil || p.ClickHouse != nil {
			return fmt.Errorf("validation: persistence.type %q must not declare other backend sub-blocks", p.Type)
		}

		if p.Memory != nil && p.Memory.MaxExecutions < 0 {
			return errors.New("validation: persistence.memory.max_executions must be >= 0")
		}

		return nil

	case PersistenceMariaDB:
		if p.MariaDB == nil {
			return errors.New("validation: persistence.type \"mariadb\" requires a persistence.mariadb sub-block")
		}

		if p.MariaDB.DSN == "" {
			return errors.New("validation: persistence.mariadb.dsn is required")
		}

		if p.Memory != nil || p.ClickHouse != nil {
			return errors.New("validation: persistence.type \"mariadb\" must not declare other backend sub-blocks")
		}

		return nil

	case PersistenceClickHouse:
		if p.ClickHouse == nil {
			return errors.New("validation: persistence.type \"clickhouse\" requires a persistence.clickhouse sub-block")
		}

		if p.ClickHouse.DSN == "" {
			return errors.New("validation: persistence.clickhouse.dsn is required")
		}

		if p.Memory != nil || p.MariaDB != nil {
			return errors.New("validation: persistence.type \"clickhouse\" must not declare other backend sub-blocks")
		}

		return nil

	default:
		return fmt.Errorf("validation: persistence.type %q is not supported (memory, mariadb, clickhouse)", p.Type)
	}
}

func validateStages(w *Workflow) error {
	if len(w.Stages) == 0 {
		return errors.New("validation: at least one stage is required")
	}

	stageNames := make(map[string]bool, len(w.Stages))

	for i, s := range w.Stages {
		if s.Name == "" {
			return fmt.Errorf("validation: stage[%d] must have a name", i)
		}

		if stageNames[s.Name] {
			return fmt.Errorf("validation: duplicate stage name %q", s.Name)
		}

		stageNames[s.Name] = true
	}

	for _, s := range w.Steps {
		if s.Stage == "" {
			return fmt.Errorf("validation: step %q must have a stage", s.ID)
		}

		if !stageNames[s.Stage] {
			return fmt.Errorf("validation: step %q references unknown stage %q", s.ID, s.Stage)
		}
	}

	return nil
}

func validateSteps(w *Workflow) error {
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

		if s.ErrorPolicy != "" && s.ErrorPolicy != "stop" && s.ErrorPolicy != "continue" && s.ErrorPolicy != "ignore" {
			return fmt.Errorf("validation: step %q has invalid error_policy %q", s.ID, s.ErrorPolicy)
		}

		if s.OnRecovery != "" && s.OnRecovery != "retry" && s.OnRecovery != "skip" && s.OnRecovery != "fail" {
			return fmt.Errorf("validation: step %q: on_recovery must be retry, skip, or fail", s.ID)
		}
	}

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

	return nil
}

func validateParams(w *Workflow) error {
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

	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}

	return 0
}

func validateTrigger(w *Workflow) error {
	if w.Trigger == nil {
		return nil
	}

	t := w.Trigger
	count := boolToInt(t.HTTP != nil) + boolToInt(t.Webhook != nil) +
		boolToInt(t.Schedule != nil) + boolToInt(t.RabbitMQ != nil)

	if count > 1 {
		return errors.New("validation: workflow can only have one trigger (http, webhook, schedule, or rabbitmq)")
	}

	if t.HTTP != nil {
		if t.HTTP.Path == "" {
			return errors.New("validation: http trigger must have a path")
		}

		if t.HTTP.Method == "" {
			return errors.New("validation: http trigger must have a method")
		}
	}

	if t.Webhook != nil && t.Webhook.Path == "" {
		return errors.New("validation: webhook trigger must have a path")
	}

	if t.Schedule != nil && t.Schedule.Cron == "" {
		return errors.New("validation: schedule trigger must have a cron expression")
	}

	if t.RabbitMQ != nil {
		if t.RabbitMQ.URL == "" {
			return errors.New("validation: rabbitmq trigger must have a url")
		}

		if t.RabbitMQ.Queue == "" {
			return errors.New("validation: rabbitmq trigger must have a queue")
		}
	}

	return nil
}

func validateTesting(w *Workflow) error {
	for _, s := range w.Steps {
		names := make(map[string]bool, len(s.Testing))

		for i, tc := range s.Testing {
			if tc.Name == "" {
				return fmt.Errorf("validation: step %q testing[%d] must have a name", s.ID, i)
			}

			if names[tc.Name] {
				return fmt.Errorf("validation: step %q has duplicate test case name %q", s.ID, tc.Name)
			}

			names[tc.Name] = true

			if tc.Output != nil && tc.Error != nil {
				return fmt.Errorf("validation: step %q test case %q cannot have both output and error", s.ID, tc.Name)
			}

			if tc.Expect != nil && tc.Expect.Status != "" {
				switch tc.Expect.Status {
				case "success", "failed", "skipped":
				default:
					return fmt.Errorf("validation: step %q test case %q has invalid expect status %q", s.ID, tc.Name, tc.Expect.Status)
				}
			}
		}
	}

	return nil
}

func validateOnError(w *Workflow) error {
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

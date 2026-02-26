package parser

import (
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ParserTestSuite struct {
	suite.Suite
}

func TestParser(t *testing.T) {
	suite.Run(t, new(ParserTestSuite))
}

func (s *ParserTestSuite) SetupTest() {
	// required by convention
}

func (s *ParserTestSuite) TestParseBytes_ValidWorkflow() {
	yaml := `
version: "2.0"
name: "test-workflow"
description: "A test"
tags: ["test"]
params:
  - name: env
    type: string
    required: true
    default: "staging"
env:
  API_URL: "https://api.example.com"
steps:
  - id: step1
    action: log
    title: "Log something"
    config:
      message: "hello"
  - id: step2
    action: exec
    title: "Run command"
    depends_on: [step1]
    config:
      command: ["echo", "hello"]
    retry:
      max_attempts: 3
      delay: "5s"
`
	w, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)

	s.Equal("2.0", w.Version)
	s.Equal("test-workflow", w.Name)
	s.Equal("A test", w.Description)
	s.Equal([]string{"test"}, w.Tags)
	s.Len(w.Params, 1)
	s.Equal("env", w.Params[0].Name)
	s.Equal("string", w.Params[0].Type)
	s.True(w.Params[0].Required)
	s.Equal("staging", w.Params[0].Default)
	s.Equal("https://api.example.com", w.Env["API_URL"])
	s.Len(w.Steps, 2)
	s.Equal("step1", w.Steps[0].ID)
	s.Equal("log", w.Steps[0].Action)
	s.Equal([]string{"step1"}, w.Steps[1].DependsOn)
	s.Equal(3, w.Steps[1].Retry.MaxAttempts)
	s.Equal("5s", w.Steps[1].Retry.Delay)
}

func (s *ParserTestSuite) TestParseBytes_WithTrigger() {
	yaml := `
version: "2.0"
name: "api-workflow"
trigger:
  http:
    method: POST
    path: /api/users
steps:
  - id: handle
    action: js
    config:
      script: "return 'ok'"
`
	w, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)
	s.Require().NotNil(w.Trigger)
	s.Require().NotNil(w.Trigger.HTTP)
	s.Equal("POST", w.Trigger.HTTP.Method)
	s.Equal("/api/users", w.Trigger.HTTP.Path)
}

func (s *ParserTestSuite) TestParseBytes_WithWebhookTrigger() {
	yaml := `
version: "2.0"
name: "webhook-workflow"
trigger:
  webhook:
    path: /webhooks/github
    secret: "my-secret"
    filter: "trigger.body.ref == 'refs/heads/main'"
steps:
  - id: deploy
    action: exec
    config:
      command: ["./deploy.sh"]
`
	w, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)
	s.Require().NotNil(w.Trigger)
	s.Require().NotNil(w.Trigger.Webhook)
	s.Equal("/webhooks/github", w.Trigger.Webhook.Path)
	s.Equal("my-secret", w.Trigger.Webhook.Secret)
}

func (s *ParserTestSuite) TestParseBytes_WithOnError() {
	yaml := `
version: "2.0"
name: "error-handling"
steps:
  - id: deploy
    action: exec
    config:
      command: ["kubectl", "apply"]
    on_error:
      - id: rollback
        action: exec
        config:
          command: ["kubectl", "rollout", "undo"]
`
	w, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)
	s.Len(w.Steps[0].OnError, 1)
	s.Equal("rollback", w.Steps[0].OnError[0].ID)
}

func (s *ParserTestSuite) TestValidate_MissingVersion() {
	w := &Workflow{Name: "test", Steps: []Step{{ID: "s1", Action: "log"}}}
	err := Validate(w)
	s.ErrorContains(err, "version is required")
}

func (s *ParserTestSuite) TestValidate_UnsupportedVersion() {
	w := &Workflow{Version: "1.0", Name: "test", Steps: []Step{{ID: "s1", Action: "log"}}}
	err := Validate(w)
	s.ErrorContains(err, "unsupported version")
}

func (s *ParserTestSuite) TestValidate_MissingName() {
	w := &Workflow{Version: "2.0", Steps: []Step{{ID: "s1", Action: "log"}}}
	err := Validate(w)
	s.ErrorContains(err, "name is required")
}

func (s *ParserTestSuite) TestValidate_NoSteps() {
	w := &Workflow{Version: "2.0", Name: "test"}
	err := Validate(w)
	s.ErrorContains(err, "at least one step")
}

func (s *ParserTestSuite) TestValidate_MissingStepID() {
	w := &Workflow{Version: "2.0", Name: "test", Steps: []Step{{Action: "log"}}}
	err := Validate(w)
	s.ErrorContains(err, "must have an id")
}

func (s *ParserTestSuite) TestValidate_DuplicateStepID() {
	w := &Workflow{Version: "2.0", Name: "test", Steps: []Step{
		{ID: "s1", Action: "log"},
		{ID: "s1", Action: "exec"},
	}}
	err := Validate(w)
	s.ErrorContains(err, "duplicate step id")
}

func (s *ParserTestSuite) TestValidate_MissingAction() {
	w := &Workflow{Version: "2.0", Name: "test", Steps: []Step{{ID: "s1"}}}
	err := Validate(w)
	s.ErrorContains(err, "must have an action")
}

func (s *ParserTestSuite) TestValidate_UnknownDependency() {
	w := &Workflow{Version: "2.0", Name: "test", Steps: []Step{
		{ID: "s1", Action: "log", DependsOn: []string{"nonexistent"}},
	}}
	err := Validate(w)
	s.ErrorContains(err, "unknown step")
}

func (s *ParserTestSuite) TestValidate_SelfDependency() {
	w := &Workflow{Version: "2.0", Name: "test", Steps: []Step{
		{ID: "s1", Action: "log", DependsOn: []string{"s1"}},
	}}
	err := Validate(w)
	s.ErrorContains(err, "cannot depend on itself")
}

func (s *ParserTestSuite) TestValidate_InvalidParamType() {
	w := &Workflow{Version: "2.0", Name: "test",
		Params: []Param{{Name: "p", Type: "unknown"}},
		Steps:  []Step{{ID: "s1", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "unsupported type")
}

func (s *ParserTestSuite) TestValidate_BothTriggers() {
	w := &Workflow{Version: "2.0", Name: "test",
		Trigger: &Trigger{
			HTTP:    &HTTPTrigger{Method: "GET", Path: "/test"},
			Webhook: &WebhookTrigger{Path: "/hook"},
		},
		Steps: []Step{{ID: "s1", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "only have one trigger")
}

func (s *ParserTestSuite) TestValidate_ErrorPolicyValid() {
	for _, policy := range []string{"", "stop", "continue", "ignore"} {
		w := &Workflow{Version: "2.0", Name: "test", Steps: []Step{
			{ID: "s1", Action: "log", ErrorPolicy: policy},
		}}
		err := Validate(w)
		s.NoError(err, "policy %q should be valid", policy)
	}
}

func (s *ParserTestSuite) TestValidate_ErrorPolicyInvalid() {
	w := &Workflow{Version: "2.0", Name: "test", Steps: []Step{
		{ID: "s1", Action: "log", ErrorPolicy: "retry"},
	}}
	err := Validate(w)
	s.ErrorContains(err, "invalid error_policy")
}

func (s *ParserTestSuite) TestValidate_ScheduleTrigger() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{Schedule: &ScheduleTrigger{Cron: "*/5 * * * *"}},
		Steps:   []Step{{ID: "s1", Action: "log"}},
	}
	err := Validate(w)
	s.NoError(err)
}

func (s *ParserTestSuite) TestValidate_ScheduleTriggerEmptyCron() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{Schedule: &ScheduleTrigger{Cron: ""}},
		Steps:   []Step{{ID: "s1", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "cron expression")
}

func (s *ParserTestSuite) TestValidate_MultipleTriggers() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{
			HTTP:     &HTTPTrigger{Method: "GET", Path: "/test"},
			Schedule: &ScheduleTrigger{Cron: "*/5 * * * *"},
		},
		Steps: []Step{{ID: "s1", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "only have one trigger")
}

func (s *ParserTestSuite) TestParse_ValidFile() {
	// Write a temporary YAML file
	tmpFile := s.T().TempDir() + "/test.yaml"
	content := `version: "2.0"
name: "file-test"
steps:
  - id: step1
    action: log
    config:
      message: "hello"
`
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	s.Require().NoError(err)

	wf, err := Parse(tmpFile)
	s.Require().NoError(err)
	s.Equal("file-test", wf.Name)
	s.Len(wf.Steps, 1)
}

func (s *ParserTestSuite) TestParse_FileNotFound() {
	_, err := Parse("/nonexistent/path/file.yaml")
	s.Error(err)
	s.Contains(err.Error(), "read workflow file")
}

func (s *ParserTestSuite) TestParse_InvalidYAML() {
	tmpFile := s.T().TempDir() + "/bad.yaml"
	err := os.WriteFile(tmpFile, []byte(":::invalid:::yaml"), 0644)
	s.Require().NoError(err)

	_, err = Parse(tmpFile)
	s.Error(err)
}

func (s *ParserTestSuite) TestParseBytes_InvalidYAML() {
	_, err := ParseBytes([]byte("not: valid: yaml: ["))
	s.Error(err)
	s.Contains(err.Error(), "parse YAML")
}

func (s *ParserTestSuite) TestParseBytes_ValidationFails() {
	yaml := `
version: "1.0"
name: "test"
steps:
  - id: s1
    action: log
`
	_, err := ParseBytes([]byte(yaml))
	s.Error(err)
	s.Contains(err.Error(), "unsupported version")
}

func (s *ParserTestSuite) TestValidate_HTTPTriggerMissingPath() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{HTTP: &HTTPTrigger{Method: "GET"}},
		Steps:   []Step{{ID: "s1", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "path")
}

func (s *ParserTestSuite) TestValidate_HTTPTriggerMissingMethod() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{HTTP: &HTTPTrigger{Path: "/test"}},
		Steps:   []Step{{ID: "s1", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "method")
}

func (s *ParserTestSuite) TestValidate_WebhookTriggerMissingPath() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{Webhook: &WebhookTrigger{}},
		Steps:   []Step{{ID: "s1", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "path")
}

func (s *ParserTestSuite) TestValidate_RabbitMQTriggerMissingURL() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{RabbitMQ: &RabbitMQTrigger{Queue: "q1"}},
		Steps:   []Step{{ID: "s1", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "url")
}

func (s *ParserTestSuite) TestValidate_RabbitMQTriggerMissingQueue() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{RabbitMQ: &RabbitMQTrigger{URL: "amqp://localhost"}},
		Steps:   []Step{{ID: "s1", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "queue")
}

func (s *ParserTestSuite) TestValidate_ParamMissingName() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Params:  []Param{{Type: "string"}},
		Steps:   []Step{{ID: "s1", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have a name")
}

func (s *ParserTestSuite) TestValidate_ParamMissingType() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Params:  []Param{{Name: "p"}},
		Steps:   []Step{{ID: "s1", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have a type")
}

func (s *ParserTestSuite) TestValidate_OnError_MissingID() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Steps: []Step{{
			ID:     "s1",
			Action: "log",
			OnError: []Step{{Action: "log"}},
		}},
	}
	err := Validate(w)
	s.ErrorContains(err, "on_error")
	s.ErrorContains(err, "must have an id")
}

func (s *ParserTestSuite) TestValidate_OnError_MissingAction() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Steps: []Step{{
			ID:     "s1",
			Action: "log",
			OnError: []Step{{ID: "err1"}},
		}},
	}
	err := Validate(w)
	s.ErrorContains(err, "on_error")
	s.ErrorContains(err, "must have an action")
}

func (s *ParserTestSuite) TestValidate_WorkflowOnError_MissingID() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Steps:   []Step{{ID: "s1", Action: "log"}},
		OnError: []Step{{Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "workflow on_error")
	s.ErrorContains(err, "must have an id")
}

func (s *ParserTestSuite) TestValidate_WorkflowOnError_MissingAction() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Steps:   []Step{{ID: "s1", Action: "log"}},
		OnError: []Step{{ID: "err1"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "workflow on_error")
	s.ErrorContains(err, "must have an action")
}

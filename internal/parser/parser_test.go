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
stages:
  - name: default
steps:
  - id: step1
    stage: default
    action: log
    title: "Log something"
    config:
      message: "hello"
  - id: step2
    stage: default
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
stages:
  - name: default
steps:
  - id: handle
    stage: default
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
stages:
  - name: default
steps:
  - id: deploy
    stage: default
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
stages:
  - name: default
steps:
  - id: deploy
    stage: default
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
	w := &Workflow{Version: "2.0", Name: "test",
		Stages: []Stage{{Name: "default"}},
		Steps:  []Step{{Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have an id")
}

func (s *ParserTestSuite) TestValidate_DuplicateStepID() {
	w := &Workflow{Version: "2.0", Name: "test",
		Stages: []Stage{{Name: "default"}},
		Steps: []Step{
			{ID: "s1", Stage: "default", Action: "log"},
			{ID: "s1", Stage: "default", Action: "exec"},
		}}
	err := Validate(w)
	s.ErrorContains(err, "duplicate step id")
}

func (s *ParserTestSuite) TestValidate_MissingAction() {
	w := &Workflow{Version: "2.0", Name: "test",
		Stages: []Stage{{Name: "default"}},
		Steps:  []Step{{ID: "s1", Stage: "default"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have an action")
}

func (s *ParserTestSuite) TestValidate_UnknownDependency() {
	w := &Workflow{Version: "2.0", Name: "test",
		Stages: []Stage{{Name: "default"}},
		Steps: []Step{
			{ID: "s1", Stage: "default", Action: "log", DependsOn: []string{"nonexistent"}},
		}}
	err := Validate(w)
	s.ErrorContains(err, "unknown step")
}

func (s *ParserTestSuite) TestValidate_SelfDependency() {
	w := &Workflow{Version: "2.0", Name: "test",
		Stages: []Stage{{Name: "default"}},
		Steps: []Step{
			{ID: "s1", Stage: "default", Action: "log", DependsOn: []string{"s1"}},
		}}
	err := Validate(w)
	s.ErrorContains(err, "cannot depend on itself")
}

func (s *ParserTestSuite) TestValidate_InvalidParamType() {
	w := &Workflow{Version: "2.0", Name: "test",
		Params: []Param{{Name: "p", Type: "unknown"}},
		Stages: []Stage{{Name: "default"}},
		Steps:  []Step{{ID: "s1", Stage: "default", Action: "log"}},
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
		Stages: []Stage{{Name: "default"}},
		Steps:  []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "only have one trigger")
}

func (s *ParserTestSuite) TestValidate_ErrorPolicyValid() {
	for _, policy := range []string{"", "stop", "continue", "ignore"} {
		w := &Workflow{Version: "2.0", Name: "test",
			Stages: []Stage{{Name: "default"}},
			Steps: []Step{
				{ID: "s1", Stage: "default", Action: "log", ErrorPolicy: policy},
			}}
		err := Validate(w)
		s.NoError(err, "policy %q should be valid", policy)
	}
}

func (s *ParserTestSuite) TestValidate_ErrorPolicyInvalid() {
	w := &Workflow{Version: "2.0", Name: "test",
		Stages: []Stage{{Name: "default"}},
		Steps: []Step{
			{ID: "s1", Stage: "default", Action: "log", ErrorPolicy: "retry"},
		}}
	err := Validate(w)
	s.ErrorContains(err, "invalid error_policy")
}

func (s *ParserTestSuite) TestValidate_ScheduleTrigger() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{Schedule: &ScheduleTrigger{Cron: "*/5 * * * *"}},
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.NoError(err)
}

func (s *ParserTestSuite) TestValidate_ScheduleTriggerEmptyCron() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{Schedule: &ScheduleTrigger{Cron: ""}},
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
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
		Stages: []Stage{{Name: "default"}},
		Steps:  []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "only have one trigger")
}

func (s *ParserTestSuite) TestParse_ValidFile() {
	// Write a temporary YAML file
	tmpFile := s.T().TempDir() + "/test.yaml"
	content := `version: "2.0"
name: "file-test"
stages:
  - name: default
steps:
  - id: step1
    stage: default
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
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "path")
}

func (s *ParserTestSuite) TestValidate_HTTPTriggerMissingMethod() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{HTTP: &HTTPTrigger{Path: "/test"}},
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "method")
}

func (s *ParserTestSuite) TestValidate_WebhookTriggerMissingPath() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{Webhook: &WebhookTrigger{}},
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "path")
}

func (s *ParserTestSuite) TestValidate_RabbitMQTriggerMissingURL() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{RabbitMQ: &RabbitMQTrigger{Queue: "q1"}},
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "url")
}

func (s *ParserTestSuite) TestValidate_RabbitMQTriggerMissingQueue() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Trigger: &Trigger{RabbitMQ: &RabbitMQTrigger{URL: "amqp://localhost"}},
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "queue")
}

func (s *ParserTestSuite) TestValidate_ParamMissingName() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Params:  []Param{{Type: "string"}},
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have a name")
}

func (s *ParserTestSuite) TestValidate_ParamMissingType() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Params:  []Param{{Name: "p"}},
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have a type")
}

func (s *ParserTestSuite) TestValidate_OnError_MissingID() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps: []Step{{
			ID:     "s1",
			Stage:  "default",
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
		Stages:  []Stage{{Name: "default"}},
		Steps: []Step{{
			ID:     "s1",
			Stage:  "default",
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
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
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
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
		OnError: []Step{{ID: "err1"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "workflow on_error")
	s.ErrorContains(err, "must have an action")
}

func (s *ParserTestSuite) TestParseBytes_WithTesting() {
	yaml := `
version: "2.0"
name: "test-wf"
stages:
  - name: default
steps:
  - id: step1
    stage: default
    action: log
    config:
      message: "hello"
    testing:
      - name: "happy-path"
        output:
          rows:
            - id: 42
      - name: "db-error"
        error:
          message: "connection refused"
      - name: "check-result"
        expect:
          output:
            result: 99
          status: "success"
`
	w, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)
	s.Len(w.Steps[0].Testing, 3)

	// happy-path: mock output
	tc0 := w.Steps[0].Testing[0]
	s.Equal("happy-path", tc0.Name)
	s.NotNil(tc0.Output)
	s.Nil(tc0.Error)
	s.Nil(tc0.Expect)

	// db-error: mock error
	tc1 := w.Steps[0].Testing[1]
	s.Equal("db-error", tc1.Name)
	s.Nil(tc1.Output)
	s.Require().NotNil(tc1.Error)
	s.Equal("connection refused", tc1.Error.Message)

	// check-result: expect
	tc2 := w.Steps[0].Testing[2]
	s.Equal("check-result", tc2.Name)
	s.Nil(tc2.Output)
	s.Nil(tc2.Error)
	s.Require().NotNil(tc2.Expect)
	s.Equal("success", tc2.Expect.Status)
	s.NotNil(tc2.Expect.Output)
}

func (s *ParserTestSuite) TestValidate_TestingNameRequired() {
	w := &Workflow{
		Version: "2.0", Name: "test",
		Stages: []Stage{{Name: "default"}},
		Steps: []Step{{
			ID: "s1", Stage: "default", Action: "log",
			Testing: []TestCase{{Output: map[string]any{"ok": true}}},
		}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have a name")
}

func (s *ParserTestSuite) TestValidate_TestingDuplicateName() {
	w := &Workflow{
		Version: "2.0", Name: "test",
		Stages: []Stage{{Name: "default"}},
		Steps: []Step{{
			ID: "s1", Stage: "default", Action: "log",
			Testing: []TestCase{
				{Name: "case1", Output: "a"},
				{Name: "case1", Output: "b"},
			},
		}},
	}
	err := Validate(w)
	s.ErrorContains(err, "duplicate test case name")
}

func (s *ParserTestSuite) TestValidate_TestingOutputAndError() {
	w := &Workflow{
		Version: "2.0", Name: "test",
		Stages: []Stage{{Name: "default"}},
		Steps: []Step{{
			ID: "s1", Stage: "default", Action: "log",
			Testing: []TestCase{
				{Name: "bad", Output: "x", Error: &TestCaseError{Message: "err"}},
			},
		}},
	}
	err := Validate(w)
	s.ErrorContains(err, "cannot have both output and error")
}

func (s *ParserTestSuite) TestValidate_TestingInvalidExpectStatus() {
	w := &Workflow{
		Version: "2.0", Name: "test",
		Stages: []Stage{{Name: "default"}},
		Steps: []Step{{
			ID: "s1", Stage: "default", Action: "log",
			Testing: []TestCase{
				{Name: "bad", Expect: &TestCaseExpect{Status: "unknown"}},
			},
		}},
	}
	err := Validate(w)
	s.ErrorContains(err, "invalid expect status")
}

func (s *ParserTestSuite) TestValidate_TestingValidExpectStatuses() {
	for _, status := range []string{"success", "failed", "skipped"} {
		w := &Workflow{
			Version: "2.0", Name: "test",
			Stages: []Stage{{Name: "default"}},
			Steps: []Step{{
				ID: "s1", Stage: "default", Action: "log",
				Testing: []TestCase{
					{Name: "ok", Expect: &TestCaseExpect{Status: status}},
				},
			}},
		}
		err := Validate(w)
		s.NoError(err, "status %q should be valid", status)
	}
}

func (s *ParserTestSuite) TestParseBytes_GroupAction() {
	yaml := `
version: "2.0"
name: test-group-action
params:
  - name: customer_id
    type: string
    required: true
stages:
  - name: default
steps:
  - id: tag-customer
    stage: default
    action: group
    config:
      key: "customer-{{ params.customer_id }}"
  - id: step1
    stage: default
    action: log
    depends_on: [tag-customer]
    config:
      message: hello
`
	wf, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)
	s.Equal("group", wf.Steps[0].Action)
	s.Equal("customer-{{ params.customer_id }}", wf.Steps[0].Config["key"])
}

func (s *ParserTestSuite) TestParseBytes_OnRecoveryValues() {
	yaml := `
version: "2.0"
name: test-on-recovery
stages:
  - name: default
steps:
  - id: step-retry
    stage: default
    action: http
    on_recovery: retry
    config:
      url: http://example.com
  - id: step-skip
    stage: default
    action: http
    on_recovery: skip
    depends_on: [step-retry]
    config:
      url: http://example.com
  - id: step-fail
    stage: default
    action: http
    on_recovery: fail
    depends_on: [step-skip]
    config:
      url: http://example.com
  - id: step-default
    stage: default
    action: log
    depends_on: [step-fail]
    config:
      message: hello
`
	wf, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)
	s.Equal("retry", wf.Steps[0].OnRecovery)
	s.Equal("skip", wf.Steps[1].OnRecovery)
	s.Equal("fail", wf.Steps[2].OnRecovery)
	s.Empty(wf.Steps[3].OnRecovery)
}

func (s *ParserTestSuite) TestParseBytes_OnRecoveryInvalidValue() {
	yaml := `
version: "2.0"
name: test-invalid-recovery
stages:
  - name: default
steps:
  - id: step1
    stage: default
    action: http
    on_recovery: explode
    config:
      url: http://example.com
`
	_, err := ParseBytes([]byte(yaml))
	s.Error(err)
	s.Contains(err.Error(), "on_recovery")
}

func (s *ParserTestSuite) TestParseBytes_IdempotencyKey() {
	yaml := `
version: "2.0"
name: test-idempotency
trigger:
  http:
    method: POST
    path: /api/test
    idempotency_key: "{{ trigger.body.customer_id }}"
stages:
  - name: default
steps:
  - id: step1
    stage: default
    action: log
    config:
      message: hello
`
	wf, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)
	s.Equal("{{ trigger.body.customer_id }}", wf.Trigger.HTTP.IdempotencyKey)
}

func (s *ParserTestSuite) TestParseBytes_IdempotencyKeyOptional() {
	yaml := `
version: "2.0"
name: test-no-idempotency
trigger:
  http:
    method: POST
    path: /api/test
stages:
  - name: default
steps:
  - id: step1
    stage: default
    action: log
    config:
      message: hello
`
	wf, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)
	s.Empty(wf.Trigger.HTTP.IdempotencyKey)
}

func (s *ParserTestSuite) TestValidate_TestingValidCases() {
	w := &Workflow{
		Version: "2.0", Name: "test",
		Stages: []Stage{{Name: "default"}},
		Steps: []Step{{
			ID: "s1", Stage: "default", Action: "log",
			Testing: []TestCase{
				{Name: "mock-output", Output: map[string]any{"ok": true}},
				{Name: "mock-error", Error: &TestCaseError{Message: "fail"}},
				{Name: "expect-only", Expect: &TestCaseExpect{Status: "success"}},
			},
		}},
	}
	err := Validate(w)
	s.NoError(err)
}

func (s *ParserTestSuite) TestValidate_PersistenceOmittedIsValid() {
	w := newValidPersistenceWorkflow()
	w.Persistence = nil
	s.NoError(Validate(w))
}

func (s *ParserTestSuite) TestValidate_PersistenceMemoryDefault() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{} // empty type means memory
	s.NoError(Validate(w))
}

func (s *ParserTestSuite) TestValidate_PersistenceMemoryWithMaxExecutions() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:   PersistenceMemory,
		Memory: &MemoryPersistence{MaxExecutions: 50},
	}
	s.NoError(Validate(w))
}

func (s *ParserTestSuite) TestValidate_PersistenceMemoryNegativeMaxRejected() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:   PersistenceMemory,
		Memory: &MemoryPersistence{MaxExecutions: -1},
	}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "max_executions")
}

func (s *ParserTestSuite) TestValidate_PersistenceMariaDB() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:    PersistenceMariaDB,
		MariaDB: &MariaDBPersistence{DSN: "user:pass@tcp(localhost:3306)/tailflow"},
	}
	s.NoError(Validate(w))
}

func (s *ParserTestSuite) TestValidate_PersistenceMariaDBMissingSubBlock() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{Type: PersistenceMariaDB}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "persistence.mariadb")
}

func (s *ParserTestSuite) TestValidate_PersistenceMariaDBMissingDSN() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:    PersistenceMariaDB,
		MariaDB: &MariaDBPersistence{},
	}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "dsn is required")
}

func (s *ParserTestSuite) TestValidate_PersistenceClickHouse() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:       PersistenceClickHouse,
		ClickHouse: &ClickHousePersistence{DSN: "clickhouse://localhost:9000/tailflow"},
	}
	s.NoError(Validate(w))
}

func (s *ParserTestSuite) TestValidate_PersistenceClickHouseMissingSubBlock() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{Type: PersistenceClickHouse}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "persistence.clickhouse")
}

func (s *ParserTestSuite) TestValidate_PersistenceClickHouseMissingDSN() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:       PersistenceClickHouse,
		ClickHouse: &ClickHousePersistence{},
	}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "dsn is required")
}

func (s *ParserTestSuite) TestValidate_PersistenceUnknownType() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{Type: "redis"}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "not supported")
}

func (s *ParserTestSuite) TestValidate_PersistenceMariaDBWithExtraSubBlockRejected() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:       PersistenceMariaDB,
		MariaDB:    &MariaDBPersistence{DSN: "user@/db"},
		ClickHouse: &ClickHousePersistence{DSN: "clickhouse://x/y"},
	}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "must not declare other backend sub-blocks")
}

func (s *ParserTestSuite) TestParseBytes_PersistenceMariaDBYAML() {
	yaml := `
version: "2.0"
name: "test"
persistence:
  type: mariadb
  mariadb:
    dsn: "user:pass@tcp(localhost:3306)/tailflow"
    table_prefix: "tf_"
stages:
  - name: default
steps:
  - id: s1
    stage: default
    action: log
`
	w, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)
	s.Require().NotNil(w.Persistence)
	s.Equal("mariadb", w.Persistence.Type)
	s.Require().NotNil(w.Persistence.MariaDB)
	s.Equal("user:pass@tcp(localhost:3306)/tailflow", w.Persistence.MariaDB.DSN)
	s.Equal("tf_", w.Persistence.MariaDB.TablePrefix)
}

func newValidPersistenceWorkflow() *Workflow {
	return &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
}

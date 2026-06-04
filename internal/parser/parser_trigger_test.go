package parser

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

func (s *ParserTestSuite) TestValidate_BothTriggers() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
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

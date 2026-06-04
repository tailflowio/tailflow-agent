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

func (s *ParserTestSuite) TestParse_ValidFile() {
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

func newValidPersistenceWorkflow() *Workflow {
	return &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
}

package fx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

type WorkflowTestSuite struct {
	suite.Suite
}

func TestWorkflowTestSuite(t *testing.T) {
	suite.Run(t, new(WorkflowTestSuite))
}

func (s *WorkflowTestSuite) TestNewWorkflow_Success() {
	dir := s.T().TempDir()
	wfPath := filepath.Join(dir, "wf.yaml")

	s.Require().NoError(os.WriteFile(wfPath, []byte(`name: t
version: "2.0"
stages:
  - name: default
steps:
  - id: noop
    stage: default
    action: log
    config:
      message: hi
`), 0o644))

	out, err := NewWorkflow(WorkflowIn{Config: Config{WorkflowPath: wfPath}})

	s.Require().NoError(err)
	s.Require().NotNil(out.Workflow)
	s.Equal("t", out.Workflow.Name)
}

func (s *WorkflowTestSuite) TestNewWorkflow_WhenFileNotFound() {
	_, err := NewWorkflow(WorkflowIn{
		Config: Config{WorkflowPath: "/nonexistent/path/wf.yaml"},
	})

	s.Require().Error(err)
	s.Contains(err.Error(), "parse workflow")
}

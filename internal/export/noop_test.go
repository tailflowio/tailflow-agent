package export

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"
)

type NoopPortsTestSuite struct {
	suite.Suite

	context context.Context
}

func TestNoopPorts(t *testing.T) {
	suite.Run(t, new(NoopPortsTestSuite))
}

func (s *NoopPortsTestSuite) SetupTest() {
	s.context = context.Background()
}

func (s *NoopPortsTestSuite) TestNoopExporter_StartShutdownNoPanic() {
	exporter := NewNoopExporter()
	s.NotPanics(func() {
		exporter.Start(s.context)
		exporter.Shutdown()
	})
}

func (s *NoopPortsTestSuite) TestNoopClaimer_AlwaysGrants() {
	claimer := NewNoopClaimer()

	result, err := claimer.ClaimExecution(s.context, "exec-1", "wf", "key-1")
	s.Require().NoError(err)
	s.True(result.Claimed)
}

func (s *NoopPortsTestSuite) TestNoopRecoverer_YieldsNothing() {
	recoverer := NewNoopRecoverer()

	recovered, err := recoverer.RecoverExecutions(s.context, "agent-1")
	s.Require().NoError(err)
	s.Nil(recovered)
}

func (s *NoopPortsTestSuite) TestRecoveredExecution_JSONBoundary() {
	raw := `{
        "execution_id": "exec-1",
        "workflow_name": "wf",
        "status": "running",
        "params": "{\"env\":\"prod\"}",
        "steps": {"step1": {"status": "success"}}
    }`

	var rec RecoveredExecution

	err := json.Unmarshal([]byte(raw), &rec)
	s.Require().NoError(err)

	s.Equal("exec-1", rec.ExecutionID)
	s.Equal("wf", rec.WorkflowName)
	s.Equal("running", rec.Status)
	s.Equal(`{"env":"prod"}`, rec.RawParams)
	s.JSONEq(`{"step1": {"status": "success"}}`, string(rec.RawSteps))

	s.Nil(rec.Params)
	s.Nil(rec.Steps)
}

package fx

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/parser"
)

type ExportPortsTestSuite struct {
	suite.Suite
}

func TestExportPortsTestSuite(t *testing.T) {
	suite.Run(t, new(ExportPortsTestSuite))
}

func (s *ExportPortsTestSuite) TestNewExportPorts_NoopWhenURLEmpty() {
	out := NewExportPorts(ExportPortsIn{
		Config:   Config{},
		Workflow: &parser.Workflow{},
		Logger:   slog.Default(),
	})
	s.NotNil(out.Claimer)
	s.NotNil(out.Exporter)
	s.NotNil(out.Recoverer)
}

func (s *ExportPortsTestSuite) TestResolveTriggerType() {
	s.Equal("", resolveTriggerType(&parser.Workflow{}))
	s.Equal("http", resolveTriggerType(&parser.Workflow{Trigger: &parser.Trigger{HTTP: &parser.HTTPTrigger{}}}))
	s.Equal("webhook", resolveTriggerType(&parser.Workflow{Trigger: &parser.Trigger{Webhook: &parser.WebhookTrigger{}}}))
	s.Equal("schedule", resolveTriggerType(&parser.Workflow{Trigger: &parser.Trigger{Schedule: &parser.ScheduleTrigger{}}}))
}

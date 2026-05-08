package fx

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ExportPortsTestSuite struct {
	suite.Suite
}

func TestExportPortsTestSuite(t *testing.T) {
	suite.Run(t, new(ExportPortsTestSuite))
}

func (s *ExportPortsTestSuite) TestNewExportPorts_AlwaysWiresNoops() {
	out := NewExportPorts(ExportPortsIn{})
	s.NotNil(out.Claimer)
	s.NotNil(out.Exporter)
	s.NotNil(out.Recoverer)
}

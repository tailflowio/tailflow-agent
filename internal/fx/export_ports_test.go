package fx

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/store"
	"github.com/tailflow/tailflow/internal/store/mariadb"
)

type ExportPortsTestSuite struct {
	suite.Suite
}

func TestExportPortsTestSuite(t *testing.T) {
	suite.Run(t, new(ExportPortsTestSuite))
}

func (s *ExportPortsTestSuite) TestNewExportPorts_MemoryUsesMemoryClaimer() {
	out := NewExportPorts(ExportPortsIn{Workflow: &parser.Workflow{}})

	_, ok := out.Claimer.(*export.MemoryClaimer)
	s.True(ok)
}

func (s *ExportPortsTestSuite) TestNewExportPorts_ExplicitMemoryUsesMemoryClaimer() {
	wf := &parser.Workflow{
		Persistence: &parser.Persistence{Type: parser.PersistenceMemory},
	}

	out := NewExportPorts(ExportPortsIn{Workflow: wf})

	_, ok := out.Claimer.(*export.MemoryClaimer)
	s.True(ok)
}

func (s *ExportPortsTestSuite) TestNewExportPorts_MariaDBUsesStoreClaimer() {
	db, _, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	s.Require().NoError(err)
	defer func() { _ = db.Close() }()

	st, err := mariadb.NewWithDB(db, "tf_")
	s.Require().NoError(err)

	wf := &parser.Workflow{
		Persistence: &parser.Persistence{Type: parser.PersistenceMariaDB},
	}

	out := NewExportPorts(ExportPortsIn{Workflow: wf, Store: st})

	s.Same(st, out.Claimer)
}

func (s *ExportPortsTestSuite) TestNewExportPorts_MariaDBFallsBackToMemoryWhenStoreMismatch() {
	wf := &parser.Workflow{
		Persistence: &parser.Persistence{Type: parser.PersistenceMariaDB},
	}

	out := NewExportPorts(ExportPortsIn{Workflow: wf})

	_, ok := out.Claimer.(*export.MemoryClaimer)
	s.True(ok)
}

func (s *ExportPortsTestSuite) TestNewExportPorts_UnknownBackendUsesNoopClaimer() {
	wf := &parser.Workflow{
		Persistence: &parser.Persistence{Type: "unknown"},
	}

	out := NewExportPorts(ExportPortsIn{Workflow: wf})

	_, isMemory := out.Claimer.(*export.MemoryClaimer)
	s.False(isMemory)
	s.NotNil(out.Claimer)
}

func (s *ExportPortsTestSuite) TestNewExportPorts_ExporterRecovererStayNoop() {
	out := NewExportPorts(ExportPortsIn{Workflow: &parser.Workflow{}})
	s.NotNil(out.Exporter)
	s.NotNil(out.Recoverer)
}

func (s *ExportPortsTestSuite) TestNewExportPorts_MemoryBackendRecovererWhenEnabled() {
	wf := &parser.Workflow{Recovery: true}
	memStore := store.NewExecutionStore(10)

	out := NewExportPorts(ExportPortsIn{Workflow: wf, Store: memStore})

	_, ok := out.Recoverer.(*store.MemoryRecoverer)
	s.True(ok)
}

func (s *ExportPortsTestSuite) TestNewExportPorts_MariaDBRecovererWhenEnabled() {
	db, _, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	s.Require().NoError(err)
	defer func() { _ = db.Close() }()

	st, err := mariadb.NewWithDB(db, "tf_")
	s.Require().NoError(err)

	wf := &parser.Workflow{Recovery: true}

	out := NewExportPorts(ExportPortsIn{Workflow: wf, Store: st})

	s.Same(st, out.Recoverer)
}

func (s *ExportPortsTestSuite) TestNewExportPorts_NoopWhenRecoveryDisabled() {
	wf := &parser.Workflow{Recovery: false}
	memStore := store.NewExecutionStore(10)

	out := NewExportPorts(ExportPortsIn{Workflow: wf, Store: memStore})

	_, isMemory := out.Recoverer.(*store.MemoryRecoverer)
	s.False(isMemory)
	s.NotNil(out.Recoverer)
}

func (s *ExportPortsTestSuite) TestNewExportPorts_NoopRecovererWhenUnknownStore() {
	wf := &parser.Workflow{Recovery: true}

	out := NewExportPorts(ExportPortsIn{Workflow: wf})

	_, isMemory := out.Recoverer.(*store.MemoryRecoverer)
	s.False(isMemory)
	s.NotNil(out.Recoverer)
}

func (s *ExportPortsTestSuite) TestNewExportPorts_ClaimerExporterAlwaysNoopOfRecovery() {
	wf := &parser.Workflow{Recovery: true}
	memStore := store.NewExecutionStore(10)

	out := NewExportPorts(ExportPortsIn{Workflow: wf, Store: memStore})

	_, ok := out.Claimer.(*export.MemoryClaimer)
	s.True(ok)
	s.NotNil(out.Exporter)
}

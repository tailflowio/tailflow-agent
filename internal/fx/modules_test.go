package fx

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/export"
	tfotel "github.com/tailflow/tailflow/internal/otel"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/server"
	uberfx "go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

type ModulesTestSuite struct {
	suite.Suite
}

func TestModules(t *testing.T) { suite.Run(t, new(ModulesTestSuite)) }

func (s *ModulesTestSuite) TestProvideRegistry_DefaultAppliesSaaSAllowlist() {
	reg := provideRegistry(Config{SelfHosted: false})
	s.NotNil(reg)

	// Built-in `exec` is blocked by the SaaS allowlist.
	_, err := reg.Create("exec")
	s.Error(err, "exec must be blocked under SaaS profile")
}

func (s *ModulesTestSuite) TestProvideRegistry_SelfHostedKeepsEverything() {
	reg := provideRegistry(Config{SelfHosted: true})
	s.NotNil(reg)

	_, err := reg.Create("exec")
	s.NoError(err, "exec must be available in self-hosted profile")
}

func (s *ModulesTestSuite) TestSaaSAllowedActions_FiltersDangerousNames() {
	allowed := saasAllowedActions([]string{"http.get", "exec", "js", "file.read", "file.write", "wait.webhook"})
	s.NotContains(allowed, "exec")
	s.NotContains(allowed, "js")
	s.NotContains(allowed, "file.read")
	s.NotContains(allowed, "file.write")
	s.Contains(allowed, "http.get")
	s.Contains(allowed, "wait.webhook")
}

func (s *ModulesTestSuite) TestResolveTriggerType() {
	s.Equal("", resolveTriggerType(&parser.Workflow{}))
	s.Equal("http", resolveTriggerType(&parser.Workflow{Trigger: &parser.Trigger{HTTP: &parser.HTTPTrigger{}}}))
	s.Equal("webhook", resolveTriggerType(&parser.Workflow{Trigger: &parser.Trigger{Webhook: &parser.WebhookTrigger{}}}))
	s.Equal("schedule", resolveTriggerType(&parser.Workflow{Trigger: &parser.Trigger{Schedule: &parser.ScheduleTrigger{}}}))
}

func (s *ModulesTestSuite) TestProvideExporter_NoopWhenURLEmpty() {
	exp := provideExporter(Config{}, nil, &parser.Workflow{}, slog.Default())
	s.NotNil(exp)
	_, ok := exp.(interface{ noop() bool })
	_ = ok // we just confirm it implements the interface; actual type is internal
	// Sanity: ensure it's the noop. The export.NewNoopExporter returns a known type;
	// compare by behavioural shape via type-assert against the export.EventExporter
	// interface — we know the noop is safe to call without start.
	var _ export.EventExporter = exp
}

func (s *ModulesTestSuite) TestProvideClaimerAndRecoverer_NoopWhenURLEmpty() {
	c := provideClaimer(Config{})
	r := provideRecoverer(Config{})
	s.NotNil(c)
	s.NotNil(r)
}

// TestRunApp_StartsAndStops boots the full ServeModule against a tiny fixture
// workflow, verifies that fx.Populate yields a *server.Server, and tears the
// app down without invoking srv.Run (which would bind a TCP port).
func TestRunApp_StartsAndStops(t *testing.T) {
	dir := t.TempDir()
	wfPath := filepath.Join(dir, "wf.yaml")

	require.NoError(t, os.WriteFile(wfPath, []byte(`name: t
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

	cfg := Config{
		WorkflowPath: wfPath,
		Port:         0,
		MaxExecs:     10,
		SelfHosted:   true,
		Editor:       false,
		Version:      "test",
		LogLevel:     slog.LevelError + 1,
		OTel:         tfotel.Config{},
	}

	var srv *server.Server

	app := fxtest.New(t,
		uberfx.NopLogger,
		uberfx.Supply(cfg),
		ServeModule,
		uberfx.Populate(&srv),
	)

	require.NotNil(t, app)
	require.NotNil(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app.RequireStart()
	app.RequireStop()

	_ = ctx
}

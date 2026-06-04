package fx

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	tfotel "github.com/tailflow/tailflow/internal/otel"
	"github.com/tailflow/tailflow/internal/server"
	uberfx "go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

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
		Unsafe:       true,
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

	app.RequireStart()
	app.RequireStop()
}

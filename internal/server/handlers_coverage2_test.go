package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/store"
)

// errReader is an io.Reader that always returns an error, used to trigger
// io.ReadAll failure in handlePutWorkflowRaw.
type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

// TestPutWorkflowRaw_ReadBodyError covers handlePutWorkflowRaw – io.ReadAll fails.
func (s *HandlersWorkflowRawTestSuite) TestPutWorkflowRaw_ReadBodyError() {
	dir := s.T().TempDir()
	filePath := dir + "/workflow.yaml"
	srv := newTestServerRaw(s.T(), filePath, true)

	req := httptest.NewRequest("PUT", "/api/workflow/raw", errReader{})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusBadRequest, w.Code)
	s.Contains(w.Body.String(), "read body")
}

// newTestServerSingleChildLoop builds a workflow where step_b has goto step_a,
// and step_a has step_b as its only child, exercising buildLoopBodies –
// the current == s.ID branch when walking the loop body chain back.
func newTestServerSingleChildLoop(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "single-child-loop-workflow"
stages:
  - name: default
steps:
  - id: step_a
    action: log
    stage: default
    config:
      message: "a"
  - id: step_b
    action: log
    stage: default
    depends_on: [step_a]
    config:
      message: "b"
    goto:
      target: step_a
      max_iterations: 3
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	return newTestServerFromWorkflow(t, wf)
}

// newTestServerRootLoop builds a workflow where the loop step is at the root (depth 0)
// and has a pipeline — exercises treeContinuation at depth 0.
func newTestServerRootLoop(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "root-loop-workflow"
stages:
  - name: default
steps:
  - id: root_loop
    action: loop
    stage: default
    config:
      items: [1, 2]
      actions:
        - action: log
          title: "Log item"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	return newTestServerFromWorkflow(t, wf)
}

// newTestServerDeepLoopTree builds a workflow with a loop step at depth >= 2,
// exercising treeContinuation depth > 1 loop body.
func newTestServerDeepLoopTree(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "deep-loop-tree"
stages:
  - name: default
steps:
  - id: root
    action: log
    stage: default
    config:
      message: "root"
  - id: level1
    action: log
    stage: default
    depends_on: [root]
    config:
      message: "level1"
  - id: level2_loop
    action: loop
    stage: default
    depends_on: [level1]
    config:
      items: [1]
      actions:
        - action: log
          title: "Deep item"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	return newTestServerFromWorkflow(t, wf)
}

// newTestServerNonSiblingConvergent builds a workflow where the convergent node's
// parents are NOT siblings (have different grandparents), exercising the
// allSiblings=false branch in resolveConvergent.
func newTestServerNonSiblingConvergent(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "non-sibling-convergent"
stages:
  - name: default
steps:
  - id: root_a
    action: log
    stage: default
    config:
      message: "root_a"
  - id: root_b
    action: log
    stage: default
    config:
      message: "root_b"
  - id: step_a
    action: log
    stage: default
    depends_on: [root_a]
    config:
      message: "a"
  - id: step_b
    action: log
    stage: default
    depends_on: [root_b]
    config:
      message: "b"
  - id: step_end
    action: log
    stage: default
    depends_on: [step_a, step_b]
    config:
      message: "end"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	return newTestServerFromWorkflow(t, wf)
}

// newTestServerThreeWayConvergent builds a workflow with 3 sibling parents
// converging on one node, exercising the "┤" middle marker in resolveConvergent.
func newTestServerThreeWayConvergent(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "three-way-convergent"
stages:
  - name: default
steps:
  - id: root
    action: log
    stage: default
    config:
      message: "root"
  - id: step_a
    action: log
    stage: default
    depends_on: [root]
    config:
      message: "a"
  - id: step_b
    action: log
    stage: default
    depends_on: [root]
    config:
      message: "b"
  - id: step_c
    action: log
    stage: default
    depends_on: [root]
    config:
      message: "c"
  - id: step_end
    action: log
    stage: default
    depends_on: [step_a, step_b, step_c]
    config:
      message: "end"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	return newTestServerFromWorkflow(t, wf)
}

// newTestServerDeepTreeNonLastParent builds a workflow with a loop step at depth 2
// whose parent is NOT the last sibling, exercising treeContinuation "│   " branch.
func newTestServerDeepTreeNonLastParent(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "deep-non-last"
stages:
  - name: default
steps:
  - id: root
    action: log
    stage: default
    config:
      message: "root"
  - id: child_a
    action: log
    stage: default
    depends_on: [root]
    config:
      message: "child a"
  - id: child_b
    action: log
    stage: default
    depends_on: [root]
    config:
      message: "child b (last)"
  - id: grandchild_a
    action: loop
    stage: default
    depends_on: [child_a]
    config:
      items: [1]
      actions:
        - action: log
          title: "In child_a"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	return newTestServerFromWorkflow(t, wf)
}

// newTestServerFromWorkflow creates a minimal Server with the given workflow.
func newTestServerFromWorkflow(t testing.TB, wf *parser.Workflow) *Server {
	t.Helper()

	bus := event.NewBus()
	if tc, ok := t.(interface{ Cleanup(func()) }); ok {
		tc.Cleanup(bus.Close)
	}

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := engine.NewExecutor(reg, bus, logger, nil, nil, nil)

	return New(Config{
		Port:           0,
		Executor:       exec,
		Workflow:       wf,
		ExecutionStore: store.NewExecutionStore(10),
		EventBus:       bus,
		Logger:         logger,
	})
}

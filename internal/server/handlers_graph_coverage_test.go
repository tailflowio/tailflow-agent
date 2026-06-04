package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

// TestHandleGetWorkflowGraph_ConvergentDAG covers resolveConvergent, treePrefix and treeContinuation.
func (s *HandlersTestSuite) TestHandleGetWorkflowGraph_ConvergentDAG() {
	srv := newTestServerConvergent(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &graph)
	s.Require().NoError(err)

	nodes := graph["nodes"].([]any)
	s.Len(nodes, 4)

	tree := graph["tree"].([]any)
	s.NotEmpty(tree)

	var foundEnd bool
	for _, n := range nodes {
		node := n.(map[string]any)
		if node["id"] == "step_end" {
			foundEnd = true
		}
	}
	s.True(foundEnd, "convergent step_end should appear in nodes")
}

// TestHandleGetWorkflowGraph_DeepTree covers treePrefix and treeContinuation depth>1 paths.
func (s *HandlersTestSuite) TestHandleGetWorkflowGraph_DeepTree() {
	srv := newTestServerDeepTree(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &graph)
	s.Require().NoError(err)

	nodes := graph["nodes"].([]any)
	s.Len(nodes, 4)

	tree := graph["tree"].([]any)
	s.NotEmpty(tree)
}

// TestHandleGetWorkflowGraph_LoopBodiesMultipleChildren covers buildLoopBodies – loop target node has >1 child.
func (s *HandlersTestSuite) TestHandleGetWorkflowGraph_LoopBodiesMultipleChildren() {
	srv := newTestServerGraph(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

// TestBuildTreeLines_WithMergeMarker covers render – step with mergeMarker non-empty.
func (s *HandlersTestSuite) TestBuildTreeLines_WithMergeMarker() {
	srv := newTestServerConvergent(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	json.Unmarshal(w.Body.Bytes(), &graph)

	tree := graph["tree"].([]any)
	s.NotEmpty(tree)

	var foundMerge bool
	for _, line := range tree {
		tl := line.(map[string]any)
		if merge, ok := tl["merge"].(string); ok && merge != "" {
			foundMerge = true
		}
	}

	s.True(foundMerge, "at least one tree line should have a non-empty merge marker")
}

// TestBuildLoopBodies_MultipleChildren covers buildLoopBodies – node has >1 child (break path).
func (s *HandlersTestSuite) TestBuildLoopBodies_MultipleChildren() {
	srv := newTestServerGraph(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

// TestBracketChar_EndIndexNotAnnotation covers bracketChar – idx == endIdx && !isAnnotation.
func (s *HandlersTestSuite) TestBracketChar_EndIndexNotAnnotation() {
	r := &treeBuilder{}
	r.loops = []loopDisplay{{startIdx: 0, endIdx: 2}}

	s.Equal("╭ ", r.bracketChar(0, false))
	s.Equal("│ ", r.bracketChar(1, false))
	s.Equal("│ ", r.bracketChar(2, false))
	s.Equal("╰ ", r.bracketChar(2, true))
	s.Equal("  ", r.bracketChar(3, false))
}

// TestTreePrefix_DeepNested covers treePrefix for depth>1 nodes.
func (s *HandlersTestSuite) TestTreePrefix_DeepNested() {
	srv := newTestServerDeepTree(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	json.Unmarshal(w.Body.Bytes(), &graph)

	tree := graph["tree"].([]any)
	var grandchildLine map[string]any
	for _, line := range tree {
		tl := line.(map[string]any)
		if tl["name"] == "grandchild" {
			grandchildLine = tl
			break
		}
	}

	s.NotNil(grandchildLine, "grandchild should appear in tree")
	prefix := grandchildLine["prefix"].(string)
	s.NotEmpty(prefix)
}

// TestTreeContinuation_DeepPipeline covers treeContinuation for a loop step with depth>0.
func (s *HandlersTestSuite) TestTreeContinuation_DeepPipeline() {
	yaml := `version: "2.0"
name: "pipeline-deep"
stages:
  - name: default
steps:
  - id: root
    action: log
    stage: default
    config:
      message: "root"
  - id: deep_loop
    action: loop
    stage: default
    depends_on: [root]
    config:
      items: [1, 2]
      actions:
        - action: log
          title: "Log item"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	s.Require().NoError(err)

	bus := event.NewBus()
	s.T().Cleanup(bus.Close)

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := engine.NewExecutor(reg, bus, logger, nil, nil, nil)

	srv := New(Config{
		Port:           0,
		Executor:       exec,
		Workflow:       wf,
		ExecutionStore: store.NewExecutionStore(10),
		EventBus:       bus,
		Logger:         logger,
	})

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	json.Unmarshal(w.Body.Bytes(), &graph)

	tree := graph["tree"].([]any)
	var foundPipeline bool
	for _, line := range tree {
		tl := line.(map[string]any)
		if tl["type"] == "pipeline" {
			foundPipeline = true
		}
	}
	s.True(foundPipeline, "deep loop should emit pipeline tree lines")
}

// TestBuildGraph_VisitedNodeSkipped covers buildGraph – DAG traversal skips already visited nodes.
func (s *HandlersTestSuite) TestBuildGraph_VisitedNodeSkipped() {
	srv := newTestServerConvergent(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	json.Unmarshal(w.Body.Bytes(), &graph)

	nodes := graph["nodes"].([]any)
	seen := make(map[string]bool)
	for _, n := range nodes {
		node := n.(map[string]any)
		id := node["id"].(string)
		s.False(seen[id], "node %q should appear only once", id)
		seen[id] = true
	}
}

// TestHandleListExecutions_EndBeyondTotal covers paginateExecutions – end > len(execs) clamp.
func (s *HandlersTestSuite) TestHandleListExecutions_EndBeyondTotal() {
	srv := newTestServer(s.T())

	for i := range 3 {
		srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
			ID: fmt.Sprintf("e-end-beyond-%d", i), WorkflowName: "test", Status: runtime.StatusSuccess,
			StartedAt: time.Now(),
		})
	}

	req := httptest.NewRequest("GET", "/api/executions?offset=0&limit=100", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal(float64(3), resp["total"])
}

// TestBuildGraph_NonSiblingConvergent covers resolveConvergent non-sibling parents path.
func (s *HandlersTestSuite) TestBuildGraph_NonSiblingConvergent() {
	srv := newTestServerNonSiblingConvergent(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &graph)
	s.Require().NoError(err)
	nodes := graph["nodes"].([]any)
	s.Len(nodes, 5)
}

// TestBuildGraph_ThreeWayConvergent covers the "┤" middle marker in resolveConvergent.
func (s *HandlersTestSuite) TestBuildGraph_ThreeWayConvergent() {
	srv := newTestServerThreeWayConvergent(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &graph)
	s.Require().NoError(err)
	nodes := graph["nodes"].([]any)
	s.Len(nodes, 5)

	tree := graph["tree"].([]any)
	var found中 bool
	for _, line := range tree {
		tl := line.(map[string]any)
		if merge, ok := tl["merge"].(string); ok && strings.Contains(merge, "┤") {
			found中 = true
		}
	}
	s.True(found中, "three-way convergent should produce ┤ merge marker")
}

// TestBuildGraph_DeepTreeNonLastParent covers treeContinuation loop body with parent.isLast=false.
func (s *HandlersTestSuite) TestBuildGraph_DeepTreeNonLastParent() {
	srv := newTestServerDeepTreeNonLastParent(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &graph)
	s.Require().NoError(err)

	tree := graph["tree"].([]any)
	var foundPipeline bool
	for _, line := range tree {
		tl := line.(map[string]any)
		if tl["type"] == "pipeline" {
			foundPipeline = true
		}
	}
	s.True(foundPipeline)
}

// TestBuildGraph_SingleChildLoop covers buildLoopBodies – chain returns to goto source.
func (s *HandlersTestSuite) TestBuildGraph_SingleChildLoop() {
	srv := newTestServerSingleChildLoop(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &graph)
	s.Require().NoError(err)
	nodes := graph["nodes"].([]any)
	s.Len(nodes, 2)
}

// TestBuildGraph_RootLoop covers treeContinuation at depth 0 for pipeline item.
func (s *HandlersTestSuite) TestBuildGraph_RootLoop() {
	srv := newTestServerRootLoop(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &graph)
	s.Require().NoError(err)

	tree := graph["tree"].([]any)
	var foundPipeline bool
	for _, line := range tree {
		tl := line.(map[string]any)
		if tl["type"] == "pipeline" {
			foundPipeline = true
		}
	}
	s.True(foundPipeline)
}

// TestBuildGraph_DeepLoopTree covers treeContinuation depth > 1 loop body.
func (s *HandlersTestSuite) TestBuildGraph_DeepLoopTree() {
	srv := newTestServerDeepLoopTree(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &graph)
	s.Require().NoError(err)

	tree := graph["tree"].([]any)
	var foundPipeline bool
	for _, line := range tree {
		tl := line.(map[string]any)
		if tl["type"] == "pipeline" {
			foundPipeline = true
		}
	}
	s.True(foundPipeline)
}

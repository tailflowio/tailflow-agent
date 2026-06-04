package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tailflow/tailflow/internal/parser"
)

// TestPutWorkflowRaw_ValidateFailsAfterParse covers handlePutWorkflowRaw –
// parser.ParseBytes succeeds but parser.Validate fails (version mismatch).
func (s *HandlersWorkflowRawTestSuite) TestPutWorkflowRaw_ValidateFailsAfterParse() {
	dir := s.T().TempDir()
	filePath := dir + "/workflow.yaml"
	srv := newTestServerRaw(s.T(), filePath, true)

	body := `version: "1.0"
name: "old-version"
stages:
  - name: default
steps:
  - id: greet
    action: log
    stage: default
    config:
      message: "hello"
`
	req := httptest.NewRequest("PUT", "/api/workflow/raw", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusBadRequest, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.False(resp["valid"].(bool))
	s.NotEmpty(resp["errors"])
}

// TestBuildGraph_ConvergentWithChildren covers resolveConvergent – collect closure
// walks descendants of the convergent node.
func (s *HandlersTestSuite) TestBuildGraph_ConvergentWithChildren() {
	srv := newTestServerConvergentWithChildren(s.T())

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

// newTestServerConvergentWithChildren creates a diamond workflow where the convergent
// node (step_end) has additional children.
func newTestServerConvergentWithChildren(t testing.TB) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "convergent-children"
stages:
  - name: default
steps:
  - id: step_start
    action: log
    stage: default
    config:
      message: "start"
  - id: step_a
    action: log
    stage: default
    depends_on: [step_start]
    config:
      message: "a"
  - id: step_b
    action: log
    stage: default
    depends_on: [step_start]
    config:
      message: "b"
  - id: step_end
    action: log
    stage: default
    depends_on: [step_a, step_b]
    config:
      message: "end"
  - id: step_after
    action: log
    stage: default
    depends_on: [step_end]
    config:
      message: "after"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	return newTestServerFromWorkflow(t, wf)
}

// TestBuildGraph_FourLevelDepth covers treePrefix and treeContinuation reversal loops
// at depth >= 3.
func (s *HandlersTestSuite) TestBuildGraph_FourLevelDepth() {
	srv := newTestServerFourLevels(s.T())

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

// newTestServerFourLevels creates root → level1 → level2 → level3_loop (pipeline step).
func newTestServerFourLevels(t testing.TB) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "four-levels"
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
  - id: level2
    action: log
    stage: default
    depends_on: [level1]
    config:
      message: "level2"
  - id: level3_loop
    action: loop
    stage: default
    depends_on: [level2]
    config:
      items: [1]
      actions:
        - action: log
          title: "Deep item"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	return newTestServerFromWorkflow(t, wf)
}

// TestBuildGraph_FourLevelDepthNonLastParent covers treeContinuation with a non-last
// parent at depth 2+.
func (s *HandlersTestSuite) TestBuildGraph_FourLevelDepthNonLastParent() {
	srv := newTestServerFourLevelsNonLast(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	json.Unmarshal(w.Body.Bytes(), &graph)

	nodes := graph["nodes"].([]any)
	s.Len(nodes, 5)
}

// newTestServerFourLevelsNonLast creates root → level1a,level1b → level2 → level3_loop.
func newTestServerFourLevelsNonLast(t testing.TB) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "four-levels-non-last"
stages:
  - name: default
steps:
  - id: root
    action: log
    stage: default
    config:
      message: "root"
  - id: level1a
    action: log
    stage: default
    depends_on: [root]
    config:
      message: "level1a"
  - id: level2
    action: log
    stage: default
    depends_on: [level1a]
    config:
      message: "level2"
  - id: level3_loop
    action: loop
    stage: default
    depends_on: [level2]
    config:
      items: [1]
      actions:
        - action: log
          title: "Deep item"
  - id: level1b
    action: log
    stage: default
    depends_on: [root]
    config:
      message: "level1b (last)"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	return newTestServerFromWorkflow(t, wf)
}

// TestBuildGraph_InsertIdxConvergent covers resolveConvergent insertIdx loop –
// walks past deeper siblings of lastParent, marking them non-last.
func (s *HandlersTestSuite) TestBuildGraph_InsertIdxConvergent() {
	srv := newTestServerInsertIdxConvergent(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var graph map[string]any
	json.Unmarshal(w.Body.Bytes(), &graph)

	nodes := graph["nodes"].([]any)
	s.Len(nodes, 5)
}

// newTestServerInsertIdxConvergent builds:
//
//	step_start → step_a, step_b
//	step_b → step_b_child
//	step_end depends on [step_a, step_b]
func newTestServerInsertIdxConvergent(t testing.TB) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "insert-idx-convergent"
stages:
  - name: default
steps:
  - id: step_start
    action: log
    stage: default
    config:
      message: "start"
  - id: step_a
    action: log
    stage: default
    depends_on: [step_start]
    config:
      message: "a"
  - id: step_b
    action: log
    stage: default
    depends_on: [step_start]
    config:
      message: "b"
  - id: step_b_child
    action: log
    stage: default
    depends_on: [step_b]
    config:
      message: "b child"
  - id: step_end
    action: log
    stage: default
    depends_on: [step_a, step_b]
    config:
      message: "end"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	return newTestServerFromWorkflow(t, wf)
}

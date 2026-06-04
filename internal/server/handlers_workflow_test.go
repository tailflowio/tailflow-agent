package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

func (s *HandlersTestSuite) TestGetWorkflow() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/workflow", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var wf map[string]any
	json.Unmarshal(w.Body.Bytes(), &wf)
	s.Equal("test-workflow", wf["name"])
}

func (s *HandlersTestSuite) TestGetWorkflowGraph() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var graph map[string]any
	json.Unmarshal(w.Body.Bytes(), &graph)
	nodes := graph["nodes"].([]any)
	s.Len(nodes, 1)
}

func (s *HandlersTestSuite) TestValidateWorkflow() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/workflow/validate", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.True(resp["valid"].(bool))
}

func (s *HandlersTestSuite) TestUI() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Body.String(), "TailFlow")
}

// TestGetMetrics_ReturnsSnapshot covers handleGetMetrics.
func (s *HandlersTestSuite) TestGetMetrics_ReturnsSnapshot() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Header().Get("Content-Type"), "application/json")
}

// TestGetWorkflow_WithCronTrigger covers handleGetWorkflow – cron trigger path (next_run).
func (s *HandlersTestSuite) TestGetWorkflow_WithCronTrigger() {
	srv := newTestServerCron(s.T())

	req := httptest.NewRequest("GET", "/api/workflow", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Equal("cron-workflow", resp["name"])
	s.NotNil(resp["next_run"], "should include next_run for cron trigger")
}

// TestValidateWorkflow_Invalid covers handleValidateWorkflow – invalid workflow.
func (s *HandlersTestSuite) TestValidateWorkflow_Invalid() {
	srv := newTestServer(s.T())
	srv.config.Workflow.Version = ""

	req := httptest.NewRequest("POST", "/api/workflow/validate", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.False(resp["valid"].(bool))
	s.NotEmpty(resp["errors"])
}

// TestGetWorkflowActivity_WithRunningAndWaitingSteps covers handleGetWorkflowActivity – running and waiting executions.
func (s *HandlersTestSuite) TestGetWorkflowActivity_WithRunningAndWaitingSteps() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID:           "exec-run-1",
		WorkflowName: "test-workflow",
		Status:       runtime.StatusRunning,
		StartedAt:    time.Now(),
		Steps: map[string]*runtime.StepResult{
			"greet": {Status: runtime.StatusRunning},
		},
	})

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID:           "exec-wait-1",
		WorkflowName: "test-workflow",
		Status:       runtime.StatusWaiting,
		StartedAt:    time.Now(),
		Steps: map[string]*runtime.StepResult{
			"greet": {Status: runtime.StatusWaiting},
		},
	})

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID:           "exec-done-1",
		WorkflowName: "test-workflow",
		Status:       runtime.StatusSuccess,
		StartedAt:    time.Now(),
		Steps: map[string]*runtime.StepResult{
			"greet": {Status: runtime.StatusSuccess},
		},
	})

	req := httptest.NewRequest("GET", "/api/workflow/activity", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)

	steps := resp["steps"].(map[string]any)
	greet := steps["greet"].(map[string]any)
	s.NotEmpty(greet["running"])
	s.NotEmpty(greet["waiting"])
}

// TestGetWorkflowActivity_Empty covers handleGetWorkflowActivity – no running/waiting execs.
func (s *HandlersTestSuite) TestGetWorkflowActivity_Empty() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/activity", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

// TestGetWorkflowActivity_RunningExecCompletedSteps covers handleGetWorkflowActivity – running exec with completed steps.
func (s *HandlersTestSuite) TestGetWorkflowActivity_RunningExecCompletedSteps() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID:           "exec-1",
		WorkflowName: "test-workflow",
		Status:       runtime.StatusRunning,
		StartedAt:    time.Now(),
		Steps: map[string]*runtime.StepResult{
			"greet": {Status: runtime.StatusSuccess},
		},
	})

	req := httptest.NewRequest("GET", "/api/workflow/activity", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	steps := resp["steps"].(map[string]any)
	s.Empty(steps)
}

// TestGetVersion_ReturnsVersion covers handleGetVersion – completely untested.
func (s *HandlersTestSuite) TestGetVersion_ReturnsVersion() {
	srv := newTestServer(s.T())
	srv.config.Version = "1.2.3"

	req := httptest.NewRequest("GET", "/api/version", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Header().Get("Content-Type"), "application/json")

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Equal("1.2.3", resp["version"])
}

// TestGetVersion_EmptyVersion covers handleGetVersion – empty version string.
func (s *HandlersTestSuite) TestGetVersion_EmptyVersion() {
	srv := newTestServer(s.T())
	srv.config.Version = ""

	req := httptest.NewRequest("GET", "/api/version", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Equal("", resp["version"])
}

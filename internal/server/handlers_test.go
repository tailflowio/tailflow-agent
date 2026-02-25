package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/store"
)

type HandlersTestSuite struct {
	suite.Suite
}

func TestHandlers(t *testing.T) {
	suite.Run(t, new(HandlersTestSuite))
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "test-workflow"
description: "A test workflow"
tags: ["test"]
params:
  - name: env
    type: string
    default: "staging"
steps:
  - id: greet
    action: log
    title: "Log greeting"
    config:
      message: "Hello from test"
`
	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	bus := event.NewBus()
	t.Cleanup(bus.Close)

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := engine.NewExecutor(reg, bus, logger)

	return New(Config{
		Port:           0,
		Executor:       exec,
		Workflow:       wf,
		ExecutionStore: store.NewExecutionStore(10),
		EventBus:       bus,
		Logger:         logger,
	})
}

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

func (s *HandlersTestSuite) TestListExecutions_Empty() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/executions", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

func (s *HandlersTestSuite) TestRunWorkflow_ReturnsExecutionID() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/workflow/run", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusAccepted, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.NotEmpty(resp["execution_id"], "execution_id should be returned")
	s.Equal("running", resp["status"])

	// Execution should be in the store immediately
	execID := resp["execution_id"].(string)
	exec, err := srv.config.ExecutionStore.Get(execID)
	s.Require().NoError(err)
	s.Equal("running", exec.Status)
	s.Equal("test-workflow", exec.WorkflowName)

	// Wait for async execution to finish
	s.Eventually(func() bool {
		exec, err := srv.config.ExecutionStore.Get(execID)
		return err == nil && exec.Status == "success"
	}, 2*time.Second, 10*time.Millisecond)
}

func (s *HandlersTestSuite) TestGetStepDetail() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/steps/greet", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	step := resp["step"].(map[string]any)
	s.Equal("greet", step["id"])
	s.Equal("log", step["action"])

	metrics := resp["metrics"].(map[string]any)
	s.Equal(float64(0), metrics["total_executions"])
}

func (s *HandlersTestSuite) TestGetStepDetail_NotFound() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/steps/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

func (s *HandlersTestSuite) TestUI() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Body.String(), "TailFlow")
}

func (s *HandlersTestSuite) TestWaitWebhook_NoRegistration() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/wait/test-exec-id/callback", strings.NewReader(`{"data":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

func (s *HandlersTestSuite) TestWaitWebhook_WithRegistration() {
	srv := newTestServer(s.T())

	// Register a wait
	ctx := s.T()
	_ = ctx
	ch, cleanup := srv.waitRegistry.Register("test-exec-id", "step-1", "/callback", nil)
	defer cleanup()

	// Deliver webhook in a goroutine so we can read from ch
	go func() {
		req := httptest.NewRequest("POST", "/api/wait/test-exec-id/callback", strings.NewReader(`{"data":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		s.Equal(http.StatusOK, w.Code)
	}()

	// Read from channel
	select {
	case wr := <-ch:
		s.Equal("POST", wr.Method)
		s.Equal("/callback", wr.Path)
	case <-time.After(2 * time.Second):
		s.T().Fatal("timeout waiting for wait request")
	}
}

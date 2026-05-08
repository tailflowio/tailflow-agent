package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/export/saas"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

type HandlersTestSuite struct {
	suite.Suite
}

func TestHandlers(t *testing.T) {
	suite.Run(t, new(HandlersTestSuite))
}

func (s *HandlersTestSuite) SetupTest() { // required by convention
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
stages:
  - name: default
steps:
  - id: greet
    action: log
    stage: default
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

func newTestServerCron(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "cron-workflow"
description: "A cron workflow"
trigger:
  schedule:
    cron: "*/5 * * * *"
stages:
  - name: default
steps:
  - id: greet
    action: log
    stage: default
    title: "Log greeting"
    config:
      message: "Hello from cron"
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

func newTestServerHTTPTrigger(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "http-workflow"
description: "HTTP triggered workflow"
trigger:
  http:
    method: POST
    path: /submit
stages:
  - name: default
steps:
  - id: echo
    action: log
    stage: default
    title: "Echo"
    config:
      message: "trigger received"
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

func newTestServerAsyncHTTPTrigger(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "async-http-workflow"
description: "Async HTTP triggered workflow"
trigger:
  http:
    method: POST
    path: /async-submit
    async: true
stages:
  - name: default
steps:
  - id: echo
    action: log
    stage: default
    title: "Echo"
    config:
      message: "async trigger received"
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

func newTestServerWebhookTrigger(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "webhook-workflow"
description: "Webhook triggered workflow"
trigger:
  webhook:
    path: /hook
stages:
  - name: default
steps:
  - id: echo
    action: log
    stage: default
    title: "Echo"
    config:
      message: "webhook received"
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

func newTestServerGraph(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "graph-workflow"
description: "Graph test"
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
    title: "Step B"
    depends_on: [step_a]
    when: 'steps.step_a.status == "success"'
    config:
      message: "b"
    goto:
      target: step_a
      when: 'steps.step_b.output.retry == true'
      max_iterations: 3
  - id: step_loop
    action: loop
    stage: default
    title: "Loop Step"
    depends_on: [step_a]
    config:
      items: [1, 2, 3]
      actions:
        - action: log
          title: "Log in loop"
        - action: set
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
	synctest.Test(s.T(), func(t *testing.T) {
		srv := newTestServer(t)

		ch, cleanup := srv.waitRegistry.Register("test-exec-id", "step-1", "/callback", nil)
		defer cleanup()

		go func() {
			req := httptest.NewRequest("POST", "/api/wait/test-exec-id/callback", strings.NewReader(`{"data":"test"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			s.Equal(http.StatusOK, w.Code)
		}()

		synctest.Wait()

		wr := <-ch
		s.Equal("POST", wr.Method)
		s.Equal("/callback", wr.Path)
	})
}

func (s *HandlersTestSuite) TestFinalizeExecution_NotFound() {
	srv := newTestServer(s.T())

	s.NotPanics(func() {
		srv.finalizeExecution("nonexistent-id", nil, nil, context.Background())
	})
}

// handleGetMetrics
func (s *HandlersTestSuite) TestGetMetrics_ReturnsSnapshot() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Header().Get("Content-Type"), "application/json")
}

// handleGetWorkflow – cron trigger path (next_run)
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

// handleValidateWorkflow – invalid workflow
func (s *HandlersTestSuite) TestValidateWorkflow_Invalid() {
	srv := newTestServer(s.T())
	// Break the workflow by clearing its version
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

// handleRunWorkflow – nil body
func (s *HandlersTestSuite) TestRunWorkflow_NilBody() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/workflow/run", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusAccepted, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.NotEmpty(resp["execution_id"])
}

// handleRunWorkflow – invalid JSON body (decode error path)
func (s *HandlersTestSuite) TestRunWorkflow_InvalidJSON() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/workflow/run", strings.NewReader(`not-json`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusAccepted, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.NotEmpty(resp["execution_id"])
}

// handleGetWorkflowActivity – running and waiting executions
func (s *HandlersTestSuite) TestGetWorkflowActivity_WithRunningAndWaitingSteps() {
	srv := newTestServer(s.T())

	// Add a running execution with a running step
	srv.config.ExecutionStore.Add(&store.Execution{
		ID:           "exec-run-1",
		WorkflowName: "test-workflow",
		Status:       runtime.StatusRunning,
		StartedAt:    time.Now(),
		Steps: map[string]*runtime.StepResult{
			"greet": {Status: runtime.StatusRunning},
		},
	})

	// Add a waiting execution with a waiting step
	srv.config.ExecutionStore.Add(&store.Execution{
		ID:           "exec-wait-1",
		WorkflowName: "test-workflow",
		Status:       runtime.StatusWaiting,
		StartedAt:    time.Now(),
		Steps: map[string]*runtime.StepResult{
			"greet": {Status: runtime.StatusWaiting},
		},
	})

	// Add a completed execution (should be skipped)
	srv.config.ExecutionStore.Add(&store.Execution{
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

// handleGetWorkflowActivity – no running/waiting execs
func (s *HandlersTestSuite) TestGetWorkflowActivity_Empty() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/activity", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

// handleGetWorkflowActivity – running exec with completed steps
func (s *HandlersTestSuite) TestGetWorkflowActivity_RunningExecCompletedSteps() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(&store.Execution{
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
	s.Empty(steps) // completed steps not counted
}

// buildStepHistory + buildHistoryEntry
func (s *HandlersTestSuite) TestGetStepDetail_WithHistory() {
	srv := newTestServer(s.T())

	startedAt := time.Now().Add(-2 * time.Second)
	finishedAt := time.Now()
	execFinished := time.Now()

	// Execution with step result that has all fields
	srv.config.ExecutionStore.Add(&store.Execution{
		ID:           "exec-hist-1",
		WorkflowName: "test-workflow",
		Status:       runtime.StatusSuccess,
		StartedAt:    startedAt,
		FinishedAt:   &execFinished,
		Steps: map[string]*runtime.StepResult{
			"greet": {
				Status:     runtime.StatusSuccess,
				Output:     map[string]any{"msg": "hello"},
				StartedAt:  &startedAt,
				FinishedAt: &finishedAt,
			},
		},
	})

	// Execution with step result that has error and no started_at
	srv.config.ExecutionStore.Add(&store.Execution{
		ID:           "exec-hist-2",
		WorkflowName: "test-workflow",
		Status:       runtime.StatusFailed,
		StartedAt:    startedAt,
		FinishedAt:   &execFinished,
		Steps: map[string]*runtime.StepResult{
			"greet": {
				Status: runtime.StatusFailed,
				Error:  &runtime.StepError{Message: "something went wrong"},
			},
		},
	})

	// Execution with step result that has startedAt but no finishedAt, and exec has finishedAt
	srv.config.ExecutionStore.Add(&store.Execution{
		ID:           "exec-hist-3",
		WorkflowName: "test-workflow",
		Status:       runtime.StatusFailed,
		StartedAt:    startedAt,
		FinishedAt:   &execFinished,
		Steps: map[string]*runtime.StepResult{
			"greet": {
				Status:    runtime.StatusRunning,
				StartedAt: &startedAt,
			},
		},
	})

	// Execution without the greet step (should be skipped)
	srv.config.ExecutionStore.Add(&store.Execution{
		ID:           "exec-hist-4",
		WorkflowName: "test-workflow",
		Status:       runtime.StatusSuccess,
		StartedAt:    startedAt,
		Steps: map[string]*runtime.StepResult{
			"other": {Status: runtime.StatusSuccess},
		},
	})

	req := httptest.NewRequest("GET", "/api/workflow/steps/greet", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)

	history := resp["history"].([]any)
	s.Len(history, 3) // 3 executions had step greet

	// history is newest-first: [exec-hist-3, exec-hist-2, exec-hist-1]
	// Verify entry with full timestamps (exec-hist-1 is last)
	entryFull := history[2].(map[string]any)
	s.NotEmpty(entryFull["started_at"])
	s.NotEmpty(entryFull["finished_at"])
	s.Greater(entryFull["duration_ms"].(float64), float64(0))

	// Verify entry with error (exec-hist-2)
	entryErr := history[1].(map[string]any)
	s.Equal("something went wrong", entryErr["error"])

	// Verify entry with startedAt but no step finishedAt (exec-hist-3)
	entryPartial := history[0].(map[string]any)
	s.NotEmpty(entryPartial["started_at"])
	s.NotEmpty(entryPartial["finished_at"]) // falls back to exec.FinishedAt
}

// filterByStatus
func (s *HandlersTestSuite) TestListExecutions_FilterByStatus() {
	srv := newTestServer(s.T())

	now := time.Now()
	finished := now.Add(1 * time.Second)

	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "e1", WorkflowName: "test", Status: runtime.StatusSuccess,
		StartedAt: now, FinishedAt: &finished,
	})
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "e2", WorkflowName: "test", Status: runtime.StatusFailed,
		StartedAt: now, FinishedAt: &finished,
	})
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "e3", WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: now,
	})

	// Filter by single status
	req := httptest.NewRequest("GET", "/api/executions?status=success", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal(float64(1), resp["total"])

	// Filter by multiple statuses
	req2 := httptest.NewRequest("GET", "/api/executions?status=success,failed", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	var resp2 map[string]any
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	s.Equal(float64(2), resp2["total"])
}

// sortExecutions with duration + asc/desc
func (s *HandlersTestSuite) TestListExecutions_SortByDuration() {
	srv := newTestServer(s.T())

	now := time.Now()
	shortFinish := now.Add(100 * time.Millisecond)
	longFinish := now.Add(5 * time.Second)

	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "short", WorkflowName: "test", Status: runtime.StatusSuccess,
		StartedAt: now, FinishedAt: &shortFinish,
	})
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "long", WorkflowName: "test", Status: runtime.StatusSuccess,
		StartedAt: now, FinishedAt: &longFinish,
	})
	// Running execution (no FinishedAt) – duration is 0
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "running", WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: now,
	})

	// Sort by duration (desc by default)
	req := httptest.NewRequest("GET", "/api/executions?sort=duration", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	items := resp["items"].([]any)
	s.Len(items, 3)
	// First should be the longest duration
	first := items[0].(map[string]any)
	s.Equal("long", first["id"])

	// Sort by duration asc
	req2 := httptest.NewRequest("GET", "/api/executions?sort=duration&order=asc", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	var resp2 map[string]any
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	items2 := resp2["items"].([]any)
	lastItem := items2[len(items2)-1].(map[string]any)
	s.Equal("long", lastItem["id"])
}

// sortExecutions – order=asc without specific sort (default date order reversed)
func (s *HandlersTestSuite) TestListExecutions_OrderAsc() {
	srv := newTestServer(s.T())

	now := time.Now()
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "first", WorkflowName: "test", Status: runtime.StatusSuccess, StartedAt: now,
	})
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "second", WorkflowName: "test", Status: runtime.StatusSuccess, StartedAt: now.Add(1 * time.Second),
	})

	req := httptest.NewRequest("GET", "/api/executions?order=asc", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	items := resp["items"].([]any)
	// Default order is newest-first (desc), asc reverses it
	firstItem := items[0].(map[string]any)
	s.Equal("first", firstItem["id"])
}

// paginateExecutions – offset beyond items count
func (s *HandlersTestSuite) TestListExecutions_OffsetBeyondTotal() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "e1", WorkflowName: "test", Status: runtime.StatusSuccess, StartedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/executions?offset=100", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	items := resp["items"].([]any)
	s.Len(items, 0)
}

// parseIntParam – invalid and negative values
func (s *HandlersTestSuite) TestListExecutions_InvalidPaginationParams() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "e1", WorkflowName: "test", Status: runtime.StatusSuccess, StartedAt: time.Now(),
	})

	// Invalid (non-numeric) offset → falls back to default
	req := httptest.NewRequest("GET", "/api/executions?offset=abc&limit=xyz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	items := resp["items"].([]any)
	s.Len(items, 1) // defaults applied

	// Negative offset and limit → falls back to default
	req2 := httptest.NewRequest("GET", "/api/executions?offset=-5&limit=-1", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	s.Equal(http.StatusOK, w2.Code)
}

// handleGetExecution
func (s *HandlersTestSuite) TestGetExecution_Found() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "exec-123", WorkflowName: "test-workflow", Status: runtime.StatusSuccess,
		StartedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/executions/exec-123", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal("exec-123", resp["id"])
}

func (s *HandlersTestSuite) TestGetExecution_NotFound() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/executions/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

// handleCancelExecution
func (s *HandlersTestSuite) TestCancelExecution_NotFound() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/executions/nonexistent/cancel", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

func (s *HandlersTestSuite) TestCancelExecution_NotRunning() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "exec-done", WorkflowName: "test-workflow", Status: runtime.StatusSuccess,
		StartedAt: time.Now(),
	})

	req := httptest.NewRequest("POST", "/api/executions/exec-done/cancel", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusConflict, w.Code)
}

func (s *HandlersTestSuite) TestCancelExecution_Success() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "exec-cancel", WorkflowName: "test-workflow", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	// Register a cancel function
	srv.registerCancel("exec-cancel", func() {})

	req := httptest.NewRequest("POST", "/api/executions/exec-cancel/cancel", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.True(resp["cancelled"].(bool))
}

func (s *HandlersTestSuite) TestCancelExecution_WaitingStatus() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "exec-wait-cancel", WorkflowName: "test-workflow", Status: runtime.StatusWaiting,
		StartedAt: time.Now(),
	})

	srv.registerCancel("exec-wait-cancel", func() {})

	req := httptest.NewRequest("POST", "/api/executions/exec-wait-cancel/cancel", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

func (s *HandlersTestSuite) TestCancelExecution_NoCancelFunc() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(&store.Execution{
		ID: "exec-no-cancel", WorkflowName: "test-workflow", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	// No cancel function registered
	req := httptest.NewRequest("POST", "/api/executions/exec-no-cancel/cancel", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Contains(resp["error"], "cancel function not found")
}

// handlePublicTrigger – no trigger configured
func (s *HandlersTestSuite) TestPublicTrigger_NoTrigger() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/public/anything", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

// handlePublicTrigger – HTTP trigger, sync
func (s *HandlersTestSuite) TestPublicTrigger_HTTPSync() {
	srv := newTestServerHTTPTrigger(s.T())

	req := httptest.NewRequest("POST", "/api/public/submit", strings.NewReader(`{"data":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.NotEmpty(resp["execution_id"])
	s.Equal("success", resp["status"])
}

// handlePublicTrigger – HTTP trigger, async
func (s *HandlersTestSuite) TestPublicTrigger_HTTPAsync() {
	srv := newTestServerAsyncHTTPTrigger(s.T())

	req := httptest.NewRequest("POST", "/api/public/async-submit", strings.NewReader(`{"data":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusAccepted, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Equal("running", resp["status"])

	// Wait for async execution to finish
	execID := resp["execution_id"].(string)
	s.Eventually(func() bool {
		exec, getErr := srv.config.ExecutionStore.Get(execID)
		return getErr == nil && (exec.Status == "success" || exec.Status == "failed")
	}, 2*time.Second, 10*time.Millisecond)
}

// handlePublicTrigger – webhook trigger
func (s *HandlersTestSuite) TestPublicTrigger_Webhook() {
	srv := newTestServerWebhookTrigger(s.T())

	// Webhook matches any method on the path
	req := httptest.NewRequest("POST", "/api/public/hook", strings.NewReader(`{"event":"push"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

// buildTriggerData – nil body
func (s *HandlersTestSuite) TestPublicTrigger_NilBody() {
	srv := newTestServerHTTPTrigger(s.T())

	req := httptest.NewRequest("POST", "/api/public/submit", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

// writeTriggerResponse – error path
func (s *HandlersTestSuite) TestWriteTriggerResponse_WithError() {
	srv := newTestServer(s.T())

	w := httptest.NewRecorder()
	srv.writeTriggerResponse(w, srv.config.Workflow, nil, errors.New("exec failed"), context.Background(), "exec-1")

	s.Equal(http.StatusInternalServerError, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal("exec failed", resp["error"])
}

// writeTriggerResponse – cancelled context
func (s *HandlersTestSuite) TestWriteTriggerResponse_Cancelled() {
	srv := newTestServer(s.T())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := httptest.NewRecorder()
	srv.writeTriggerResponse(w, srv.config.Workflow, nil, errors.New("cancelled"), ctx, "exec-2")

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal("cancelled", resp["status"])
	s.Equal("exec-2", resp["execution_id"])
}

// writeTriggerResponse – response action with headers
func (s *HandlersTestSuite) TestWriteTriggerResponse_WithResponseAction() {
	srv := newTestServer(s.T())

	// Add a response step to the workflow
	srv.config.Workflow.Steps = append(srv.config.Workflow.Steps, parser.Step{
		ID:     "respond",
		Action: "response",
	})

	result := &engine.ExecuteResult{
		Status: runtime.StatusSuccess,
		Steps: map[string]*runtime.StepResult{
			"respond": {
				Status: runtime.StatusSuccess,
				Output: map[string]any{
					"status":  200,
					"body":    map[string]any{"message": "ok"},
					"headers": map[string]string{"X-Custom": "custom-value"},
				},
			},
		},
	}

	w := httptest.NewRecorder()
	srv.writeTriggerResponse(w, srv.config.Workflow, result, nil, context.Background(), "exec-3")

	s.Equal(http.StatusOK, w.Code)
	s.Equal("custom-value", w.Header().Get("X-Custom"))
}

// writeTriggerResponse – response action with non-int status (falls back to 200)
func (s *HandlersTestSuite) TestWriteTriggerResponse_ResponseActionDefaultStatus() {
	srv := newTestServer(s.T())

	srv.config.Workflow.Steps = append(srv.config.Workflow.Steps, parser.Step{
		ID:     "respond2",
		Action: "response",
	})

	result := &engine.ExecuteResult{
		Status: runtime.StatusSuccess,
		Steps: map[string]*runtime.StepResult{
			"respond2": {
				Status: runtime.StatusSuccess,
				Output: map[string]any{
					"body": "plain response",
				},
			},
		},
	}

	w := httptest.NewRecorder()
	srv.writeTriggerResponse(w, srv.config.Workflow, result, nil, context.Background(), "exec-4")

	s.Equal(http.StatusOK, w.Code)
}

// writeTriggerResponse – response action where output is not map
func (s *HandlersTestSuite) TestWriteTriggerResponse_ResponseActionNonMapOutput() {
	srv := newTestServer(s.T())

	srv.config.Workflow.Steps = []parser.Step{
		{ID: "greet", Action: "log"},
		{ID: "respond3", Action: "response"},
	}

	result := &engine.ExecuteResult{
		Status: runtime.StatusSuccess,
		Steps: map[string]*runtime.StepResult{
			"respond3": {
				Status: runtime.StatusSuccess,
				Output: "not a map",
			},
		},
	}

	w := httptest.NewRecorder()
	srv.writeTriggerResponse(w, srv.config.Workflow, result, nil, context.Background(), "exec-5")

	// Falls through to the default response since output is not map[string]any
	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal("success", resp["status"])
}

// writeTriggerResponse – response action with nil output
func (s *HandlersTestSuite) TestWriteTriggerResponse_ResponseActionNilOutput() {
	srv := newTestServer(s.T())

	srv.config.Workflow.Steps = []parser.Step{
		{ID: "respond_nil", Action: "response"},
	}

	result := &engine.ExecuteResult{
		Status: runtime.StatusSuccess,
		Steps: map[string]*runtime.StepResult{
			"respond_nil": {
				Status: runtime.StatusSuccess,
				Output: nil,
			},
		},
	}

	w := httptest.NewRecorder()
	srv.writeTriggerResponse(w, srv.config.Workflow, result, nil, context.Background(), "exec-nil")

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal("success", resp["status"])
}

// handleWaitWebhook – missing path
func (s *HandlersTestSuite) TestWaitWebhook_BadPath() {
	srv := newTestServer(s.T())

	// Only executionID, no path segment
	req := httptest.NewRequest("POST", "/api/wait/test-exec-id", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusBadRequest, w.Code)
}

// handleWaitWebhook – nil body
func (s *HandlersTestSuite) TestWaitWebhook_NilBody() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/wait/test-exec-id/callback", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// No registration, so 404
	s.Equal(http.StatusNotFound, w.Code)
}

// buildGraphNode + extractLoopPipeline + buildStepEdges
func (s *HandlersTestSuite) TestGetWorkflowGraph_ComplexDAG() {
	srv := newTestServerGraph(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var graph map[string]any
	json.Unmarshal(w.Body.Bytes(), &graph)

	nodes := graph["nodes"].([]any)
	s.Len(nodes, 3)

	// step_a should use ID as label (no title)
	for _, n := range nodes {
		node := n.(map[string]any)
		if node["id"] == "step_a" {
			s.Equal("step_a", node["label"])
		}
		if node["id"] == "step_loop" {
			s.Equal("loop", node["action"])
			pipeline := node["pipeline"].([]any)
			s.Len(pipeline, 2)
		}
	}

	edges := graph["edges"].([]any)
	s.GreaterOrEqual(len(edges), 3) // depends_on edges + goto edge

	// Look for goto edge and when edge
	var foundGoto, foundWhen bool
	for _, e := range edges {
		edge := e.(map[string]any)
		if edge["type"] == "goto" {
			foundGoto = true
			s.Equal("step_b", edge["source"])
			s.Equal("step_a", edge["target"])
		}
		if edge["type"] == "when" {
			foundWhen = true
		}
	}
	s.True(foundGoto, "should have a goto edge")
	s.True(foundWhen, "should have a when edge")
}

// extractLoopPipeline – no actions key
func (s *HandlersTestSuite) TestExtractLoopPipeline_NoActions() {
	result := extractLoopPipeline(map[string]any{})
	s.Nil(result)
}

// extractLoopPipeline – actions is not []any
func (s *HandlersTestSuite) TestExtractLoopPipeline_NotArray() {
	result := extractLoopPipeline(map[string]any{"actions": "not-an-array"})
	s.Nil(result)
}

// extractLoopPipeline – actions contains non-map items
func (s *HandlersTestSuite) TestExtractLoopPipeline_NonMapItem() {
	result := extractLoopPipeline(map[string]any{
		"actions": []any{"not-a-map", 42},
	})
	s.Nil(result)
}

// extractLoopPipeline – action without name (empty string)
func (s *HandlersTestSuite) TestExtractLoopPipeline_EmptyActionName() {
	result := extractLoopPipeline(map[string]any{
		"actions": []any{
			map[string]any{"action": "", "title": "no action"},
		},
	})
	s.Nil(result) // empty action name is skipped
}

// buildActionServices – test EmitWaiting and ScheduleExecution
func (s *HandlersTestSuite) TestBuildActionServices_EmitWaiting() {
	srv := newTestServer(s.T())

	services := srv.buildActionServices()
	s.NotNil(services)
	s.NotNil(services.WaitWebhookRegister)
	s.NotNil(services.WaitRabbitMQRegister)
	s.NotNil(services.EmitWaiting)
	s.NotNil(services.ScheduleExecution)
	s.NotNil(services.Locker)
	s.NotNil(services.DBPool)
	s.NotNil(services.KVStore)
	s.NotNil(services.TxRegistry)

	// Test EmitWaiting: subscribe to bus, emit a waiting event, verify it's published
	ch := srv.config.EventBus.Subscribe(10)
	defer srv.config.EventBus.Unsubscribe(ch)

	services.EmitWaiting("exec-1", "step-1", "webhook", map[string]any{"path": "/cb"})

	select {
	case ev := <-ch:
		s.Equal(event.StepWaiting, ev.Type)
		s.Equal("exec-1", ev.ExecutionID)
		s.Equal("step-1", ev.StepID)
		s.Equal("webhook", ev.Data["wait_type"])
		s.Equal("/cb", ev.Data["path"])
	case <-time.After(time.Second):
		s.Fail("timeout waiting for event")
	}
}

func (s *HandlersTestSuite) TestBuildActionServices_ScheduleExecution() {
	srv := newTestServer(s.T())

	services := srv.buildActionServices()

	execID, err := services.ScheduleExecution(50*time.Millisecond, map[string]any{"key": "val"})
	s.NoError(err)
	s.NotEmpty(execID)

	// Verify the scheduled execution was added to the store
	exec, getErr := srv.config.ExecutionStore.Get(execID)
	s.NoError(getErr)
	s.Equal(runtime.StatusScheduled, exec.Status)

	// Wait for scheduled timer to fire
	s.Eventually(func() bool {
		// At least 2 executions in store: the scheduled one and the one triggered by the timer
		return srv.config.ExecutionStore.Count() >= 2
	}, 2*time.Second, 50*time.Millisecond)
}

// applyStepEvent – all event types
func (s *HandlersTestSuite) TestApplyStepEvent_AllTypes() {
	srv := newTestServer(s.T())

	execID := "apply-test"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	// StepStarted
	srv.applyStepEvent(execID, event.Event{
		Type: event.StepStarted, StepID: "s1",
	})
	exec, _ := srv.config.ExecutionStore.Get(execID)
	s.Equal(runtime.StatusRunning, exec.Steps["s1"].Status)

	// StepWaiting
	srv.applyStepEvent(execID, event.Event{
		Type: event.StepWaiting, StepID: "s1",
	})
	exec, _ = srv.config.ExecutionStore.Get(execID)
	s.Equal(runtime.StatusWaiting, exec.Steps["s1"].Status)

	// StepInput
	srv.applyStepEvent(execID, event.Event{
		Type: event.StepInput, StepID: "s1",
		Data: map[string]any{"input_key": "input_val"},
	})
	exec, _ = srv.config.ExecutionStore.Get(execID)
	s.NotNil(exec.Steps["s1"].Input)

	// StepCompleted
	srv.applyStepEvent(execID, event.Event{
		Type: event.StepCompleted, StepID: "s1",
		Data: map[string]any{"output": map[string]any{"result": 42}},
	})
	exec, _ = srv.config.ExecutionStore.Get(execID)
	s.Equal(runtime.StatusSuccess, exec.Steps["s1"].Status)
	s.NotNil(exec.Steps["s1"].Output)

	// StepFailed
	srv.applyStepEvent(execID, event.Event{
		Type: event.StepFailed, StepID: "s2", Message: "boom",
	})
	exec, _ = srv.config.ExecutionStore.Get(execID)
	s.Equal(runtime.StatusFailed, exec.Steps["s2"].Status)
	s.Equal("boom", exec.Steps["s2"].Error.Message)

	// StepSkipped
	srv.applyStepEvent(execID, event.Event{
		Type: event.StepSkipped, StepID: "s3",
	})
	exec, _ = srv.config.ExecutionStore.Get(execID)
	s.Equal(runtime.StatusSkipped, exec.Steps["s3"].Status)

	// StepOutput
	srv.applyStepEvent(execID, event.Event{
		Type: event.StepOutput, StepID: "s1",
		Data: map[string]any{"output": "new-output"},
	})
	exec, _ = srv.config.ExecutionStore.Get(execID)
	s.Equal("new-output", exec.Steps["s1"].Output)

	// No-op events (should not crash)
	srv.applyStepEvent(execID, event.Event{Type: event.Metrics, StepID: "s1"})
	srv.applyStepEvent(execID, event.Event{Type: event.WorkflowStarted, StepID: "s1"})
	srv.applyStepEvent(execID, event.Event{Type: event.StepLog, StepID: "s1"})
	srv.applyStepEvent(execID, event.Event{Type: event.StepGoto, StepID: "s1"})
}

// applyWorkflowCompleted
func (s *HandlersTestSuite) TestApplyWorkflowCompleted_ValidStatus() {
	srv := newTestServer(s.T())

	execID := "wfc-1"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	ts := time.Now()
	srv.applyWorkflowCompleted(execID, event.Event{
		Type:      event.WorkflowCompleted,
		Timestamp: ts,
		Data:      map[string]any{"status": "success"},
	})

	exec, _ := srv.config.ExecutionStore.Get(execID)
	s.Equal(runtime.StatusSuccess, exec.Status)
	s.NotNil(exec.FinishedAt)
}

func (s *HandlersTestSuite) TestApplyWorkflowCompleted_NoStatus() {
	srv := newTestServer(s.T())

	execID := "wfc-2"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	// Data without "status" key
	srv.applyWorkflowCompleted(execID, event.Event{
		Type: event.WorkflowCompleted,
		Data: map[string]any{},
	})

	exec, _ := srv.config.ExecutionStore.Get(execID)
	s.Equal(runtime.StatusRunning, exec.Status) // unchanged
}

// applyStepEvent – WorkflowCompleted dispatches to applyWorkflowCompleted
func (s *HandlersTestSuite) TestApplyStepEvent_WorkflowCompleted() {
	srv := newTestServer(s.T())

	execID := "wfc-dispatch"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	ts := time.Now()
	srv.applyStepEvent(execID, event.Event{
		Type:      event.WorkflowCompleted,
		StepID:    "s1",
		Timestamp: ts,
		Data:      map[string]any{"status": "failed"},
	})

	exec, _ := srv.config.ExecutionStore.Get(execID)
	s.Equal(runtime.StatusFailed, exec.Status)
}

// processEvent – StepGoto with body resets pending
func (s *HandlersTestSuite) TestProcessEvent_StepGotoResetsBody() {
	srv := newTestServer(s.T())

	execID := "pe-goto"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
		Steps: map[string]*runtime.StepResult{
			"step_b": {Status: runtime.StatusSuccess},
		},
	})

	lt := &loopTracker{}

	// First: StepGoto event with body
	completedSeen := false
	srv.processEvent(execID, event.Event{
		Type:        event.StepGoto,
		ExecutionID: execID,
		StepID:      "step_a",
		Data: map[string]any{
			"iteration": float64(1),
			"body":      []any{"step_b"},
		},
	}, lt, &completedSeen)

	// Now lt.Body is set; issue another StepGoto
	srv.processEvent(execID, event.Event{
		Type:        event.StepGoto,
		ExecutionID: execID,
		StepID:      "step_a",
		Data: map[string]any{
			"iteration": float64(2),
			"body":      []any{"step_b"},
		},
	}, lt, &completedSeen)

	exec, _ := srv.config.ExecutionStore.Get(execID)
	s.Equal("pending", exec.Steps["step_b"].Status)
}

// processEvent – event with empty StepID (no applyStepEvent call)
func (s *HandlersTestSuite) TestProcessEvent_EmptyStepID() {
	srv := newTestServer(s.T())

	execID := "pe-empty"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	lt := &loopTracker{}
	completedSeen := false

	// Event with empty StepID: should not crash and should not call applyStepEvent
	srv.processEvent(execID, event.Event{
		Type:        event.WorkflowStarted,
		ExecutionID: execID,
	}, lt, &completedSeen)
}

// processEvent – loop body event after iteration 1 is skipped from event store
func (s *HandlersTestSuite) TestProcessEvent_LoopBodyEventSkipped() {
	srv := newTestServer(s.T())

	execID := "pe-loop"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	lt := &loopTracker{}
	completedSeen := false

	// Setup: first goto with iteration=2 and body
	lt.Track(event.Event{
		Type: event.StepGoto,
		Data: map[string]any{
			"iteration": float64(2),
			"body":      []any{"step_b"},
		},
	})

	// Now a step_b event at iteration 2 should be "in loop"
	srv.processEvent(execID, event.Event{
		Type:        event.StepStarted,
		ExecutionID: execID,
		StepID:      "step_b",
	}, lt, &completedSeen)

	// Event should NOT be appended (loop body after iteration 1)
	events := srv.config.ExecutionStore.GetEvents(execID)
	s.Len(events, 0)
}

// captureEvents – events for a different executionID are ignored
func (s *HandlersTestSuite) TestCaptureEvents_IgnoresOtherExecution() {
	srv := newTestServer(s.T())

	execID := "cap-1"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	stopCapture := srv.captureEvents(execID)

	// Publish event for a different execution
	srv.config.EventBus.Publish(event.Event{
		Type:        event.StepStarted,
		ExecutionID: "other-exec",
		StepID:      "s1",
	})

	// Give it time to process
	time.Sleep(50 * time.Millisecond)

	stopCapture()

	// No events should be stored for execID
	events := srv.config.ExecutionStore.GetEvents(execID)
	s.Len(events, 0)
}

// finalizeExecution – error with cancelled context
func (s *HandlersTestSuite) TestFinalizeExecution_ErrorCancelled() {
	srv := newTestServer(s.T())

	execID := "fin-cancel"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	srv.finalizeExecution(execID, nil, errors.New("context cancelled"), ctx)

	exec, _ := srv.config.ExecutionStore.Get(execID)
	s.Equal(runtime.StatusCancelled, exec.Status)
	s.Equal("execution cancelled", exec.Error)
	s.NotNil(exec.FinishedAt)
}

// finalizeExecution – error without cancelled context
func (s *HandlersTestSuite) TestFinalizeExecution_ErrorNotCancelled() {
	srv := newTestServer(s.T())

	execID := "fin-err"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	srv.finalizeExecution(execID, nil, errors.New("something failed"), context.Background())

	exec, _ := srv.config.ExecutionStore.Get(execID)
	s.Equal(runtime.StatusFailed, exec.Status)
	s.Equal("something failed", exec.Error)
	s.NotNil(exec.FinishedAt)
}

// finalizeExecution – success with result error
func (s *HandlersTestSuite) TestFinalizeExecution_SuccessWithResultError() {
	srv := newTestServer(s.T())

	execID := "fin-res-err"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	finishedAt := time.Now()
	result := &engine.ExecuteResult{
		Status:     runtime.StatusCompletedWithErrors,
		FinishedAt: finishedAt,
		Error:      fmt.Errorf("partial failure"),
		Steps: map[string]*runtime.StepResult{
			"s1": {Status: runtime.StatusSuccess, Output: "ok"},
		},
	}

	srv.finalizeExecution(execID, result, nil, context.Background())

	exec, _ := srv.config.ExecutionStore.Get(execID)
	s.Equal(runtime.StatusCompletedWithErrors, exec.Status)
	s.Equal("partial failure", exec.Error)
	s.NotNil(exec.FinishedAt)
	s.NotNil(exec.Steps["s1"])
}

// finalizeExecution – success with steps merge
func (s *HandlersTestSuite) TestFinalizeExecution_SuccessWithSteps() {
	srv := newTestServer(s.T())

	execID := "fin-ok"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
		Steps: map[string]*runtime.StepResult{
			"s1": {Status: runtime.StatusRunning, Input: "input-data"},
		},
	})

	finishedAt := time.Now()
	started := time.Now().Add(-1 * time.Second)
	result := &engine.ExecuteResult{
		Status:     runtime.StatusSuccess,
		FinishedAt: finishedAt,
		Steps: map[string]*runtime.StepResult{
			"s1": {Status: runtime.StatusSuccess, Output: "result", StartedAt: &started, FinishedAt: &finishedAt},
			"s2": {Status: runtime.StatusSuccess, Output: "new-step"},
		},
	}

	srv.finalizeExecution(execID, result, nil, context.Background())

	exec, _ := srv.config.ExecutionStore.Get(execID)
	s.Equal(runtime.StatusSuccess, exec.Status)
	// s1 should have its Input preserved from event tracking
	s.Equal("input-data", exec.Steps["s1"].Input)
	s.Equal("result", exec.Steps["s1"].Output)
	// s2 added from engine results
	s.Equal("new-step", exec.Steps["s2"].Output)
}

// mergeStepResults – nil engineSteps
func (s *HandlersTestSuite) TestMergeStepResults_NilEngineSteps() {
	exec := &store.Execution{
		Steps: map[string]*runtime.StepResult{
			"s1": {Status: runtime.StatusRunning},
		},
	}

	mergeStepResults(exec, nil)
	// Steps should remain unchanged
	s.Equal(runtime.StatusRunning, exec.Steps["s1"].Status)
}

// mergeStepResults – nil exec.Steps
func (s *HandlersTestSuite) TestMergeStepResults_NilExecSteps() {
	exec := &store.Execution{}

	engineSteps := map[string]*runtime.StepResult{
		"s1": {Status: runtime.StatusSuccess, Output: "hello"},
	}

	mergeStepResults(exec, engineSteps)
	s.Equal(engineSteps, exec.Steps)
}

// mergeStepResults – merge with existing
func (s *HandlersTestSuite) TestMergeStepResults_MergeExisting() {
	exec := &store.Execution{
		Steps: map[string]*runtime.StepResult{
			"s1": {Status: runtime.StatusRunning, Input: "preserved"},
		},
	}

	started := time.Now()
	finished := started.Add(1 * time.Second)
	engineSteps := map[string]*runtime.StepResult{
		"s1": {Status: runtime.StatusSuccess, Output: "out", StartedAt: &started, FinishedAt: &finished},
		"s2": {Status: runtime.StatusSuccess},
	}

	mergeStepResults(exec, engineSteps)
	s.Equal(runtime.StatusSuccess, exec.Steps["s1"].Status)
	s.Equal("out", exec.Steps["s1"].Output)
	s.Equal("preserved", exec.Steps["s1"].Input) // Input kept from tracking
	s.NotNil(exec.Steps["s2"])
}

// writeJSON error path – use a broken ResponseWriter
type brokenResponseWriter struct {
	header http.Header
}

func (b *brokenResponseWriter) Header() http.Header       { return b.header }
func (b *brokenResponseWriter) WriteHeader(statusCode int) {}
func (b *brokenResponseWriter) Write(data []byte) (int, error) {
	return 0, fmt.Errorf("write failed")
}

func (s *HandlersTestSuite) TestWriteJSON_EncodingError() {
	w := &brokenResponseWriter{header: http.Header{}}
	srv := newTestServer(s.T())

	// Should not panic; the error is logged
	s.NotPanics(func() {
		srv.writeJSON(context.Background(), w, http.StatusOK, map[string]any{"key": "val"})
	})
}

// handleGetWorkflowGraph – error from BuildDAG (broken depends_on)
func (s *HandlersTestSuite) TestGetWorkflowGraph_BuildDAGError() {
	srv := newTestServer(s.T())

	// Add a step with invalid depends_on to trigger BuildDAG error
	srv.config.Workflow.Steps = append(srv.config.Workflow.Steps, parser.Step{
		ID:        "broken",
		Action:    "log",
		DependsOn: []string{"nonexistent_step"},
	})

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusInternalServerError, w.Code)
}

// buildStepEdges – step with no depends_on and no goto
func (s *HandlersTestSuite) TestBuildStepEdges_NoDeps() {
	edges := buildStepEdges(parser.Step{ID: "solo", Action: "log"})
	s.Len(edges, 0)
}

// buildStepEdges – step with depends_on but no when condition
func (s *HandlersTestSuite) TestBuildStepEdges_DepsNoWhen() {
	edges := buildStepEdges(parser.Step{
		ID:        "child",
		Action:    "log",
		DependsOn: []string{"parent"},
	})
	s.Len(edges, 1)
	s.Equal("parent", edges[0].Source)
	s.Equal("child", edges[0].Target)
	s.Empty(edges[0].Type)
}

// buildStepEdges – step with goto
func (s *HandlersTestSuite) TestBuildStepEdges_WithGoto() {
	edges := buildStepEdges(parser.Step{
		ID:     "jumper",
		Action: "log",
		Goto: &parser.GotoConfig{
			Target: "target_step",
			When:   "some condition",
		},
	})
	s.Len(edges, 1)
	s.Equal("goto", edges[0].Type)
	s.Equal("jumper", edges[0].Source)
	s.Equal("target_step", edges[0].Target)
	s.Equal("some condition", edges[0].Label)
}

// buildGraphNode – loop action with pipeline
func (s *HandlersTestSuite) TestBuildGraphNode_LoopAction() {
	node := buildGraphNode(parser.Step{
		ID:     "my_loop",
		Action: "loop",
		Title:  "My Loop",
		Config: map[string]any{
			"actions": []any{
				map[string]any{"action": "http", "title": "Call API"},
				map[string]any{"action": "log"},
			},
		},
	})

	s.Equal("My Loop", node.Label)
	s.Len(node.Pipeline, 2)
	s.Equal("http", node.Pipeline[0].Action)
	s.Equal("Call API", node.Pipeline[0].Title)
}

// buildGraphNode – non-loop action
func (s *HandlersTestSuite) TestBuildGraphNode_NonLoop() {
	node := buildGraphNode(parser.Step{
		ID:     "step1",
		Action: "http",
		Title:  "HTTP Step",
	})

	s.Equal("HTTP Step", node.Label)
	s.Nil(node.Pipeline)
}

// execDuration with finished execution
func (s *HandlersTestSuite) TestExecDuration_WithFinishedAt() {
	now := time.Now()
	later := now.Add(5 * time.Second)
	exec := &store.Execution{StartedAt: now, FinishedAt: &later}

	d := execDuration(exec)
	s.Equal(5*time.Second, d)
}

// execDuration without finished execution
func (s *HandlersTestSuite) TestExecDuration_NilFinishedAt() {
	exec := &store.Execution{StartedAt: time.Now()}

	d := execDuration(exec)
	s.Equal(time.Duration(0), d)
}

// parseIntParam – valid value
func (s *HandlersTestSuite) TestParseIntParam_ValidValue() {
	r := httptest.NewRequest("GET", "/test?offset=10", nil)
	v := parseIntParam(r, "offset", 0)
	s.Equal(10, v)
}

// parseIntParam – empty (default)
func (s *HandlersTestSuite) TestParseIntParam_Empty() {
	r := httptest.NewRequest("GET", "/test", nil)
	v := parseIntParam(r, "offset", 5)
	s.Equal(5, v)
}

// parseIntParam – invalid
func (s *HandlersTestSuite) TestParseIntParam_Invalid() {
	r := httptest.NewRequest("GET", "/test?offset=abc", nil)
	v := parseIntParam(r, "offset", 7)
	s.Equal(7, v)
}

// parseIntParam – negative
func (s *HandlersTestSuite) TestParseIntParam_Negative() {
	r := httptest.NewRequest("GET", "/test?offset=-3", nil)
	v := parseIntParam(r, "offset", 0)
	s.Equal(0, v)
}

// buildHistoryEntry – step result with no StartedAt and no exec FinishedAt
func (s *HandlersTestSuite) TestBuildHistoryEntry_NoTimestamps() {
	exec := &store.Execution{
		ID:        "e1",
		StartedAt: time.Now(),
	}
	sr := &runtime.StepResult{
		Status: runtime.StatusRunning,
	}

	entry := buildHistoryEntry(exec, sr)
	s.Equal(runtime.StatusRunning, entry.Status)
	s.NotEmpty(entry.StartedAt) // falls back to exec.StartedAt
	s.Empty(entry.FinishedAt)
}

// buildHistoryEntry – step with StartedAt but no FinishedAt and exec has FinishedAt
func (s *HandlersTestSuite) TestBuildHistoryEntry_ExecFinishedAt() {
	started := time.Now()
	execFinished := started.Add(2 * time.Second)

	exec := &store.Execution{
		ID:         "e2",
		StartedAt:  started,
		FinishedAt: &execFinished,
	}
	sr := &runtime.StepResult{
		Status:    runtime.StatusRunning,
		StartedAt: &started,
	}

	entry := buildHistoryEntry(exec, sr)
	s.NotEmpty(entry.FinishedAt)
	s.Equal(int64(0), entry.DurationMs) // DurationMs only set when both sr.StartedAt and sr.FinishedAt are set
}

// handleWaitWebhook – invalid body (not JSON)
func (s *HandlersTestSuite) TestWaitWebhook_InvalidBody() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/wait/test-exec/callback", strings.NewReader(`not-json`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// No registration, so 404 (the body decode error is logged)
	s.Equal(http.StatusNotFound, w.Code)
}

// filterByStatus – empty status
func (s *HandlersTestSuite) TestFilterByStatus_EmptyFilter() {
	execs := []*store.Execution{
		{ID: "1", Status: runtime.StatusSuccess},
		{ID: "2", Status: runtime.StatusFailed},
	}

	result := filterByStatus(execs, "")
	s.Len(result, 2) // no filtering
}

// sortExecutions – non-duration sort (default, no-op except for order)
func (s *HandlersTestSuite) TestSortExecutions_DefaultSort() {
	now := time.Now()
	execs := []*store.Execution{
		{ID: "1", StartedAt: now},
		{ID: "2", StartedAt: now.Add(1 * time.Second)},
	}

	sortExecutions(execs, "", "")
	s.Equal("1", execs[0].ID) // unchanged

	sortExecutions(execs, "", "asc")
	s.Equal("2", execs[0].ID) // reversed
}

// paginateExecutions – limit 1 offset 0
func (s *HandlersTestSuite) TestPaginateExecutions_SmallPage() {
	execs := []*store.Execution{
		{ID: "1"}, {ID: "2"}, {ID: "3"},
	}

	r := httptest.NewRequest("GET", "/test?offset=1&limit=1", nil)
	paged := paginateExecutions(execs, r)
	s.Len(paged, 1)
	s.Equal("2", paged[0].ID)
}

// captureEvents full lifecycle
func (s *HandlersTestSuite) TestCaptureEvents_FullLifecycle() {
	srv := newTestServer(s.T())

	execID := "cap-full"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	stopCapture := srv.captureEvents(execID)

	// Publish matching events
	srv.config.EventBus.Publish(event.Event{
		Type:        event.StepStarted,
		ExecutionID: execID,
		StepID:      "s1",
	})
	srv.config.EventBus.Publish(event.Event{
		Type:        event.StepCompleted,
		ExecutionID: execID,
		StepID:      "s1",
		Data:        map[string]any{"output": "done"},
	})

	time.Sleep(50 * time.Millisecond)

	stopCapture()

	events := srv.config.ExecutionStore.GetEvents(execID)
	s.Len(events, 2)

	exec, _ := srv.config.ExecutionStore.Get(execID)
	s.Equal(runtime.StatusSuccess, exec.Steps["s1"].Status)
}

// handlePublicTrigger – trigger with invalid body (decode error in buildTriggerData)
func (s *HandlersTestSuite) TestPublicTrigger_InvalidBody() {
	srv := newTestServerHTTPTrigger(s.T())

	req := httptest.NewRequest("POST", "/api/public/submit", strings.NewReader(`not-json`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Still succeeds, body decode error is logged
	s.Equal(http.StatusOK, w.Code)
}

// writeTriggerResponse – response action step not in result (step missing from result.Steps)
func (s *HandlersTestSuite) TestWriteTriggerResponse_ResponseStepNotInResult() {
	srv := newTestServer(s.T())

	srv.config.Workflow.Steps = []parser.Step{
		{ID: "respond_missing", Action: "response"},
	}

	result := &engine.ExecuteResult{
		Status: runtime.StatusSuccess,
		Steps:  map[string]*runtime.StepResult{},
	}

	w := httptest.NewRecorder()
	srv.writeTriggerResponse(w, srv.config.Workflow, result, nil, context.Background(), "exec-miss")

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal("success", resp["status"])
}

// writeTriggerResponse – response action with int status field
func (s *HandlersTestSuite) TestWriteTriggerResponse_ResponseActionWithIntStatus() {
	srv := newTestServer(s.T())

	srv.config.Workflow.Steps = []parser.Step{
		{ID: "respond_status", Action: "response"},
	}

	result := &engine.ExecuteResult{
		Status: runtime.StatusSuccess,
		Steps: map[string]*runtime.StepResult{
			"respond_status": {
				Status: runtime.StatusSuccess,
				Output: map[string]any{
					"status": 201,
					"body":   map[string]any{"created": true},
				},
			},
		},
	}

	w := httptest.NewRecorder()
	srv.writeTriggerResponse(w, srv.config.Workflow, result, nil, context.Background(), "exec-201")

	s.Equal(201, w.Code)
}

// handleGetVersion – completely untested
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

// handleGetVersion – empty version string
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

// processEvent – duplicate WorkflowCompleted is deduplicated (completedSeen branch)
func (s *HandlersTestSuite) TestProcessEvent_DuplicateWorkflowCompleted() {
	srv := newTestServer(s.T())

	execID := "pe-dedup"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	lt := &loopTracker{}
	completedSeen := false

	// First WorkflowCompleted event should be stored
	srv.processEvent(execID, event.Event{
		Type:        event.WorkflowCompleted,
		ExecutionID: execID,
		Data:        map[string]any{"status": "success"},
	}, lt, &completedSeen)

	s.True(completedSeen)
	events := srv.config.ExecutionStore.GetEvents(execID)
	s.Len(events, 1)

	// Second WorkflowCompleted event should be deduplicated (early return)
	srv.processEvent(execID, event.Event{
		Type:        event.WorkflowCompleted,
		ExecutionID: execID,
		Data:        map[string]any{"status": "success"},
	}, lt, &completedSeen)

	// Still only 1 event stored — the duplicate was dropped
	events = srv.config.ExecutionStore.GetEvents(execID)
	s.Len(events, 1)
}

func newTestServerIdempotent(t *testing.T, saasURL string) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "idempotent-workflow"
description: "HTTP workflow with idempotency"
trigger:
  http:
    method: POST
    path: /submit
    idempotency_key: "{{ trigger.body.order_id }}"
params:
  - name: order_id
    type: string
stages:
  - name: default
steps:
  - id: process
    action: log
    stage: default
    config:
      message: "processing"
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
	exec := engine.NewExecutor(reg, bus, logger, nil, nil, nil)

	var claimer export.IdempotencyClaimer = export.NewNoopClaimer()
	if saasURL != "" {
		claimer = saas.NewClaimClient(saasURL, "test-key")
	}

	return New(Config{
		Port:           0,
		Executor:       exec,
		Workflow:       wf,
		ExecutionStore: store.NewExecutionStore(10),
		EventBus:       bus,
		Logger:         logger,
		Claimer:        claimer,
	})
}

func (s *HandlersTestSuite) TestPublicTrigger_IdempotencyDeduplicated() {
	mockSaaS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"claimed":false,"execution_id":"existing-exec","status":"success"}`))
	}))
	defer mockSaaS.Close()

	srv := newTestServerIdempotent(s.T(), mockSaaS.URL)

	req := httptest.NewRequest("POST", "/api/public/submit", strings.NewReader(`{"order_id":"ord-123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Equal(true, resp["deduplicated"])
	s.Equal("existing-exec", resp["execution_id"])
	s.Equal("success", resp["status"])
}

func (s *HandlersTestSuite) TestPublicTrigger_IdempotencyClaimed() {
	mockSaaS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"claimed":true}`))
	}))
	defer mockSaaS.Close()

	srv := newTestServerIdempotent(s.T(), mockSaaS.URL)

	req := httptest.NewRequest("POST", "/api/public/submit", strings.NewReader(`{"order_id":"ord-456"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Nil(resp["deduplicated"])
	s.NotEmpty(resp["execution_id"])
}

func (s *HandlersTestSuite) TestPublicTrigger_NoIdempotencyKeyNoExportURL() {
	srv := newTestServerHTTPTrigger(s.T())

	req := httptest.NewRequest("POST", "/api/public/submit", strings.NewReader(`{"data":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Nil(resp["deduplicated"])
}

func (s *HandlersTestSuite) TestPublicTrigger_IdempotencyNoExportURL() {
	srv := newTestServerIdempotent(s.T(), "")

	req := httptest.NewRequest("POST", "/api/public/submit", strings.NewReader(`{"order_id":"ord-789"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Nil(resp["deduplicated"])
}

func (s *HandlersTestSuite) TestPublicTrigger_IdempotencyResolveError() {
	mockSaaS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"claimed":true}`))
	}))
	defer mockSaaS.Close()

	srv := newTestServerIdempotent(s.T(), mockSaaS.URL)
	srv.config.Workflow.Trigger.HTTP.IdempotencyKey = "{{ invalid_expression( }}"

	req := httptest.NewRequest("POST", "/api/public/submit", strings.NewReader(`{"order_id":"ord-789"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Nil(resp["deduplicated"])
}

func (s *HandlersTestSuite) TestPublicTrigger_IdempotencyClaimError() {
	srv := newTestServerIdempotent(s.T(), "http://127.0.0.1:1")

	req := httptest.NewRequest("POST", "/api/public/submit", strings.NewReader(`{"order_id":"ord-789"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Nil(resp["deduplicated"])
}

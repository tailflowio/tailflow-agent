package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

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

	execID := resp["execution_id"].(string)
	exec, err := srv.config.ExecutionStore.Get(context.Background(), execID)
	s.Require().NoError(err)
	s.Equal("running", exec.Status)
	s.Equal("test-workflow", exec.WorkflowName)

	s.Eventually(func() bool {
		exec, err := srv.config.ExecutionStore.Get(context.Background(), execID)
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

// TestRunWorkflow_NilBody covers handleRunWorkflow – nil body.
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

// TestRunWorkflow_InvalidJSON covers handleRunWorkflow – invalid JSON body (decode error path).
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

// TestGetStepDetail_WithHistory covers buildStepHistory + buildHistoryEntry.
func (s *HandlersTestSuite) TestGetStepDetail_WithHistory() {
	srv := newTestServer(s.T())

	startedAt := time.Now().Add(-2 * time.Second)
	finishedAt := time.Now()
	execFinished := time.Now()

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
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

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
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

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
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

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
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
	s.Len(history, 3)

	entryFull := history[2].(map[string]any)
	s.NotEmpty(entryFull["started_at"])
	s.NotEmpty(entryFull["finished_at"])
	s.Greater(entryFull["duration_ms"].(float64), float64(0))

	entryErr := history[1].(map[string]any)
	s.Equal("something went wrong", entryErr["error"])

	entryPartial := history[0].(map[string]any)
	s.NotEmpty(entryPartial["started_at"])
	s.NotEmpty(entryPartial["finished_at"])
}

// TestListExecutions_FilterByStatus covers filterByStatus.
func (s *HandlersTestSuite) TestListExecutions_FilterByStatus() {
	srv := newTestServer(s.T())

	now := time.Now()
	finished := now.Add(1 * time.Second)

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: "e1", WorkflowName: "test", Status: runtime.StatusSuccess,
		StartedAt: now, FinishedAt: &finished,
	})
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: "e2", WorkflowName: "test", Status: runtime.StatusFailed,
		StartedAt: now, FinishedAt: &finished,
	})
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: "e3", WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: now,
	})

	req := httptest.NewRequest("GET", "/api/executions?status=success", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal(float64(1), resp["total"])

	req2 := httptest.NewRequest("GET", "/api/executions?status=success,failed", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	var resp2 map[string]any
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	s.Equal(float64(2), resp2["total"])
}

// TestListExecutions_SortByDuration covers sortExecutions with duration + asc/desc.
func (s *HandlersTestSuite) TestListExecutions_SortByDuration() {
	srv := newTestServer(s.T())

	now := time.Now()
	shortFinish := now.Add(100 * time.Millisecond)
	longFinish := now.Add(5 * time.Second)

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: "short", WorkflowName: "test", Status: runtime.StatusSuccess,
		StartedAt: now, FinishedAt: &shortFinish,
	})
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: "long", WorkflowName: "test", Status: runtime.StatusSuccess,
		StartedAt: now, FinishedAt: &longFinish,
	})
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: "running", WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: now,
	})

	req := httptest.NewRequest("GET", "/api/executions?sort=duration", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	items := resp["items"].([]any)
	s.Len(items, 3)
	first := items[0].(map[string]any)
	s.Equal("long", first["id"])

	req2 := httptest.NewRequest("GET", "/api/executions?sort=duration&order=asc", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	var resp2 map[string]any
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	items2 := resp2["items"].([]any)
	lastItem := items2[len(items2)-1].(map[string]any)
	s.Equal("long", lastItem["id"])
}

// TestListExecutions_OrderAsc covers sortExecutions – order=asc without specific sort (default date order reversed).
func (s *HandlersTestSuite) TestListExecutions_OrderAsc() {
	srv := newTestServer(s.T())

	now := time.Now()
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: "first", WorkflowName: "test", Status: runtime.StatusSuccess, StartedAt: now,
	})
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: "second", WorkflowName: "test", Status: runtime.StatusSuccess, StartedAt: now.Add(1 * time.Second),
	})

	req := httptest.NewRequest("GET", "/api/executions?order=asc", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	items := resp["items"].([]any)
	firstItem := items[0].(map[string]any)
	s.Equal("first", firstItem["id"])
}

// TestListExecutions_OffsetBeyondTotal covers paginateExecutions – offset beyond items count.
func (s *HandlersTestSuite) TestListExecutions_OffsetBeyondTotal() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
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

// TestListExecutions_InvalidPaginationParams covers parseIntParam – invalid and negative values.
func (s *HandlersTestSuite) TestListExecutions_InvalidPaginationParams() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: "e1", WorkflowName: "test", Status: runtime.StatusSuccess, StartedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/executions?offset=abc&limit=xyz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	items := resp["items"].([]any)
	s.Len(items, 1)

	req2 := httptest.NewRequest("GET", "/api/executions?offset=-5&limit=-1", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	s.Equal(http.StatusOK, w2.Code)
}

// TestGetExecution_Found covers handleGetExecution.
func (s *HandlersTestSuite) TestGetExecution_Found() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
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

// TestCancelExecution_NotFound covers handleCancelExecution.
func (s *HandlersTestSuite) TestCancelExecution_NotFound() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/executions/nonexistent/cancel", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

func (s *HandlersTestSuite) TestCancelExecution_NotRunning() {
	srv := newTestServer(s.T())

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
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

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: "exec-cancel", WorkflowName: "test-workflow", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

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

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
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

	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: "exec-no-cancel", WorkflowName: "test-workflow", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	req := httptest.NewRequest("POST", "/api/executions/exec-no-cancel/cancel", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Contains(resp["error"], "cancel function not found")
}

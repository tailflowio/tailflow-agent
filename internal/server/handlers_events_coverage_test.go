package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

// TestHandleListEvents_Success covers handleListEvents – happy path.
func (s *HandlersTestSuite) TestHandleListEvents_Success() {
	srv := newTestServer(s.T())

	execID := "events-test"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID:           execID,
		WorkflowName: "test-workflow",
		Status:       runtime.StatusSuccess,
		StartedAt:    time.Now(),
	})

	for i := range 5 {
		srv.config.ExecutionStore.AppendEvent(context.Background(), execID, event.Event{
			Type:        event.StepStarted,
			ExecutionID: execID,
			StepID:      "greet",
			Data:        map[string]any{"i": i},
		})
	}

	req := httptest.NewRequest("GET", "/api/executions/"+execID+"/events/list?offset=0&limit=3", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Equal(float64(5), resp["total"])
	s.Equal(float64(0), resp["offset"])
	s.Equal(float64(3), resp["limit"])
	s.True(resp["hasMore"].(bool))
	s.Len(resp["events"].([]any), 3)
}

// TestHandleListEvents_DefaultLimits covers handleListEvents – zero/negative limit/offset clamped.
func (s *HandlersTestSuite) TestHandleListEvents_DefaultLimits() {
	srv := newTestServer(s.T())

	execID := "events-default"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test-workflow", Status: runtime.StatusSuccess, StartedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/executions/"+execID+"/events/list?offset=0&limit=0", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal(float64(50), resp["limit"])
}

// TestHandleListEvents_LimitClampedAt200 covers handleListEvents – limit > 200.
func (s *HandlersTestSuite) TestHandleListEvents_LimitClampedAt200() {
	srv := newTestServer(s.T())

	execID := "events-clamp"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test-workflow", Status: runtime.StatusSuccess, StartedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/executions/"+execID+"/events/list?limit=999", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal(float64(50), resp["limit"])
}

// TestHandleListEvents_NegativeOffset covers handleListEvents – negative offset clamped.
func (s *HandlersTestSuite) TestHandleListEvents_NegativeOffset() {
	srv := newTestServer(s.T())

	execID := "events-neg"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test-workflow", Status: runtime.StatusSuccess, StartedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/executions/"+execID+"/events/list?offset=-10", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

// TestHandleListEvents_StoreError covers handleListEvents – store returns error.
func (s *HandlersTestSuite) TestHandleListEvents_StoreError() {
	es := newErrStore()
	es.failGetEvents = true
	srv := newTestServerWithErrStore(s.T(), es)

	req := httptest.NewRequest("GET", "/api/executions/any-id/events/list", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusInternalServerError, w.Code)
}

// TestHandleListEvents_StartBeforeZero covers handleListEvents – offset+limit > total makes start negative.
func (s *HandlersTestSuite) TestHandleListEvents_StartBeforeZero() {
	srv := newTestServer(s.T())

	execID := "events-start-neg"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test-workflow", Status: runtime.StatusSuccess, StartedAt: time.Now(),
	})

	for range 2 {
		srv.config.ExecutionStore.AppendEvent(context.Background(), execID, event.Event{
			Type:        event.StepStarted,
			ExecutionID: execID,
		})
	}

	req := httptest.NewRequest("GET", "/api/executions/"+execID+"/events/list?offset=0&limit=5", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal(float64(2), resp["total"])
}

// TestHandleListEvents_EndBeforeZero covers handleListEvents – offset beyond total makes end negative.
func (s *HandlersTestSuite) TestHandleListEvents_EndBeforeZero() {
	srv := newTestServer(s.T())

	execID := "events-end-neg"
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID: execID, WorkflowName: "test-workflow", Status: runtime.StatusSuccess, StartedAt: time.Now(),
	})

	srv.config.ExecutionStore.AppendEvent(context.Background(), execID, event.Event{
		Type: event.StepStarted, ExecutionID: execID,
	})

	req := httptest.NewRequest("GET", "/api/executions/"+execID+"/events/list?offset=5&limit=2", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	events, _ := resp["events"].([]any)
	s.Len(events, 0)
}

// TestHandleListExecutions_StoreError covers handleListExecutions – store returns error.
func (s *HandlersTestSuite) TestHandleListExecutions_StoreError() {
	es := newErrStore()
	es.failList = true
	srv := newTestServerWithErrStore(s.T(), es)

	req := httptest.NewRequest("GET", "/api/executions", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusInternalServerError, w.Code)
}

// TestHandleGetAllStepMetrics_Success covers handleGetAllStepMetrics – happy path.
func (s *HandlersTestSuite) TestHandleGetAllStepMetrics_Success() {
	srv := newTestServer(s.T())

	now := time.Now()
	finished := now.Add(1 * time.Second)
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID:           "metrics-1",
		WorkflowName: "test-workflow",
		Status:       runtime.StatusSuccess,
		StartedAt:    now,
		FinishedAt:   &finished,
		Steps: map[string]*runtime.StepResult{
			"greet": {Status: runtime.StatusSuccess, StartedAt: &now, FinishedAt: &finished},
		},
	})

	req := httptest.NewRequest("GET", "/api/workflow/steps/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
}

// TestHandleGetAllStepMetrics_AllStepMetricsError covers handleGetAllStepMetrics – GetAllStepMetrics error.
func (s *HandlersTestSuite) TestHandleGetAllStepMetrics_AllStepMetricsError() {
	es := newErrStore()
	es.failAllStepMetrics = true
	srv := newTestServerWithErrStore(s.T(), es)

	req := httptest.NewRequest("GET", "/api/workflow/steps/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusInternalServerError, w.Code)
}

// TestHandleGetAllStepMetrics_ListError covers handleGetAllStepMetrics – List error.
func (s *HandlersTestSuite) TestHandleGetAllStepMetrics_ListError() {
	es := newErrStore()
	es.failList = true
	srv := newTestServerWithErrStore(s.T(), es)

	req := httptest.NewRequest("GET", "/api/workflow/steps/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusInternalServerError, w.Code)
}

// TestHandleGetAllStepMetrics_StepInExecNotInMetrics covers the branch where
// a step appears in an execution but not in the allMetrics map.
func (s *HandlersTestSuite) TestHandleGetAllStepMetrics_StepInExecNotInMetrics() {
	srv := newTestServer(s.T())

	now := time.Now()
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID:           "metrics-extra",
		WorkflowName: "test-workflow",
		Status:       runtime.StatusSuccess,
		StartedAt:    now,
		Steps: map[string]*runtime.StepResult{
			"unknown_step": {Status: runtime.StatusSuccess},
		},
	})

	req := httptest.NewRequest("GET", "/api/workflow/steps/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

// TestHandleGetAllStepMetrics_NilSteps covers handleGetAllStepMetrics – execution with nil Steps skipped.
func (s *HandlersTestSuite) TestHandleGetAllStepMetrics_NilSteps() {
	srv := newTestServer(s.T())

	now := time.Now()
	srv.config.ExecutionStore.Add(context.Background(), &store.Execution{
		ID:           "metrics-nil-steps",
		WorkflowName: "test-workflow",
		Status:       runtime.StatusSuccess,
		StartedAt:    now,
		Steps:        nil,
	})

	req := httptest.NewRequest("GET", "/api/workflow/steps/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

// TestHandleGetStepDetail_BuildStepHistoryError covers handleGetStepDetail – buildStepHistory error.
func (s *HandlersTestSuite) TestHandleGetStepDetail_BuildStepHistoryError() {
	es := newErrStore()
	es.failList = true
	srv := newTestServerWithErrStore(s.T(), es)

	req := httptest.NewRequest("GET", "/api/workflow/steps/greet", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusInternalServerError, w.Code)
}

// TestHandleGetStepDetail_GetStepMetricsError covers handleGetStepDetail – GetStepMetrics error.
func (s *HandlersTestSuite) TestHandleGetStepDetail_GetStepMetricsError() {
	es := newErrStore()
	es.failStepMetrics = true
	srv := newTestServerWithErrStore(s.T(), es)

	req := httptest.NewRequest("GET", "/api/workflow/steps/greet", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusInternalServerError, w.Code)
}

// TestHandleGetWorkflowActivity_StepExecCountsError covers handleGetWorkflowActivity – StepExecCounts error.
func (s *HandlersTestSuite) TestHandleGetWorkflowActivity_StepExecCountsError() {
	es := newErrStore()
	es.failStepExecCounts = true
	srv := newTestServerWithErrStore(s.T(), es)

	req := httptest.NewRequest("GET", "/api/workflow/activity", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusInternalServerError, w.Code)
}

// TestHandleGetWorkflowActivity_ListError covers handleGetWorkflowActivity – List returns error.
func (s *HandlersTestSuite) TestHandleGetWorkflowActivity_ListError() {
	es := newErrStore()
	es.failList = true
	srv := newTestServerWithErrStore(s.T(), es)

	req := httptest.NewRequest("GET", "/api/workflow/activity", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusInternalServerError, w.Code)
}

// TestHandleGetWorkflowRaw_NoFilePath covers handleGetWorkflowRaw – no file path set.
func (s *HandlersTestSuite) TestHandleGetWorkflowRaw_NoFilePath() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/raw", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

// TestHandleGetWorkflowRaw_FileNotFound covers handleGetWorkflowRaw – file read fails.
func (s *HandlersTestSuite) TestHandleGetWorkflowRaw_FileNotFound() {
	srv := newTestServerWithRawFile(s.T(), "/nonexistent/path/workflow.yaml")

	req := httptest.NewRequest("GET", "/api/workflow/raw", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusInternalServerError, w.Code)
}

// TestHandleGetWorkflowRaw_Success covers handleGetWorkflowRaw – file exists and is returned.
func (s *HandlersTestSuite) TestHandleGetWorkflowRaw_Success() {
	dir := s.T().TempDir()
	filePath := dir + "/workflow.yaml"

	content := []byte(`version: "2.0"
name: "raw-test"
stages:
  - name: default
steps:
  - id: x
    action: log
    stage: default
    config:
      message: "test"
`)
	err := os.WriteFile(filePath, content, 0o644)
	s.Require().NoError(err)

	srv := newTestServerWithRawFile(s.T(), filePath)

	req := httptest.NewRequest("GET", "/api/workflow/raw", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.Equal("text/plain; charset=utf-8", w.Header().Get("Content-Type"))
	s.Contains(w.Body.String(), "raw-test")
}

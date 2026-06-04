package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

// TestPublicTrigger_NoTrigger covers handlePublicTrigger – no trigger configured.
func (s *HandlersTestSuite) TestPublicTrigger_NoTrigger() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/public/anything", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

// TestPublicTrigger_HTTPSync covers handlePublicTrigger – HTTP trigger, sync.
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

// TestPublicTrigger_HTTPAsync covers handlePublicTrigger – HTTP trigger, async.
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

	execID := resp["execution_id"].(string)
	s.Eventually(func() bool {
		exec, getErr := srv.config.ExecutionStore.Get(context.Background(), execID)
		return getErr == nil && (exec.Status == "success" || exec.Status == "failed")
	}, 2*time.Second, 10*time.Millisecond)
}

// TestPublicTrigger_Webhook covers handlePublicTrigger – webhook trigger.
func (s *HandlersTestSuite) TestPublicTrigger_Webhook() {
	srv := newTestServerWebhookTrigger(s.T())

	req := httptest.NewRequest("POST", "/api/public/hook", strings.NewReader(`{"event":"push"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

// TestPublicTrigger_NilBody covers buildTriggerData – nil body.
func (s *HandlersTestSuite) TestPublicTrigger_NilBody() {
	srv := newTestServerHTTPTrigger(s.T())

	req := httptest.NewRequest("POST", "/api/public/submit", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

// TestPublicTrigger_InvalidBody covers handlePublicTrigger – trigger with invalid body (decode error in buildTriggerData).
func (s *HandlersTestSuite) TestPublicTrigger_InvalidBody() {
	srv := newTestServerHTTPTrigger(s.T())

	req := httptest.NewRequest("POST", "/api/public/submit", strings.NewReader(`not-json`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

// TestPublicTrigger_NoIdempotencyKeyNoExportURL covers the path where trigger has no idempotency key.
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

// TestWriteTriggerResponse_WithError covers writeTriggerResponse – error path.
func (s *HandlersTestSuite) TestWriteTriggerResponse_WithError() {
	srv := newTestServer(s.T())

	w := httptest.NewRecorder()
	srv.writeTriggerResponse(w, srv.config.Workflow, nil, errors.New("exec failed"), context.Background(), "exec-1")

	s.Equal(http.StatusInternalServerError, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal("exec failed", resp["error"])
}

// TestWriteTriggerResponse_Cancelled covers writeTriggerResponse – cancelled context.
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

// TestWriteTriggerResponse_WithResponseAction covers writeTriggerResponse – response action with headers.
func (s *HandlersTestSuite) TestWriteTriggerResponse_WithResponseAction() {
	srv := newTestServer(s.T())

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

// TestWriteTriggerResponse_ResponseActionDefaultStatus covers writeTriggerResponse – response action with non-int status (falls back to 200).
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

// TestWriteTriggerResponse_ResponseActionNonMapOutput covers writeTriggerResponse – response action where output is not map.
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

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	s.Equal("success", resp["status"])
}

// TestWriteTriggerResponse_ResponseActionNilOutput covers writeTriggerResponse – response action with nil output.
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

// TestWriteTriggerResponse_ResponseStepNotInResult covers writeTriggerResponse – response action step not in result (step missing from result.Steps).
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

// TestWriteTriggerResponse_ResponseActionWithIntStatus covers writeTriggerResponse – response action with int status field.
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

// TestWaitWebhook_BadPath covers handleWaitWebhook – missing path.
func (s *HandlersTestSuite) TestWaitWebhook_BadPath() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/wait/test-exec-id", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusBadRequest, w.Code)
}

// TestWaitWebhook_NilBody covers handleWaitWebhook – nil body.
func (s *HandlersTestSuite) TestWaitWebhook_NilBody() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/wait/test-exec-id/callback", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

// TestWaitWebhook_InvalidBody covers handleWaitWebhook – invalid body (not JSON).
func (s *HandlersTestSuite) TestWaitWebhook_InvalidBody() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("POST", "/api/wait/test-exec/callback", strings.NewReader(`not-json`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}


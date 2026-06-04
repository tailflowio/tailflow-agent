package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/store"
)

func newTestServerIdempotent(t *testing.T, claimer export.IdempotencyClaimer) *Server {
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

	if claimer == nil {
		claimer = export.NewNoopClaimer()
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
	claimer := &stubClaimer{result: &export.ClaimResult{
		Claimed:             false,
		ExistingExecutionID: "existing-exec",
		ExistingStatus:      "success",
	}}
	srv := newTestServerIdempotent(s.T(), claimer)

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
	claimer := &stubClaimer{result: &export.ClaimResult{Claimed: true}}
	srv := newTestServerIdempotent(s.T(), claimer)

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

func (s *HandlersTestSuite) TestPublicTrigger_ClaimAndExecutionShareSameID() {
	claimer := &recordingClaimer{result: &export.ClaimResult{Claimed: true}}
	srv := newTestServerIdempotent(s.T(), claimer)

	req := httptest.NewRequest("POST", "/api/public/submit", strings.NewReader(`{"order_id":"ord-share"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.Equal(1, claimer.capturedCalls)
	s.NotEmpty(claimer.capturedID)

	exec, err := srv.config.ExecutionStore.Get(context.Background(), claimer.capturedID)
	s.Require().NoError(err)
	s.Equal(claimer.capturedID, exec.ID)
}

func (s *HandlersTestSuite) TestPublicTrigger_SameKeyTwiceSingleExecution() {
	srv := newTestServerIdempotent(s.T(), export.NewMemoryClaimer())

	firstID := s.publicTriggerExecutionID(srv, `{"order_id":"ord-e2e"}`)
	s.NotEmpty(firstID)

	req := httptest.NewRequest("POST", "/api/public/submit", strings.NewReader(`{"order_id":"ord-e2e"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Equal(true, resp["deduplicated"])
	s.Equal(firstID, resp["execution_id"])

	count, err := srv.config.ExecutionStore.Count(context.Background())
	s.Require().NoError(err)
	s.Equal(1, count)
}

func (s *HandlersTestSuite) publicTriggerExecutionID(srv *Server, body string) string {
	s.T().Helper()

	req := httptest.NewRequest("POST", "/api/public/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Nil(resp["deduplicated"])

	id, ok := resp["execution_id"].(string)
	s.Require().True(ok)

	return id
}

func (s *HandlersTestSuite) TestPublicTrigger_IdempotencyNoExportURL() {
	srv := newTestServerIdempotent(s.T(), nil)

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
	claimer := &stubClaimer{result: &export.ClaimResult{Claimed: true}}
	srv := newTestServerIdempotent(s.T(), claimer)
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
	srv := newTestServerIdempotent(s.T(), &stubClaimer{err: errStubClaim})

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

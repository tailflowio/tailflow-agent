package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/store"
)

type HandlersUnsafeTestSuite struct {
	suite.Suite
}

func TestHandlersUnsafe(t *testing.T) {
	suite.Run(t, new(HandlersUnsafeTestSuite))
}

func (s *HandlersUnsafeTestSuite) SetupTest() {
	// required by convention
}

func newTestServerWithTrigger(t *testing.T) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "api-users"
description: "Create user API"
tags: ["api"]

trigger:
  http:
    method: POST
    path: /users

stages:
  - name: default

steps:
  - id: echo
    action: js
    stage: default
    title: "Echo trigger body"
    config:
      script: |
        return { email: trigger.body.email, method: trigger.method };

  - id: respond
    action: response
    stage: default
    title: "Send response"
    depends_on: [echo]
    config:
      status: 201
      body:
        email: "{{ steps.echo.output.email }}"
        method: "{{ steps.echo.output.method }}"
        message: "created"
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
	exec := engine.NewExecutor(reg, bus, logger, wf.Sensitive, nil, nil)

	return New(Config{
		Port:           0,
		Executor:       exec,
		Workflow:       wf,
		ExecutionStore: store.NewExecutionStore(10),
		EventBus:       bus,
		Logger:         logger,
	})
}

func (s *HandlersUnsafeTestSuite) TestPublicTrigger_HTTP() {
	srv := newTestServerWithTrigger(s.T())

	body := `{"email":"test@example.com"}`
	req := httptest.NewRequest("POST", "/api/public/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(201, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Equal("test@example.com", resp["email"])
	s.Equal("POST", resp["method"])
	s.Equal("created", resp["message"])
}

func (s *HandlersUnsafeTestSuite) TestPublicTrigger_NotFound() {
	srv := newTestServerWithTrigger(s.T())

	req := httptest.NewRequest("GET", "/api/public/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

func (s *HandlersUnsafeTestSuite) TestPublicTrigger_WrongMethod() {
	srv := newTestServerWithTrigger(s.T())

	req := httptest.NewRequest("GET", "/api/public/users", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

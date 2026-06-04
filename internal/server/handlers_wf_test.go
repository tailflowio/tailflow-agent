package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/store"
	"github.com/tailflow/tailflow/pkg/api"
)

type HandlersWorkflowRawTestSuite struct {
	suite.Suite
}

func TestHandlersWorkflowRawTestSuite(t *testing.T) {
	suite.Run(t, new(HandlersWorkflowRawTestSuite))
}

func (s *HandlersWorkflowRawTestSuite) SetupTest() {} // required by convention

func newTestServerRaw(t *testing.T, filePath string, editorEnabled bool) *Server {
	t.Helper()

	yaml := `version: "2.0"
name: "raw-workflow"
stages:
  - name: default
steps:
  - id: greet
    action: log
    stage: default
    config:
      message: "hello"
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
		FilePath:       filePath,
		EditorEnabled:  editorEnabled,
	})
}

func (s *HandlersWorkflowRawTestSuite) TestPutWorkflowRaw_EditorDisabled() {
	srv := newTestServerRaw(s.T(), "", false)

	req := httptest.NewRequest("PUT", "/api/workflow/raw", strings.NewReader("anything"))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusForbidden, w.Code)
}

func (s *HandlersWorkflowRawTestSuite) TestPutWorkflowRaw_NoFilePath() {
	srv := newTestServerRaw(s.T(), "", true)

	req := httptest.NewRequest("PUT", "/api/workflow/raw", strings.NewReader("anything"))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

func (s *HandlersWorkflowRawTestSuite) TestPutWorkflowRaw_EmptyBody() {
	dir := s.T().TempDir()
	filePath := filepath.Join(dir, "workflow.yaml")
	srv := newTestServerRaw(s.T(), filePath, true)

	req := httptest.NewRequest("PUT", "/api/workflow/raw", strings.NewReader(""))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusBadRequest, w.Code)
}

func (s *HandlersWorkflowRawTestSuite) TestPutWorkflowRaw_ParseError() {
	dir := s.T().TempDir()
	filePath := filepath.Join(dir, "workflow.yaml")
	srv := newTestServerRaw(s.T(), filePath, true)

	req := httptest.NewRequest("PUT", "/api/workflow/raw", strings.NewReader("\tnot: [valid"))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusBadRequest, w.Code)

	var resp api.ValidateResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.False(resp.Valid)
	s.NotEmpty(resp.Errors)
}

func (s *HandlersWorkflowRawTestSuite) TestPutWorkflowRaw_ValidateError() {
	dir := s.T().TempDir()
	filePath := filepath.Join(dir, "workflow.yaml")
	srv := newTestServerRaw(s.T(), filePath, true)

	body := `version: "2.0"
name: "no-steps"
stages:
  - name: default
`
	req := httptest.NewRequest("PUT", "/api/workflow/raw", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusBadRequest, w.Code)

	var resp api.ValidateResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.False(resp.Valid)
	s.NotEmpty(resp.Errors)
}

func (s *HandlersWorkflowRawTestSuite) TestPutWorkflowRaw_WriteError() {
	dir := s.T().TempDir()
	err := os.Chmod(dir, 0o500)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	filePath := filepath.Join(dir, "workflow.yaml")
	srv := newTestServerRaw(s.T(), filePath, true)

	body := `version: "2.0"
name: "valid"
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

	s.Equal(http.StatusInternalServerError, w.Code)
}

func (s *HandlersWorkflowRawTestSuite) TestPutWorkflowRaw_Success() {
	dir := s.T().TempDir()
	filePath := filepath.Join(dir, "workflow.yaml")
	srv := newTestServerRaw(s.T(), filePath, true)

	body := `version: "2.0"
name: "valid"
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

	s.Equal(http.StatusOK, w.Code)

	var resp api.ValidateResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.True(resp.Valid)
}

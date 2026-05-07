package saas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type RecoveryClientTestSuite struct {
	suite.Suite
}

func TestRecoveryClient(t *testing.T) {
	suite.Run(t, new(RecoveryClientTestSuite))
}

func (s *RecoveryClientTestSuite) SetupTest() {}

func (s *RecoveryClientTestSuite) TestRecoverExecutions_ReturnsExecutions() {
	now := time.Now().Format(time.RFC3339)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/api/v1/agent/recovery", r.URL.Path)
		s.Equal("agent-1", r.URL.Query().Get("agent_id"))
		s.Equal("Bearer test-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")

		resp := []map[string]any{
			{
				"execution_id":  "exec-001",
				"workflow_name": "onboarding",
				"status":        "waiting",
				"params":        `{"customer_id":"user-42"}`,
				"steps": []map[string]any{
					{"step_id": "step1", "status": "success", "started_at": now, "finished_at": now},
					{"step_id": "step2", "status": "running", "started_at": now},
				},
			},
		}

		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewRecoveryClient(srv.URL, "test-key")

	execs, err := client.RecoverExecutions(context.Background(), "agent-1")
	s.Require().NoError(err)
	s.Len(execs, 1)
	s.Equal("exec-001", execs[0].ExecutionID)
	s.Equal("onboarding", execs[0].WorkflowName)
	s.Equal("user-42", execs[0].Params["customer_id"])
	s.Len(execs[0].Steps, 2)
	s.Equal("success", execs[0].Steps["step1"].Status)
	s.Equal("running", execs[0].Steps["step2"].Status)
}

func (s *RecoveryClientTestSuite) TestRecoverExecutions_SaaSUnreachable() {
	client := NewRecoveryClient("http://127.0.0.1:1", "key")

	execs, err := client.RecoverExecutions(context.Background(), "agent-1")
	s.Error(err)
	s.Nil(execs)
}

func (s *RecoveryClientTestSuite) TestRecoverExecutions_SaaSReturnsError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewRecoveryClient(srv.URL, "key")

	execs, err := client.RecoverExecutions(context.Background(), "agent-1")
	s.Error(err)
	s.Nil(execs)
}

func (s *RecoveryClientTestSuite) TestRecoverExecutions_EmptyResponse() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{}) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewRecoveryClient(srv.URL, "key")

	execs, err := client.RecoverExecutions(context.Background(), "agent-1")
	s.NoError(err)
	s.Empty(execs)
}

func (s *RecoveryClientTestSuite) TestRecoverExecutions_InvalidURL() {
	client := NewRecoveryClient("://invalid", "key")

	execs, err := client.RecoverExecutions(context.Background(), "agent-1")
	s.Error(err)
	s.Nil(execs)
}

func (s *RecoveryClientTestSuite) TestRecoverExecutions_InvalidJSON() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	client := NewRecoveryClient(srv.URL, "key")

	execs, err := client.RecoverExecutions(context.Background(), "agent-1")
	s.Error(err)
	s.Nil(execs)
}

func (s *RecoveryClientTestSuite) TestRecoverExecutions_InvalidParams() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		resp := []map[string]any{
			{
				"execution_id":  "exec-001",
				"workflow_name": "test",
				"status":        "running",
				"params":        `not valid json`,
			},
		}

		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewRecoveryClient(srv.URL, "key")

	execs, err := client.RecoverExecutions(context.Background(), "agent-1")
	s.Error(err)
	s.Nil(execs)
}

func (s *RecoveryClientTestSuite) TestRecoverExecutions_NullSteps() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		resp := []map[string]any{
			{
				"execution_id":  "exec-001",
				"workflow_name": "test",
				"status":        "running",
				"params":        `{"key":"val"}`,
			},
		}

		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewRecoveryClient(srv.URL, "key")

	execs, err := client.RecoverExecutions(context.Background(), "agent-1")
	s.Require().NoError(err)
	s.Len(execs, 1)
	s.Nil(execs[0].Steps)
	s.Equal("val", execs[0].Params["key"])
}

func (s *RecoveryClientTestSuite) TestRecoverExecutions_StepWithOutput() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		resp := []map[string]any{
			{
				"execution_id":  "exec-001",
				"workflow_name": "test",
				"status":        "running",
				"params":        `{}`,
				"steps": []map[string]any{
					{"step_id": "s1", "status": "success", "output_data": `{"items":[1,2,3]}`},
				},
			},
		}

		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewRecoveryClient(srv.URL, "key")

	execs, err := client.RecoverExecutions(context.Background(), "agent-1")
	s.Require().NoError(err)
	s.Len(execs[0].Steps, 1)

	output, ok := execs[0].Steps["s1"].Output.(map[string]any)
	s.True(ok)
	s.NotNil(output["items"])
}

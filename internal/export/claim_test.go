package export

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ClaimClientTestSuite struct {
	suite.Suite
}

func TestClaimClient(t *testing.T) {
	suite.Run(t, new(ClaimClientTestSuite))
}

func (s *ClaimClientTestSuite) SetupTest() {}

func (s *ClaimClientTestSuite) TestClaimExecution_NewExecution() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/api/executions/claim", r.URL.Path)
		s.Equal("Bearer test-key", r.Header.Get("Authorization"))

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		s.Equal("user-42", body["idempotency_key"])

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"claimed": true})
	}))
	defer srv.Close()

	client := NewClaimClient(srv.URL, "test-key")
	result, err := client.ClaimExecution(context.Background(), "exec-1", "onboarding", "user-42")
	s.Require().NoError(err)
	s.True(result.Claimed)
}

func (s *ClaimClientTestSuite) TestClaimExecution_Duplicate() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"claimed":      false,
			"execution_id": "exec-existing",
			"status":       "running",
		})
	}))
	defer srv.Close()

	client := NewClaimClient(srv.URL, "test-key")
	result, err := client.ClaimExecution(context.Background(), "exec-new", "onboarding", "user-42")
	s.Require().NoError(err)
	s.False(result.Claimed)
	s.Equal("exec-existing", result.ExistingExecutionID)
	s.Equal("running", result.ExistingStatus)
}

func (s *ClaimClientTestSuite) TestClaimExecution_SaaSUnreachable() {
	client := NewClaimClient("http://127.0.0.1:1", "key")
	result, err := client.ClaimExecution(context.Background(), "exec-1", "wf", "key-1")
	s.Require().NoError(err)
	s.True(result.Claimed)
}

func (s *ClaimClientTestSuite) TestClaimExecution_SaaSReturnsError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClaimClient(srv.URL, "key")
	result, err := client.ClaimExecution(context.Background(), "exec-1", "wf", "key-1")
	s.Require().NoError(err)
	s.True(result.Claimed)
}

func (s *ClaimClientTestSuite) TestClaimExecution_InvalidURL() {
	client := NewClaimClient("://invalid", "key")
	result, err := client.ClaimExecution(context.Background(), "exec-1", "wf", "key-1")
	s.Require().NoError(err)
	s.True(result.Claimed)
}

func (s *ClaimClientTestSuite) TestClaimExecution_InvalidJSON() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	client := NewClaimClient(srv.URL, "key")
	result, err := client.ClaimExecution(context.Background(), "exec-1", "wf", "key-1")
	s.Require().NoError(err)
	s.True(result.Claimed)
}

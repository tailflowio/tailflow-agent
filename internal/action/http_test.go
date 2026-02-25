package action

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type HTTPActionTestSuite struct {
	suite.Suite
}

func TestHTTPAction(t *testing.T) {
	suite.Run(t, new(HTTPActionTestSuite))
}

func (s *HTTPActionTestSuite) SetupTest() {}

func (s *HTTPActionTestSuite) TestGET() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("GET", r.Method)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"msg": "hello"})
	}))
	defer srv.Close()

	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"url": srv.URL,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	outMap := out.(map[string]any)
	s.Equal(200, outMap["status"])
	body := outMap["body"].(map[string]any)
	s.Equal("hello", body["msg"])
}

func (s *HTTPActionTestSuite) TestPOST() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("POST", r.Method)
		s.Equal("application/json", r.Header.Get("Content-Type"))
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		s.Equal("Alice", body["name"])
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]string{"id": "123"})
	}))
	defer srv.Close()

	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"method": "POST",
		"url":    srv.URL,
		"body":   map[string]any{"name": "Alice"},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	outMap := out.(map[string]any)
	s.Equal(201, outMap["status"])
}

func (s *HTTPActionTestSuite) TestWithHeaders() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("Bearer token123", r.Header.Get("Authorization"))
		w.WriteHeader(200)
	}))
	defer srv.Close()

	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"url": srv.URL,
		"headers": map[string]any{
			"Authorization": "Bearer token123",
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal(200, out.(map[string]any)["status"])
}

func (s *HTTPActionTestSuite) TestValidateMissingURL() {
	a := NewHTTPAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
}

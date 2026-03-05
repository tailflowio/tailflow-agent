package action

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func (s *HTTPActionTestSuite) TestValidateOK() {
	a := NewHTTPAction()
	err := a.Validate(newTestContext(map[string]any{"url": "http://example.com"}))
	s.NoError(err)
}

func (s *HTTPActionTestSuite) TestPOSTStringBody() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("POST", r.Method)
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"method": "post",
		"url":    srv.URL,
		"body":   "raw string body",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal(200, out.(map[string]any)["status"])
	// non-JSON response should be returned as string
	s.Equal("ok", out.(map[string]any)["body"])
}

func (s *HTTPActionTestSuite) TestExpectStatusSuccess() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"url":    srv.URL,
		"expect": map[string]any{"status": 200},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal(200, out.(map[string]any)["status"])
}

func (s *HTTPActionTestSuite) TestExpectStatusMismatch() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"url":    srv.URL,
		"expect": map[string]any{"status": 200},
	})

	out, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "expected status 200")
	s.Equal(500, out.(map[string]any)["status"])
}

func (s *HTTPActionTestSuite) TestExpectStatusFloat64Mismatch() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"url":    srv.URL,
		"expect": map[string]any{"status": float64(200)},
	})

	out, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "expected status 200")
	s.Equal(404, out.(map[string]any)["status"])
}

func (s *HTTPActionTestSuite) TestExpectStatusFloat64Match() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"url":    srv.URL,
		"expect": map[string]any{"status": float64(200)},
	})

	_, err := a.Execute(ctx)
	s.NoError(err)
}

func (s *HTTPActionTestSuite) TestWithTimeout() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"url":     srv.URL,
		"timeout": "10s",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal(200, out.(map[string]any)["status"])
}

func (s *HTTPActionTestSuite) TestInvalidURL() {
	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"url": "://invalid",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *HTTPActionTestSuite) TestWithContextDeadline() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"url": srv.URL,
	})
	// Set a context with a deadline
	deadlineCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx.Context = deadlineCtx

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal(200, out.(map[string]any)["status"])
}

func (s *HTTPActionTestSuite) TestBodyMarshalError() {
	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"method": "POST",
		"url":    "http://example.com",
		"body":   make(chan int), // channels are not JSON serializable
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "marshal body")
}

func (s *HTTPActionTestSuite) TestConnectionRefused() {
	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"url":     "http://127.0.0.1:1",
		"timeout": "100ms",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "request failed")
}

func (s *HTTPActionTestSuite) TestDefaultContentTypeForBody() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(200)
	}))
	defer srv.Close()

	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"method": "POST",
		"url":    srv.URL,
		"body":   map[string]any{"key": "value"},
	})

	_, err := a.Execute(ctx)
	s.NoError(err)
}

func (s *HTTPActionTestSuite) TestCustomContentTypeWithBody() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("text/plain", r.Header.Get("Content-Type"))
		w.WriteHeader(200)
	}))
	defer srv.Close()

	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"method": "POST",
		"url":    srv.URL,
		"body":   "plain text",
		"headers": map[string]any{
			"Content-Type": "text/plain",
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal(200, out.(map[string]any)["status"])
}

// parseConfigDuration: timeout is not a string (e.g. an int) returns 0, false.
func (s *HTTPActionTestSuite) TestParseConfigDuration_NonStringTimeout() {
	ctx := newTestContext(map[string]any{
		"url":     "http://example.com",
		"timeout": 42,
	})
	d, ok := parseConfigDuration(ctx)
	s.False(ok)
	s.Equal(time.Duration(0), d)
}

// parseConfigDuration: timeout is an invalid duration string returns 0, false.
func (s *HTTPActionTestSuite) TestParseConfigDuration_InvalidDurationString() {
	ctx := newTestContext(map[string]any{
		"url":     "http://example.com",
		"timeout": "not-a-duration",
	})
	d, ok := parseConfigDuration(ctx)
	s.False(ok)
	s.Equal(time.Duration(0), d)
}

// parseConfigDuration: timeout is a boolean (non-string) returns 0, false.
func (s *HTTPActionTestSuite) TestParseConfigDuration_BoolTimeout() {
	ctx := newTestContext(map[string]any{
		"url":     "http://example.com",
		"timeout": true,
	})
	d, ok := parseConfigDuration(ctx)
	s.False(ok)
	s.Equal(time.Duration(0), d)
}

// resolveHTTPTimeout: non-string timeout falls through to default 30s.
func (s *HTTPActionTestSuite) TestResolveHTTPTimeout_NonStringTimeoutUsesDefault() {
	ctx := newTestContext(map[string]any{
		"url":     "http://example.com",
		"timeout": 42,
	})
	d := resolveHTTPTimeout(ctx)
	s.Equal(30*time.Second, d)
}

// resolveHTTPTimeout: invalid duration string falls through to default 30s.
func (s *HTTPActionTestSuite) TestResolveHTTPTimeout_InvalidDurationUsesDefault() {
	ctx := newTestContext(map[string]any{
		"url":     "http://example.com",
		"timeout": "xyz",
	})
	d := resolveHTTPTimeout(ctx)
	s.Equal(30*time.Second, d)
}

func (s *HTTPActionTestSuite) TestReadResponseBodyError() {
	// Create a server that hijacks the connection and sends a partial response
	// with a Content-Length that exceeds the actual body, then abruptly closes
	// the connection. This triggers the io.ReadAll error path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		conn, bufrw, err := hj.Hijack()
		if err != nil {
			return
		}

		bufrw.WriteString("HTTP/1.1 200 OK\r\n")
		bufrw.WriteString("Content-Length: 1000000\r\n")
		bufrw.WriteString("\r\n")
		bufrw.WriteString("partial")
		bufrw.Flush()

		// Abruptly close with TCP RST
		tc, ok := conn.(*net.TCPConn)
		if ok {
			tc.SetLinger(0)
		}
		conn.Close()
	}))
	defer srv.Close()

	a := NewHTTPAction()
	ctx := newTestContext(map[string]any{
		"url":     srv.URL,
		"timeout": "5s",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "read response")
}

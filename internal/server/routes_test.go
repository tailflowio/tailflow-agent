package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type RoutesTestSuite struct {
	suite.Suite
}

func TestRoutes(t *testing.T) {
	suite.Run(t, new(RoutesTestSuite))
}

func (s *RoutesTestSuite) SetupTest() {}

func (s *RoutesTestSuite) TestSPARouting_Root() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Body.String(), "html")
}

func (s *RoutesTestSuite) TestSPARouting_FallbackToIndex() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/some/spa/route", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Body.String(), "html")
}

func (s *RoutesTestSuite) TestSPARouting_ApiNotFound() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/api/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

func (s *RoutesTestSuite) TestSPARouting_StaticAsset() {
	srv := newTestServer(s.T())

	// Request an existing static asset file
	req := httptest.NewRequest("GET", "/assets/DashboardView-jnNJTy3N.js", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Should serve the actual JS file
	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Header().Get("Content-Type"), "javascript")
}

func (s *RoutesTestSuite) TestHandleFallbackUI_Root() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.handleFallbackUI(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Header().Get("Content-Type"), "text/html")
	s.Contains(w.Body.String(), "TailSafe")
}

func (s *RoutesTestSuite) TestHandleFallbackUI_IndexHTML() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/index.html", nil)
	w := httptest.NewRecorder()
	srv.handleFallbackUI(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Body.String(), "TailSafe")
}

func (s *RoutesTestSuite) TestHandleFallbackUI_OtherPath_Returns404() {
	srv := newTestServer(s.T())

	req := httptest.NewRequest("GET", "/other/path", nil)
	w := httptest.NewRecorder()
	srv.handleFallbackUI(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

func (s *RoutesTestSuite) TestSetupUIRoutes_DistFSError_UsesFallback() {
	original := distSubFS
	s.T().Cleanup(func() { distSubFS = original })

	distSubFS = func() (fs.FS, error) {
		return nil, fs.ErrNotExist
	}

	// Create a fresh server that will call setupUIRoutes with the failing distSubFS
	srv := newTestServer(s.T())
	// The server already called setupUIRoutes during New(). We need to recreate routes.
	srv.mux = http.NewServeMux()
	srv.setupAPIRoutes()
	srv.setupUIRoutes()

	// Now "/" should hit the fallback handler
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Body.String(), "TailSafe")

	// Non-root, non-index path should be 404
	req = httptest.NewRequest("GET", "/something", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusNotFound, w.Code)
}

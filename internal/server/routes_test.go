package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/web"
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

	// Pick a JS asset dynamically — vite content-hashes filenames so we
	// can't hardcode one without breaking on every build.
	entries, err := web.DistFS.ReadDir("dist/assets")
	s.Require().NoError(err)

	var assetName string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".js") {
			assetName = e.Name()
			break
		}
	}
	s.Require().NotEmpty(assetName, "no js asset found in embedded dist")

	req := httptest.NewRequest("GET", "/assets/"+assetName, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

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

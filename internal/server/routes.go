package server

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/tailflow/tailflow/web"
)

// distSubFS returns the embedded UI filesystem. Override in tests.
var distSubFS = func() (fs.FS, error) { return fs.Sub(web.DistFS, "dist") }

func (s *Server) setupRoutes() {
	s.setupAPIRoutes()
	s.setupUIRoutes()
}

func (s *Server) setupAPIRoutes() {
	s.mux.HandleFunc("GET /api/workflow", s.handleGetWorkflow)
	s.mux.HandleFunc("GET /api/workflow/raw", s.handleGetWorkflowRaw)
	s.mux.HandleFunc("PUT /api/workflow/raw", s.handlePutWorkflowRaw)
	s.mux.HandleFunc("GET /api/workflow/graph", s.handleGetWorkflowGraph)
	s.mux.HandleFunc("GET /api/workflow/activity", s.handleGetWorkflowActivity)
	s.mux.HandleFunc("POST /api/workflow/validate", s.handleValidateWorkflow)
	s.mux.HandleFunc("POST /api/workflow/run", s.handleRunWorkflow)
	s.mux.HandleFunc("GET /api/workflow/steps/metrics", s.handleGetAllStepMetrics)
	s.mux.HandleFunc("GET /api/workflow/steps/{id}", s.handleGetStepDetail)

	s.mux.HandleFunc("GET /api/version", s.handleGetVersion)
	s.mux.HandleFunc("GET /api/metrics", s.handleGetMetrics)

	s.mux.HandleFunc("GET /api/executions", s.handleListExecutions)
	s.mux.HandleFunc("GET /api/executions/{id}", s.handleGetExecution)
	s.mux.HandleFunc("POST /api/executions/{id}/cancel", s.handleCancelExecution)
	s.mux.HandleFunc("GET /api/executions/{id}/events/list", s.handleListEvents)
	s.mux.HandleFunc("GET /api/executions/{id}/events", s.handleSSE)
	s.mux.HandleFunc("GET /api/events", s.handleGlobalSSE)

	s.mux.HandleFunc("/api/public/", s.handlePublicTrigger)
	s.mux.HandleFunc("/api/wait/", s.handleWaitWebhook)
}

func (s *Server) setupUIRoutes() {
	distFS, err := distSubFS()
	if err != nil {
		s.mux.HandleFunc("/", s.handleFallbackUI)
		return
	}

	fileServer := http.FileServer(http.FS(distFS))

	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		f, err := distFS.Open(strings.TrimPrefix(path, "/"))
		if err != nil {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)

			return
		}

		_ = f.Close()

		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) handleFallbackUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>TailSafe</title>
<style>
body{background:#0c0d12;color:#e4e5ed;font-family:Inter,-apple-system,sans-serif;
display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}
.c{text-align:center}h1{font-size:2rem;margin-bottom:0.5rem}
p{color:#8c8ea0;font-size:0.9rem}
code{background:#1a1b25;padding:4px 8px;border-radius:6px;font-size:0.85rem;color:#818cf8}
</style>
</head>
<body><div class="c"><h1>TailSafe</h1><p>Build the frontend:</p>
<p><code>cd web/frontend && npm run build</code></p></div></body>
</html>`))
}

package server

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/tailflow/tailflow/web"
)

func (s *Server) setupRoutes() {
	// API - Workflow (single workflow)
	s.mux.HandleFunc("GET /api/workflow", s.handleGetWorkflow)
	s.mux.HandleFunc("GET /api/workflow/graph", s.handleGetWorkflowGraph)
	s.mux.HandleFunc("GET /api/workflow/activity", s.handleGetWorkflowActivity)
	s.mux.HandleFunc("POST /api/workflow/validate", s.handleValidateWorkflow)
	s.mux.HandleFunc("POST /api/workflow/run", s.handleRunWorkflow)
	s.mux.HandleFunc("GET /api/workflow/steps/{id}", s.handleGetStepDetail)

	// API - Metrics
	s.mux.HandleFunc("GET /api/metrics", s.handleGetMetrics)

	// API - Executions
	s.mux.HandleFunc("GET /api/executions", s.handleListExecutions)
	s.mux.HandleFunc("GET /api/executions/{id}", s.handleGetExecution)
	s.mux.HandleFunc("POST /api/executions/{id}/cancel", s.handleCancelExecution)
	s.mux.HandleFunc("GET /api/executions/{id}/events", s.handleSSE)
	s.mux.HandleFunc("GET /api/events", s.handleGlobalSSE)

	// API - Public (triggers)
	s.mux.HandleFunc("/api/public/", s.handlePublicTrigger)

	// API - Wait webhook
	s.mux.HandleFunc("/api/wait/", s.handleWaitWebhook)

	// Embedded UI (SPA)
	distFS, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		// Fallback to simple HTML
		s.mux.HandleFunc("/", s.handleFallbackUI)
		return
	}

	fileServer := http.FileServer(http.FS(distFS))

	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// API routes are already handled above
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve static file
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Check if file exists in dist
		f, err := distFS.Open(strings.TrimPrefix(path, "/"))
		if err != nil {
			// SPA fallback: serve index.html for all non-file routes
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)

			return
		}

		f.Close()

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

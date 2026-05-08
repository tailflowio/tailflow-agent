package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tailflow/tailflow/internal/runtime"
)

func (s *Server) handleWaitWebhook(w http.ResponseWriter, r *http.Request) {
	// Parse URL: /api/wait/{executionID}/{path...}
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/wait/")

	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) < 2 {
		s.writeError(r.Context(), w, http.StatusBadRequest, "expected /api/wait/{executionID}/{path...}")
		return
	}

	executionID := parts[0]
	path := "/" + parts[1]

	var body any
	if r.Body != nil {
		decodeErr := json.NewDecoder(r.Body).Decode(&body)
		if decodeErr != nil {
			s.config.Logger.DebugContext(r.Context(), "failed to decode wait request body", "error", decodeErr)
		}
	}

	req := runtime.WaitRequest{
		Method:  r.Method,
		Path:    path,
		Headers: headerMap(r.Header),
		Query:   r.URL.Query(),
		Body:    body,
	}

	err := s.waitRegistry.Deliver(executionID, path, req)
	if err != nil {
		s.writeError(r.Context(), w, http.StatusNotFound, err.Error())
		return
	}

	s.writeJSON(r.Context(), w, http.StatusOK, map[string]any{"delivered": true})
}

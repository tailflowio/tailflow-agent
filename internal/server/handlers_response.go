package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/tailflow/tailflow/pkg/api"
)

func (s *Server) writeJSON(ctx context.Context, w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		s.config.Logger.WarnContext(ctx, "failed to write JSON response", "error", err)
	}
}

func (s *Server) writeError(ctx context.Context, w http.ResponseWriter, status int, msg string) {
	s.writeJSON(ctx, w, status, api.ErrorResponse{Error: msg})
}

func headerMap(h http.Header) map[string]string {
	headers := make(map[string]string, len(h))
	for k := range h {
		headers[k] = h.Get(k)
	}

	return headers
}

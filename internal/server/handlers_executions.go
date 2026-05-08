package server

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	execs, err := s.config.ExecutionStore.List(r.Context()) // newest-first
	if err != nil {
		s.writeError(r.Context(), w, http.StatusInternalServerError, err.Error())
		return
	}

	execs = filterByStatus(execs, r.URL.Query().Get("status"))
	total := len(execs)

	sortExecutions(execs, r.URL.Query().Get("sort"), r.URL.Query().Get("order"))

	paged := paginateExecutions(execs, r)

	s.writeJSON(r.Context(), w, http.StatusOK, map[string]any{"items": paged, "total": total})
}

func filterByStatus(execs []*store.Execution, statusFilter string) []*store.Execution {
	if statusFilter == "" {
		return execs
	}

	statuses := strings.Split(statusFilter, ",")
	statusSet := make(map[string]bool, len(statuses))

	for _, st := range statuses {
		statusSet[st] = true
	}

	filtered := make([]*store.Execution, 0, len(execs))

	for _, e := range execs {
		if statusSet[e.Status] {
			filtered = append(filtered, e)
		}
	}

	return filtered
}

func sortExecutions(execs []*store.Execution, sortBy, order string) {
	if sortBy == "duration" {
		sort.Slice(execs, func(i, j int) bool {
			di := execDuration(execs[i])
			dj := execDuration(execs[j])

			return di > dj // desc by default
		})
	}
	// date sort = default order from store (newest first = desc)

	if order == "asc" {
		slices.Reverse(execs)
	}
}

func paginateExecutions(execs []*store.Execution, r *http.Request) []*store.Execution {
	offset := parseIntParam(r, "offset", 0)
	limit := parseIntParam(r, "limit", 20)

	if offset > len(execs) {
		offset = len(execs)
	}

	end := offset + limit
	if end > len(execs) {
		end = len(execs)
	}

	return execs[offset:end]
}

func execDuration(e *store.Execution) time.Duration {
	if e.FinishedAt == nil {
		return 0
	}

	return e.FinishedAt.Sub(e.StartedAt)
}

func parseIntParam(r *http.Request, name string, defaultVal int) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return defaultVal
	}

	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}

	return v
}

func (s *Server) handleGetExecution(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	exec, err := s.config.ExecutionStore.Get(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		s.writeError(r.Context(), w, status, err.Error())
		return
	}

	s.writeJSON(r.Context(), w, http.StatusOK, exec)
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")

	offset, _ := strconv.Atoi(offsetStr)
	limit, _ := strconv.Atoi(limitStr)

	if limit <= 0 || limit > 200 {
		limit = 50
	}

	if offset < 0 {
		offset = 0
	}

	allEvents, err := s.config.ExecutionStore.GetEvents(r.Context(), id)
	if err != nil {
		s.writeError(r.Context(), w, http.StatusInternalServerError, err.Error())
		return
	}

	total := len(allEvents)

	// Paginate from the end (newest first)
	start := total - offset - limit
	end := total - offset

	if start < 0 {
		start = 0
	}

	if end < 0 {
		end = 0
	}

	page := allEvents[start:end]

	// Reverse the page so newest is first
	for i, j := 0, len(page)-1; i < j; i, j = i+1, j-1 {
		page[i], page[j] = page[j], page[i]
	}

	s.writeJSON(r.Context(), w, http.StatusOK, map[string]any{
		"events":  page,
		"total":   total,
		"offset":  offset,
		"limit":   limit,
		"hasMore": offset+limit < total,
	})
}

func (s *Server) handleCancelExecution(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	exec, err := s.config.ExecutionStore.Get(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		s.writeError(r.Context(), w, status, err.Error())
		return
	}

	if exec.Status != runtime.StatusRunning && exec.Status != runtime.StatusWaiting {
		s.writeError(r.Context(), w, http.StatusConflict, fmt.Sprintf("execution is %s, not cancellable", exec.Status))
		return
	}

	if !s.cancelExecution(id) {
		s.writeError(r.Context(), w, http.StatusNotFound, "execution cancel function not found")
		return
	}

	s.writeJSON(r.Context(), w, http.StatusOK, map[string]any{"cancelled": true})
}

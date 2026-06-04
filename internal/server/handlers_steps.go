package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

func (s *Server) handleGetAllStepMetrics(w http.ResponseWriter, r *http.Request) {
	allMetrics, err := s.config.ExecutionStore.GetAllStepMetrics(r.Context())
	if err != nil {
		s.writeError(r.Context(), w, http.StatusInternalServerError, err.Error())
		return
	}

	execs, err := s.config.ExecutionStore.List(r.Context())
	if err != nil {
		s.writeError(r.Context(), w, http.StatusInternalServerError, err.Error())
		return
	}

	type stepStats struct {
		TotalExecutions int      `json:"total_executions"`
		SuccessCount    int      `json:"success_count"`
		FailureCount    int      `json:"failure_count"`
		AvgDurationMs   int64    `json:"avg_duration_ms"`
		History         []string `json:"history"`
	}

	result := make(map[string]*stepStats, len(allMetrics))

	for stepID, m := range allMetrics {
		result[stepID] = &stepStats{
			TotalExecutions: m.TotalExecutions,
			SuccessCount:    m.SuccessCount,
			FailureCount:    m.FailureCount,
			AvgDurationMs:   m.AvgDurationMs,
			History:         []string{},
		}
	}

	maxHistory := 20

	start := 0
	if len(execs) > maxHistory {
		start = len(execs) - maxHistory
	}

	for i := start; i < len(execs); i++ {
		exec := execs[i]
		if exec.Steps == nil {
			continue
		}

		for stepID, sr := range exec.Steps {
			st := result[stepID]
			if st == nil {
				st = &stepStats{History: []string{}}
				result[stepID] = st
			}

			st.History = append(st.History, sr.Status)
		}
	}

	s.writeJSON(r.Context(), w, http.StatusOK, result)
}

func (s *Server) handleGetStepDetail(w http.ResponseWriter, r *http.Request) {
	stepID := r.PathValue("id")

	step := s.findStep(stepID)
	if step == nil {
		s.writeError(r.Context(), w, http.StatusNotFound, fmt.Sprintf("step %q not found", stepID))
		return
	}

	history, err := s.buildStepHistory(r.Context(), stepID)
	if err != nil {
		s.writeError(r.Context(), w, http.StatusInternalServerError, err.Error())
		return
	}

	metrics, err := s.config.ExecutionStore.GetStepMetrics(r.Context(), stepID)
	if err != nil {
		s.writeError(r.Context(), w, http.StatusInternalServerError, err.Error())
		return
	}

	if metrics == nil {
		metrics = &store.StepMetrics{}
	}

	s.writeJSON(r.Context(), w, http.StatusOK, map[string]any{
		"step":    step,
		"history": history,
		"metrics": metrics,
	})
}

func (s *Server) findStep(stepID string) *parser.Step {
	for i := range s.config.Workflow.Steps {
		if s.config.Workflow.Steps[i].ID == stepID {
			return &s.config.Workflow.Steps[i]
		}
	}

	return nil
}

type stepHistoryEntry struct {
	ExecutionID string `json:"execution_id"`
	Status      string `json:"status"`
	StartedAt   string `json:"started_at"`
	FinishedAt  string `json:"finished_at,omitempty"`
	DurationMs  int64  `json:"duration_ms"`
	Output      any    `json:"output,omitempty"`
	Error       string `json:"error,omitempty"`
}

func (s *Server) buildStepHistory(ctx context.Context, stepID string) ([]stepHistoryEntry, error) {
	execs, err := s.config.ExecutionStore.List(ctx)
	if err != nil {
		return nil, err
	}

	history := make([]stepHistoryEntry, 0, len(execs))

	for _, exec := range execs {
		sr, ok := exec.Steps[stepID]
		if !ok {
			continue
		}

		entry := buildHistoryEntry(exec, sr)
		history = append(history, entry)
	}

	return history, nil
}

func buildHistoryEntry(exec *store.Execution, sr *runtime.StepResult) stepHistoryEntry {
	var errStr string
	if sr.Error != nil {
		errStr = sr.Error.Message
	}

	entry := stepHistoryEntry{
		ExecutionID: exec.ID,
		Status:      sr.Status,
		Output:      sr.Output,
		Error:       errStr,
	}

	entry.StartedAt = exec.StartedAt.Format(time.RFC3339)
	if sr.StartedAt != nil {
		entry.StartedAt = sr.StartedAt.Format(time.RFC3339)
	}

	switch {
	case sr.StartedAt != nil && sr.FinishedAt != nil:
		entry.FinishedAt = sr.FinishedAt.Format(time.RFC3339)
		entry.DurationMs = sr.FinishedAt.Sub(*sr.StartedAt).Milliseconds()
	case exec.FinishedAt != nil:
		entry.FinishedAt = exec.FinishedAt.Format(time.RFC3339)
	}

	return entry
}

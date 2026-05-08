package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/pkg/api"
)

func (s *Server) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(r.Context(), w, http.StatusOK, map[string]any{
		"version":        s.config.Version,
		"editor_enabled": s.config.EditorEnabled,
		"workflow_file":  s.config.FilePath,
	})
}

func (s *Server) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(r.Context(), w, http.StatusOK, s.metrics.Snapshot())
}

func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	wf := s.config.Workflow

	// Wrap response with next_run if cron trigger is configured
	if wf.Trigger != nil && wf.Trigger.Schedule != nil {
		nextRun := NextRun(wf.Trigger.Schedule.Cron)
		resp := struct {
			*parser.Workflow
			NextRun *time.Time `json:"next_run,omitempty"`
		}{Workflow: wf, NextRun: nextRun}
		s.writeJSON(r.Context(), w, http.StatusOK, resp)

		return
	}

	s.writeJSON(r.Context(), w, http.StatusOK, wf)
}

func (s *Server) handleGetWorkflowRaw(w http.ResponseWriter, r *http.Request) {
	if s.config.FilePath == "" {
		s.writeError(r.Context(), w, http.StatusNotFound, "no workflow file path")
		return
	}
	data, err := os.ReadFile(s.config.FilePath)
	if err != nil {
		s.writeError(r.Context(), w, http.StatusInternalServerError, "read failed: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) handlePutWorkflowRaw(w http.ResponseWriter, r *http.Request) {
	if !s.config.EditorEnabled {
		s.writeError(r.Context(), w, http.StatusForbidden, "editor disabled — restart with --editor")
		return
	}
	if s.config.FilePath == "" {
		s.writeError(r.Context(), w, http.StatusNotFound, "no workflow file path")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(r.Context(), w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	defer func() { _ = r.Body.Close() }()
	if len(body) == 0 {
		s.writeError(r.Context(), w, http.StatusBadRequest, "empty body")
		return
	}

	wf, parseErr := parser.ParseBytes(body)
	if parseErr != nil {
		s.writeJSON(r.Context(), w, http.StatusBadRequest, api.ValidateResponse{Valid: false, Errors: []string{parseErr.Error()}})
		return
	}
	if validateErr := parser.Validate(wf); validateErr != nil {
		s.writeJSON(r.Context(), w, http.StatusBadRequest, api.ValidateResponse{Valid: false, Errors: []string{validateErr.Error()}})
		return
	}

	if writeErr := os.WriteFile(s.config.FilePath, body, 0o644); writeErr != nil {
		s.writeError(r.Context(), w, http.StatusInternalServerError, "write failed: "+writeErr.Error())
		return
	}

	s.config.Workflow = wf
	s.writeJSON(r.Context(), w, http.StatusOK, api.ValidateResponse{Valid: true})
}

func (s *Server) handleGetWorkflowGraph(w http.ResponseWriter, r *http.Request) {
	dag, err := engine.BuildDAG(s.config.Workflow.Steps)
	if err != nil {
		s.writeError(r.Context(), w, http.StatusInternalServerError, err.Error())
		return
	}

	graph := buildGraph(s.config.Workflow, dag)
	s.writeJSON(r.Context(), w, http.StatusOK, graph)
}

func (s *Server) handleValidateWorkflow(w http.ResponseWriter, r *http.Request) {
	err := parser.Validate(s.config.Workflow)
	if err != nil {
		s.writeJSON(r.Context(), w, http.StatusOK, api.ValidateResponse{Valid: false, Errors: []string{err.Error()}})
		return
	}

	s.writeJSON(r.Context(), w, http.StatusOK, api.ValidateResponse{Valid: true})
}

//nolint:contextcheck // intentional: async execution outlives request
func (s *Server) handleRunWorkflow(w http.ResponseWriter, r *http.Request) {
	var req api.RunRequest
	if r.Body != nil {
		decodeErr := json.NewDecoder(r.Body).Decode(&req)
		if decodeErr != nil {
			s.config.Logger.DebugContext(r.Context(), "failed to decode run request body", "error", decodeErr)
		}
	}

	executionID := s.runWorkflowAsync(req.Params)

	s.writeJSON(r.Context(), w, http.StatusAccepted, api.RunResponse{
		ExecutionID: executionID,
		Status:      runtime.StatusRunning,
	})
}

func (s *Server) handleGetWorkflowActivity(w http.ResponseWriter, r *http.Request) {
	type stepActivity struct {
		Running []string `json:"running"`
		Waiting []string `json:"waiting"`
	}

	execs := s.config.ExecutionStore.List()
	activity := make(map[string]*stepActivity)

	for _, exec := range execs {
		if exec.Status != runtime.StatusRunning && exec.Status != runtime.StatusWaiting {
			continue
		}

		for stepID, step := range exec.Steps {
			if step.Status != runtime.StatusRunning && step.Status != runtime.StatusWaiting {
				continue
			}

			if activity[stepID] == nil {
				activity[stepID] = &stepActivity{}
			}

			if step.Status == runtime.StatusRunning {
				activity[stepID].Running = append(activity[stepID].Running, exec.ID)

				continue
			}

			activity[stepID].Waiting = append(activity[stepID].Waiting, exec.ID)
		}
	}

	s.writeJSON(r.Context(), w, http.StatusOK, map[string]any{
		"steps":       activity,
		"exec_counts": s.config.ExecutionStore.StepExecCounts(),
	})
}

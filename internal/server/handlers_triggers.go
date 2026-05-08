package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

func (s *Server) handlePublicTrigger(w http.ResponseWriter, r *http.Request) {
	// Match against the single workflow's trigger
	path := r.URL.Path[len("/api/public"):]
	wf := s.config.Workflow

	if wf.Trigger == nil {
		s.writeError(r.Context(), w, http.StatusNotFound, fmt.Sprintf("no workflow matches %s %s", r.Method, path))
		return
	}

	matched := (wf.Trigger.HTTP != nil && wf.Trigger.HTTP.Path == path && r.Method == wf.Trigger.HTTP.Method) ||
		(wf.Trigger.Webhook != nil && wf.Trigger.Webhook.Path == path)

	if !matched {
		s.writeError(r.Context(), w, http.StatusNotFound, fmt.Sprintf("no workflow matches %s %s", r.Method, path))
		return
	}

	s.executeTriggerWorkflow(w, r, wf)
}

//nolint:contextcheck // execution context derives from s.ctx, intentionally outlives request
func (s *Server) executeTriggerWorkflow(w http.ResponseWriter, r *http.Request, wf *parser.Workflow) {
	triggerData, params := s.buildTriggerData(r)

	if s.handleIdempotencyCheck(w, r, wf, triggerData, params) {
		return
	}

	executionID, opts, stopCapture := s.prepareTriggerExecution(wf, triggerData, params)

	execCtx, cancel := context.WithCancel(s.ctx)
	s.registerCancel(executionID, cancel)

	if wf.Trigger.HTTP != nil && wf.Trigger.HTTP.Async {
		s.runTriggerAsync(executionID, wf, params, opts, execCtx, stopCapture)
		s.writeJSON(r.Context(), w, http.StatusAccepted, map[string]any{
			"execution_id": executionID,
			"status":       runtime.StatusRunning,
		})

		return
	}

	s.runTriggerSync(w, wf, params, opts, execCtx, executionID, stopCapture)
}

func (s *Server) handleIdempotencyCheck(
	w http.ResponseWriter, r *http.Request,
	wf *parser.Workflow, triggerData, params map[string]any,
) bool {
	if wf.Trigger == nil || wf.Trigger.HTTP == nil || wf.Trigger.HTTP.IdempotencyKey == "" {
		return false
	}

	eval := runtime.NewExprEvaluator()
	ctx := map[string]any{
		"trigger": triggerData,
		"params":  params,
	}

	resolvedKey, evalErr := eval.ResolveTemplate(wf.Trigger.HTTP.IdempotencyKey, ctx)
	if evalErr != nil || resolvedKey == "" {
		return false
	}

	claimResult, claimErr := s.config.Claimer.ClaimExecution(r.Context(), uuid.New().String(), wf.Name, resolvedKey)
	if claimErr != nil {
		s.config.Logger.Warn("idempotency claim failed, proceeding with execution", "error", claimErr)
		return false
	}

	if claimResult.Claimed {
		return false
	}

	s.writeJSON(r.Context(), w, http.StatusOK, map[string]any{
		"execution_id": claimResult.ExistingExecutionID,
		"status":       claimResult.ExistingStatus,
		"deduplicated": true,
	})

	return true
}

func (s *Server) buildTriggerData(r *http.Request) (map[string]any, map[string]any) {
	var body any
	if r.Body != nil {
		decodeErr := json.NewDecoder(r.Body).Decode(&body)
		if decodeErr != nil {
			s.config.Logger.DebugContext(r.Context(), "failed to decode trigger request body", "error", decodeErr)
		}
	}

	params := make(map[string]any)
	triggerData := map[string]any{
		"method":  r.Method,
		"path":    r.URL.Path,
		"query":   r.URL.Query(),
		"body":    body,
		"headers": headerMap(r.Header),
	}

	return triggerData, params
}

func (s *Server) prepareTriggerExecution(
	wf *parser.Workflow, triggerData, params map[string]any,
) (string, engine.ExecuteOptions, func()) {
	executionID := uuid.New().String()
	exec := &store.Execution{
		ID:           executionID,
		WorkflowName: wf.Name,
		Status:       runtime.StatusRunning,
		Params:       s.sensitive.MaskMap(params),
		StartedAt:    time.Now(),
	}
	s.config.ExecutionStore.Add(exec)

	stopCapture := s.captureEvents(executionID)
	services := s.buildActionServices()

	opts := engine.ExecuteOptions{
		ExecutionID: executionID,
		TriggerData: triggerData,
		Services:    services,
	}

	return executionID, opts, stopCapture
}

func (s *Server) runTriggerAsync(
	executionID string, wf *parser.Workflow, params map[string]any,
	opts engine.ExecuteOptions, execCtx context.Context, stopCapture func(),
) {
	go func() {
		defer s.unregisterCancel(executionID)

		result, err := s.config.Executor.Execute(execCtx, wf, params, opts)

		stopCapture()
		s.finalizeExecution(executionID, result, err, execCtx)
		s.ensureWorkflowCompleted(executionID, result, err, execCtx)
	}()
}

func (s *Server) runTriggerSync(
	w http.ResponseWriter, wf *parser.Workflow, params map[string]any,
	opts engine.ExecuteOptions, execCtx context.Context, executionID string, stopCapture func(),
) {
	defer s.unregisterCancel(executionID)

	result, err := s.config.Executor.Execute(execCtx, wf, params, opts)

	stopCapture()

	s.finalizeExecution(executionID, result, err, execCtx)
	s.ensureWorkflowCompleted(executionID, result, err, execCtx)
	s.writeTriggerResponse(w, wf, result, err, execCtx, executionID)
}

func (s *Server) writeTriggerResponse(
	w http.ResponseWriter, wf *parser.Workflow, result *engine.ExecuteResult,
	err error, execCtx context.Context, executionID string,
) {
	if err != nil {
		if execCtx.Err() != nil {
			s.writeJSON(execCtx, w, http.StatusOK, map[string]any{"status": runtime.StatusCancelled, "execution_id": executionID})
			return
		}

		s.writeError(execCtx, w, http.StatusInternalServerError, err.Error())

		return
	}

	outMap := findResponseStepOutput(wf, result)
	if outMap != nil {
		writeResponseStepOutput(w, outMap)
		s.writeJSON(execCtx, w, responseStatus(outMap), outMap["body"])

		return
	}

	s.writeJSON(execCtx, w, http.StatusOK, map[string]any{
		"status":       result.Status,
		"execution_id": executionID,
	})
}

func findResponseStepOutput(wf *parser.Workflow, result *engine.ExecuteResult) map[string]any {
	for _, step := range wf.Steps {
		if step.Action != "response" {
			continue
		}

		sr, ok := result.Steps[step.ID]
		if !ok || sr.Output == nil {
			continue
		}

		outMap, ok := sr.Output.(map[string]any)
		if ok {
			return outMap
		}
	}

	return nil
}

func responseStatus(outMap map[string]any) int {
	s, ok := outMap["status"].(int)
	if ok {
		return s
	}

	return http.StatusOK
}

func writeResponseStepOutput(w http.ResponseWriter, outMap map[string]any) {
	h, ok := outMap["headers"].(map[string]string)
	if !ok {
		return
	}

	for k, v := range h {
		w.Header().Set(k, v)
	}
}

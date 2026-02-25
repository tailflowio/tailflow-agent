package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
	"github.com/tailflow/tailflow/pkg/api"
	"github.com/tailflow/tailflow/pkg/workflow"
)

func (s *Server) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.metrics.Snapshot())
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
		writeJSON(w, http.StatusOK, resp)
		return
	}

	writeJSON(w, http.StatusOK, wf)
}

func (s *Server) handleGetWorkflowGraph(w http.ResponseWriter, r *http.Request) {
	dag, err := engine.BuildDAG(s.config.Workflow.Steps)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	graph := buildGraph(s.config.Workflow, dag)
	writeJSON(w, http.StatusOK, graph)
}

func (s *Server) handleValidateWorkflow(w http.ResponseWriter, r *http.Request) {
	err := parser.Validate(s.config.Workflow)
	if err != nil {
		writeJSON(w, http.StatusOK, api.ValidateResponse{Valid: false, Errors: []string{err.Error()}})
		return
	}

	writeJSON(w, http.StatusOK, api.ValidateResponse{Valid: true})
}

//nolint:contextcheck // intentional: async execution outlives request
func (s *Server) handleRunWorkflow(w http.ResponseWriter, r *http.Request) {
	var req api.RunRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	executionID := s.runWorkflowAsync(req.Params)

	writeJSON(w, http.StatusAccepted, api.RunResponse{
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
			} else {
				activity[stepID].Waiting = append(activity[stepID].Waiting, exec.ID)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"steps":      activity,
		"exec_counts": s.config.ExecutionStore.StepExecCounts(),
	})
}

func (s *Server) handleGetStepDetail(w http.ResponseWriter, r *http.Request) {
	stepID := r.PathValue("id")

	// Find the step in the workflow definition
	var step *parser.Step

	for i := range s.config.Workflow.Steps {
		if s.config.Workflow.Steps[i].ID == stepID {
			step = &s.config.Workflow.Steps[i]

			break
		}
	}

	if step == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("step %q not found", stepID))
		return
	}

	// Build history from stored executions
	execs := s.config.ExecutionStore.List()

	type historyEntry struct {
		ExecutionID string `json:"execution_id"`
		Status      string `json:"status"`
		StartedAt   string `json:"started_at"`
		FinishedAt  string `json:"finished_at,omitempty"`
		DurationMs  int64  `json:"duration_ms"`
		Output      any    `json:"output,omitempty"`
		Error       string `json:"error,omitempty"`
	}

	history := make([]historyEntry, 0, len(execs))

	for _, exec := range execs {
		sr, ok := exec.Steps[stepID]
		if !ok {
			continue
		}

		var errStr string
		if sr.Error != nil {
			errStr = sr.Error.Message
		}
		entry := historyEntry{
			ExecutionID: exec.ID,
			Status:      sr.Status,
			Output:      sr.Output,
			Error:       errStr,
		}

		if sr.StartedAt != nil {
			entry.StartedAt = sr.StartedAt.Format(time.RFC3339)
		} else {
			entry.StartedAt = exec.StartedAt.Format(time.RFC3339)
		}

		if sr.StartedAt != nil && sr.FinishedAt != nil {
			entry.FinishedAt = sr.FinishedAt.Format(time.RFC3339)
			entry.DurationMs = sr.FinishedAt.Sub(*sr.StartedAt).Milliseconds()
		} else if exec.FinishedAt != nil {
			entry.FinishedAt = exec.FinishedAt.Format(time.RFC3339)
		}

		history = append(history, entry)
	}

	// Read cached metrics (computed every 5s by background goroutine)
	metrics := s.config.ExecutionStore.GetStepMetrics(stepID)
	if metrics == nil {
		metrics = &store.StepMetrics{}
	}

	resp := map[string]any{
		"step":    step,
		"history": history,
		"metrics": metrics,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	execs := s.config.ExecutionStore.List() // newest-first

	// Filter by status (comma-separated)
	if statusFilter := r.URL.Query().Get("status"); statusFilter != "" {
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
		execs = filtered
	}

	total := len(execs)

	// Sort
	sortBy := r.URL.Query().Get("sort")  // "date" (default) or "duration"
	order := r.URL.Query().Get("order")   // "desc" (default) or "asc"

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

	// Pagination
	offset := parseIntParam(r, "offset", 0)
	limit := parseIntParam(r, "limit", 20)

	if offset > len(execs) {
		offset = len(execs)
	}
	end := offset + limit
	if end > len(execs) {
		end = len(execs)
	}
	paged := execs[offset:end]

	writeJSON(w, http.StatusOK, map[string]any{"items": paged, "total": total})
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

	exec, err := s.config.ExecutionStore.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, exec)
}

func (s *Server) handleCancelExecution(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	exec, err := s.config.ExecutionStore.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if exec.Status != runtime.StatusRunning && exec.Status != runtime.StatusWaiting {
		writeError(w, http.StatusConflict, fmt.Sprintf("execution is %s, not cancellable", exec.Status))
		return
	}

	if !s.cancelExecution(id) {
		writeError(w, http.StatusNotFound, "execution cancel function not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
}

func (s *Server) handlePublicTrigger(w http.ResponseWriter, r *http.Request) {
	// Match against the single workflow's trigger
	path := r.URL.Path[len("/api/public"):]
	wf := s.config.Workflow

	if wf.Trigger == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no workflow matches %s %s", r.Method, path))
		return
	}

	matched := false
	if wf.Trigger.HTTP != nil && wf.Trigger.HTTP.Path == path && r.Method == wf.Trigger.HTTP.Method {
		matched = true
	}

	if wf.Trigger.Webhook != nil && wf.Trigger.Webhook.Path == path {
		matched = true
	}

	if !matched {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no workflow matches %s %s", r.Method, path))
		return
	}

	s.executeTriggerWorkflow(w, r, wf) //nolint:contextcheck // intentional: trigger execution may outlive request
}

func (s *Server) executeTriggerWorkflow(w http.ResponseWriter, r *http.Request, wf *parser.Workflow) {
	var body any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	params := make(map[string]any)
	triggerData := map[string]any{
		"method": r.Method,
		"path":   r.URL.Path,
		"query":  r.URL.Query(),
		"body":   body,
	}

	triggerData["headers"] = headerMap(r.Header)

	// Pre-generate execution ID and store immediately
	executionID := uuid.New().String()
	exec := &store.Execution{
		ID:           executionID,
		WorkflowName: wf.Name,
		Status:       runtime.StatusRunning,
		Params:       params,
		StartedAt:    time.Now(),
	}
	s.config.ExecutionStore.Add(exec)

	stopCapture := s.captureEvents(executionID)

	// Build services for wait actions
	services := s.buildActionServices()

	opts := engine.ExecuteOptions{
		ExecutionID: executionID,
		TriggerData: triggerData,
		Services:    services,
	}

	// Cancellable context for this execution
	execCtx, cancel := context.WithCancel(context.Background())
	s.registerCancel(executionID, cancel)

	// Async mode: return 202 immediately, run in background
	if wf.Trigger.HTTP != nil && wf.Trigger.HTTP.Async {
		go func() {
			defer stopCapture()
			defer s.unregisterCancel(executionID)

			result, err := s.config.Executor.Execute(execCtx, wf, params, opts)
			s.finalizeExecution(executionID, result, err, execCtx)
		}()

		writeJSON(w, http.StatusAccepted, map[string]any{
			"execution_id": executionID,
			"status":       runtime.StatusRunning,
		})

		return
	}

	// Sync mode: block until workflow completes
	defer s.unregisterCancel(executionID)
	result, err := s.config.Executor.Execute(execCtx, wf, params, opts)

	stopCapture()

	s.finalizeExecution(executionID, result, err, execCtx)

	if err != nil {
		if execCtx.Err() != nil {
			writeJSON(w, http.StatusOK, map[string]any{"status": runtime.StatusCancelled, "execution_id": executionID})
			return
		}

		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	// Check for response action output
	for _, step := range wf.Steps {
		if step.Action == "response" {
			if sr, ok := result.Steps[step.ID]; ok && sr.Output != nil {
				if outMap, ok := sr.Output.(map[string]any); ok {
					status := 200
					if s, ok := outMap["status"].(int); ok {
						status = s
					}

					if h, ok := outMap["headers"].(map[string]string); ok {
						for k, v := range h {
							w.Header().Set(k, v)
						}
					}

					writeJSON(w, status, outMap["body"])

					return
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":       result.Status,
		"execution_id": executionID,
	})
}

func (s *Server) handleWaitWebhook(w http.ResponseWriter, r *http.Request) {
	// Parse URL: /api/wait/{executionID}/{path...}
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/wait/")

	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "expected /api/wait/{executionID}/{path...}")
		return
	}

	executionID := parts[0]
	path := "/" + parts[1]

	var body any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
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
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"delivered": true})
}

func buildGraph(wf *parser.Workflow, _ *engine.DAG) workflow.Graph {
	graph := workflow.Graph{
		Nodes: []workflow.GraphNode{},
		Edges: []workflow.GraphEdge{},
	}

	for _, step := range wf.Steps {
		label := step.Title
		if label == "" {
			label = step.ID
		}

		node := workflow.GraphNode{
			ID:     step.ID,
			Label:  label,
			Action: step.Action,
			Type:   "step",
			When:   step.When,
		}

		// Extract pipeline sub-actions for loop steps
		if step.Action == "loop" {
			if rawActions, ok := step.Config["actions"]; ok {
				if arr, ok := rawActions.([]any); ok {
					for _, item := range arr {
						if m, ok := item.(map[string]any); ok {
							actName, _ := m["action"].(string)
							actTitle, _ := m["title"].(string)

							if actName != "" {
								node.Pipeline = append(node.Pipeline, workflow.PipelineAction{
									Action: actName,
									Title:  actTitle,
								})
							}
						}
					}
				}
			}
		}

		graph.Nodes = append(graph.Nodes, node)
		for _, dep := range step.DependsOn {
			edge := workflow.GraphEdge{
				Source: dep,
				Target: step.ID,
			}
			// Only mark the edge as "when" if the source step is referenced
			// in the when expression (e.g. "steps.check_changed.output...")
			if step.When != "" && strings.Contains(step.When, "steps."+dep+".") {
				edge.Type = "when"
				edge.Label = step.When
			}
			graph.Edges = append(graph.Edges, edge)
		}

		if step.Goto != nil {
			graph.Edges = append(graph.Edges, workflow.GraphEdge{
				Source: step.ID,
				Target: step.Goto.Target,
				Type:   "goto",
				Label:  step.Goto.When,
			})
		}
	}

	return graph
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data) //nolint:errchkjson // HTTP response writer
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, api.ErrorResponse{Error: msg})
}

// buildActionServices creates the ActionServices wired to this server's
// event bus and wait registry.
func (s *Server) buildActionServices() *runtime.ActionServices {
	return &runtime.ActionServices{
		WaitWebhookRegister:  s.waitRegistry.Register,
		WaitRabbitMQRegister: s.rmqWaitMgr.Register,
		EmitWaiting: func(executionID, stepID string, waitType string, details map[string]any) {
			data := map[string]any{"wait_type": waitType}
			for k, v := range details {
				data[k] = v
			}
			s.config.EventBus.Publish(event.Event{
				Type:        event.StepWaiting,
				Timestamp:   time.Now(),
				ExecutionID: executionID,
				StepID:      stepID,
				Message:     fmt.Sprintf("step %q waiting for %s", stepID, waitType),
				Data:        data,
			})
		},
		ScheduleExecution: func(delay time.Duration, params map[string]any) (string, error) {
			executionID := uuid.New().String()
			exec := &store.Execution{
				ID:           executionID,
				WorkflowName: s.config.Workflow.Name,
				Status:       runtime.StatusScheduled,
				Params:       params,
				StartedAt:    time.Now(),
			}
			s.config.ExecutionStore.Add(exec)

			timer := time.AfterFunc(delay, func() {
				s.runWorkflowAsync(params)
			})
			s.addScheduledTimer(timer)

			return executionID, nil
		},
		Locker:     s.locker,
		DBPool:     s.dbPool,
		KVStore:    s.kvStore,
		TxRegistry: runtime.NewMemoryTxRegistry(),
	}
}

// captureEvents subscribes to the event bus, stores events and updates
// step state in real-time for a given execution. Returns a stop function.
func (s *Server) captureEvents(executionID string) func() {
	ch := s.config.EventBus.Subscribe(200)
	done := make(chan struct{})

	go func() {
		defer close(done)

		lt := &loopTracker{}

		for ev := range ch {
			if ev.ExecutionID != executionID {
				continue
			}

			lt.Track(ev)

			// Reset body steps to pending in the store on goto so
			// dashboard step dots stay coherent during loops.
			if ev.Type == event.StepGoto && lt.Body != nil {
				for sid := range lt.Body {
					s.config.ExecutionStore.UpdateStep(executionID, sid, func(r *runtime.StepResult) {
						r.Status = "pending"
					})
				}
			}

			// Store event — skip loop body events after iteration 1
			// to prevent unbounded memory growth. step.goto events are
			// always stored so the frontend can track iteration count.
			if !lt.InLoop(ev) {
				s.config.ExecutionStore.AppendEvent(executionID, ev)
			}

			// Update step state in real-time (always, even during loops)
			if ev.StepID == "" {
				continue
			}

			switch ev.Type {
			case event.StepStarted:
				s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
					r.Status = runtime.StatusRunning
				})
				s.config.ExecutionStore.IncrStepExecCount(ev.StepID)
			case event.StepWaiting:
				s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
					r.Status = runtime.StatusWaiting
				})
			case event.StepInput:
				s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
					r.Input = ev.Data
				})
			case event.StepCompleted:
				s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
					r.Status = runtime.StatusSuccess
					if o, ok := ev.Data["output"]; ok {
						r.Output = o
					}
				})
			case event.StepFailed:
				s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
					r.Status = runtime.StatusFailed
					r.Error = &runtime.StepError{Message: ev.Message, Code: "action_failed", StepID: ev.StepID}
				})
			case event.StepSkipped:
				s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
					r.Status = runtime.StatusSkipped
				})
			case event.StepOutput:
				s.config.ExecutionStore.UpdateStep(executionID, ev.StepID, func(r *runtime.StepResult) {
					if o, ok := ev.Data["output"]; ok {
						r.Output = o
					}
				})
			case event.WorkflowCompleted:
				// Update execution status atomically so any GET request
				// arriving before finalizeExecution sees the correct
				// terminal status.
				if statusStr, ok := ev.Data["status"].(string); ok {
					ts := ev.Timestamp
					s.config.ExecutionStore.UpdateExecution(executionID, func(exec *store.Execution) {
						exec.Status = statusStr
						exec.FinishedAt = &ts
					})
				}
			case event.WorkflowStarted, event.StepLog, event.StepGoto:
			}
		}
	}()

	return func() {
		s.config.EventBus.Unsubscribe(ch)
		<-done // wait for goroutine to drain
	}
}

// finalizeExecution updates the stored execution with the engine result.
// Uses UpdateExecution to hold the store lock during the entire mutation,
// preventing races with captureEvents which also updates the execution.
func (s *Server) finalizeExecution(
	executionID string, result *engine.ExecuteResult, err error, execCtx context.Context,
) {
	s.config.ExecutionStore.UpdateExecution(executionID, func(exec *store.Execution) {
		if err != nil {
			if execCtx.Err() != nil {
				exec.Status = runtime.StatusCancelled
				exec.Error = "execution cancelled"
			} else {
				exec.Status = runtime.StatusFailed
				exec.Error = err.Error()
			}
		} else {
			exec.Status = result.Status
			mergeStepResults(exec, result.Steps)
			now := result.FinishedAt
			exec.FinishedAt = &now
			if result.Error != nil {
				exec.Error = result.Error.Error()
			}
		}

		now := time.Now()
		if exec.FinishedAt == nil {
			exec.FinishedAt = &now
		}
	})
}

// headerMap converts http.Header to a flat map[string]string.
func headerMap(h http.Header) map[string]string {
	headers := make(map[string]string, len(h))
	for k := range h {
		headers[k] = h.Get(k)
	}

	return headers
}

// mergeStepResults merges engine results into the execution, keeping
// Input data that was set via event tracking during execution.
func mergeStepResults(exec *store.Execution, engineSteps map[string]*runtime.StepResult) {
	if engineSteps == nil {
		return
	}

	if exec.Steps == nil {
		exec.Steps = engineSteps
		return
	}

	for id, sr := range engineSteps {
		existing, ok := exec.Steps[id]
		if !ok {
			exec.Steps[id] = sr
			continue
		}
		// Keep Input from event tracking, take everything else from engine
		existing.Status = sr.Status
		existing.Output = sr.Output
		existing.Error = sr.Error
		existing.StartedAt = sr.StartedAt
		existing.FinishedAt = sr.FinishedAt
	}
}

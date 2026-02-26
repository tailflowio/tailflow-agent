package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
		decodeErr := json.NewDecoder(r.Body).Decode(&req)
		if decodeErr != nil {
			s.config.Logger.DebugContext(r.Context(), "failed to decode run request body", "error", decodeErr)
		}
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
		"steps":       activity,
		"exec_counts": s.config.ExecutionStore.StepExecCounts(),
	})
}

func (s *Server) handleGetStepDetail(w http.ResponseWriter, r *http.Request) {
	stepID := r.PathValue("id")

	step := s.findStep(stepID)
	if step == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("step %q not found", stepID))
		return
	}

	history := s.buildStepHistory(stepID)

	metrics := s.config.ExecutionStore.GetStepMetrics(stepID)
	if metrics == nil {
		metrics = &store.StepMetrics{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
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

func (s *Server) buildStepHistory(stepID string) []stepHistoryEntry {
	execs := s.config.ExecutionStore.List()
	history := make([]stepHistoryEntry, 0, len(execs))

	for _, exec := range execs {
		sr, ok := exec.Steps[stepID]
		if !ok {
			continue
		}

		entry := buildHistoryEntry(exec, sr)
		history = append(history, entry)
	}

	return history
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

	return entry
}

func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	execs := s.config.ExecutionStore.List() // newest-first

	execs = filterByStatus(execs, r.URL.Query().Get("status"))
	total := len(execs)

	sortExecutions(execs, r.URL.Query().Get("sort"), r.URL.Query().Get("order"))

	paged := paginateExecutions(execs, r)

	writeJSON(w, http.StatusOK, map[string]any{"items": paged, "total": total})
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

	matched := (wf.Trigger.HTTP != nil && wf.Trigger.HTTP.Path == path && r.Method == wf.Trigger.HTTP.Method) ||
		(wf.Trigger.Webhook != nil && wf.Trigger.Webhook.Path == path)

	if !matched {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no workflow matches %s %s", r.Method, path))
		return
	}

	s.executeTriggerWorkflow(w, r, wf) //nolint:contextcheck // intentional: trigger execution may outlive request
}

func (s *Server) executeTriggerWorkflow(w http.ResponseWriter, r *http.Request, wf *parser.Workflow) {
	triggerData, params := s.buildTriggerData(r)
	executionID, opts, stopCapture := s.prepareTriggerExecution(wf, triggerData, params)

	execCtx, cancel := context.WithCancel(context.Background())
	s.registerCancel(executionID, cancel)

	if wf.Trigger.HTTP != nil && wf.Trigger.HTTP.Async {
		s.runTriggerAsync(executionID, wf, params, opts, execCtx, stopCapture)
		writeJSON(w, http.StatusAccepted, map[string]any{
			"execution_id": executionID,
			"status":       runtime.StatusRunning,
		})

		return
	}

	s.runTriggerSync(w, wf, params, opts, execCtx, executionID, stopCapture)
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
		defer stopCapture()
		defer s.unregisterCancel(executionID)

		result, err := s.config.Executor.Execute(execCtx, wf, params, opts)
		s.finalizeExecution(executionID, result, err, execCtx)
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
	s.writeTriggerResponse(w, wf, result, err, execCtx, executionID)
}

// writeTriggerResponse writes the HTTP response for a synchronous trigger execution.
func (s *Server) writeTriggerResponse(
	w http.ResponseWriter, wf *parser.Workflow, result *engine.ExecuteResult,
	err error, execCtx context.Context, executionID string,
) {
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
		graph.Nodes = append(graph.Nodes, buildGraphNode(step))
		graph.Edges = append(graph.Edges, buildStepEdges(step)...)
	}

	return graph
}

func buildGraphNode(step parser.Step) workflow.GraphNode {
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

	if step.Action == "loop" {
		node.Pipeline = extractLoopPipeline(step.Config)
	}

	return node
}

func extractLoopPipeline(config map[string]any) []workflow.PipelineAction {
	rawActions, ok := config["actions"]
	if !ok {
		return nil
	}

	arr, ok := rawActions.([]any)
	if !ok {
		return nil
	}

	var pipeline []workflow.PipelineAction

	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		actName, _ := m["action"].(string)
		actTitle, _ := m["title"].(string)

		if actName != "" {
			pipeline = append(pipeline, workflow.PipelineAction{
				Action: actName,
				Title:  actTitle,
			})
		}
	}

	return pipeline
}

func buildStepEdges(step parser.Step) []workflow.GraphEdge {
	edges := make([]workflow.GraphEdge, 0, len(step.DependsOn)+1)

	for _, dep := range step.DependsOn {
		edge := workflow.GraphEdge{
			Source: dep,
			Target: step.ID,
		}

		if step.When != "" && strings.Contains(step.When, "steps."+dep+".") {
			edge.Type = "when"
			edge.Label = step.When
		}

		edges = append(edges, edge)
	}

	if step.Goto != nil {
		edges = append(edges, workflow.GraphEdge{
			Source: step.ID,
			Target: step.Goto.Target,
			Type:   "goto",
			Label:  step.Goto.When,
		})
	}

	return edges
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		slog.Warn("failed to write JSON response", "error", err)
	}
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
		TxRegistry: runtime.NewMemoryTxRegistry(s.config.Logger),
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

			s.processEvent(executionID, ev, lt)
		}
	}()

	return func() {
		s.config.EventBus.Unsubscribe(ch)
		<-done // wait for goroutine to drain
	}
}

func (s *Server) processEvent(executionID string, ev event.Event, lt *loopTracker) {
	lt.Track(ev)

	// Reset body steps to pending on goto so dashboard stays coherent during loops
	if ev.Type == event.StepGoto && lt.Body != nil {
		for sid := range lt.Body {
			s.config.ExecutionStore.UpdateStep(executionID, sid, func(r *runtime.StepResult) {
				r.Status = "pending"
			})
		}
	}

	// Skip loop body events after iteration 1 to prevent unbounded memory growth
	if !lt.InLoop(ev) {
		s.config.ExecutionStore.AppendEvent(executionID, ev)
	}

	if ev.StepID == "" {
		return
	}

	s.applyStepEvent(executionID, ev)
}

func (s *Server) applyStepEvent(executionID string, ev event.Event) {
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
		s.applyWorkflowCompleted(executionID, ev)
	case event.Metrics, event.WorkflowStarted, event.StepLog, event.StepGoto:
	}
}

func (s *Server) applyWorkflowCompleted(executionID string, ev event.Event) {
	statusStr, ok := ev.Data["status"].(string)
	if !ok {
		return
	}

	ts := ev.Timestamp

	s.config.ExecutionStore.UpdateExecution(executionID, func(exec *store.Execution) {
		exec.Status = statusStr
		exec.FinishedAt = &ts
	})
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

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

func (s *Server) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(r.Context(), w, http.StatusOK, map[string]any{
		"version":         s.config.Version,
		"editor_enabled":  s.config.EditorEnabled,
		"workflow_file":   s.config.FilePath,
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

func (s *Server) handleGetAllStepMetrics(w http.ResponseWriter, r *http.Request) {
	allMetrics := s.config.ExecutionStore.GetAllStepMetrics()
	execs := s.config.ExecutionStore.List()

	type stepStats struct {
		TotalExecutions int     `json:"total_executions"`
		SuccessCount    int     `json:"success_count"`
		FailureCount    int     `json:"failure_count"`
		AvgDurationMs   int64   `json:"avg_duration_ms"`
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

	history := s.buildStepHistory(stepID)

	metrics := s.config.ExecutionStore.GetStepMetrics(stepID)
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

func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	execs := s.config.ExecutionStore.List() // newest-first

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

	exec, err := s.config.ExecutionStore.Get(id)
	if err != nil {
		s.writeError(r.Context(), w, http.StatusNotFound, err.Error())
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

	allEvents := s.config.ExecutionStore.GetEvents(id)
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

	exec, err := s.config.ExecutionStore.Get(id)
	if err != nil {
		s.writeError(r.Context(), w, http.StatusNotFound, err.Error())
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

func buildGraph(wf *parser.Workflow, dag *engine.DAG) workflow.Graph {
	graph := workflow.Graph{
		Nodes: []workflow.GraphNode{},
		Edges: []workflow.GraphEdge{},
	}

	stepIndex := make(map[string]parser.Step, len(wf.Steps))
	for _, s := range wf.Steps {
		stepIndex[s.ID] = s
	}

	loopTargets := make(map[string]bool)
	for _, s := range wf.Steps {
		if s.Goto != nil {
			loopTargets[s.Goto.Target] = true
		}
	}

	loopBodies := buildLoopBodies(wf, dag)

	visited := make(map[string]bool)
	var walk func(node *engine.DAGNode, depth int, parentID string, isLast bool)
	walk = func(node *engine.DAGNode, depth int, parentID string, isLast bool) {
		if visited[node.Step.ID] {
			return
		}

		visited[node.Step.ID] = true

		step := stepIndex[node.Step.ID]
		gn := buildGraphNode(step)
		gn.Depth = depth
		gn.ParentID = parentID
		gn.IsLast = isLast
		gn.InLoop = loopBodies[step.ID]
		gn.IsLoopStart = loopTargets[step.ID]

		if step.Goto != nil {
			gn.GotoTarget = step.Goto.Target
			gn.GotoMax = step.Goto.MaxIterations
		}

		graph.Nodes = append(graph.Nodes, gn)
		graph.Edges = append(graph.Edges, buildStepEdges(step)...)

		for i, child := range node.Children {
			last := i == len(node.Children)-1
			walk(child, depth+1, node.Step.ID, last)
		}
	}

	for i, root := range dag.Roots {
		last := i == len(dag.Roots)-1
		walk(root, 0, "", last)
	}

	// Sort nodes by YAML declaration order so the progress strip and any other
	// linear consumer see steps in the order the author wrote them. The DFS
	// above is needed to compute depth/parentID/IsLast for tree rendering, but
	// it leaves nodes in traversal order (e.g. a second root with shared
	// descendants ends up at the tail of the slice).
	yamlOrder := make(map[string]int, len(wf.Steps))
	for i, s := range wf.Steps {
		yamlOrder[s.ID] = i
	}

	sort.SliceStable(graph.Nodes, func(i, j int) bool {
		return yamlOrder[graph.Nodes[i].ID] < yamlOrder[graph.Nodes[j].ID]
	})

	graph.Tree = buildTreeLines(wf, dag)
	graph.Stages = buildStageInfos(wf)

	return graph
}

func buildStageInfos(wf *parser.Workflow) []workflow.StageInfo {
	stages := make([]workflow.StageInfo, 0, len(wf.Stages))

	for _, s := range wf.Stages {
		si := workflow.StageInfo{
			Name:        s.Name,
			Description: s.Description,
			Steps:       []string{},
		}

		for _, step := range wf.Steps {
			if step.Stage == s.Name {
				si.Steps = append(si.Steps, step.ID)
			}
		}

		stages = append(stages, si)
	}

	return stages
}

func buildTreeLines(wf *parser.Workflow, dag *engine.DAG) []workflow.TreeLine {
	r := &treeBuilder{
		steps: make(map[string]*treeStep, len(wf.Steps)),
	}

	stepIndex := make(map[string]parser.Step, len(wf.Steps))
	for _, s := range wf.Steps {
		stepIndex[s.ID] = s
	}

	visited := make(map[string]bool)
	var dfs func(node *engine.DAGNode, depth int, parentID string, isLast bool)
	dfs = func(node *engine.DAGNode, depth int, parentID string, isLast bool) {
		if visited[node.Step.ID] {
			return
		}
		visited[node.Step.ID] = true

		s := stepIndex[node.Step.ID]
		title := s.Title
		if title == "" {
			title = s.ID
		}

		ts := &treeStep{
			id: s.ID, title: title, action: s.Action,
			depth: depth, parentID: parentID, isLast: isLast,
			when: s.When,
		}
		if s.Goto != nil {
			ts.gotoTarget = s.Goto.Target
			ts.gotoMax = s.Goto.MaxIterations
		}
		if s.Action == "loop" {
			ts.pipeline = extractLoopPipeline(s.Config)
		}

		r.steps[s.ID] = ts
		r.order = append(r.order, s.ID)

		for i, child := range node.Children {
			dfs(child, depth+1, node.Step.ID, i == len(node.Children)-1)
		}
	}

	for i, root := range dag.Roots {
		dfs(root, 0, "", i == len(dag.Roots)-1)
	}

	r.resolveConvergent(dag)
	r.buildLoops()
	r.markLoopBodies()

	return r.render()
}

type treeStep struct {
	id, title, action, parentID string
	depth                       int
	isLast                      bool
	gotoTarget                  string
	gotoMax                     int
	pipeline                    []workflow.PipelineAction
	mergeMarker                 string
	when                        string
	inLoop                      bool
}

type loopDisplay struct {
	startIdx, endIdx int
}

type treeBuilder struct {
	steps map[string]*treeStep
	order []string
	loops []loopDisplay
}

func (r *treeBuilder) resolveConvergent(dag *engine.DAG) {
	processed := make(map[string]bool)

	for changed := true; changed; {
		changed = false

		for _, id := range r.order {
			if processed[id] {
				continue
			}

			node := dag.Nodes[id]
			if node == nil || len(node.Parents) <= 1 {
				continue
			}

			parentIDs := make([]string, 0, len(node.Parents))
			sharedParent := ""
			allSiblings := true

			for i, p := range node.Parents {
				pid := p.Step.ID
				st := r.steps[pid]
				if st == nil {
					allSiblings = false
					break
				}
				if i == 0 {
					sharedParent = st.parentID
				} else if st.parentID != sharedParent {
					allSiblings = false
					break
				}
				parentIDs = append(parentIDs, pid)
			}

			if !allSiblings {
				processed[id] = true
				continue
			}

			orderIdx := make(map[string]int, len(r.order))
			for i, oid := range r.order {
				orderIdx[oid] = i
			}
			sort.Slice(parentIDs, func(a, b int) bool {
				return orderIdx[parentIDs[a]] < orderIdx[parentIDs[b]]
			})

			descSet := map[string]bool{id: true}
			var collect func(string)
			collect = func(nid string) {
				dn := dag.Nodes[nid]
				if dn == nil {
					return
				}
				for _, c := range dn.Children {
					if !descSet[c.Step.ID] {
						descSet[c.Step.ID] = true
						collect(c.Step.ID)
					}
				}
			}
			collect(id)

			var subtree, remaining []string
			for _, oid := range r.order {
				if descSet[oid] {
					subtree = append(subtree, oid)
				} else {
					remaining = append(remaining, oid)
				}
			}

			lastParentID := parentIDs[len(parentIDs)-1]
			lastParentIdx := -1
			for i, oid := range remaining {
				if oid == lastParentID {
					lastParentIdx = i
					break
				}
			}
			if lastParentIdx == -1 {
				processed[id] = true
				continue
			}

			lastParentDepth := r.steps[lastParentID].depth
			insertIdx := lastParentIdx + 1
			for insertIdx < len(remaining) && r.steps[remaining[insertIdx]].depth > lastParentDepth {
				insertIdx++
			}

			for i := insertIdx - 1; i > lastParentIdx; i-- {
				if r.steps[remaining[i]].parentID == lastParentID {
					r.steps[remaining[i]].isLast = false
					break
				}
			}

			newOrder := make([]string, 0, len(r.order))
			newOrder = append(newOrder, remaining[:insertIdx]...)
			newOrder = append(newOrder, subtree...)
			newOrder = append(newOrder, remaining[insertIdx:]...)
			r.order = newOrder

			childSt := r.steps[id]
			childSt.parentID = lastParentID
			childSt.isLast = true

			for i, pid := range parentIDs {
				pst := r.steps[pid]
				switch {
				case i == 0:
					pst.mergeMarker = "┐"
				case i == len(parentIDs)-1:
					pst.mergeMarker = "┘"
				default:
					pst.mergeMarker = "┤"
				}
			}

			processed[id] = true
			changed = true
			break
		}
	}
}

func (r *treeBuilder) buildLoops() {
	r.loops = nil
	for i, id := range r.order {
		st := r.steps[id]
		if st.gotoTarget == "" {
			continue
		}
		startIdx := -1
		for j, oid := range r.order {
			if oid == st.gotoTarget {
				startIdx = j
				break
			}
		}
		if startIdx >= 0 {
			r.loops = append(r.loops, loopDisplay{startIdx: startIdx, endIdx: i})
		}
	}
}

func (r *treeBuilder) markLoopBodies() {
	for _, ld := range r.loops {
		for i := ld.startIdx; i <= ld.endIdx; i++ {
			r.steps[r.order[i]].inLoop = true
		}
	}
}

func (r *treeBuilder) bracketChar(idx int, isAnnotation bool) string {
	if len(r.loops) == 0 {
		return ""
	}
	for _, ld := range r.loops {
		if idx == ld.startIdx {
			return "╭ "
		}
		if idx == ld.endIdx {
			if isAnnotation {
				return "╰ "
			}
			return "│ "
		}
		if idx > ld.startIdx && idx < ld.endIdx {
			return "│ "
		}
	}
	return "  "
}

func (r *treeBuilder) treePrefix(id string) string {
	st := r.steps[id]
	if st.depth == 0 {
		return ""
	}

	own := "├── "
	if st.isLast {
		own = "└── "
	}

	var parts []string
	cur := st.parentID
	for d := st.depth - 1; d > 0; d-- {
		parent := r.steps[cur]
		if parent.isLast {
			parts = append(parts, "    ")
		} else {
			parts = append(parts, "│   ")
		}
		cur = parent.parentID
	}

	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}

	return strings.Join(parts, "") + own
}

func (r *treeBuilder) render() []workflow.TreeLine {
	var lines []workflow.TreeLine

	for idx, id := range r.order {
		st := r.steps[id]
		bracket := r.bracketChar(idx, false)
		prefix := r.treePrefix(id)

		merge := ""
		if st.mergeMarker != "" {
			merge = " ──" + st.mergeMarker
		}

		lines = append(lines, workflow.TreeLine{
			StepID:     st.id,
			Prefix:     bracket + prefix,
			Name:       st.id,
			Label:      st.title,
			Action:     st.action,
			Merge:      merge,
			Type:       "step",
			Depth:      st.depth,
			InLoop:     st.inLoop,
			When:       st.when,
			GotoTarget: st.gotoTarget,
			GotoMax:    st.gotoMax,
		})

		for i, pa := range st.pipeline {
			connector := "├─ "
			if i == len(st.pipeline)-1 {
				connector = "└─ "
			}

			cont := r.treeContinuation(id)
			paBracket := r.bracketChar(idx, false)
			paLabel := pa.Action
			if pa.Title != "" {
				paLabel = pa.Title
			}

			lines = append(lines, workflow.TreeLine{
				StepID: st.id,
				Prefix: paBracket + cont + connector,
				Name:   fmt.Sprintf("%d. %s", i+1, pa.Action),
				Label:  paLabel,
				Type:   "pipeline",
			})
		}

		if st.gotoTarget != "" {
			closeBracket := r.bracketChar(idx, true)
			gotoLabel := "↻ goto " + st.gotoTarget
			if st.gotoMax > 0 {
				gotoLabel += fmt.Sprintf(" (max %d)", st.gotoMax)
			}

			lines = append(lines, workflow.TreeLine{
				Prefix: closeBracket,
				Name:   gotoLabel,
				Type:   "goto",
			})
		}
	}

	return lines
}

func (r *treeBuilder) treeContinuation(id string) string {
	st := r.steps[id]

	own := "│   "
	if st.isLast {
		own = "    "
	}

	if st.depth == 0 {
		return own
	}

	var parts []string
	cur := st.parentID
	for d := st.depth - 1; d > 0; d-- {
		parent := r.steps[cur]
		if parent.isLast {
			parts = append(parts, "    ")
		} else {
			parts = append(parts, "│   ")
		}
		cur = parent.parentID
	}

	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}

	return strings.Join(parts, "") + own
}

func resolveConvergentNodes(graph *workflow.Graph, dag *engine.DAG) {
	nodeIdx := make(map[string]int, len(graph.Nodes))
	for i, n := range graph.Nodes {
		nodeIdx[n.ID] = i
	}

	processed := make(map[string]bool)

	for changed := true; changed; {
		changed = false

		for _, gn := range graph.Nodes {
			if processed[gn.ID] {
				continue
			}

			dagNode := dag.Nodes[gn.ID]
			if dagNode == nil || len(dagNode.Parents) <= 1 {
				continue
			}

			parentIDs := make([]string, 0, len(dagNode.Parents))
			sharedParent := ""
			allSiblings := true

			for i, p := range dagNode.Parents {
				pid := p.Step.ID
				pidx, exists := nodeIdx[pid]

				if !exists {
					allSiblings = false
					break
				}

				if i == 0 {
					sharedParent = graph.Nodes[pidx].ParentID
				} else if graph.Nodes[pidx].ParentID != sharedParent {
					allSiblings = false
					break
				}

				parentIDs = append(parentIDs, pid)
			}

			if !allSiblings {
				processed[gn.ID] = true
				continue
			}

			sort.Slice(parentIDs, func(a, b int) bool {
				return nodeIdx[parentIDs[a]] < nodeIdx[parentIDs[b]]
			})

			lastParentID := parentIDs[len(parentIDs)-1]
			lastParentIdx, ok := nodeIdx[lastParentID]
			if !ok {
				processed[gn.ID] = true
				continue
			}

			lastParentDepth := graph.Nodes[lastParentIdx].Depth

			descSet := map[string]bool{gn.ID: true}
			var collectDesc func(string)
			collectDesc = func(nid string) {
				dn := dag.Nodes[nid]
				if dn == nil {
					return
				}
				for _, c := range dn.Children {
					if !descSet[c.Step.ID] {
						descSet[c.Step.ID] = true
						collectDesc(c.Step.ID)
					}
				}
			}
			collectDesc(gn.ID)

			var subtree, remaining []workflow.GraphNode
			for _, n := range graph.Nodes {
				if descSet[n.ID] {
					subtree = append(subtree, n)
				} else {
					remaining = append(remaining, n)
				}
			}

			insertIdx := -1
			for i, n := range remaining {
				if n.ID == lastParentID {
					insertIdx = i + 1
					break
				}
			}

			if insertIdx == -1 {
				processed[gn.ID] = true
				continue
			}

			for insertIdx < len(remaining) && remaining[insertIdx].Depth > lastParentDepth {
				insertIdx++
			}

			newNodes := make([]workflow.GraphNode, 0, len(graph.Nodes))
			newNodes = append(newNodes, remaining[:insertIdx]...)
			newNodes = append(newNodes, subtree...)
			newNodes = append(newNodes, remaining[insertIdx:]...)
			graph.Nodes = newNodes

			for i, n := range graph.Nodes {
				if n.ID == gn.ID {
					graph.Nodes[i].ParentID = lastParentID
					graph.Nodes[i].IsLast = true
				}
			}

			nodeIdx = make(map[string]int, len(graph.Nodes))
			for i, n := range graph.Nodes {
				nodeIdx[n.ID] = i
			}

			changed = true

			break
		}
	}
}

func buildLoopBodies(wf *parser.Workflow, dag *engine.DAG) map[string]bool {
	bodies := make(map[string]bool)

	for _, s := range wf.Steps {
		if s.Goto == nil {
			continue
		}

		current := s.Goto.Target
		visited := make(map[string]bool)

		for current != "" && !visited[current] {
			visited[current] = true
			bodies[current] = true

			if current == s.ID {
				break
			}

			node := dag.Nodes[current]
			if node != nil && len(node.Children) == 1 {
				current = node.Children[0].Step.ID
			} else {
				break
			}
		}
	}

	return bodies
}

func buildGraphNode(step parser.Step) workflow.GraphNode {
	label := step.Title
	if label == "" {
		label = step.ID
	}

	node := workflow.GraphNode{
		ID:         step.ID,
		Label:      label,
		Action:     step.Action,
		Type:       "step",
		When:       step.When,
		OnRecovery: step.OnRecovery,
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

	pipeline := make([]workflow.PipelineAction, 0, len(arr))

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

	if len(pipeline) == 0 {
		return nil
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

func (s *Server) captureEvents(executionID string) func() {
	// Blocking subscription: the store is the source of truth for the API,
	// so we can't afford dropped events. The buffer is generous (10k) to
	// absorb bursts without back-pressuring the engine in practice.
	ch := s.config.EventBus.SubscribeBlocking(10_000)
	done := make(chan struct{})

	go func() {
		defer close(done)

		lt := &loopTracker{}
		completedSeen := false

		for ev := range ch {
			if ev.ExecutionID != executionID {
				continue
			}

			s.processEvent(executionID, ev, lt, &completedSeen)
		}
	}()

	return func() {
		s.config.EventBus.Unsubscribe(ch)
		<-done // wait for goroutine to drain
	}
}

func (s *Server) processEvent(executionID string, ev event.Event, lt *loopTracker, completedSeen *bool) {
	lt.Track(ev)

	// Deduplicate workflow.completed — only store the first one
	if ev.Type == event.WorkflowCompleted {
		if *completedSeen {
			return
		}

		*completedSeen = true
	}

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

			o, ok := ev.Data["output"]
			if ok {
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
			o, ok := ev.Data["output"]
			if ok {
				r.Output = o
			}
		})
	case event.WorkflowCompleted:
		s.applyWorkflowCompleted(executionID, ev)
	case event.Metrics, event.WorkflowStarted, event.StepLog, event.StepGoto, event.ExecutionState, event.ExecutionGroup:
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

func (s *Server) finalizeExecution(
	executionID string, result *engine.ExecuteResult, err error, execCtx context.Context,
) {
	s.config.ExecutionStore.UpdateExecution(executionID, func(exec *store.Execution) {
		switch {
		case err != nil && execCtx.Err() != nil:
			exec.Status = runtime.StatusCancelled
			exec.Error = "execution cancelled"
		case err != nil:
			exec.Status = runtime.StatusFailed
			exec.Error = err.Error()
		default:
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

func headerMap(h http.Header) map[string]string {
	headers := make(map[string]string, len(h))
	for k := range h {
		headers[k] = h.Get(k)
	}

	return headers
}

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

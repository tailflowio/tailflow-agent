package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

// brokenResponseWriter simulates a ResponseWriter that always fails on Write.
type brokenResponseWriter struct {
	header http.Header
}

func (b *brokenResponseWriter) Header() http.Header { return b.header }
func (b *brokenResponseWriter) WriteHeader(_ int)   {}
func (b *brokenResponseWriter) Write(_ []byte) (int, error) {
	return 0, fmt.Errorf("write failed")
}

func (s *HandlersTestSuite) TestWriteJSON_EncodingError() {
	w := &brokenResponseWriter{header: http.Header{}}
	srv := newTestServer(s.T())

	s.NotPanics(func() {
		srv.writeJSON(context.Background(), w, http.StatusOK, map[string]any{"key": "val"})
	})
}

// TestGetWorkflowGraph_BuildDAGError covers handleGetWorkflowGraph – error from BuildDAG (broken depends_on).
func (s *HandlersTestSuite) TestGetWorkflowGraph_BuildDAGError() {
	srv := newTestServer(s.T())

	srv.config.Workflow.Steps = append(srv.config.Workflow.Steps, parser.Step{
		ID:        "broken",
		Action:    "log",
		DependsOn: []string{"nonexistent_step"},
	})

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusInternalServerError, w.Code)
}

// TestGetWorkflowGraph_ComplexDAG covers buildGraphNode + extractLoopPipeline + buildStepEdges.
func (s *HandlersTestSuite) TestGetWorkflowGraph_ComplexDAG() {
	srv := newTestServerGraph(s.T())

	req := httptest.NewRequest("GET", "/api/workflow/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)

	var graph map[string]any
	json.Unmarshal(w.Body.Bytes(), &graph)

	nodes := graph["nodes"].([]any)
	s.Len(nodes, 3)

	for _, n := range nodes {
		node := n.(map[string]any)
		if node["id"] == "step_a" {
			s.Equal("step_a", node["label"])
		}
		if node["id"] == "step_loop" {
			s.Equal("loop", node["action"])
			pipeline := node["pipeline"].([]any)
			s.Len(pipeline, 2)
		}
	}

	edges := graph["edges"].([]any)
	s.GreaterOrEqual(len(edges), 3)

	var foundGoto, foundWhen bool
	for _, e := range edges {
		edge := e.(map[string]any)
		if edge["type"] == "goto" {
			foundGoto = true
			s.Equal("step_b", edge["source"])
			s.Equal("step_a", edge["target"])
		}
		if edge["type"] == "when" {
			foundWhen = true
		}
	}
	s.True(foundGoto, "should have a goto edge")
	s.True(foundWhen, "should have a when edge")
}

// TestBuildStepEdges_NoDeps covers buildStepEdges – step with no depends_on and no goto.
func (s *HandlersTestSuite) TestBuildStepEdges_NoDeps() {
	edges := buildStepEdges(parser.Step{ID: "solo", Action: "log"})
	s.Len(edges, 0)
}

// TestBuildStepEdges_DepsNoWhen covers buildStepEdges – step with depends_on but no when condition.
func (s *HandlersTestSuite) TestBuildStepEdges_DepsNoWhen() {
	edges := buildStepEdges(parser.Step{
		ID:        "child",
		Action:    "log",
		DependsOn: []string{"parent"},
	})
	s.Len(edges, 1)
	s.Equal("parent", edges[0].Source)
	s.Equal("child", edges[0].Target)
	s.Empty(edges[0].Type)
}

// TestBuildStepEdges_WithGoto covers buildStepEdges – step with goto.
func (s *HandlersTestSuite) TestBuildStepEdges_WithGoto() {
	edges := buildStepEdges(parser.Step{
		ID:     "jumper",
		Action: "log",
		Goto: &parser.GotoConfig{
			Target: "target_step",
			When:   "some condition",
		},
	})
	s.Len(edges, 1)
	s.Equal("goto", edges[0].Type)
	s.Equal("jumper", edges[0].Source)
	s.Equal("target_step", edges[0].Target)
	s.Equal("some condition", edges[0].Label)
}

// TestBuildGraphNode_LoopAction covers buildGraphNode – loop action with pipeline.
func (s *HandlersTestSuite) TestBuildGraphNode_LoopAction() {
	node := buildGraphNode(parser.Step{
		ID:    "my_loop",
		Action: "loop",
		Title:  "My Loop",
		Config: map[string]any{
			"actions": []any{
				map[string]any{"action": "http", "title": "Call API"},
				map[string]any{"action": "log"},
			},
		},
	})

	s.Equal("My Loop", node.Label)
	s.Len(node.Pipeline, 2)
	s.Equal("http", node.Pipeline[0].Action)
	s.Equal("Call API", node.Pipeline[0].Title)
}

// TestBuildGraphNode_NonLoop covers buildGraphNode – non-loop action.
func (s *HandlersTestSuite) TestBuildGraphNode_NonLoop() {
	node := buildGraphNode(parser.Step{
		ID:     "step1",
		Action: "http",
		Title:  "HTTP Step",
	})

	s.Equal("HTTP Step", node.Label)
	s.Nil(node.Pipeline)
}

// TestExecDuration_WithFinishedAt covers execDuration with finished execution.
func (s *HandlersTestSuite) TestExecDuration_WithFinishedAt() {
	now := time.Now()
	later := now.Add(5 * time.Second)
	exec := &store.Execution{StartedAt: now, FinishedAt: &later}

	d := execDuration(exec)
	s.Equal(5*time.Second, d)
}

// TestExecDuration_NilFinishedAt covers execDuration without finished execution.
func (s *HandlersTestSuite) TestExecDuration_NilFinishedAt() {
	exec := &store.Execution{StartedAt: time.Now()}

	d := execDuration(exec)
	s.Equal(time.Duration(0), d)
}

// TestParseIntParam_ValidValue covers parseIntParam – valid value.
func (s *HandlersTestSuite) TestParseIntParam_ValidValue() {
	r := httptest.NewRequest("GET", "/test?offset=10", nil)
	v := parseIntParam(r, "offset", 0)
	s.Equal(10, v)
}

// TestParseIntParam_Empty covers parseIntParam – empty (default).
func (s *HandlersTestSuite) TestParseIntParam_Empty() {
	r := httptest.NewRequest("GET", "/test", nil)
	v := parseIntParam(r, "offset", 5)
	s.Equal(5, v)
}

// TestParseIntParam_Invalid covers parseIntParam – invalid.
func (s *HandlersTestSuite) TestParseIntParam_Invalid() {
	r := httptest.NewRequest("GET", "/test?offset=abc", nil)
	v := parseIntParam(r, "offset", 7)
	s.Equal(7, v)
}

// TestParseIntParam_Negative covers parseIntParam – negative.
func (s *HandlersTestSuite) TestParseIntParam_Negative() {
	r := httptest.NewRequest("GET", "/test?offset=-3", nil)
	v := parseIntParam(r, "offset", 0)
	s.Equal(0, v)
}

// TestBuildHistoryEntry_NoTimestamps covers buildHistoryEntry – step result with no StartedAt and no exec FinishedAt.
func (s *HandlersTestSuite) TestBuildHistoryEntry_NoTimestamps() {
	exec := &store.Execution{
		ID:        "e1",
		StartedAt: time.Now(),
	}
	sr := &runtime.StepResult{
		Status: runtime.StatusRunning,
	}

	entry := buildHistoryEntry(exec, sr)
	s.Equal(runtime.StatusRunning, entry.Status)
	s.NotEmpty(entry.StartedAt)
	s.Empty(entry.FinishedAt)
}

// TestBuildHistoryEntry_ExecFinishedAt covers buildHistoryEntry – step with StartedAt but no FinishedAt and exec has FinishedAt.
func (s *HandlersTestSuite) TestBuildHistoryEntry_ExecFinishedAt() {
	started := time.Now()
	execFinished := started.Add(2 * time.Second)

	exec := &store.Execution{
		ID:         "e2",
		StartedAt:  started,
		FinishedAt: &execFinished,
	}
	sr := &runtime.StepResult{
		Status:    runtime.StatusRunning,
		StartedAt: &started,
	}

	entry := buildHistoryEntry(exec, sr)
	s.NotEmpty(entry.FinishedAt)
	s.Equal(int64(0), entry.DurationMs)
}

// TestBuildActionServices_EmitWaiting covers buildActionServices – test EmitWaiting and ScheduleExecution.
func (s *HandlersTestSuite) TestBuildActionServices_EmitWaiting() {
	srv := newTestServer(s.T())

	services := srv.buildActionServices()
	s.NotNil(services)
	s.NotNil(services.WaitWebhookRegister)
	s.NotNil(services.WaitRabbitMQRegister)
	s.NotNil(services.EmitWaiting)
	s.NotNil(services.ScheduleExecution)
	s.NotNil(services.Locker)
	s.NotNil(services.DBPool)
	s.NotNil(services.KVStore)
	s.NotNil(services.TxRegistry)

	ch := srv.config.EventBus.Subscribe(10)
	defer srv.config.EventBus.Unsubscribe(ch)

	services.EmitWaiting("exec-1", "step-1", "webhook", map[string]any{"path": "/cb"})

	select {
	case ev := <-ch:
		s.Equal(event.StepWaiting, ev.Type)
		s.Equal("exec-1", ev.ExecutionID)
		s.Equal("step-1", ev.StepID)
		s.Equal("webhook", ev.Data["wait_type"])
		s.Equal("/cb", ev.Data["path"])
	case <-time.After(time.Second):
		s.Fail("timeout waiting for event")
	}
}

func (s *HandlersTestSuite) TestBuildActionServices_ScheduleExecution() {
	srv := newTestServer(s.T())

	services := srv.buildActionServices()

	execID, err := services.ScheduleExecution(50*time.Millisecond, map[string]any{"key": "val"})
	s.NoError(err)
	s.NotEmpty(execID)

	exec, getErr := srv.config.ExecutionStore.Get(context.Background(), execID)
	s.NoError(getErr)
	s.Equal(runtime.StatusScheduled, exec.Status)

	s.Eventually(func() bool {
		count, _ := srv.config.ExecutionStore.Count(context.Background())
		return count >= 2
	}, 2*time.Second, 50*time.Millisecond)
}

// TestFilterByStatus_EmptyFilter covers filterByStatus – empty status.
func (s *HandlersTestSuite) TestFilterByStatus_EmptyFilter() {
	execs := []*store.Execution{
		{ID: "1", Status: runtime.StatusSuccess},
		{ID: "2", Status: runtime.StatusFailed},
	}

	result := filterByStatus(execs, "")
	s.Len(result, 2)
}

// TestSortExecutions_DefaultSort covers sortExecutions – non-duration sort (default, no-op except for order).
func (s *HandlersTestSuite) TestSortExecutions_DefaultSort() {
	now := time.Now()
	execs := []*store.Execution{
		{ID: "1", StartedAt: now},
		{ID: "2", StartedAt: now.Add(1 * time.Second)},
	}

	sortExecutions(execs, "", "")
	s.Equal("1", execs[0].ID)

	sortExecutions(execs, "", "asc")
	s.Equal("2", execs[0].ID)
}

// TestPaginateExecutions_SmallPage covers paginateExecutions – limit 1 offset 0.
func (s *HandlersTestSuite) TestPaginateExecutions_SmallPage() {
	execs := []*store.Execution{
		{ID: "1"}, {ID: "2"}, {ID: "3"},
	}

	r := httptest.NewRequest("GET", "/test?offset=1&limit=1", nil)
	paged := paginateExecutions(execs, r)
	s.Len(paged, 1)
	s.Equal("2", paged[0].ID)
}

// TestExtractLoopPipeline_NoActions covers extractLoopPipeline – no actions key.
func (s *HandlersTestSuite) TestExtractLoopPipeline_NoActions() {
	result := extractLoopPipeline(map[string]any{})
	s.Nil(result)
}

// TestExtractLoopPipeline_NotArray covers extractLoopPipeline – actions is not []any.
func (s *HandlersTestSuite) TestExtractLoopPipeline_NotArray() {
	result := extractLoopPipeline(map[string]any{"actions": "not-an-array"})
	s.Nil(result)
}

// TestExtractLoopPipeline_NonMapItem covers extractLoopPipeline – actions contains non-map items.
func (s *HandlersTestSuite) TestExtractLoopPipeline_NonMapItem() {
	result := extractLoopPipeline(map[string]any{
		"actions": []any{"not-a-map", 42},
	})
	s.Nil(result)
}

// TestExtractLoopPipeline_EmptyActionName covers extractLoopPipeline – action without name (empty string).
func (s *HandlersTestSuite) TestExtractLoopPipeline_EmptyActionName() {
	result := extractLoopPipeline(map[string]any{
		"actions": []any{
			map[string]any{"action": "", "title": "no action"},
		},
	})
	s.Nil(result)
}

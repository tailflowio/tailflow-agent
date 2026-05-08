package saas

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/event"
)

type request struct {
	Path string
	Body map[string]any
}

func collectRequests(t *testing.T) (*httptest.Server, *[]request, *sync.Mutex) {
	t.Helper()

	var (
		reqs []request
		mu   sync.Mutex
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			t.Logf("decode error: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mu.Lock()
		reqs = append(reqs, request{Path: r.URL.Path, Body: body})
		mu.Unlock()

		if r.URL.Path == "/api/v1/agent/register" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"agent_id": "saas-assigned-id"})
			return
		}

		w.WriteHeader(http.StatusOK)
	}))

	return srv, &reqs, &mu
}

func getRequests(mu *sync.Mutex, reqs *[]request) []request {
	mu.Lock()
	defer mu.Unlock()
	copiedRequests := make([]request, len(*reqs))
	copy(copiedRequests, *reqs)
	return copiedRequests
}

func newTestConfig(url string, bus *event.Bus) Config {
	return Config{
		ExportURL:           url,
		APIKey:              "test-key",
		EventBus:            bus,
		Logger:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		WorkflowName:        "test-wf",
		WorkflowDescription: "A test workflow",
		WorkflowTags:        []string{"test", "ci"},
		TriggerType:         "http",
		StepsCount:          3,
		Version:             "1.0.0",
		FlushInterval:       50 * time.Millisecond,
		HeartbeatInterval:   50 * time.Millisecond,
	}
}

type ExporterTestSuite struct {
	suite.Suite
}

func TestExporter(t *testing.T) {
	suite.Run(t, new(ExporterTestSuite))
}

func (s *ExporterTestSuite) SetupTest() { // required by convention
}

func (s *ExporterTestSuite) TestRegistration_AssignsAgentID() {
	srv, reqs, mu := collectRequests(s.T())
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp := NewExporter(newTestConfig(srv.URL, bus))
	exp.Start(ctx)

	// Wait for the registration request to arrive
	s.Eventually(func() bool {
		for _, r := range getRequests(mu, reqs) {
			if r.Path == "/api/v1/agent/register" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	got := getRequests(mu, reqs)

	var found bool
	for _, r := range got {
		if r.Path == "/api/v1/agent/register" {
			found = true
			s.Equal("test-wf", r.Body["workflow_name"])
			s.Equal("A test workflow", r.Body["workflow_description"])
			s.Equal("http", r.Body["trigger_type"])
			s.Equal("1.0.0", r.Body["version"])
			// agent_id should NOT be in the request — it's assigned by the SaaS
			_, hasID := r.Body["agent_id"]
			s.False(hasID, "register request should not contain agent_id")
			break
		}
	}

	s.Require().True(found, "no /register request received")

	// Verify agent_id was assigned from response
	agentID, ok := exp.isRegistered()
	s.Require().True(ok, "expected exporter to be registered")
	s.Equal("saas-assigned-id", agentID)
}

func (s *ExporterTestSuite) TestBatchFlush() {
	srv, reqs, mu := collectRequests(s.T())
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp := NewExporter(newTestConfig(srv.URL, bus))
	exp.Start(ctx)

	// Wait for registration first
	s.Eventually(func() bool {
		_, ok := exp.isRegistered()
		return ok
	}, 2*time.Second, 10*time.Millisecond)

	bus.Publish(event.Event{
		Type:        event.StepStarted,
		Timestamp:   time.Now(),
		ExecutionID: "exec-1",
		StepID:      "step-1",
	})
	bus.Publish(event.Event{
		Type:        event.StepCompleted,
		Timestamp:   time.Now(),
		ExecutionID: "exec-1",
		StepID:      "step-1",
	})

	// Wait for the ingest request to arrive after batch flush
	s.Eventually(func() bool {
		for _, r := range getRequests(mu, reqs) {
			if r.Path == "/api/v1/agent/ingest" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	got := getRequests(mu, reqs)

	var ingestFound bool
	for _, r := range got {
		if r.Path == "/api/v1/agent/ingest" {
			ingestFound = true
			s.Equal("saas-assigned-id", r.Body["agent_id"])
			events, ok := r.Body["events"].([]any)
			s.Require().True(ok, "expected events array")
			s.NotEmpty(events, "expected at least one event in ingest")
			break
		}
	}

	s.Require().True(ingestFound, "no /ingest request received")
}

func (s *ExporterTestSuite) TestMetricsFiltered() {
	srv, reqs, mu := collectRequests(s.T())
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp := NewExporter(newTestConfig(srv.URL, bus))
	exp.Start(ctx)

	// Wait for registration first
	s.Eventually(func() bool {
		_, ok := exp.isRegistered()
		return ok
	}, 2*time.Second, 10*time.Millisecond)

	bus.Publish(event.Event{
		Type:      event.Metrics,
		Timestamp: time.Now(),
		Data:      map[string]any{"cpu_percent": 42.0},
	})
	bus.Publish(event.Event{
		Type:        event.StepStarted,
		Timestamp:   time.Now(),
		ExecutionID: "exec-1",
		StepID:      "step-1",
	})

	// Wait for the ingest request containing the StepStarted event
	s.Eventually(func() bool {
		for _, r := range getRequests(mu, reqs) {
			if r.Path == "/api/v1/agent/ingest" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	got := getRequests(mu, reqs)

	for _, r := range got {
		if r.Path == "/api/v1/agent/ingest" {
			events, ok := r.Body["events"].([]any)
			if !ok {
				continue
			}
			for _, raw := range events {
				ev, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				s.NotEqual(string(event.Metrics), ev["type"], "metrics event should have been filtered out")
			}
		}
	}
}

func (s *ExporterTestSuite) TestHeartbeat_SendsAgentStatus() {
	srv, reqs, mu := collectRequests(s.T())
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp := NewExporter(newTestConfig(srv.URL, bus))
	exp.Start(ctx)

	// Wait for a heartbeat request to arrive
	s.Eventually(func() bool {
		for _, r := range getRequests(mu, reqs) {
			if r.Path == "/api/v1/agent/heartbeat" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	got := getRequests(mu, reqs)

	var found bool
	for _, r := range got {
		if r.Path == "/api/v1/agent/heartbeat" {
			found = true
			s.Equal("saas-assigned-id", r.Body["agent_id"])
			s.Contains(r.Body, "uptime_s", "expected uptime_s in heartbeat")
			s.Contains(r.Body, "active_executions", "expected active_executions in heartbeat")
			break
		}
	}

	s.Require().True(found, "no /heartbeat request received")
}

func (s *ExporterTestSuite) TestActiveExecutionTracking() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(newTestConfig("http://localhost:0", bus))

	exp.trackExecution(event.Event{
		Type:        event.WorkflowStarted,
		ExecutionID: "exec-1",
	})
	exp.trackExecution(event.Event{
		Type:        event.WorkflowStarted,
		ExecutionID: "exec-2",
	})

	exp.mu.Lock()
	s.Len(exp.activeExecutions, 2)
	exp.mu.Unlock()

	exp.trackExecution(event.Event{
		Type:        event.WorkflowCompleted,
		ExecutionID: "exec-1",
	})

	exp.mu.Lock()
	s.Len(exp.activeExecutions, 1)
	exp.mu.Unlock()
}

func (s *ExporterTestSuite) TestRetryOnError() {
	var (
		mu       sync.Mutex
		attempts int
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/register" {
			w.WriteHeader(http.StatusOK)
			return
		}

		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()

		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"agent_id": "saas-retry-id"})
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := newTestConfig(srv.URL, bus)
	exp := NewExporter(cfg)
	exp.Start(ctx)

	// Wait for at least 3 attempts and successful registration
	s.Eventually(func() bool {
		mu.Lock()
		got := attempts
		mu.Unlock()
		return got >= 3
	}, 10*time.Second, 50*time.Millisecond)

	s.Eventually(func() bool {
		_, ok := exp.isRegistered()
		return ok
	}, 5*time.Second, 50*time.Millisecond)

	mu.Lock()
	got := attempts
	mu.Unlock()

	s.GreaterOrEqual(got, 3, "expected at least 3 attempts")

	agentID, ok := exp.isRegistered()
	s.Require().True(ok, "expected exporter to be registered after retries")
	s.Equal("saas-retry-id", agentID)
}

func (s *ExporterTestSuite) TestServerConfigPush() {
	var (
		mu         sync.Mutex
		heartbeats int
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/register" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"agent_id": "saas-config-id"})
			return
		}

		if r.URL.Path == "/api/v1/agent/heartbeat" {
			mu.Lock()
			heartbeats++
			n := heartbeats
			mu.Unlock()

			if n == 1 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"heartbeat_interval_s": 1.0,
					"flush_interval_s":     0.5,
				})
				return
			}
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := newTestConfig(srv.URL, bus)
	cfg.HeartbeatInterval = 2 * time.Second
	cfg.FlushInterval = 2 * time.Second
	exp := NewExporter(cfg)
	exp.Start(ctx)

	// Wait for the config to be applied (intervals updated after first heartbeat)
	s.Eventually(func() bool {
		exp.mu.Lock()
		hb := exp.heartbeatInterval
		exp.mu.Unlock()
		return hb == 1*time.Second
	}, 5*time.Second, 50*time.Millisecond)

	s.Eventually(func() bool {
		exp.mu.Lock()
		fl := exp.flushInterval
		exp.mu.Unlock()
		return fl == 500*time.Millisecond
	}, 5*time.Second, 50*time.Millisecond)

	// Wait for at least 2 heartbeats (second one uses the new 1s interval)
	s.Eventually(func() bool {
		mu.Lock()
		got := heartbeats
		mu.Unlock()
		return got >= 2
	}, 10*time.Second, 50*time.Millisecond)

	exp.mu.Lock()
	hb := exp.heartbeatInterval
	fl := exp.flushInterval
	exp.mu.Unlock()

	s.Equal(1*time.Second, hb)
	s.Equal(500*time.Millisecond, fl)

	mu.Lock()
	got := heartbeats
	mu.Unlock()

	s.GreaterOrEqual(got, 2, "expected at least 2 heartbeats")
}

func (s *ExporterTestSuite) TestApplyServerConfigMinimums() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(newTestConfig("http://localhost:0", bus))

	tooFast := 0.1
	body, _ := json.Marshal(serverConfig{HeartbeatIntervalS: &tooFast})

	exp.mu.Lock()
	before := exp.heartbeatInterval
	exp.mu.Unlock()

	changed := exp.applyServerConfig(body)

	exp.mu.Lock()
	after := exp.heartbeatInterval
	exp.mu.Unlock()

	s.False(changed, "should not have changed with value below minimum")
	s.Equal(before, after, "heartbeat interval should not have changed")

	tooFastFlush := 0.01
	body, _ = json.Marshal(serverConfig{FlushIntervalS: &tooFastFlush})

	exp.mu.Lock()
	beforeFlush := exp.flushInterval
	exp.mu.Unlock()

	exp.applyServerConfig(body)

	exp.mu.Lock()
	afterFlush := exp.flushInterval
	exp.mu.Unlock()

	s.Equal(beforeFlush, afterFlush, "flush interval should not have changed")
}

func (s *ExporterTestSuite) TestBufferingBeforeRegistration() {
	var (
		mu          sync.Mutex
		registerOK  bool
		ingestCalls []map[string]any
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/register" {
			mu.Lock()
			ok := registerOK
			mu.Unlock()

			if !ok {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"agent_id": "delayed-id"})
			return
		}

		if r.URL.Path == "/api/v1/agent/ingest" {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			ingestCalls = append(ingestCalls, body)
			mu.Unlock()
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := newTestConfig(srv.URL, bus)
	cfg.FlushInterval = 50 * time.Millisecond
	exp := NewExporter(cfg)
	exp.Start(ctx)

	// Publish events BEFORE registration succeeds
	bus.Publish(event.Event{
		Type:        event.StepStarted,
		Timestamp:   time.Now(),
		ExecutionID: "exec-1",
		StepID:      "step-1",
	})

	// Verify events are NOT sent while registration is failing
	s.Never(func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(ingestCalls) > 0
	}, 200*time.Millisecond, 10*time.Millisecond)

	// Now allow registration
	mu.Lock()
	registerOK = true
	mu.Unlock()

	// Wait for register retry + flush to deliver the buffered events
	s.Eventually(func() bool {
		mu.Lock()
		got := len(ingestCalls)
		mu.Unlock()
		return got > 0
	}, 5*time.Second, 50*time.Millisecond)

	mu.Lock()
	got := len(ingestCalls)
	mu.Unlock()

	s.Require().Greater(got, 0, "expected ingest call after registration, got none")

	// Verify the buffered event was sent with the SaaS-assigned ID
	mu.Lock()
	firstIngest := ingestCalls[0]
	mu.Unlock()

	s.Equal("delayed-id", firstIngest["agent_id"])
}

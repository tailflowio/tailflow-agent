package export

import (
	"context"
	"encoding/json"
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

// collectRequests creates a test server that records requests and returns
// an agent_id on /register.
func collectRequests(t *testing.T) (*httptest.Server, *[]request, *sync.Mutex) {
	t.Helper()

	var (
		reqs []request
		mu   sync.Mutex
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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
	cp := make([]request, len(*reqs))
	copy(cp, *reqs)
	return cp
}

func newTestConfig(url string, bus *event.Bus) Config {
	return Config{
		ExportURL:           url,
		APIKey:              "test-key",
		EventBus:            bus,
		Logger:              slog.Default(),
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

func (s *ExporterTestSuite) TestRegistration() {
	srv, reqs, mu := collectRequests(s.T())
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp := New(newTestConfig(srv.URL, bus))
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
			if r.Body["workflow_name"] != "test-wf" {
				s.T().Errorf("expected workflow_name=test-wf, got %v", r.Body["workflow_name"])
			}
			if r.Body["workflow_description"] != "A test workflow" {
				s.T().Errorf("expected workflow_description, got %v", r.Body["workflow_description"])
			}
			if r.Body["trigger_type"] != "http" {
				s.T().Errorf("expected trigger_type=http, got %v", r.Body["trigger_type"])
			}
			if r.Body["version"] != "1.0.0" {
				s.T().Errorf("expected version=1.0.0, got %v", r.Body["version"])
			}
			// agent_id should NOT be in the request — it's assigned by the SaaS
			if _, hasID := r.Body["agent_id"]; hasID {
				s.T().Error("register request should not contain agent_id")
			}
			break
		}
	}

	if !found {
		s.T().Fatal("no /register request received")
	}

	// Verify agent_id was assigned from response
	agentID, ok := exp.isRegistered()
	if !ok {
		s.T().Fatal("expected exporter to be registered")
	}
	if agentID != "saas-assigned-id" {
		s.T().Errorf("expected agent_id=saas-assigned-id, got %v", agentID)
	}
}

func (s *ExporterTestSuite) TestBatchFlush() {
	srv, reqs, mu := collectRequests(s.T())
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp := New(newTestConfig(srv.URL, bus))
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
			if r.Body["agent_id"] != "saas-assigned-id" {
				s.T().Errorf("expected agent_id=saas-assigned-id in ingest, got %v", r.Body["agent_id"])
			}
			events, ok := r.Body["events"].([]any)
			if !ok {
				s.T().Fatalf("expected events array, got %T", r.Body["events"])
			}
			if len(events) == 0 {
				s.T().Fatal("expected at least one event in ingest")
			}
			break
		}
	}

	if !ingestFound {
		s.T().Fatal("no /ingest request received")
	}
}

func (s *ExporterTestSuite) TestMetricsFiltered() {
	srv, reqs, mu := collectRequests(s.T())
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp := New(newTestConfig(srv.URL, bus))
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
				if ev["type"] == string(event.Metrics) {
					s.T().Fatal("metrics event should have been filtered out")
				}
			}
		}
	}
}

func (s *ExporterTestSuite) TestHeartbeat() {
	srv, reqs, mu := collectRequests(s.T())
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp := New(newTestConfig(srv.URL, bus))
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
			if r.Body["agent_id"] != "saas-assigned-id" {
				s.T().Errorf("expected agent_id=saas-assigned-id in heartbeat, got %v", r.Body["agent_id"])
			}
			if _, ok := r.Body["uptime_s"]; !ok {
				s.T().Error("expected uptime_s in heartbeat")
			}
			if _, ok := r.Body["active_executions"]; !ok {
				s.T().Error("expected active_executions in heartbeat")
			}
			break
		}
	}

	if !found {
		s.T().Fatal("no /heartbeat request received")
	}
}

func (s *ExporterTestSuite) TestActiveExecutionTracking() {
	bus := event.NewBus()
	defer bus.Close()

	exp := New(newTestConfig("http://localhost:0", bus))

	exp.trackExecution(event.Event{
		Type:        event.WorkflowStarted,
		ExecutionID: "exec-1",
	})
	exp.trackExecution(event.Event{
		Type:        event.WorkflowStarted,
		ExecutionID: "exec-2",
	})

	exp.mu.Lock()
	if len(exp.activeExecutions) != 2 {
		s.T().Errorf("expected 2 active executions, got %d", len(exp.activeExecutions))
	}
	exp.mu.Unlock()

	exp.trackExecution(event.Event{
		Type:        event.WorkflowCompleted,
		ExecutionID: "exec-1",
	})

	exp.mu.Lock()
	if len(exp.activeExecutions) != 1 {
		s.T().Errorf("expected 1 active execution, got %d", len(exp.activeExecutions))
	}
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
	exp := New(cfg)
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

	if got < 3 {
		s.T().Errorf("expected at least 3 attempts, got %d", got)
	}

	agentID, ok := exp.isRegistered()
	if !ok {
		s.T().Fatal("expected exporter to be registered after retries")
	}
	if agentID != "saas-retry-id" {
		s.T().Errorf("expected agent_id=saas-retry-id, got %v", agentID)
	}
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
	exp := New(cfg)
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

	if hb != 1*time.Second {
		s.T().Errorf("expected heartbeat interval 1s, got %v", hb)
	}
	if fl != 500*time.Millisecond {
		s.T().Errorf("expected flush interval 500ms, got %v", fl)
	}

	mu.Lock()
	got := heartbeats
	mu.Unlock()

	if got < 2 {
		s.T().Errorf("expected at least 2 heartbeats, got %d", got)
	}
}

func (s *ExporterTestSuite) TestApplyServerConfigMinimums() {
	bus := event.NewBus()
	defer bus.Close()

	exp := New(newTestConfig("http://localhost:0", bus))

	tooFast := 0.1
	body, _ := json.Marshal(serverConfig{HeartbeatIntervalS: &tooFast})

	exp.mu.Lock()
	before := exp.heartbeatInterval
	exp.mu.Unlock()

	changed := exp.applyServerConfig(body)

	exp.mu.Lock()
	after := exp.heartbeatInterval
	exp.mu.Unlock()

	if changed {
		s.T().Error("should not have changed with value below minimum")
	}
	if before != after {
		s.T().Errorf("heartbeat interval should not have changed: before=%v after=%v", before, after)
	}

	tooFastFlush := 0.01
	body, _ = json.Marshal(serverConfig{FlushIntervalS: &tooFastFlush})

	exp.mu.Lock()
	beforeFlush := exp.flushInterval
	exp.mu.Unlock()

	exp.applyServerConfig(body)

	exp.mu.Lock()
	afterFlush := exp.flushInterval
	exp.mu.Unlock()

	if beforeFlush != afterFlush {
		s.T().Errorf("flush interval should not have changed: before=%v after=%v", beforeFlush, afterFlush)
	}
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
	exp := New(cfg)
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

	if got == 0 {
		s.T().Fatal("expected ingest call after registration, got none")
	}

	// Verify the buffered event was sent with the SaaS-assigned ID
	mu.Lock()
	firstIngest := ingestCalls[0]
	mu.Unlock()

	if firstIngest["agent_id"] != "delayed-id" {
		s.T().Errorf("expected agent_id=delayed-id in ingest, got %v", firstIngest["agent_id"])
	}
}

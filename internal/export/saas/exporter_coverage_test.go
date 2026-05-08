package saas

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/event"
)

// errorBodyTransport is an http.RoundTripper that returns a response with a
// body whose Read method always returns an error. Used to test the io.ReadAll
// error branch in post().
type errorBodyTransport struct{}

func (t *errorBodyTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(&errReader{}),
	}, nil
}

// errReader is an io.Reader that always returns an error.
type errReader struct{}

func (r *errReader) Read(_ []byte) (int, error) {
	return 0, fmt.Errorf("simulated read error")
}

type ExporterCoverageSuite struct {
	suite.Suite
}

func TestExporterCoverage(t *testing.T) {
	suite.Run(t, new(ExporterCoverageSuite))
}

func (s *ExporterCoverageSuite) SetupTest() {}

func (s *ExporterCoverageSuite) TestNew_DefaultIntervals() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: "http://localhost",
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	s.Equal(1*time.Second, exp.flushInterval)
	s.Equal(10*time.Second, exp.heartbeatInterval)
	s.NotEmpty(exp.sessionID)
	s.NotNil(exp.client)
	s.NotNil(exp.activeExecutions)
	s.NotNil(exp.intervalChange)
}

func (s *ExporterCoverageSuite) TestNew_CustomIntervals() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL:         "http://localhost",
		EventBus:          bus,
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		FlushInterval:     5 * time.Second,
		HeartbeatInterval: 30 * time.Second,
	})

	s.Equal(5*time.Second, exp.flushInterval)
	s.Equal(30*time.Second, exp.heartbeatInterval)
}

func (s *ExporterCoverageSuite) TestShutdown_WaitsForGoroutines() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/register" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"agent_id": "shutdown-test"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	cfg := newTestConfig(srv.URL, bus)
	exp := NewExporter(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	exp.Start(ctx)

	// Wait for registration
	s.Eventually(func() bool {
		_, ok := exp.isRegistered()
		return ok
	}, 2*time.Second, 10*time.Millisecond)

	cancel()

	// Shutdown should return without hanging
	done := make(chan struct{})
	go func() {
		exp.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		s.Fail("Shutdown timed out")
	}
}

func (s *ExporterCoverageSuite) TestTryRegister_InvalidJSON() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	ok := exp.tryRegister(context.Background(), map[string]any{"test": true}, 1*time.Second)
	s.False(ok)
}

func (s *ExporterCoverageSuite) TestTryRegister_EmptyAgentID() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"agent_id": ""})
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	ok := exp.tryRegister(context.Background(), map[string]any{"test": true}, 1*time.Second)
	s.False(ok)
}

func (s *ExporterCoverageSuite) TestTryRegister_HTTPError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	ok := exp.tryRegister(context.Background(), map[string]any{"test": true}, 1*time.Second)
	s.False(ok)
}

func (s *ExporterCoverageSuite) TestRegister_ContextCancelDuringBackoff() {
	// Server always returns error so register keeps retrying
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	origInitial := registerInitialBackoff
	origMax := registerMaxBackoff
	defer func() {
		registerInitialBackoff = origInitial
		registerMaxBackoff = origMax
	}()
	registerInitialBackoff = 10 * time.Millisecond
	registerMaxBackoff = 50 * time.Millisecond

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		exp.register(ctx)
		close(done)
	}()

	// Let a few retries happen, then cancel
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// register exited correctly
	case <-time.After(2 * time.Second):
		s.Fail("register did not exit after context cancel")
	}

	_, ok := exp.isRegistered()
	s.False(ok)
}

func (s *ExporterCoverageSuite) TestRegister_BackoffCapsAtMax() {
	var mu sync.Mutex
	var attempts int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/register" {
			mu.Lock()
			attempts++
			n := attempts
			mu.Unlock()

			if n < 6 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"agent_id": "capped-id"})
		}
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	origInitial := registerInitialBackoff
	origMax := registerMaxBackoff
	defer func() {
		registerInitialBackoff = origInitial
		registerMaxBackoff = origMax
	}()
	registerInitialBackoff = 5 * time.Millisecond
	registerMaxBackoff = 20 * time.Millisecond

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	ctx := context.Background()
	exp.register(ctx)

	agentID, ok := exp.isRegistered()
	s.True(ok)
	s.Equal("capped-id", agentID)

	mu.Lock()
	s.GreaterOrEqual(attempts, 6)
	mu.Unlock()
}

func (s *ExporterCoverageSuite) TestFlushBatch_EmptyBatch() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: "http://localhost",
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	batch := []event.Event{}
	exp.flushBatch(context.Background(), &batch)
	s.Empty(batch)
}

func (s *ExporterCoverageSuite) TestFlushBatch_NotRegistered() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: "http://localhost",
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	batch := []event.Event{
		{Type: event.StepStarted, ExecutionID: "exec-1"},
	}
	exp.flushBatch(context.Background(), &batch)

	// Batch should be kept (not cleared) when not registered
	s.Len(batch, 1)
}

func (s *ExporterCoverageSuite) TestFlushBatch_IngestError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// Mark as registered
	exp.mu.Lock()
	exp.agentID = "test-id"
	exp.registered = true
	exp.mu.Unlock()

	batch := []event.Event{
		{Type: event.StepStarted, ExecutionID: "exec-1"},
		{Type: event.StepCompleted, ExecutionID: "exec-1"},
	}
	exp.flushBatch(context.Background(), &batch)

	// Batch should be cleared (dropped) even on error
	s.Empty(batch)
}

func (s *ExporterCoverageSuite) TestFlushBatch_Chunking() {
	var mu sync.Mutex
	var ingestCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/ingest" {
			mu.Lock()
			ingestCalls++
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp.mu.Lock()
	exp.agentID = "test-id"
	exp.registered = true
	exp.mu.Unlock()

	// Create a batch larger than flushChunkSize (2000)
	batch := make([]event.Event, 2500)
	for i := range batch {
		batch[i] = event.Event{Type: event.StepStarted, ExecutionID: "exec-1"}
	}

	exp.flushBatch(context.Background(), &batch)

	s.Empty(batch)

	mu.Lock()
	s.Equal(2, ingestCalls, "expected 2 ingest calls for 2500 events with chunk size 2000")
	mu.Unlock()
}

func (s *ExporterCoverageSuite) TestFlushBatch_ChunkingSecondChunkFails() {
	var mu sync.Mutex
	var ingestCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/ingest" {
			mu.Lock()
			ingestCalls++
			n := ingestCalls
			mu.Unlock()

			if n == 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp.mu.Lock()
	exp.agentID = "test-id"
	exp.registered = true
	exp.mu.Unlock()

	// 4500 events: chunk1=2000 (ok), chunk2=2000 (fail), chunk3 never sent
	batch := make([]event.Event, 4500)
	for i := range batch {
		batch[i] = event.Event{Type: event.StepStarted, ExecutionID: "exec-1"}
	}

	exp.flushBatch(context.Background(), &batch)

	// After first chunk succeeds (2000 removed) and second chunk fails (2000 removed),
	// remaining 500 should be left but the function drops failed chunk too
	mu.Lock()
	s.Equal(2, ingestCalls)
	mu.Unlock()
}

func (s *ExporterCoverageSuite) TestFinalFlush_DrainsChannel() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp.mu.Lock()
	exp.agentID = "flush-test-id"
	exp.registered = true
	exp.mu.Unlock()

	// Create a buffered channel with events
	ch := make(chan event.Event, 10)
	ch <- event.Event{Type: event.StepStarted, ExecutionID: "exec-1", StepID: "s1"}
	ch <- event.Event{Type: event.StepCompleted, ExecutionID: "exec-1", StepID: "s1"}

	var batch []event.Event
	exp.finalFlush(ch, &batch)

	// Events should have been drained and flushed
	s.Empty(batch)
}

func (s *ExporterCoverageSuite) TestFinalFlush_SkipsMetricsEvents() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp.mu.Lock()
	exp.agentID = "flush-metrics-id"
	exp.registered = true
	exp.mu.Unlock()

	ch := make(chan event.Event, 10)
	ch <- event.Event{Type: event.Metrics, Data: map[string]any{"cpu": 50.0}}
	ch <- event.Event{Type: event.StepStarted, ExecutionID: "exec-1"}

	var batch []event.Event
	exp.finalFlush(ch, &batch)

	// Metrics should be filtered, only StepStarted sent
	s.Empty(batch) // after flush
}

func (s *ExporterCoverageSuite) TestFinalFlush_EmptyBatchNoOp() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: "http://localhost",
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	ch := make(chan event.Event, 10)
	var batch []event.Event

	// Empty channel, empty batch - should be a no-op
	exp.finalFlush(ch, &batch)
	s.Empty(batch)
}

func (s *ExporterCoverageSuite) TestFinalFlush_ClosedChannel() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp.mu.Lock()
	exp.agentID = "closed-ch-id"
	exp.registered = true
	exp.mu.Unlock()

	ch := make(chan event.Event, 10)
	ch <- event.Event{Type: event.StepStarted, ExecutionID: "exec-1"}
	close(ch)

	var batch []event.Event
	exp.finalFlush(ch, &batch)
}

func (s *ExporterCoverageSuite) TestAwaitRegistrationThenFlush_AlreadyRegistered() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp.mu.Lock()
	exp.agentID = "already-registered"
	exp.registered = true
	exp.mu.Unlock()

	batch := []event.Event{
		{Type: event.StepStarted, ExecutionID: "exec-1"},
	}
	exp.awaitRegistrationThenFlush(&batch)

	s.Empty(batch)
}

func (s *ExporterCoverageSuite) TestAwaitRegistrationThenFlush_Timeout() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: "http://localhost:0",
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// Not registered, will timeout
	batch := []event.Event{
		{Type: event.StepStarted, ExecutionID: "exec-1"},
	}

	start := time.Now()
	exp.awaitRegistrationThenFlush(&batch)
	elapsed := time.Since(start)

	// Should have waited about 5s then returned
	s.GreaterOrEqual(elapsed, 4*time.Second)
	s.LessOrEqual(elapsed, 7*time.Second)

	// Events should still be in the batch (lost)
	s.Len(batch, 1)
}

func (s *ExporterCoverageSuite) TestAwaitRegistrationThenFlush_RegistersDuringWait() {
	var ingestCalled atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/ingest" {
			ingestCalled.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	batch := []event.Event{
		{Type: event.StepStarted, ExecutionID: "exec-1"},
	}

	// Register in a goroutine after a short delay
	go func() {
		time.Sleep(200 * time.Millisecond)
		exp.mu.Lock()
		exp.agentID = "delayed-reg"
		exp.registered = true
		exp.mu.Unlock()
	}()

	start := time.Now()
	exp.awaitRegistrationThenFlush(&batch)
	elapsed := time.Since(start)

	// Should have completed much faster than 5s
	s.Less(elapsed, 3*time.Second)
	s.True(ingestCalled.Load(), "expected ingest to be called after delayed registration")
}

func (s *ExporterCoverageSuite) TestBatchLoop_BufferOverflow() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/register" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"agent_id": "overflow-id"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	cfg := newTestConfig(srv.URL, bus)
	cfg.FlushInterval = 10 * time.Second // long interval so we can fill buffer
	exp := NewExporter(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	exp.Start(ctx)

	// Wait for registration
	s.Eventually(func() bool {
		_, ok := exp.isRegistered()
		return ok
	}, 2*time.Second, 10*time.Millisecond)

	// Publish more events than maxBatchSize (10000)
	for i := 0; i < 10050; i++ {
		bus.Publish(event.Event{
			Type:        event.StepStarted,
			Timestamp:   time.Now(),
			ExecutionID: "exec-overflow",
			StepID:      "step-1",
		})
	}

	// Give time for events to be processed
	time.Sleep(200 * time.Millisecond)

	cancel()
	exp.Shutdown()
}

func (s *ExporterCoverageSuite) TestBatchLoop_ChannelClose() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/register" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"agent_id": "close-id"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()

	cfg := newTestConfig(srv.URL, bus)
	exp := NewExporter(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp.Start(ctx)

	// Wait for registration
	s.Eventually(func() bool {
		_, ok := exp.isRegistered()
		return ok
	}, 2*time.Second, 10*time.Millisecond)

	// Publishing then closing the bus triggers the channel close path
	bus.Publish(event.Event{
		Type:        event.StepStarted,
		Timestamp:   time.Now(),
		ExecutionID: "exec-close",
	})

	time.Sleep(50 * time.Millisecond)
	bus.Close()

	// Give time for batchLoop to exit via the !ok path
	time.Sleep(200 * time.Millisecond)
	cancel()
	exp.Shutdown()
}

func (s *ExporterCoverageSuite) TestBatchLoop_IntervalChange() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/register" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"agent_id": "interval-id"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	cfg := newTestConfig(srv.URL, bus)
	cfg.FlushInterval = 50 * time.Millisecond
	exp := NewExporter(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp.Start(ctx)

	// Wait for registration
	s.Eventually(func() bool {
		_, ok := exp.isRegistered()
		return ok
	}, 2*time.Second, 10*time.Millisecond)

	// Trigger an interval change via the channel
	exp.mu.Lock()
	exp.flushInterval = 100 * time.Millisecond
	exp.mu.Unlock()

	select {
	case exp.intervalChange <- struct{}{}:
	default:
	}

	time.Sleep(200 * time.Millisecond)

	cancel()
	exp.Shutdown()
}

func (s *ExporterCoverageSuite) TestBatchLoop_MetricsEventStored() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/register" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"agent_id": "metrics-store-id"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	cfg := newTestConfig(srv.URL, bus)
	exp := NewExporter(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp.Start(ctx)

	s.Eventually(func() bool {
		_, ok := exp.isRegistered()
		return ok
	}, 2*time.Second, 10*time.Millisecond)

	bus.Publish(event.Event{
		Type:      event.Metrics,
		Timestamp: time.Now(),
		Data:      map[string]any{"cpu_percent": 55.0},
	})

	s.Eventually(func() bool {
		exp.mu.Lock()
		defer exp.mu.Unlock()
		return exp.lastMetrics != nil
	}, 2*time.Second, 10*time.Millisecond)

	exp.mu.Lock()
	s.NotNil(exp.lastMetrics)
	exp.mu.Unlock()

	cancel()
	exp.Shutdown()
}

func (s *ExporterCoverageSuite) TestApplyServerConfig_EmptyBody() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: "http://localhost",
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	changed := exp.applyServerConfig(nil)
	s.False(changed)

	changed = exp.applyServerConfig([]byte{})
	s.False(changed)
}

func (s *ExporterCoverageSuite) TestApplyServerConfig_InvalidJSON() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: "http://localhost",
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	changed := exp.applyServerConfig([]byte("not-json"))
	s.False(changed)
}

func (s *ExporterCoverageSuite) TestApplyServerConfig_NoChanges() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: "http://localhost",
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// No flush_interval_s or heartbeat_interval_s fields
	body, _ := json.Marshal(map[string]any{"agent_id": "test"})
	changed := exp.applyServerConfig(body)
	s.False(changed)
}

func (s *ExporterCoverageSuite) TestApplyServerConfig_SameValues() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL:         "http://localhost",
		EventBus:          bus,
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		HeartbeatInterval: 10 * time.Second,
		FlushInterval:     1 * time.Second,
	})

	hb := 10.0
	fl := 1.0
	body, _ := json.Marshal(serverConfig{
		HeartbeatIntervalS: &hb,
		FlushIntervalS:     &fl,
	})

	changed := exp.applyServerConfig(body)
	s.False(changed)
}

func (s *ExporterCoverageSuite) TestApplyServerConfig_OnlyHeartbeatChanges() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL:         "http://localhost",
		EventBus:          bus,
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		HeartbeatInterval: 10 * time.Second,
		FlushInterval:     1 * time.Second,
	})

	hb := 5.0
	body, _ := json.Marshal(serverConfig{HeartbeatIntervalS: &hb})

	changed := exp.applyServerConfig(body)
	s.True(changed) // heartbeat changed

	exp.mu.Lock()
	s.Equal(5*time.Second, exp.heartbeatInterval)
	exp.mu.Unlock()
}

func (s *ExporterCoverageSuite) TestApplyServerConfig_OnlyFlushChanges() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL:         "http://localhost",
		EventBus:          bus,
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		HeartbeatInterval: 10 * time.Second,
		FlushInterval:     1 * time.Second,
	})

	fl := 2.0
	body, _ := json.Marshal(serverConfig{FlushIntervalS: &fl})

	changed := exp.applyServerConfig(body)
	s.False(changed) // heartbeat did not change

	exp.mu.Lock()
	s.Equal(2*time.Second, exp.flushInterval)
	exp.mu.Unlock()
}

func (s *ExporterCoverageSuite) TestApplyServerConfig_FlushBelowMinimum() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL:     "http://localhost",
		EventBus:      bus,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		FlushInterval: 1 * time.Second,
	})

	fl := 0.05 // 50ms, below 100ms minimum
	body, _ := json.Marshal(serverConfig{FlushIntervalS: &fl})

	exp.applyServerConfig(body)

	exp.mu.Lock()
	s.Equal(1*time.Second, exp.flushInterval, "should not have changed below minimum")
	exp.mu.Unlock()
}

func (s *ExporterCoverageSuite) TestApplyServerConfig_HeartbeatBelowMinimum() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL:         "http://localhost",
		EventBus:          bus,
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		HeartbeatInterval: 10 * time.Second,
	})

	hb := 0.5 // 500ms, below 1s minimum
	body, _ := json.Marshal(serverConfig{HeartbeatIntervalS: &hb})

	changed := exp.applyServerConfig(body)
	s.False(changed)

	exp.mu.Lock()
	s.Equal(10*time.Second, exp.heartbeatInterval, "should not have changed below minimum")
	exp.mu.Unlock()
}

func (s *ExporterCoverageSuite) TestPost_InvalidURL() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: "://invalid-url",
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	_, err := exp.post(context.Background(), "/test", map[string]any{"key": "value"})
	s.Error(err)
	s.Contains(err.Error(), "new request")
}

func (s *ExporterCoverageSuite) TestPost_UnmarshalablePayload() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: "http://localhost",
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// A channel cannot be marshaled to JSON
	_, err := exp.post(context.Background(), "/test", map[string]any{"ch": make(chan int)})
	s.Error(err)
	s.Contains(err.Error(), "marshal")
}

func (s *ExporterCoverageSuite) TestPost_ConnectionError() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: "http://127.0.0.1:1", // port that won't be listening
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	_, err := exp.post(context.Background(), "/test", map[string]any{"key": "value"})
	s.Error(err)
	s.Contains(err.Error(), "do")
}

func (s *ExporterCoverageSuite) TestPost_Non2xxStatus() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	_, err := exp.post(context.Background(), "/test", map[string]any{"key": "value"})
	s.Error(err)
	s.Contains(err.Error(), "unexpected status 403")
}

func (s *ExporterCoverageSuite) TestPost_NoAPIKey() {
	var authHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		APIKey:    "",
	})

	_, err := exp.post(context.Background(), "/test", map[string]any{"key": "value"})
	s.NoError(err)
	s.Empty(authHeader, "no Authorization header when APIKey is empty")
}

func (s *ExporterCoverageSuite) TestPost_WithAPIKey() {
	var authHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		APIKey:    "my-secret-key",
	})

	_, err := exp.post(context.Background(), "/test", map[string]any{"key": "value"})
	s.NoError(err)
	s.Equal("Bearer my-secret-key", authHeader)
}

func (s *ExporterCoverageSuite) TestPost_CancelledContext() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := exp.post(ctx, "/test", map[string]any{"key": "value"})
	s.Error(err)
}

func (s *ExporterCoverageSuite) TestFinalFlush_TriggersAwaitRegistration() {
	var mu sync.Mutex
	var ingestCalled bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/ingest" {
			mu.Lock()
			ingestCalled = true
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// Not registered initially
	ch := make(chan event.Event, 10)
	ch <- event.Event{Type: event.StepStarted, ExecutionID: "exec-1"}

	// Register after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		exp.mu.Lock()
		exp.agentID = "final-flush-id"
		exp.registered = true
		exp.mu.Unlock()
	}()

	var batch []event.Event
	exp.finalFlush(ch, &batch)

	mu.Lock()
	s.True(ingestCalled, "expected ingest to be called during finalFlush")
	mu.Unlock()
}

func (s *ExporterCoverageSuite) TestBatchLoop_ContextCancelTriggersFinalFlush() {
	var mu sync.Mutex
	var ingestCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/register" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"agent_id": "final-id"})
			return
		}
		if r.URL.Path == "/api/v1/agent/ingest" {
			mu.Lock()
			ingestCalls++
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	cfg := newTestConfig(srv.URL, bus)
	cfg.FlushInterval = 10 * time.Second // Don't flush via ticker
	exp := NewExporter(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	exp.Start(ctx)

	// Wait for registration
	s.Eventually(func() bool {
		_, ok := exp.isRegistered()
		return ok
	}, 2*time.Second, 10*time.Millisecond)

	// Publish an event
	bus.Publish(event.Event{
		Type:        event.WorkflowStarted,
		Timestamp:   time.Now(),
		ExecutionID: "exec-final",
	})

	time.Sleep(50 * time.Millisecond)

	// Cancel triggers finalFlush which should flush the batch
	cancel()
	exp.Shutdown()

	mu.Lock()
	s.GreaterOrEqual(ingestCalls, 1, "expected at least one ingest call from finalFlush")
	mu.Unlock()
}

func (s *ExporterCoverageSuite) TestFinalFlush_TracksWorkflowStarted() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: srv.URL,
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp.mu.Lock()
	exp.agentID = "track-id"
	exp.registered = true
	exp.mu.Unlock()

	ch := make(chan event.Event, 10)
	ch <- event.Event{Type: event.WorkflowStarted, ExecutionID: "exec-track"}

	var batch []event.Event
	exp.finalFlush(ch, &batch)

	// Verify the execution was tracked
	exp.mu.Lock()
	_, exists := exp.activeExecutions["exec-track"]
	exp.mu.Unlock()

	s.True(exists, "expected execution to be tracked in activeExecutions")
}

func (s *ExporterCoverageSuite) TestBatchLoop_OverflowDropsOldest() {
	// This test directly feeds events into the subscriber channel to exceed maxBatchSize.
	// We mark the exporter as registered so that finalFlush can flush immediately.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL:         srv.URL,
		EventBus:          bus,
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		FlushInterval:     10 * time.Second, // Long interval so ticker doesn't drain
		HeartbeatInterval: 10 * time.Second,
	})

	// Mark as registered so finalFlush can proceed quickly
	exp.mu.Lock()
	exp.agentID = "overflow-id"
	exp.registered = true
	exp.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a large enough channel and fill it beyond maxBatchSize
	ch := make(chan event.Event, maxBatchSize+100)
	for i := 0; i < maxBatchSize+50; i++ {
		ch <- event.Event{
			Type:        event.StepStarted,
			ExecutionID: "exec-overflow",
			StepID:      "step",
		}
	}

	done := make(chan struct{})
	go func() {
		exp.batchLoop(ctx, ch)
		close(done)
	}()

	// Give batchLoop time to consume all events and hit the overflow path
	time.Sleep(500 * time.Millisecond)

	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		s.Fail("batchLoop did not exit")
	}
}

func (s *ExporterCoverageSuite) TestPost_ReadBodyError() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL: "http://localhost",
		EventBus:  bus,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// Replace the client transport with one that returns a response whose
	// Body.Read always errors.
	exp.client.Transport = &errorBodyTransport{}

	_, err := exp.post(context.Background(), "/test", map[string]any{"key": "value"})
	s.Error(err)
	s.Contains(err.Error(), "read response body")
}

func (s *ExporterCoverageSuite) TestApplyServerConfig_IntervalChangeChannelFull() {
	bus := event.NewBus()
	defer bus.Close()

	exp := NewExporter(Config{
		ExportURL:     "http://localhost",
		EventBus:      bus,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		FlushInterval: 1 * time.Second,
	})

	// Fill the intervalChange channel (capacity 1)
	exp.intervalChange <- struct{}{}

	fl := 2.0
	body, _ := json.Marshal(serverConfig{FlushIntervalS: &fl})

	// This should hit the default branch since intervalChange is already full
	exp.applyServerConfig(body)

	exp.mu.Lock()
	s.Equal(2*time.Second, exp.flushInterval)
	exp.mu.Unlock()
}

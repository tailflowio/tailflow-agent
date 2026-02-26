package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/export"
	"github.com/tailflow/tailflow/internal/metrics"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

// newRabbitMQConsumerFn creates a RabbitMQConsumer. Override in tests.
var newRabbitMQConsumerFn = NewRabbitMQConsumer

// newRedisKVStoreFn wraps runtime.NewRedisKVStore for testing.
var newRedisKVStoreFn = func(url string) (runtime.KVStore, error) {
	return runtime.NewRedisKVStore(url)
}

// Config holds server configuration.
type Config struct {
	Port           int
	Executor       *engine.Executor
	Workflow       *parser.Workflow
	FilePath       string
	ExecutionStore store.ExecutionStore
	EventBus       *event.Bus
	Logger         *slog.Logger
	ExportURL      string
	APIKey         string
	ExporterName   string
	Version        string
}

// Server is the HTTP server for the API and embedded UI.
type Server struct {
	config          Config
	mux             *http.ServeMux
	srv             *http.Server
	waitRegistry    *WaitRegistry
	rmqWaitMgr      *RabbitMQWaitManager
	cancelMu        sync.RWMutex
	cancels         map[string]context.CancelFunc // executionID -> cancel
	locker          runtime.Locker
	dbPool          runtime.DBPool
	kvStore         runtime.KVStore
	scheduledTimers []*time.Timer
	timerMu         sync.Mutex
	metrics         *metrics.Collector
	exporter        *export.Exporter
	sensitive       *engine.SensitiveRegistry
}

// New creates a new server.
func New(config Config) *Server {
	var kvStore runtime.KVStore

	redisURL := os.Getenv("REDIS_URL")
	if redisURL != "" {
		rs, err := newRedisKVStoreFn(redisURL)
		if err != nil {
			config.Logger.Warn("failed to connect to Redis, falling back to in-memory KV store", "error", err)

			kvStore = runtime.NewMemoryKVStore()
		} else {
			config.Logger.Info("using Redis KV store")

			kvStore = rs
		}
	} else {
		kvStore = runtime.NewMemoryKVStore()
	}

	s := &Server{
		config:       config,
		mux:          http.NewServeMux(),
		waitRegistry: NewWaitRegistry(),
		rmqWaitMgr:   NewRabbitMQWaitManager(config.Logger),
		cancels:      make(map[string]context.CancelFunc),
		locker:       runtime.NewMemoryLocker(),
		dbPool:       runtime.NewMemoryDBPool(),
		kvStore:      kvStore,
		metrics:      metrics.New(),
		sensitive:    engine.NewSensitiveRegistry(config.Workflow.Sensitive),
	}
	s.setupRoutes()

	return s
}

func (s *Server) Run(ctx context.Context) error {
	s.metrics.Start(ctx)
	s.startExporter(ctx)
	s.startMetricsRefresh(ctx)

	cronSched, err := s.startCronScheduler() //nolint:contextcheck
	if err != nil {
		return err
	}

	rmqConsumer, err := s.startRabbitMQConsumer(ctx)
	if err != nil {
		return err
	}

	s.srv = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Port),
		Handler: s.mux,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	errCh := make(chan error, 1)

	go func() {
		s.config.Logger.Info("server started", "port", s.config.Port, "url", fmt.Sprintf("http://localhost:%d", s.config.Port))

		listenErr := s.srv.ListenAndServe()
		if listenErr != nil && listenErr != http.ErrServerClosed {
			errCh <- listenErr
		}
	}()

	select {
	case <-ctx.Done():
		return s.shutdownServices(cronSched, rmqConsumer) //nolint:contextcheck
	case listenErr := <-errCh:
		return listenErr
	}
}

func (s *Server) startExporter(ctx context.Context) {
	if s.config.ExportURL == "" {
		return
	}

	var triggerType string

	t := s.config.Workflow.Trigger
	if t != nil {
		switch {
		case t.HTTP != nil:
			triggerType = "http"
		case t.Webhook != nil:
			triggerType = "webhook"
		case t.Schedule != nil:
			triggerType = "schedule"
		case t.RabbitMQ != nil:
			triggerType = "rabbitmq"
		}
	}

	s.exporter = export.New(export.Config{
		ExportURL:           s.config.ExportURL,
		APIKey:              s.config.APIKey,
		AgentName:           s.config.ExporterName,
		EventBus:            s.config.EventBus,
		Logger:              s.config.Logger,
		WorkflowName:        s.config.Workflow.Name,
		WorkflowDescription: s.config.Workflow.Description,
		WorkflowTags:        s.config.Workflow.Tags,
		TriggerType:         triggerType,
		StepsCount:          len(s.config.Workflow.Steps),
		Version:             s.config.Version,
		Revision:            s.config.Workflow.Revision,
	})
	s.exporter.Start(ctx)
}

func (s *Server) startMetricsRefresh(ctx context.Context) {
	go func() {
		s.config.ExecutionStore.RefreshStepMetrics()

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.config.ExecutionStore.RefreshStepMetrics()

				snap := s.metrics.Snapshot()
				s.config.EventBus.Publish(event.Event{
					Type:      event.Metrics,
					Timestamp: time.Now(),
					Data: map[string]any{
						"cpu_percent":  snap.CPUPercent,
						"rss_kb":       snap.RSSKB,
						"goroutines":   snap.Goroutines,
						"heap_mb":      snap.HeapMB,
						"net_rx_bytes": snap.NetRxBytes,
						"net_tx_bytes": snap.NetTxBytes,
						"uptime_s":     snap.UptimeS,
						"available":    snap.Available,
					},
				})
			}
		}
	}()
}

func (s *Server) startCronScheduler() (*CronScheduler, error) {
	wf := s.config.Workflow
	if wf.Trigger == nil || wf.Trigger.Schedule == nil {
		return nil, nil
	}

	cronSched := NewCronScheduler(s.config.Logger)
	cronExpr := wf.Trigger.Schedule.Cron

	err := cronSched.Add(cronExpr, func() {
		s.config.Logger.Info("cron trigger fired", "cron", cronExpr)
		s.runWorkflowAsync(nil)
	})
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	cronSched.Start()

	return cronSched, nil
}

func (s *Server) startRabbitMQConsumer(ctx context.Context) (*RabbitMQConsumer, error) {
	wf := s.config.Workflow
	if wf.Trigger == nil || wf.Trigger.RabbitMQ == nil {
		return nil, nil
	}

	rmqConsumer := newRabbitMQConsumerFn(wf.Trigger.RabbitMQ, s.config.Logger)

	onMessage := func(triggerData map[string]any, ackFn func(bool)) { //nolint:contextcheck
		s.config.Logger.Info("rabbitmq message received", "queue", wf.Trigger.RabbitMQ.Queue)

		opts := asyncRunOpts{TriggerData: triggerData}
		if ackFn != nil {
			opts.OnComplete = func(_ string, success bool) {
				ackFn(success)
			}
		}

		s.runWorkflowAsync(nil, opts)
	}

	err := rmqConsumer.Start(ctx, onMessage)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq consumer: %w", err)
	}

	return rmqConsumer, nil
}

func (s *Server) shutdownServices(cronSched *CronScheduler, rmqConsumer *RabbitMQConsumer) error {
	s.config.Logger.Info("shutting down server")

	s.cancelAllExecutions()

	if cronSched != nil {
		cronSched.Stop()
	}

	if rmqConsumer != nil {
		rmqConsumer.Stop()
	}

	s.rmqWaitMgr.Close()

	if s.dbPool != nil {
		s.dbPool.Close()
	}

	if s.kvStore != nil {
		s.kvStore.Close()
	}

	s.cancelScheduledTimers()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	return s.srv.Shutdown(shutdownCtx)
}

// asyncRunOpts holds optional settings for runWorkflowAsync.
type asyncRunOpts struct {
	TriggerData map[string]any
	OnComplete  func(executionID string, success bool)
}

// runWorkflowAsync starts an async workflow execution and returns the execution ID.
func (s *Server) runWorkflowAsync(params map[string]any, opts ...asyncRunOpts) string {
	wf := s.config.Workflow
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

	var opt asyncRunOpts
	if len(opts) > 0 {
		opt = opts[0]
	}

	execCtx, cancel := context.WithCancel(context.Background())
	s.registerCancel(executionID, cancel)

	go func() {
		defer stopCapture()
		defer s.unregisterCancel(executionID)

		result, err := s.config.Executor.Execute(execCtx, wf, params, engine.ExecuteOptions{
			ExecutionID: executionID,
			Services:    services,
			TriggerData: opt.TriggerData,
		})
		s.finalizeExecution(executionID, result, err, execCtx)

		if opt.OnComplete != nil {
			success := err == nil && result != nil && result.Status == runtime.StatusSuccess
			opt.OnComplete(executionID, success)
		}
	}()

	return executionID
}

// Handler returns the HTTP handler (for testing).
func (s *Server) Handler() http.Handler {
	return s.mux
}

// WaitRegistry returns the server's wait registry.
func (s *Server) WaitRegistry() *WaitRegistry {
	return s.waitRegistry
}

// registerCancel stores a cancel function for a running execution.
func (s *Server) registerCancel(executionID string, cancel context.CancelFunc) {
	s.cancelMu.Lock()
	s.cancels[executionID] = cancel
	s.cancelMu.Unlock()
}

// unregisterCancel removes the cancel function for a finished execution.
func (s *Server) unregisterCancel(executionID string) {
	s.cancelMu.Lock()
	delete(s.cancels, executionID)
	s.cancelMu.Unlock()
}

// cancelAllExecutions cancels every running execution for graceful shutdown.
func (s *Server) cancelAllExecutions() {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()

	for _, cancel := range s.cancels {
		cancel()
	}
}

// cancelExecution cancels a running execution. Returns false if not found.
func (s *Server) cancelExecution(executionID string) bool {
	s.cancelMu.RLock()
	cancel, ok := s.cancels[executionID]
	s.cancelMu.RUnlock()

	if ok {
		cancel()
	}

	return ok
}

// addScheduledTimer registers a timer for cleanup on shutdown.
func (s *Server) addScheduledTimer(t *time.Timer) {
	s.timerMu.Lock()
	s.scheduledTimers = append(s.scheduledTimers, t)
	s.timerMu.Unlock()
}

// cancelScheduledTimers stops all pending scheduled timers.
func (s *Server) cancelScheduledTimers() {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()

	for _, t := range s.scheduledTimers {
		t.Stop()
	}

	s.scheduledTimers = nil
}

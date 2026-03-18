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

var newRedisKVStoreFn = func(ctx context.Context, url string) (runtime.KVStore, error) {
	return runtime.NewRedisKVStore(ctx, url)
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
	ctx             context.Context
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

func New(config Config) *Server {
	var kvStore runtime.KVStore

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		kvStore = runtime.NewMemoryKVStore()
	} else {
		rs, err := newRedisKVStoreFn(context.Background(), redisURL)
		if err != nil {
			config.Logger.Warn("failed to connect to Redis, falling back to in-memory KV store", "error", err)

			kvStore = runtime.NewMemoryKVStore()
		} else {
			config.Logger.Info("using Redis KV store")

			kvStore = rs
		}
	}

	s := &Server{
		config:       config,
		mux:          http.NewServeMux(),
		ctx:          context.Background(),
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
	s.ctx = ctx
	s.metrics.Start(ctx)
	s.startExporter(ctx)
	s.recoverExecutions(ctx)
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

func (s *Server) recoverExecutions(ctx context.Context) {
	if s.config.ExportURL == "" || !s.config.Workflow.Recovery {
		return
	}

	s.config.Logger.Info("recovery: checking for recoverable executions", "agent", s.config.ExporterName)

	recoveryClient := export.NewRecoveryClient(s.config.ExportURL, s.config.APIKey)

	recovered, err := recoveryClient.RecoverExecutions(ctx, s.config.ExporterName)
	if err != nil {
		s.config.Logger.Error("recovery: failed to fetch executions", "error", err)
		return
	}

	s.config.Logger.Info("recovery: found executions", "count", len(recovered))

	for _, rec := range recovered {
		if rec.WorkflowName != s.config.Workflow.Name {
			s.config.Logger.Warn("recovery: workflow not found",
				"workflow", rec.WorkflowName,
				"execution", rec.ExecutionID,
			)

			continue
		}

		s.config.Logger.Info("recovery: resuming execution",
			"execution", rec.ExecutionID,
			"workflow", rec.WorkflowName,
			"steps_recovered", len(rec.Steps),
		)

		for stepID, sr := range rec.Steps {
			s.config.Logger.Info("recovery: step state",
				"step", stepID,
				"status", sr.Status,
			)
		}

		s.runWorkflowAsync(rec.Params, asyncRunOpts{ //nolint:contextcheck
			ExecutionID:    rec.ExecutionID,
			Resumed:        true,
			RecoveredSteps: rec.Steps,
		})
	}
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
						"memory_bytes": snap.RSSKB * 1024,
						"goroutines":   snap.Goroutines,
						"net_rx_bytes": snap.NetRxBytes,
						"net_tx_bytes": snap.NetTxBytes,
						"uptime_s":     snap.UptimeS,
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
		_ = s.dbPool.Close()
	}

	if s.kvStore != nil {
		_ = s.kvStore.Close()
	}

	s.cancelScheduledTimers()

	if s.exporter != nil {
		s.exporter.Shutdown()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	return s.srv.Shutdown(shutdownCtx)
}

// asyncRunOpts holds optional settings for runWorkflowAsync.
type asyncRunOpts struct {
	TriggerData    map[string]any
	OnComplete     func(executionID string, success bool)
	ExecutionID    string
	Resumed        bool
	RecoveredSteps map[string]*runtime.StepResult
}

func (s *Server) runWorkflowAsync(params map[string]any, opts ...asyncRunOpts) string {
	wf := s.config.Workflow

	var opt asyncRunOpts

	if len(opts) > 0 {
		opt = opts[0]
	}

	executionID := opt.ExecutionID
	if executionID == "" {
		executionID = uuid.New().String()
	}

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

	execCtx, cancel := context.WithCancel(s.ctx)
	s.registerCancel(executionID, cancel)

	go func() {
		defer s.unregisterCancel(executionID)

		result, err := s.config.Executor.Execute(execCtx, wf, params, engine.ExecuteOptions{
			ExecutionID:    executionID,
			Services:       services,
			TriggerData:    opt.TriggerData,
			Resumed:        opt.Resumed,
			RecoveredSteps: opt.RecoveredSteps,
		})

		stopCapture()
		s.finalizeExecution(executionID, result, err, execCtx)

		shutdownWithRecovery := execCtx.Err() != nil && wf.Recovery
		if !shutdownWithRecovery {
			s.ensureWorkflowCompleted(executionID, result, err, execCtx)
		}

		if opt.OnComplete != nil {
			success := err == nil && result != nil && result.Status == runtime.StatusSuccess
			opt.OnComplete(executionID, success)
		}
	}()

	return executionID
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) WaitRegistry() *WaitRegistry {
	return s.waitRegistry
}

func (s *Server) registerCancel(executionID string, cancel context.CancelFunc) {
	s.cancelMu.Lock()
	s.cancels[executionID] = cancel
	s.cancelMu.Unlock()
}

func (s *Server) unregisterCancel(executionID string) {
	s.cancelMu.Lock()
	delete(s.cancels, executionID)
	s.cancelMu.Unlock()
}

func (s *Server) cancelAllExecutions() {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()

	for _, cancel := range s.cancels {
		cancel()
	}
}

func (s *Server) cancelExecution(executionID string) bool {
	s.cancelMu.RLock()
	cancel, ok := s.cancels[executionID]
	s.cancelMu.RUnlock()

	if ok {
		cancel()
	}

	return ok
}

func (s *Server) addScheduledTimer(t *time.Timer) {
	s.timerMu.Lock()
	s.scheduledTimers = append(s.scheduledTimers, t)
	s.timerMu.Unlock()
}

func (s *Server) ensureWorkflowCompleted(
	executionID string, result *engine.ExecuteResult, err error, execCtx context.Context,
) {
	// Check if workflow.completed was already stored by captureEvents.
	for _, ev := range s.config.ExecutionStore.GetEvents(executionID) {
		if ev.Type == event.WorkflowCompleted {
			return // already there, nothing to do
		}
	}

	status := runtime.StatusSuccess

	switch {
	case err != nil && execCtx.Err() != nil:
		status = runtime.StatusCancelled
	case err != nil:
		status = runtime.StatusFailed
	case result != nil:
		status = result.Status
	}

	completedEv := event.Event{
		Type:        event.WorkflowCompleted,
		Timestamp:   time.Now(),
		ExecutionID: executionID,
		Data:        map[string]any{"status": status},
	}

	s.config.ExecutionStore.AppendEvent(executionID, completedEv)
	s.config.EventBus.Publish(completedEv)
}

func (s *Server) cancelScheduledTimers() {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()

	for _, t := range s.scheduledTimers {
		t.Stop()
	}

	s.scheduledTimers = nil
}

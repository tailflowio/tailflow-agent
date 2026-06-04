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
	EditorEnabled  bool
	ExecutionStore store.ExecutionStore
	EventBus       *event.Bus
	Logger         *slog.Logger
	Version        string
	// Exporter, Claimer, Recoverer are the export ports. Default to noop
	// implementations when nil — see export.NewNoop*.
	Exporter  export.EventExporter
	Claimer   export.IdempotencyClaimer
	Recoverer export.ExecutionRecoverer
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
	sensitive       *engine.SensitiveRegistry
}

func New(config Config) *Server {
	if config.Exporter == nil {
		config.Exporter = export.NewNoopExporter()
	}

	if config.Claimer == nil {
		config.Claimer = export.NewNoopClaimer()
	}

	if config.Recoverer == nil {
		config.Recoverer = export.NewNoopRecoverer()
	}

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

func (s *Server) cancelScheduledTimers() {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()

	for _, t := range s.scheduledTimers {
		t.Stop()
	}

	s.scheduledTimers = nil
}

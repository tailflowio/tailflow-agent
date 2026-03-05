package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

type ServerTestSuite struct {
	suite.Suite
}

func TestServer(t *testing.T) {
	suite.Run(t, new(ServerTestSuite))
}

func (s *ServerTestSuite) SetupTest() {
}

func (s *ServerTestSuite) TestNew_InitializesServer() {
	srv := newTestServer(s.T())

	s.NotNil(srv)
	s.NotNil(srv.mux)
	s.NotNil(srv.waitRegistry)
	s.NotNil(srv.cancels)
}

func (s *ServerTestSuite) TestHandler_ReturnsNonNil() {
	srv := newTestServer(s.T())

	handler := srv.Handler()

	s.NotNil(handler)
}

func (s *ServerTestSuite) TestRegisterCancel_StoresFunction() {
	srv := newTestServer(s.T())

	called := false
	srv.registerCancel("exec-1", func() { called = true })

	ok := srv.cancelExecution("exec-1")
	s.True(ok)
	s.True(called)
}

func (s *ServerTestSuite) TestCancelExecution_ReturnsFalse_WhenNotFound() {
	srv := newTestServer(s.T())

	ok := srv.cancelExecution("nonexistent")

	s.False(ok)
}

func (s *ServerTestSuite) TestCancelAllExecutions() {
	srv := newTestServer(s.T())

	var count atomic.Int32
	srv.registerCancel("exec-1", func() { count.Add(1) })
	srv.registerCancel("exec-2", func() { count.Add(1) })
	srv.registerCancel("exec-3", func() { count.Add(1) })

	srv.cancelAllExecutions()

	s.Equal(int32(3), count.Load())
}

func (s *ServerTestSuite) TestRun_StartsAndShutdowns() {
	srv := newTestServer(s.T())

	ln, err := net.Listen("tcp", ":0")
	s.Require().NoError(err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	srv.config.Port = port
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()

	s.Require().Eventually(func() bool {
		resp, dialErr := http.Get(fmt.Sprintf("http://localhost:%d/api/workflow", port))
		if dialErr != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond)

	cancel()

	select {
	case runErr := <-errCh:
		s.NoError(runErr)
	case <-time.After(5 * time.Second):
		s.Fail("server did not shut down in time")
	}
}

func (s *ServerTestSuite) TestNew_WithRedisURL_FailsGracefully() {
	original := newRedisKVStoreFn
	s.T().Cleanup(func() { newRedisKVStoreFn = original })

	newRedisKVStoreFn = func(_ context.Context, url string) (runtime.KVStore, error) {
		return nil, errors.New("redis connect failed")
	}

	os.Setenv("REDIS_URL", "redis://bad-host:6379")
	s.T().Cleanup(func() { os.Unsetenv("REDIS_URL") })

	srv := newTestServer(s.T())
	s.NotNil(srv.kvStore, "should fall back to in-memory KV store")
}

func (s *ServerTestSuite) TestNew_WithRedisURL_Success() {
	original := newRedisKVStoreFn
	s.T().Cleanup(func() { newRedisKVStoreFn = original })

	mockKV := runtime.NewMemoryKVStore()
	newRedisKVStoreFn = func(_ context.Context, url string) (runtime.KVStore, error) {
		return mockKV, nil
	}

	os.Setenv("REDIS_URL", "redis://localhost:6379")
	s.T().Cleanup(func() { os.Unsetenv("REDIS_URL") })

	srv := newTestServer(s.T())
	s.NotNil(srv.kvStore)
}

func (s *ServerTestSuite) TestWaitRegistry_ReturnsNonNil() {
	srv := newTestServer(s.T())
	wr := srv.WaitRegistry()
	s.NotNil(wr)
	s.Equal(srv.waitRegistry, wr)
}

func (s *ServerTestSuite) TestAddScheduledTimer_AppendsTimer() {
	srv := newTestServer(s.T())

	t1 := time.NewTimer(1 * time.Hour)
	defer t1.Stop()
	t2 := time.NewTimer(1 * time.Hour)
	defer t2.Stop()

	srv.addScheduledTimer(t1)
	srv.addScheduledTimer(t2)

	s.Len(srv.scheduledTimers, 2)
}

func (s *ServerTestSuite) TestCancelScheduledTimers_StopsAllTimers() {
	srv := newTestServer(s.T())

	t1 := time.NewTimer(1 * time.Hour)
	t2 := time.NewTimer(1 * time.Hour)

	srv.addScheduledTimer(t1)
	srv.addScheduledTimer(t2)

	srv.cancelScheduledTimers()

	s.Nil(srv.scheduledTimers)
}

func (s *ServerTestSuite) TestStartExporter_NoExportURL_Noop() {
	srv := newTestServer(s.T())
	srv.config.ExportURL = ""
	srv.startExporter(context.Background())
	s.Nil(srv.exporter)
}

func (s *ServerTestSuite) TestStartExporter_HTTPTrigger() {
	srv := newTestServerWithWorkflow(s.T(), `version: "2.0"
name: "test"
trigger:
  http:
    method: GET
    path: /test
steps:
  - id: greet
    action: log
    config:
      message: "hello"
`)
	srv.config.ExportURL = "http://localhost:9999"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv.startExporter(ctx)
	s.NotNil(srv.exporter)
}

func (s *ServerTestSuite) TestStartExporter_WebhookTrigger() {
	srv := newTestServerWithWorkflow(s.T(), `version: "2.0"
name: "test"
trigger:
  webhook:
    path: /hook
steps:
  - id: greet
    action: log
    config:
      message: "hello"
`)
	srv.config.ExportURL = "http://localhost:9999"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv.startExporter(ctx)
	s.NotNil(srv.exporter)
}

func (s *ServerTestSuite) TestStartExporter_ScheduleTrigger() {
	srv := newTestServerWithWorkflow(s.T(), `version: "2.0"
name: "test"
trigger:
  schedule:
    cron: "* * * * *"
steps:
  - id: greet
    action: log
    config:
      message: "hello"
`)
	srv.config.ExportURL = "http://localhost:9999"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv.startExporter(ctx)
	s.NotNil(srv.exporter)
}

func (s *ServerTestSuite) TestStartExporter_RabbitMQTrigger() {
	srv := newTestServerWithWorkflow(s.T(), `version: "2.0"
name: "test"
trigger:
  rabbitmq:
    url: "amqp://localhost:5672"
    queue: "test"
steps:
  - id: greet
    action: log
    config:
      message: "hello"
`)
	srv.config.ExportURL = "http://localhost:9999"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv.startExporter(ctx)
	s.NotNil(srv.exporter)
}

func (s *ServerTestSuite) TestStartExporter_NilTrigger() {
	srv := newTestServer(s.T())
	srv.config.Workflow.Trigger = nil
	srv.config.ExportURL = "http://localhost:9999"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv.startExporter(ctx)
	s.NotNil(srv.exporter)
}

func (s *ServerTestSuite) TestStartCronScheduler_NilTrigger() {
	srv := newTestServer(s.T())
	srv.config.Workflow.Trigger = nil
	cronSched, err := srv.startCronScheduler()
	s.NoError(err)
	s.Nil(cronSched)
}

func (s *ServerTestSuite) TestStartCronScheduler_NoSchedule() {
	srv := newTestServer(s.T())
	srv.config.Workflow.Trigger = &parser.Trigger{}
	cronSched, err := srv.startCronScheduler()
	s.NoError(err)
	s.Nil(cronSched)
}

func (s *ServerTestSuite) TestStartCronScheduler_ValidCron() {
	srv := newTestServer(s.T())
	srv.config.Workflow.Trigger = &parser.Trigger{
		Schedule: &parser.ScheduleTrigger{
			Cron: "@every 1h",
		},
	}

	cronSched, err := srv.startCronScheduler()
	s.Require().NoError(err)
	s.NotNil(cronSched)
	cronSched.Stop()
}

func (s *ServerTestSuite) TestStartCronScheduler_InvalidCron() {
	srv := newTestServer(s.T())
	srv.config.Workflow.Trigger = &parser.Trigger{
		Schedule: &parser.ScheduleTrigger{
			Cron: "invalid cron",
		},
	}

	cronSched, err := srv.startCronScheduler()
	s.Require().Error(err)
	s.Contains(err.Error(), "invalid cron expression")
	s.Nil(cronSched)
}

func (s *ServerTestSuite) TestStartRabbitMQConsumer_NilTrigger() {
	srv := newTestServer(s.T())
	srv.config.Workflow.Trigger = nil
	consumer, err := srv.startRabbitMQConsumer(context.Background())
	s.NoError(err)
	s.Nil(consumer)
}

func (s *ServerTestSuite) TestStartRabbitMQConsumer_NoRabbitMQ() {
	srv := newTestServer(s.T())
	srv.config.Workflow.Trigger = &parser.Trigger{}
	consumer, err := srv.startRabbitMQConsumer(context.Background())
	s.NoError(err)
	s.Nil(consumer)
}

func (s *ServerTestSuite) TestStartRabbitMQConsumer_StartError() {
	original := newRabbitMQConsumerFn
	s.T().Cleanup(func() { newRabbitMQConsumerFn = original })

	newRabbitMQConsumerFn = func(config *parser.RabbitMQTrigger, logger *slog.Logger) *RabbitMQConsumer {
		c := NewRabbitMQConsumer(config, logger)
		c.dial = func(url string) (amqpConn, error) {
			return nil, errors.New("dial failed")
		}
		return c
	}

	srv := newTestServer(s.T())
	srv.config.Workflow.Trigger = &parser.Trigger{
		RabbitMQ: &parser.RabbitMQTrigger{
			URL:   "amqp://localhost:5672",
			Queue: "test-queue",
		},
	}

	consumer, err := srv.startRabbitMQConsumer(context.Background())
	s.Require().Error(err)
	s.Contains(err.Error(), "rabbitmq consumer")
	s.Nil(consumer)
}

func (s *ServerTestSuite) TestStartRabbitMQConsumer_Success() {
	original := newRabbitMQConsumerFn
	s.T().Cleanup(func() { newRabbitMQConsumerFn = original })

	deliveries := make(chan amqp.Delivery)
	newRabbitMQConsumerFn = func(config *parser.RabbitMQTrigger, logger *slog.Logger) *RabbitMQConsumer {
		c := NewRabbitMQConsumer(config, logger)
		mockCh := &mockAMQPChan{
			consumeFn: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
				return deliveries, nil
			},
		}
		c.dial = func(url string) (amqpConn, error) {
			return &mockAMQPConn{
				channelFn: func() (amqpChan, error) {
					return mockCh, nil
				},
			}, nil
		}
		return c
	}

	srv := newTestServer(s.T())
	srv.config.Workflow.Trigger = &parser.Trigger{
		RabbitMQ: &parser.RabbitMQTrigger{
			URL:   "amqp://localhost:5672",
			Queue: "test-queue",
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	consumer, err := srv.startRabbitMQConsumer(ctx)
	s.Require().NoError(err)
	s.NotNil(consumer)
	consumer.Stop()
}

func (s *ServerTestSuite) TestStartRabbitMQConsumer_OnMessage_WithAckFn() {
	original := newRabbitMQConsumerFn
	s.T().Cleanup(func() { newRabbitMQConsumerFn = original })

	deliveries := make(chan amqp.Delivery, 1)
	newRabbitMQConsumerFn = func(config *parser.RabbitMQTrigger, logger *slog.Logger) *RabbitMQConsumer {
		c := NewRabbitMQConsumer(config, logger)
		mockCh := &mockAMQPChan{
			consumeFn: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
				return deliveries, nil
			},
		}
		c.dial = func(url string) (amqpConn, error) {
			return &mockAMQPConn{
				channelFn: func() (amqpChan, error) {
					return mockCh, nil
				},
			}, nil
		}
		return c
	}

	srv := newTestServer(s.T())
	srv.config.Workflow.Trigger = &parser.Trigger{
		RabbitMQ: &parser.RabbitMQTrigger{
			URL:          "amqp://localhost:5672",
			Queue:        "test-queue",
			AckOnSuccess: true,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumer, err := srv.startRabbitMQConsumer(ctx)
	s.Require().NoError(err)
	s.NotNil(consumer)
	defer consumer.Stop()

	ack := &mockAcknowledger{}
	deliveries <- amqp.Delivery{
		Body:         []byte(`{"key":"value"}`),
		Acknowledger: ack,
	}

	// Wait for execution to complete
	s.Eventually(func() bool {
		execs := srv.config.ExecutionStore.List()
		for _, e := range execs {
			if e.Status != "running" {
				return true
			}
		}
		return false
	}, 5*time.Second, 50*time.Millisecond)
}

func (s *ServerTestSuite) TestShutdownServices_WithCronAndRMQ() {
	srv := newTestServer(s.T())

	ln, err := net.Listen("tcp", ":0")
	s.Require().NoError(err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	srv.config.Port = port
	srv.srv = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: srv.mux,
	}

	go func() { _ = srv.srv.ListenAndServe() }()

	s.Eventually(func() bool {
		resp, dialErr := http.Get(fmt.Sprintf("http://localhost:%d/api/workflow", port))
		if dialErr != nil {
			return false
		}
		resp.Body.Close()
		return true
	}, 3*time.Second, 50*time.Millisecond)

	cronSched := NewCronScheduler(srv.config.Logger)
	_ = cronSched.Add("@every 1h", func() {})
	cronSched.Start()

	mockCh := &mockAMQPChan{}
	mockConn := &mockAMQPConn{}
	rmqConsumer := NewRabbitMQConsumer(&parser.RabbitMQTrigger{
		URL:   "amqp://localhost:5672",
		Queue: "test-queue",
	}, srv.config.Logger)
	rmqConsumer.ch = mockCh
	rmqConsumer.conn = mockConn

	err = srv.shutdownServices(cronSched, rmqConsumer)
	s.NoError(err)
	s.True(mockCh.closed)
	s.True(mockConn.closed)
}

func (s *ServerTestSuite) TestRunWorkflowAsync_WithOnComplete() {
	srv := newTestServer(s.T())

	completeCalled := make(chan bool, 1)
	opts := asyncRunOpts{
		OnComplete: func(executionID string, success bool) {
			completeCalled <- success
		},
	}

	execID := srv.runWorkflowAsync(nil, opts)
	s.NotEmpty(execID)

	select {
	case success := <-completeCalled:
		s.True(success)
	case <-time.After(5 * time.Second):
		s.Fail("OnComplete was not called")
	}
}

func (s *ServerTestSuite) TestRunWorkflowAsync_WithTriggerData() {
	srv := newTestServer(s.T())

	triggerData := map[string]any{
		"body": "hello",
	}
	opts := asyncRunOpts{
		TriggerData: triggerData,
	}

	execID := srv.runWorkflowAsync(nil, opts)
	s.NotEmpty(execID)

	s.Eventually(func() bool {
		exec, err := srv.config.ExecutionStore.Get(execID)
		return err == nil && exec.Status != "running"
	}, 5*time.Second, 50*time.Millisecond)
}

func (s *ServerTestSuite) TestRun_ListenError() {
	srv := newTestServer(s.T())

	ln, err := net.Listen("tcp", ":0")
	s.Require().NoError(err)
	port := ln.Addr().(*net.TCPAddr).Port

	srv.config.Port = port
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()

	select {
	case runErr := <-errCh:
		s.Error(runErr, "should fail because port is already bound")
	case <-time.After(5 * time.Second):
		s.Fail("server did not return error in time")
	}

	ln.Close()
}

func (s *ServerTestSuite) TestRun_CronSchedulerError() {
	srv := newTestServer(s.T())
	srv.config.Workflow.Trigger = &parser.Trigger{
		Schedule: &parser.ScheduleTrigger{
			Cron: "invalid cron!!!",
		},
	}
	srv.config.Port = 0

	err := srv.Run(context.Background())
	s.Require().Error(err)
	s.Contains(err.Error(), "invalid cron expression")
}

func (s *ServerTestSuite) TestRun_RabbitMQConsumerError() {
	original := newRabbitMQConsumerFn
	s.T().Cleanup(func() { newRabbitMQConsumerFn = original })

	newRabbitMQConsumerFn = func(config *parser.RabbitMQTrigger, logger *slog.Logger) *RabbitMQConsumer {
		c := NewRabbitMQConsumer(config, logger)
		c.dial = func(url string) (amqpConn, error) {
			return nil, errors.New("dial failed")
		}
		return c
	}

	srv := newTestServer(s.T())
	srv.config.Workflow.Trigger = &parser.Trigger{
		RabbitMQ: &parser.RabbitMQTrigger{
			URL:   "amqp://localhost:5672",
			Queue: "test-queue",
		},
	}
	srv.config.Port = 0

	err := srv.Run(context.Background())
	s.Require().Error(err)
	s.Contains(err.Error(), "rabbitmq consumer")
}

func (s *ServerTestSuite) TestStartMetricsRefresh_PublishesMetrics() {
	srv := newTestServer(s.T())

	ch := srv.config.EventBus.Subscribe(100)
	defer srv.config.EventBus.Unsubscribe(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.startMetricsRefresh(ctx)

	var metricsReceived bool
	timeout := time.After(3 * time.Second)

	for !metricsReceived {
		select {
		case ev := <-ch:
			if ev.Type == event.Metrics {
				metricsReceived = true
			}
		case <-timeout:
			s.Fail("did not receive metrics event")
			return
		}
	}

	s.True(metricsReceived)
}

func newTestServerWithWorkflow(t *testing.T, yaml string) *Server {
	t.Helper()

	wf, err := parser.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}

	bus := event.NewBus()
	t.Cleanup(bus.Close)

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := engine.NewExecutor(reg, bus, logger, nil)

	return New(Config{
		Port:           0,
		Executor:       exec,
		Workflow:       wf,
		ExecutionStore: store.NewExecutionStore(10),
		EventBus:       bus,
		Logger:         logger,
	})
}

func (s *ServerTestSuite) TestStartCronScheduler_CronCallbackFires() {
	srv := newTestServer(s.T())
	srv.config.Workflow.Trigger = &parser.Trigger{
		Schedule: &parser.ScheduleTrigger{
			Cron: "@every 1s",
		},
	}

	cronSched, err := srv.startCronScheduler()
	s.Require().NoError(err)
	s.Require().NotNil(cronSched)
	defer cronSched.Stop()

	// Wait for the cron to fire and create at least one execution
	s.Eventually(func() bool {
		execs := srv.config.ExecutionStore.List()
		return len(execs) > 0
	}, 5*time.Second, 100*time.Millisecond)
}

// ensureWorkflowCompleted – already stored (early return)
func (s *ServerTestSuite) TestEnsureWorkflowCompleted_AlreadyStored() {
	srv := newTestServer(s.T())

	execID := "ewc-already"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusSuccess,
		StartedAt: time.Now(),
	})

	// Pre-append a WorkflowCompleted event
	srv.config.ExecutionStore.AppendEvent(execID, event.Event{
		Type:        event.WorkflowCompleted,
		ExecutionID: execID,
		Data:        map[string]any{"status": "success"},
	})

	ch := srv.config.EventBus.Subscribe(10)
	defer srv.config.EventBus.Unsubscribe(ch)

	// Call ensureWorkflowCompleted — should return early since event already exists
	result := &engine.ExecuteResult{Status: runtime.StatusSuccess}
	srv.ensureWorkflowCompleted(execID, result, nil, context.Background())

	// No additional event should be appended
	events := srv.config.ExecutionStore.GetEvents(execID)
	s.Len(events, 1)

	// Verify nothing was published to the bus
	select {
	case <-ch:
		s.Fail("no event should be published when WorkflowCompleted already stored")
	case <-time.After(100 * time.Millisecond):
		// expected: no event published
	}
}

// ensureWorkflowCompleted – error with cancelled context
func (s *ServerTestSuite) TestEnsureWorkflowCompleted_ErrorCancelled() {
	srv := newTestServer(s.T())

	execID := "ewc-cancel"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	srv.ensureWorkflowCompleted(execID, nil, errors.New("context cancelled"), ctx)

	events := srv.config.ExecutionStore.GetEvents(execID)
	s.Require().Len(events, 1)
	s.Equal(event.WorkflowCompleted, events[0].Type)
	s.Equal(runtime.StatusCancelled, events[0].Data["status"])
}

// ensureWorkflowCompleted – error without cancelled context
func (s *ServerTestSuite) TestEnsureWorkflowCompleted_ErrorFailed() {
	srv := newTestServer(s.T())

	execID := "ewc-fail"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	srv.ensureWorkflowCompleted(execID, nil, errors.New("exec failed"), context.Background())

	events := srv.config.ExecutionStore.GetEvents(execID)
	s.Require().Len(events, 1)
	s.Equal(event.WorkflowCompleted, events[0].Type)
	s.Equal(runtime.StatusFailed, events[0].Data["status"])
}

// ensureWorkflowCompleted – success with result
func (s *ServerTestSuite) TestEnsureWorkflowCompleted_SuccessWithResult() {
	srv := newTestServer(s.T())

	execID := "ewc-result"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	result := &engine.ExecuteResult{Status: runtime.StatusCompletedWithErrors}
	srv.ensureWorkflowCompleted(execID, result, nil, context.Background())

	events := srv.config.ExecutionStore.GetEvents(execID)
	s.Require().Len(events, 1)
	s.Equal(event.WorkflowCompleted, events[0].Type)
	s.Equal(runtime.StatusCompletedWithErrors, events[0].Data["status"])
}

// ensureWorkflowCompleted – nil error and nil result (defaults to success)
func (s *ServerTestSuite) TestEnsureWorkflowCompleted_NilResultNilError() {
	srv := newTestServer(s.T())

	execID := "ewc-default"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	srv.ensureWorkflowCompleted(execID, nil, nil, context.Background())

	events := srv.config.ExecutionStore.GetEvents(execID)
	s.Require().Len(events, 1)
	s.Equal(event.WorkflowCompleted, events[0].Type)
	s.Equal(runtime.StatusSuccess, events[0].Data["status"])
}

// ensureWorkflowCompleted – publishes to event bus
func (s *ServerTestSuite) TestEnsureWorkflowCompleted_PublishesEvent() {
	srv := newTestServer(s.T())

	execID := "ewc-publish"
	srv.config.ExecutionStore.Add(&store.Execution{
		ID: execID, WorkflowName: "test", Status: runtime.StatusRunning,
		StartedAt: time.Now(),
	})

	ch := srv.config.EventBus.Subscribe(10)
	defer srv.config.EventBus.Unsubscribe(ch)

	result := &engine.ExecuteResult{Status: runtime.StatusSuccess}
	srv.ensureWorkflowCompleted(execID, result, nil, context.Background())

	select {
	case ev := <-ch:
		s.Equal(event.WorkflowCompleted, ev.Type)
		s.Equal(execID, ev.ExecutionID)
		s.Equal(runtime.StatusSuccess, ev.Data["status"])
	case <-time.After(time.Second):
		s.Fail("timeout waiting for published event")
	}
}

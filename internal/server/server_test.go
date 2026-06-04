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
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
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
		execs, _ := srv.config.ExecutionStore.List(context.Background())
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
		execs, _ := srv.config.ExecutionStore.List(context.Background())
		return len(execs) > 0
	}, 5*time.Second, 100*time.Millisecond)
}


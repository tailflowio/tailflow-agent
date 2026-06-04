package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/tailflow/tailflow/internal/parser"
)

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

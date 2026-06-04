package server

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func (s *RabbitMQWaitTestSuite) TestConsumeLoop_NackUnmatchedMessage() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)

		deliveries := make(chan amqp.Delivery, 1)
		ctx, cancel := context.WithCancel(context.Background())

		qc := &queueConsumer{}

		ack := &mockAcknowledger{}
		deliveries <- amqp.Delivery{
			Body:         []byte(`{"x":"y"}`),
			Acknowledger: ack,
		}

		go mgr.consumeLoop(ctx, qc, deliveries)

		time.Sleep(1 * time.Millisecond)
		synctest.Wait()

		s.True(ack.nackCalled, "unmatched message should be nacked")

		time.Sleep(requeueThrottleDuration + 1*time.Millisecond)
		cancel()
		synctest.Wait()
	})
}

func (s *RabbitMQWaitTestSuite) TestConsumeLoop_NackError() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)

		deliveries := make(chan amqp.Delivery, 1)
		ctx, cancel := context.WithCancel(context.Background())

		qc := &queueConsumer{}

		ack := &mockAcknowledger{nackErr: errors.New("nack failed")}
		deliveries <- amqp.Delivery{
			Body:         []byte(`{}`),
			Acknowledger: ack,
		}

		go mgr.consumeLoop(ctx, qc, deliveries)

		time.Sleep(1 * time.Millisecond)
		synctest.Wait()

		s.True(ack.nackCalled)

		time.Sleep(requeueThrottleDuration + 1*time.Millisecond)
		cancel()
		synctest.Wait()
	})
}

func (s *RabbitMQWaitTestSuite) TestConsumeLoop_ContextCancelDuringThrottle() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)

		deliveries := make(chan amqp.Delivery, 1)
		ctx, cancel := context.WithCancel(context.Background())

		qc := &queueConsumer{}

		deliveries <- amqp.Delivery{
			Body:         []byte(`{}`),
			Acknowledger: &mockAcknowledger{},
		}

		exited := false
		go func() {
			mgr.consumeLoop(ctx, qc, deliveries)
			exited = true
		}()

		time.Sleep(1 * time.Millisecond)
		synctest.Wait()

		cancel()
		synctest.Wait()

		s.True(exited, "should exit when context cancelled during throttle")
	})
}

func (s *RabbitMQWaitTestSuite) TestConsumeLoop_MatchedMessageContinues() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)

		deliveries := make(chan amqp.Delivery, 1)
		ctx, cancel := context.WithCancel(context.Background())

		w := &rmqWaiter{
			matchField: "",
			matchValue: "",
			ch:         make(chan map[string]any, 1),
			ctx:        context.Background(),
		}

		qc := &queueConsumer{
			waiters: []*rmqWaiter{w},
		}

		ack := &mockAcknowledger{}
		deliveries <- amqp.Delivery{
			Body:         []byte(`{"x":"y"}`),
			Acknowledger: ack,
		}

		go mgr.consumeLoop(ctx, qc, deliveries)

		time.Sleep(1 * time.Millisecond)
		synctest.Wait()

		s.True(ack.ackCalled)
		s.False(ack.nackCalled)

		cancel()
		synctest.Wait()
	})
}

func (s *RabbitMQWaitTestSuite) TestRemoveWaiter_UnknownQueue() {
	mgr := NewRabbitMQWaitManager(s.logger)
	mgr.connections["amqp://host:5672"] = &managedConnection{
		conn:      &mockAMQPConn{},
		consumers: make(map[string]*queueConsumer),
	}

	w := &rmqWaiter{ch: make(chan map[string]any, 1)}
	mgr.removeWaiter("amqp://host:5672", "unknown-queue", w)

	s.Len(mgr.connections, 1)
}

func (s *RabbitMQWaitTestSuite) TestRemoveWaiter_LastWaiter_CleansUp() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)
		deliveries := make(chan amqp.Delivery)
		mockCh := &mockAMQPChan{
			consumeFn: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
				return deliveries, nil
			},
		}
		mockConn := &mockAMQPConn{
			channelFn: func() (amqpChan, error) {
				return mockCh, nil
			},
		}
		mgr.dial = func(url string) (amqpConn, error) {
			return mockConn, nil
		}

		_, cleanup := mgr.Register("amqp://host:5672", "q1", "", "", nil)
		cleanup()

		synctest.Wait()

		mgr.mu.Lock()
		s.Len(mgr.connections, 0, "last waiter removal should clean up connection")
		mgr.mu.Unlock()
		s.True(mockCh.closed)
		s.True(mockConn.closed)
	})
}

func (s *RabbitMQWaitTestSuite) TestRemoveWaiter_NotLastWaiter_KeepsConsumer() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)
		deliveries := make(chan amqp.Delivery)
		mockCh := &mockAMQPChan{
			consumeFn: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
				return deliveries, nil
			},
		}
		mockConn := &mockAMQPConn{
			channelFn: func() (amqpChan, error) {
				return mockCh, nil
			},
		}
		mgr.dial = func(url string) (amqpConn, error) {
			return mockConn, nil
		}

		_, cleanup1 := mgr.Register("amqp://host:5672", "q1", "id", "1", nil)
		_, cleanup2 := mgr.Register("amqp://host:5672", "q1", "id", "2", nil)

		cleanup1()

		mgr.mu.Lock()
		s.Len(mgr.connections, 1, "connection should remain with other waiters")
		mgr.mu.Unlock()

		cleanup2()
		synctest.Wait()
	})
}

func (s *RabbitMQWaitTestSuite) TestClose_WithActiveConnections() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)
		deliveries := make(chan amqp.Delivery)
		mockCh := &mockAMQPChan{
			consumeFn: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
				return deliveries, nil
			},
		}
		mockConn := &mockAMQPConn{
			channelFn: func() (amqpChan, error) {
				return mockCh, nil
			},
		}
		mgr.dial = func(url string) (amqpConn, error) {
			return mockConn, nil
		}

		_, _ = mgr.Register("amqp://host:5672", "q1", "", "", nil)

		mgr.Close()

		s.Len(mgr.connections, 0)
		s.True(mockCh.closed)
		s.True(mockConn.closed)
	})
}

func (s *RabbitMQWaitTestSuite) TestClose_EmptyManager() {
	mgr := NewRabbitMQWaitManager(s.logger)
	s.NotPanics(func() {
		mgr.Close()
	})
}

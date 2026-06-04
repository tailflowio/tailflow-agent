package server

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	amqp "github.com/rabbitmq/amqp091-go"
)

func (s *RabbitMQWaitTestSuite) TestRegister_DialError_ClosesChannel() {
	mgr := NewRabbitMQWaitManager(s.logger)
	mgr.dial = func(url string) (amqpConn, error) {
		return nil, errors.New("dial failed")
	}

	ch, cleanup := mgr.Register("amqp://bad:5672", "q1", "id", "123", context.Background())
	s.NotNil(cleanup)

	_, ok := <-ch
	s.False(ok, "channel should be closed on dial error")
}

func (s *RabbitMQWaitTestSuite) TestRegister_DialError_CleanupIsNoOp() {
	mgr := NewRabbitMQWaitManager(s.logger)
	mgr.dial = func(url string) (amqpConn, error) {
		return nil, errors.New("dial failed")
	}

	_, cleanup := mgr.Register("amqp://bad:5672", "q1", "id", "123", context.Background())

	s.NotPanics(cleanup, "cleanup returned on dial error must be callable without panic")
}

func (s *RabbitMQWaitTestSuite) TestRegister_ChannelError_ClosesChannel() {
	mgr := NewRabbitMQWaitManager(s.logger)
	mockConn := &mockAMQPConn{
		channelFn: func() (amqpChan, error) {
			return nil, errors.New("channel failed")
		},
	}
	mgr.dial = func(url string) (amqpConn, error) {
		return mockConn, nil
	}

	ch, cleanup := mgr.Register("amqp://host:5672", "q1", "id", "123", context.Background())
	s.NotNil(cleanup)

	_, ok := <-ch
	s.False(ok, "channel should be closed on channel error")
}

func (s *RabbitMQWaitTestSuite) TestRegister_ChannelError_CleanupIsNoOp() {
	mgr := NewRabbitMQWaitManager(s.logger)
	mockConn := &mockAMQPConn{
		channelFn: func() (amqpChan, error) {
			return nil, errors.New("channel failed")
		},
	}
	mgr.dial = func(url string) (amqpConn, error) {
		return mockConn, nil
	}

	_, cleanup := mgr.Register("amqp://host:5672", "q1", "id", "123", context.Background())

	s.NotPanics(cleanup, "cleanup returned on channel error must be callable without panic")
}

func (s *RabbitMQWaitTestSuite) TestRegister_Success_ReceivesMatchingMessage() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)
		deliveries := make(chan amqp.Delivery, 1)
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

		ctx, cancel := context.WithCancel(context.Background())
		ch, _ := mgr.Register("amqp://host:5672", "q1", "order_id", "abc", ctx)

		deliveries <- amqp.Delivery{
			Body:         []byte(`{"order_id":"abc"}`),
			ContentType:  "application/json",
			RoutingKey:   "orders",
			MessageId:    "msg-1",
			Headers:      amqp.Table{"x-key": "val"},
			Acknowledger: &mockAcknowledger{},
		}

		synctest.Wait()

		result := <-ch
		s.NotNil(result)
		s.Equal("application/json", result["content_type"])
		s.Equal("orders", result["routing_key"])
		s.Equal("msg-1", result["message_id"])
		headers := result["headers"].(map[string]any)
		s.Equal("val", headers["x-key"])

		cancel()
		synctest.Wait()
	})
}

func (s *RabbitMQWaitTestSuite) TestRegister_ContextCancellation_Cleanup() {
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

		ctx, cancel := context.WithCancel(context.Background())
		_, _ = mgr.Register("amqp://host:5672", "q1", "id", "v", ctx)

		cancel()
		synctest.Wait()

		mgr.mu.Lock()
		s.Len(mgr.connections, 0)
		mgr.mu.Unlock()
	})
}

func (s *RabbitMQWaitTestSuite) TestRegister_NilContext() {
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

		ch, cleanup := mgr.Register("amqp://host:5672", "q1", "", "", nil)
		defer cleanup()

		s.NotNil(ch)
	})
}

func (s *RabbitMQWaitTestSuite) TestGetOrCreateConnection_ReusesExistingConnection() {
	mgr := NewRabbitMQWaitManager(s.logger)
	mockConn := &mockAMQPConn{}
	mgr.connections["amqp://host:5672"] = &managedConnection{
		conn:      mockConn,
		consumers: make(map[string]*queueConsumer),
	}

	mgr.mu.Lock()
	mc, err := mgr.getOrCreateConnection("amqp://host:5672")
	mgr.mu.Unlock()

	s.Require().NoError(err)
	s.Equal(mockConn, mc.conn)
}

func (s *RabbitMQWaitTestSuite) TestGetOrCreateConsumer_QosError() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)
		mockCh := &mockAMQPChan{
			qosFn: func(prefetchCount, prefetchSize int, global bool) error {
				return errors.New("qos failed")
			},
		}
		mockConn := &mockAMQPConn{
			channelFn: func() (amqpChan, error) {
				return mockCh, nil
			},
		}
		mc := &managedConnection{
			conn:      mockConn,
			consumers: make(map[string]*queueConsumer),
		}

		_, err := mgr.getOrCreateConsumer(mc, "q1")
		s.Require().Error(err)
		s.Contains(err.Error(), "qos")
		s.True(mockCh.closed)
	})
}

func (s *RabbitMQWaitTestSuite) TestGetOrCreateConsumer_ConsumeError() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)
		mockCh := &mockAMQPChan{
			consumeFn: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
				return nil, errors.New("consume failed")
			},
		}
		mockConn := &mockAMQPConn{
			channelFn: func() (amqpChan, error) {
				return mockCh, nil
			},
		}
		mc := &managedConnection{
			conn:      mockConn,
			consumers: make(map[string]*queueConsumer),
		}

		_, err := mgr.getOrCreateConsumer(mc, "q1")
		s.Require().Error(err)
		s.Contains(err.Error(), "consume")
		s.True(mockCh.closed)
	})
}

func (s *RabbitMQWaitTestSuite) TestGetOrCreateConsumer_ReusesExisting() {
	mgr := NewRabbitMQWaitManager(s.logger)
	existingQC := &queueConsumer{}
	mc := &managedConnection{
		conn:      &mockAMQPConn{},
		consumers: map[string]*queueConsumer{"q1": existingQC},
	}

	qc, err := mgr.getOrCreateConsumer(mc, "q1")
	s.Require().NoError(err)
	s.Equal(existingQC, qc)
}

func (s *RabbitMQWaitTestSuite) TestRouteMessage_InvalidJSON() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)

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
		msg := amqp.Delivery{
			Body:         []byte("not json"),
			Acknowledger: ack,
		}

		matched := mgr.routeMessage(qc, msg)
		s.True(matched)

		result := <-w.ch
		body := result["body"].(map[string]any)
		s.Equal("not json", body["raw"])
	})
}

func (s *RabbitMQWaitTestSuite) TestRouteMessage_MatchFieldMatch() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)

		w := &rmqWaiter{
			matchField: "order_id",
			matchValue: "abc",
			ch:         make(chan map[string]any, 1),
			ctx:        context.Background(),
		}

		qc := &queueConsumer{
			waiters: []*rmqWaiter{w},
		}

		ack := &mockAcknowledger{}
		msg := amqp.Delivery{
			Body:         []byte(`{"order_id":"abc"}`),
			Acknowledger: ack,
		}

		matched := mgr.routeMessage(qc, msg)
		s.True(matched)
		s.True(ack.ackCalled)
	})
}

func (s *RabbitMQWaitTestSuite) TestRouteMessage_NoMatch() {
	mgr := NewRabbitMQWaitManager(s.logger)

	w := &rmqWaiter{
		matchField: "order_id",
		matchValue: "xyz",
		ch:         make(chan map[string]any, 1),
		ctx:        context.Background(),
	}

	qc := &queueConsumer{
		waiters: []*rmqWaiter{w},
	}

	msg := amqp.Delivery{
		Body:         []byte(`{"order_id":"abc"}`),
		Acknowledger: &mockAcknowledger{},
	}

	matched := mgr.routeMessage(qc, msg)
	s.False(matched)
}

func (s *RabbitMQWaitTestSuite) TestRouteMessage_SkipsCancelledWaiter() {
	mgr := NewRabbitMQWaitManager(s.logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := &rmqWaiter{
		matchField: "",
		matchValue: "",
		ch:         make(chan map[string]any, 1),
		ctx:        ctx,
	}

	qc := &queueConsumer{
		waiters: []*rmqWaiter{w},
	}

	msg := amqp.Delivery{
		Body:         []byte(`{"x":"y"}`),
		Acknowledger: &mockAcknowledger{},
	}

	matched := mgr.routeMessage(qc, msg)
	s.False(matched, "cancelled waiter should be skipped")
}

func (s *RabbitMQWaitTestSuite) TestDeliverToWaiter_ChannelFull() {
	mgr := NewRabbitMQWaitManager(s.logger)

	w := &rmqWaiter{
		ch:  make(chan map[string]any, 1),
		ctx: context.Background(),
	}
	w.ch <- map[string]any{"old": true}

	qc := &queueConsumer{
		waiters: []*rmqWaiter{w},
	}

	msg := amqp.Delivery{
		Body:         []byte(`{}`),
		Acknowledger: &mockAcknowledger{},
	}

	ok := mgr.deliverToWaiter(qc, 0, w, msg, map[string]any{})
	s.False(ok, "should return false when channel is full")
}

func (s *RabbitMQWaitTestSuite) TestDeliverToWaiter_AckError() {
	mgr := NewRabbitMQWaitManager(s.logger)

	w := &rmqWaiter{
		ch:  make(chan map[string]any, 1),
		ctx: context.Background(),
	}

	qc := &queueConsumer{
		waiters: []*rmqWaiter{w},
	}

	ack := &mockAcknowledger{ackErr: errors.New("ack failed")}
	msg := amqp.Delivery{
		Body:         []byte(`{}`),
		Acknowledger: ack,
	}

	ok := mgr.deliverToWaiter(qc, 0, w, msg, map[string]any{})
	s.True(ok)
	s.True(ack.ackCalled)
	s.Len(qc.waiters, 0)
}

func (s *RabbitMQWaitTestSuite) TestAmqpTableToMap_NilTable() {
	result := amqpTableToMap(nil)
	s.Nil(result)
}

func (s *RabbitMQWaitTestSuite) TestAmqpTableToMap_NonNilTable() {
	table := amqp.Table{
		"key1": "value1",
		"key2": 42,
	}
	result := amqpTableToMap(table)
	s.Equal("value1", result["key1"])
	s.Equal(42, result["key2"])
}

func (s *RabbitMQWaitTestSuite) TestGetOrCreateConnection_NilDial_UsesRealAMQPDial() {
	original := amqpRawDial
	s.T().Cleanup(func() { amqpRawDial = original })

	amqpRawDial = func(url string) (rawConn, error) {
		return nil, errors.New("connection refused by mock")
	}

	mgr := NewRabbitMQWaitManager(s.logger)
	s.Nil(mgr.dial)

	mgr.mu.Lock()
	_, err := mgr.getOrCreateConnection("amqp://host:5672")
	mgr.mu.Unlock()

	s.Require().Error(err)
	s.Contains(err.Error(), "dial")
}

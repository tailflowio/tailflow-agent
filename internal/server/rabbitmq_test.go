package server

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"testing/synctest"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/parser"
)

type RabbitMQConsumerTestSuite struct {
	suite.Suite
	logger *slog.Logger
	config *parser.RabbitMQTrigger
}

func TestRabbitMQConsumer(t *testing.T) {
	suite.Run(t, new(RabbitMQConsumerTestSuite))
}

func (s *RabbitMQConsumerTestSuite) SetupTest() {
	s.logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s.config = &parser.RabbitMQTrigger{
		URL:   "amqp://localhost:5672",
		Queue: "test-queue",
	}
}

func (s *RabbitMQConsumerTestSuite) TestNewRabbitMQConsumer_InitializesFields() {
	consumer := NewRabbitMQConsumer(s.config, s.logger)

	s.Require().NotNil(consumer)
	s.Equal(s.config, consumer.config)
	s.Equal(s.logger, consumer.logger)
	s.Nil(consumer.conn)
	s.Nil(consumer.ch)
	s.Nil(consumer.dial)
}

func (s *RabbitMQConsumerTestSuite) TestStart_DialError() {
	consumer := NewRabbitMQConsumer(s.config, s.logger)
	consumer.dial = func(url string) (amqpConn, error) {
		return nil, errors.New("connection refused")
	}

	err := consumer.Start(context.Background(), func(map[string]any, func(bool)) {})

	s.Require().Error(err)
	s.Contains(err.Error(), "rabbitmq dial")
	s.Contains(err.Error(), "connection refused")
}

func (s *RabbitMQConsumerTestSuite) TestStart_ChannelError() {
	mockConn := &mockAMQPConn{
		channelFn: func() (amqpChan, error) {
			return nil, errors.New("channel open failed")
		},
	}

	consumer := NewRabbitMQConsumer(s.config, s.logger)
	consumer.dial = func(url string) (amqpConn, error) {
		return mockConn, nil
	}

	err := consumer.Start(context.Background(), func(map[string]any, func(bool)) {})

	s.Require().Error(err)
	s.Contains(err.Error(), "rabbitmq channel")
	s.Contains(err.Error(), "channel open failed")
	s.True(mockConn.closed)
}

func (s *RabbitMQConsumerTestSuite) TestStart_ConsumesMessages() {
	synctest.Test(s.T(), func(t *testing.T) {
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

		consumer := NewRabbitMQConsumer(s.config, s.logger)
		consumer.dial = func(url string) (amqpConn, error) {
			return mockConn, nil
		}

		var received map[string]any
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := consumer.Start(ctx, func(triggerData map[string]any, ackFn func(bool)) {
			received = triggerData
		})
		s.Require().NoError(err)

		deliveries <- amqp.Delivery{
			Body:         []byte("hello"),
			ContentType:  "text/plain",
			RoutingKey:   "test.key",
			MessageId:    "msg-1",
			Acknowledger: &mockAcknowledger{},
		}

		synctest.Wait()

		s.Require().NotNil(received)
		s.Equal("hello", received["body"])
		s.Equal("text/plain", received["content_type"])
		s.Equal("test.key", received["routing_key"])
		s.Equal("msg-1", received["message_id"])
		s.Equal("test-queue", received["queue"])
	})
}

func (s *RabbitMQConsumerTestSuite) TestConsumeLoop_ExitsOnContextCancel() {
	synctest.Test(s.T(), func(t *testing.T) {
		deliveries := make(chan amqp.Delivery)
		ctx, cancel := context.WithCancel(context.Background())

		consumer := NewRabbitMQConsumer(s.config, s.logger)

		exited := false
		go func() {
			consumer.consumeLoop(ctx, deliveries, func(map[string]any, func(bool)) {})
			exited = true
		}()

		cancel()
		synctest.Wait()

		s.True(exited)
	})
}

func (s *RabbitMQConsumerTestSuite) TestConsumeLoop_ExitsOnClosedChannel() {
	synctest.Test(s.T(), func(t *testing.T) {
		deliveries := make(chan amqp.Delivery)
		close(deliveries)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		consumer := NewRabbitMQConsumer(s.config, s.logger)

		exited := false
		go func() {
			consumer.consumeLoop(ctx, deliveries, func(map[string]any, func(bool)) {})
			exited = true
		}()

		synctest.Wait()

		s.True(exited)
	})
}

func (s *RabbitMQConsumerTestSuite) TestStop_NilChannelAndConn() {
	consumer := NewRabbitMQConsumer(s.config, s.logger)

	s.NotPanics(func() {
		consumer.Stop()
	})
}

func (s *RabbitMQConsumerTestSuite) TestStop_ClosesChannelAndConnection() {
	mockCh := &mockAMQPChan{}
	mockConn := &mockAMQPConn{}

	consumer := NewRabbitMQConsumer(s.config, s.logger)
	consumer.ch = mockCh
	consumer.conn = mockConn

	consumer.Stop()

	s.True(mockCh.closed)
	s.True(mockConn.closed)
}

func (s *RabbitMQConsumerTestSuite) TestOpenChannel_QosError() {
	cfg := &parser.RabbitMQTrigger{
		URL:      "amqp://localhost:5672",
		Queue:    "test-queue",
		Prefetch: 10,
	}
	consumer := NewRabbitMQConsumer(cfg, s.logger)

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
	consumer.dial = func(url string) (amqpConn, error) {
		return mockConn, nil
	}

	_, err := consumer.openChannel()
	s.Require().Error(err)
	s.Contains(err.Error(), "rabbitmq qos")
	s.True(mockCh.closed)
	s.True(mockConn.closed)
}

func (s *RabbitMQConsumerTestSuite) TestOpenChannel_ConsumeError() {
	consumer := NewRabbitMQConsumer(s.config, s.logger)

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
	consumer.dial = func(url string) (amqpConn, error) {
		return mockConn, nil
	}

	_, err := consumer.openChannel()
	s.Require().Error(err)
	s.Contains(err.Error(), "rabbitmq consume")
	s.True(mockCh.closed)
	s.True(mockConn.closed)
}

func (s *RabbitMQConsumerTestSuite) TestOpenChannel_WithPrefetch_Success() {
	cfg := &parser.RabbitMQTrigger{
		URL:      "amqp://localhost:5672",
		Queue:    "test-queue",
		Prefetch: 5,
	}
	consumer := NewRabbitMQConsumer(cfg, s.logger)

	deliveries := make(chan amqp.Delivery)
	qosCalled := false
	mockCh := &mockAMQPChan{
		qosFn: func(prefetchCount, prefetchSize int, global bool) error {
			qosCalled = true
			s.Equal(5, prefetchCount)
			return nil
		},
		consumeFn: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
			return deliveries, nil
		},
	}
	mockConn := &mockAMQPConn{
		channelFn: func() (amqpChan, error) {
			return mockCh, nil
		},
	}
	consumer.dial = func(url string) (amqpConn, error) {
		return mockConn, nil
	}

	msgs, err := consumer.openChannel()
	s.Require().NoError(err)
	s.NotNil(msgs)
	s.True(qosCalled)
}

func (s *RabbitMQConsumerTestSuite) TestHandleMessage_AckOnSuccess_True_Success() {
	cfg := &parser.RabbitMQTrigger{
		URL:          "amqp://localhost:5672",
		Queue:        "test-queue",
		AckOnSuccess: true,
	}
	consumer := NewRabbitMQConsumer(cfg, s.logger)

	ack := &mockAcknowledger{}
	msg := amqp.Delivery{
		Body:         []byte("hello"),
		ContentType:  "text/plain",
		RoutingKey:   "key",
		MessageId:    "m1",
		Acknowledger: ack,
	}

	var receivedAckFn func(bool)
	consumer.handleMessage(msg, func(triggerData map[string]any, ackFn func(bool)) {
		receivedAckFn = ackFn
		s.Equal("test-queue", triggerData["queue"])
	})

	s.Require().NotNil(receivedAckFn)

	// Call ackFn with success=true
	receivedAckFn(true)
	s.True(ack.ackCalled)
	s.False(ack.nackCalled)
}

func (s *RabbitMQConsumerTestSuite) TestHandleMessage_AckOnSuccess_True_Failure() {
	cfg := &parser.RabbitMQTrigger{
		URL:          "amqp://localhost:5672",
		Queue:        "test-queue",
		AckOnSuccess: true,
	}
	consumer := NewRabbitMQConsumer(cfg, s.logger)

	ack := &mockAcknowledger{}
	msg := amqp.Delivery{
		Body:         []byte("hello"),
		Acknowledger: ack,
	}

	var receivedAckFn func(bool)
	consumer.handleMessage(msg, func(triggerData map[string]any, ackFn func(bool)) {
		receivedAckFn = ackFn
	})

	s.Require().NotNil(receivedAckFn)

	// Call ackFn with success=false => nack
	receivedAckFn(false)
	s.False(ack.ackCalled)
	s.True(ack.nackCalled)
}

func (s *RabbitMQConsumerTestSuite) TestHandleMessage_AckOnSuccess_AckError() {
	cfg := &parser.RabbitMQTrigger{
		URL:          "amqp://localhost:5672",
		Queue:        "test-queue",
		AckOnSuccess: true,
	}
	consumer := NewRabbitMQConsumer(cfg, s.logger)

	ack := &mockAcknowledger{ackErr: errors.New("ack err")}
	msg := amqp.Delivery{
		Body:         []byte("hello"),
		Acknowledger: ack,
	}

	var receivedAckFn func(bool)
	consumer.handleMessage(msg, func(triggerData map[string]any, ackFn func(bool)) {
		receivedAckFn = ackFn
	})

	// Should not panic when ack fails
	receivedAckFn(true)
	s.True(ack.ackCalled)
}

func (s *RabbitMQConsumerTestSuite) TestHandleMessage_AckOnSuccess_NackError() {
	cfg := &parser.RabbitMQTrigger{
		URL:          "amqp://localhost:5672",
		Queue:        "test-queue",
		AckOnSuccess: true,
	}
	consumer := NewRabbitMQConsumer(cfg, s.logger)

	ack := &mockAcknowledger{nackErr: errors.New("nack err")}
	msg := amqp.Delivery{
		Body:         []byte("hello"),
		Acknowledger: ack,
	}

	var receivedAckFn func(bool)
	consumer.handleMessage(msg, func(triggerData map[string]any, ackFn func(bool)) {
		receivedAckFn = ackFn
	})

	receivedAckFn(false)
	s.True(ack.nackCalled)
}

func (s *RabbitMQConsumerTestSuite) TestHandleMessage_NoAckOnSuccess_AckError() {
	consumer := NewRabbitMQConsumer(s.config, s.logger)

	ack := &mockAcknowledger{ackErr: errors.New("ack err")}
	msg := amqp.Delivery{
		Body:         []byte("hello"),
		Acknowledger: ack,
	}

	var receivedNilAckFn bool
	consumer.handleMessage(msg, func(triggerData map[string]any, ackFn func(bool)) {
		receivedNilAckFn = (ackFn == nil)
	})

	s.True(receivedNilAckFn, "ackFn should be nil when AckOnSuccess is false")
	s.True(ack.ackCalled)
}

func (s *RabbitMQConsumerTestSuite) TestOpenChannel_NilDial_UsesRealAMQPDial() {
	original := amqpRawDial
	s.T().Cleanup(func() { amqpRawDial = original })

	deliveries := make(chan amqp.Delivery)
	amqpRawDial = func(url string) (rawConn, error) {
		s.Equal("amqp://localhost:5672", url)
		return &mockRawConn{
			channelFn: func() (*amqp.Channel, error) {
				return nil, nil
			},
			closeFn: func() error { return nil },
		}, nil
	}

	consumer := NewRabbitMQConsumer(s.config, s.logger)
	// dial is nil, so openChannel should use realAMQPDial
	s.Nil(consumer.dial)

	// We need the channel to return a working consume
	// But since realAMQPDial wraps the connection and calls conn.Channel() which returns nil,
	// the Channel() call on the wrapped conn will return (nil, nil) from the mock.
	// However, that won't work because Qos/Consume are called on a nil *amqp.Channel.
	// Let's just test that the dial=nil path is exercised, which goes through realAMQPDial.
	// The consume call will fail since we can't mock at the amqpChan level here.
	// We just need to verify dial == nil is covered.

	// With Prefetch=0, no Qos call. But Consume on nil channel will panic.
	// So let's make the Channel() call return an error.
	amqpRawDial = func(url string) (rawConn, error) {
		return &mockRawConn{
			channelFn: func() (*amqp.Channel, error) {
				return nil, errors.New("channel failed from nil dial")
			},
			closeFn: func() error { return nil },
		}, nil
	}

	_, err := consumer.openChannel()
	s.Require().Error(err)
	s.Contains(err.Error(), "rabbitmq channel")

	_ = deliveries // suppress unused
}

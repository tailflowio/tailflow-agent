package server

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

// mockAMQPConn is a test double for the amqpConn interface.
type mockAMQPConn struct {
	channelFn func() (amqpChan, error)
	closeFn   func() error
	closed    bool
}

func (m *mockAMQPConn) Channel() (amqpChan, error) {
	if m.channelFn != nil {
		return m.channelFn()
	}
	return nil, nil
}

func (m *mockAMQPConn) Close() error {
	m.closed = true
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

// mockAMQPChan is a test double for the amqpChan interface.
type mockAMQPChan struct {
	qosFn     func(prefetchCount, prefetchSize int, global bool) error
	consumeFn func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
	closeFn   func() error
	closed    bool
}

func (m *mockAMQPChan) Qos(prefetchCount, prefetchSize int, global bool) error {
	if m.qosFn != nil {
		return m.qosFn(prefetchCount, prefetchSize, global)
	}
	return nil
}

func (m *mockAMQPChan) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	if m.consumeFn != nil {
		return m.consumeFn(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
	}
	return nil, nil
}

func (m *mockAMQPChan) Close() error {
	m.closed = true
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

// mockAcknowledger satisfies the amqp.Acknowledger interface for testing.
type mockAcknowledger struct {
	ackCalled  bool
	nackCalled bool
	ackErr     error
	nackErr    error
}

func (m *mockAcknowledger) Ack(tag uint64, multiple bool) error {
	m.ackCalled = true
	return m.ackErr
}

func (m *mockAcknowledger) Nack(tag uint64, multiple bool, requeue bool) error {
	m.nackCalled = true
	return m.nackErr
}

func (m *mockAcknowledger) Reject(tag uint64, requeue bool) error {
	return nil
}

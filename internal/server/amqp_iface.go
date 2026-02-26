package server

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

// rawConn is the subset of *amqp.Connection used by realAMQPDial.
type rawConn interface {
	Channel() (*amqp.Channel, error)
	Close() error
}

// amqpRawDial is the low-level dial function (amqp.Dial by default).
// Tests can replace it to avoid hitting a real broker.
var amqpRawDial func(url string) (rawConn, error) = func(url string) (rawConn, error) {
	return amqp.Dial(url)
}

// amqpDialer dials an AMQP server and returns a connection.
type amqpDialer func(url string) (amqpConn, error)

// amqpChan is the subset of *amqp.Channel used by this package.
type amqpChan interface {
	Qos(prefetchCount, prefetchSize int, global bool) error
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
	Close() error
}

// amqpConn is the subset of *amqp.Connection used by this package.
type amqpConn interface {
	Channel() (amqpChan, error)
	Close() error
}

// realAMQPConn wraps *amqp.Connection to satisfy amqpConn via function fields.
type realAMQPConn struct {
	channelFn func() (amqpChan, error)
	closeFn   func() error
}

func (r *realAMQPConn) Channel() (amqpChan, error) { return r.channelFn() }
func (r *realAMQPConn) Close() error               { return r.closeFn() }

// realAMQPDial is the production dialer.
func realAMQPDial(url string) (amqpConn, error) {
	c, err := amqpRawDial(url)
	if err != nil {
		return nil, err
	}

	return &realAMQPConn{
		channelFn: func() (amqpChan, error) { return c.Channel() },
		closeFn:   c.Close,
	}, nil
}

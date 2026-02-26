package server

import (
	"fmt"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/suite"
)

type AMQPIfaceTestSuite struct {
	suite.Suite
}

func TestAMQPIface(t *testing.T) {
	suite.Run(t, new(AMQPIfaceTestSuite))
}

func (s *AMQPIfaceTestSuite) SetupTest() {
	// required by convention
}

type mockRawConn struct {
	channelFn func() (*amqp.Channel, error)
	closeFn   func() error
}

func (m *mockRawConn) Channel() (*amqp.Channel, error) {
	if m.channelFn != nil {
		return m.channelFn()
	}
	return nil, nil
}

func (m *mockRawConn) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

func (s *AMQPIfaceTestSuite) TestRealAMQPConn_Channel_Success() {
	expected := &mockAMQPChan{}
	conn := &realAMQPConn{
		channelFn: func() (amqpChan, error) {
			return expected, nil
		},
		closeFn: func() error { return nil },
	}

	ch, err := conn.Channel()
	s.Require().NoError(err)
	s.Equal(expected, ch)
}

func (s *AMQPIfaceTestSuite) TestRealAMQPConn_Channel_Error() {
	conn := &realAMQPConn{
		channelFn: func() (amqpChan, error) {
			return nil, fmt.Errorf("channel open failed")
		},
		closeFn: func() error { return nil },
	}

	ch, err := conn.Channel()
	s.Require().Error(err)
	s.Nil(ch)
	s.Contains(err.Error(), "channel open failed")
}

func (s *AMQPIfaceTestSuite) TestRealAMQPConn_Close_Success() {
	conn := &realAMQPConn{
		channelFn: func() (amqpChan, error) { return nil, nil },
		closeFn:   func() error { return nil },
	}

	err := conn.Close()
	s.Require().NoError(err)
}

func (s *AMQPIfaceTestSuite) TestRealAMQPConn_Close_Error() {
	conn := &realAMQPConn{
		channelFn: func() (amqpChan, error) { return nil, nil },
		closeFn:   func() error { return fmt.Errorf("close failed") },
	}

	err := conn.Close()
	s.Require().Error(err)
	s.Contains(err.Error(), "close failed")
}

func (s *AMQPIfaceTestSuite) TestRealAMQPDial_Success() {
	original := amqpRawDial
	defer func() { amqpRawDial = original }()

	channelCalled := false
	closeCalled := false

	amqpRawDial = func(url string) (rawConn, error) {
		s.Equal("amqp://fakehost:5672", url)
		return &mockRawConn{
			channelFn: func() (*amqp.Channel, error) {
				channelCalled = true
				return nil, nil
			},
			closeFn: func() error {
				closeCalled = true
				return nil
			},
		}, nil
	}

	conn, err := realAMQPDial("amqp://fakehost:5672")
	s.Require().NoError(err)
	s.Require().NotNil(conn)

	// Exercise the closures to cover the wrapper lines.
	_, _ = conn.Channel()
	s.True(channelCalled)

	_ = conn.Close()
	s.True(closeCalled)
}

func (s *AMQPIfaceTestSuite) TestRealAMQPDial_Error() {
	original := amqpRawDial
	defer func() { amqpRawDial = original }()

	amqpRawDial = func(url string) (rawConn, error) {
		return nil, fmt.Errorf("dial failed")
	}

	conn, err := realAMQPDial("amqp://localhost:59999")
	s.Require().Error(err)
	s.Nil(conn)
	s.Contains(err.Error(), "dial failed")
}

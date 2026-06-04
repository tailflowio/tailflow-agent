package action

import (
	"errors"
	"fmt"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/suite"
)

type RabbitMQShovelWrappersTestSuite struct {
	suite.Suite
}

func TestRabbitMQShovelWrappers(t *testing.T) {
	suite.Run(t, new(RabbitMQShovelWrappersTestSuite))
}

func (s *RabbitMQShovelWrappersTestSuite) SetupTest() {}

func (s *RabbitMQShovelWrappersTestSuite) TestRealShovelDest_ConfirmDelegates() {
	var calledNoWait bool
	ch := &mockShovelRawChan{
		confirmFn: func(noWait bool) error {
			calledNoWait = noWait
			return nil
		},
	}

	dest := &realShovelDest{ch: ch}

	err := dest.Confirm(true)

	s.NoError(err)
	s.True(calledNoWait, "Confirm should delegate noWait=true to the underlying channel")
}

func (s *RabbitMQShovelWrappersTestSuite) TestRealShovelDest_ConfirmReturnsError() {
	ch := &mockShovelRawChan{
		confirmFn: func(noWait bool) error {
			return errors.New("confirm failed")
		},
	}

	dest := &realShovelDest{ch: ch}

	err := dest.Confirm(false)

	s.Require().Error(err)
	s.Equal("confirm failed", err.Error())
}

func (s *RabbitMQShovelWrappersTestSuite) TestRealShovelDest_PublishWithDeferredConfirmDelegates() {
	var capturedExchange, capturedKey string
	var capturedMandatory, capturedImmediate bool
	var capturedMsg amqp.Publishing

	ch := &mockShovelRawChan{
		publishFn: func(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (*amqp.DeferredConfirmation, error) {
			capturedExchange = exchange
			capturedKey = key
			capturedMandatory = mandatory
			capturedImmediate = immediate
			capturedMsg = msg
			return &amqp.DeferredConfirmation{}, nil
		},
	}

	dest := &realShovelDest{ch: ch}
	pub := amqp.Publishing{Body: []byte("test-body"), ContentType: "text/plain"}

	conf, err := dest.PublishWithDeferredConfirm("test-exchange", "test-key", true, false, pub)

	s.Require().NoError(err)
	s.NotNil(conf)
	s.Equal("test-exchange", capturedExchange)
	s.Equal("test-key", capturedKey)
	s.True(capturedMandatory)
	s.False(capturedImmediate)
	s.Equal([]byte("test-body"), capturedMsg.Body)
	s.Equal("text/plain", capturedMsg.ContentType)
}

func (s *RabbitMQShovelWrappersTestSuite) TestRealShovelDest_PublishWithDeferredConfirmReturnsError() {
	ch := &mockShovelRawChan{
		publishFn: func(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (*amqp.DeferredConfirmation, error) {
			return nil, errors.New("publish failed")
		},
	}

	dest := &realShovelDest{ch: ch}

	conf, err := dest.PublishWithDeferredConfirm("", "q", false, false, amqp.Publishing{})

	s.Require().Error(err)
	s.Equal("publish failed", err.Error())
	s.Nil(conf)
}

func (s *RabbitMQShovelWrappersTestSuite) TestShovelDialFn_Default_DialFails() {
	_, err := shovelDialFn("amqp://localhost:59999")
	s.Require().Error(err)
}

func (s *RabbitMQShovelWrappersTestSuite) TestShovelDialFn_RawDialError_PropagatesError() {
	original := shovelRawDialFn
	defer func() { shovelRawDialFn = original }()

	shovelRawDialFn = func(_ string) (shovelAMQPConn, error) {
		return nil, fmt.Errorf("raw dial refused")
	}

	_, err := shovelDialFn("amqp://any")
	s.Require().Error(err)
	s.Contains(err.Error(), "raw dial refused")
}

func (s *RabbitMQShovelWrappersTestSuite) TestShovelDialFn_RawDialSuccess_ReturnsConnector() {
	original := shovelRawDialFn
	defer func() { shovelRawDialFn = original }()

	conn := &mockAMQPConnection{
		channelFn: func() (*amqp.Channel, error) { return nil, nil },
		closeFn:   func() error { return nil },
	}

	shovelRawDialFn = func(_ string) (shovelAMQPConn, error) {
		return conn, nil
	}

	connector, err := shovelDialFn("amqp://any")
	s.Require().NoError(err)
	s.NotNil(connector)
}

func (s *RabbitMQShovelWrappersTestSuite) TestShovelRawDialFn_DialError_NoNetwork() {
	c, err := shovelRawDialFn("not://invalid")
	s.Require().Error(err)
	s.Nil(c)
}

type mockAMQPConnection struct {
	channelFn func() (*amqp.Channel, error)
	closeFn   func() error
}

func (m *mockAMQPConnection) Channel() (*amqp.Channel, error) {
	if m.channelFn != nil {
		return m.channelFn()
	}

	return nil, nil
}

func (m *mockAMQPConnection) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}

	return nil
}

func (s *RabbitMQShovelWrappersTestSuite) TestWrapShovelConn_ChannelSuccess() {
	conn := &mockAMQPConnection{
		channelFn: func() (*amqp.Channel, error) {
			return nil, nil
		},
		closeFn: func() error { return nil },
	}

	wrapped := wrapShovelConn(conn)
	s.NotNil(wrapped)

	ch, err := wrapped.Channel()
	s.NoError(err)
	s.Nil(ch)
}

func (s *RabbitMQShovelWrappersTestSuite) TestWrapShovelConn_ChannelError() {
	conn := &mockAMQPConnection{
		channelFn: func() (*amqp.Channel, error) {
			return nil, errors.New("channel failed")
		},
	}

	wrapped := wrapShovelConn(conn)

	_, err := wrapped.Channel()
	s.Require().Error(err)
	s.Contains(err.Error(), "channel failed")
}

func (s *RabbitMQShovelWrappersTestSuite) TestWrapShovelConn_Close() {
	closed := false
	conn := &mockAMQPConnection{
		closeFn: func() error {
			closed = true

			return nil
		},
	}

	wrapped := wrapShovelConn(conn)

	err := wrapped.Close()
	s.NoError(err)
	s.True(closed)
}

func (s *RabbitMQShovelWrappersTestSuite) TestRealShovelConn_Channel_Success() {
	expected := &mockShovelRawChan{}
	conn := &realShovelConn{
		channelFn: func() (shovelRawChan, error) {
			return expected, nil
		},
	}

	ch, err := conn.Channel()

	s.Require().NoError(err)
	s.Equal(expected, ch)
}

func (s *RabbitMQShovelWrappersTestSuite) TestRealShovelConn_Channel_Error() {
	conn := &realShovelConn{
		channelFn: func() (shovelRawChan, error) {
			return nil, fmt.Errorf("channel open failed")
		},
	}

	ch, err := conn.Channel()

	s.Require().Error(err)
	s.Equal("channel open failed", err.Error())
	s.Nil(ch)
}

func (s *RabbitMQShovelWrappersTestSuite) TestRealShovelConn_Close_Success() {
	conn := &realShovelConn{
		closeFn: func() error {
			return nil
		},
	}

	err := conn.Close()

	s.NoError(err)
}

func (s *RabbitMQShovelWrappersTestSuite) TestRealShovelConn_Close_Error() {
	conn := &realShovelConn{
		closeFn: func() error {
			return fmt.Errorf("close connection failed")
		},
	}

	err := conn.Close()

	s.Require().Error(err)
	s.Equal("close connection failed", err.Error())
}

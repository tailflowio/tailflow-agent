package action

import (
	"errors"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/suite"
)

type mockShovelRawChan struct {
	getFn     func(queue string, autoAck bool) (amqp.Delivery, bool, error)
	confirmFn func(noWait bool) error
	publishFn func(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (*amqp.DeferredConfirmation, error)
	closeFn   func() error
	closed    bool
}

func (m *mockShovelRawChan) Get(queue string, autoAck bool) (amqp.Delivery, bool, error) {
	if m.getFn != nil {
		return m.getFn(queue, autoAck)
	}
	return amqp.Delivery{}, false, nil
}

func (m *mockShovelRawChan) Confirm(noWait bool) error {
	if m.confirmFn != nil {
		return m.confirmFn(noWait)
	}
	return nil
}

func (m *mockShovelRawChan) PublishWithDeferredConfirm(
	exchange, key string,
	mandatory, immediate bool,
	msg amqp.Publishing,
) (*amqp.DeferredConfirmation, error) {
	if m.publishFn != nil {
		return m.publishFn(exchange, key, mandatory, immediate, msg)
	}
	return nil, nil
}

func (m *mockShovelRawChan) Close() error {
	m.closed = true
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

type mockShovelConnector struct {
	channelFn func() (shovelRawChan, error)
	closeFn   func() error
	closed    bool
}

func (m *mockShovelConnector) Channel() (shovelRawChan, error) {
	return m.channelFn()
}

func (m *mockShovelConnector) Close() error {
	m.closed = true
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

type RabbitMQShovelChannelsTestSuite struct {
	suite.Suite
}

func TestRabbitMQShovelChannels(t *testing.T) {
	suite.Run(t, new(RabbitMQShovelChannelsTestSuite))
}

func (s *RabbitMQShovelChannelsTestSuite) SetupTest() {}

func (s *RabbitMQShovelChannelsTestSuite) TestOpenShovelChannelsImpl_DialSourceFails() {
	orig := shovelDialFn
	defer func() { shovelDialFn = orig }()

	shovelDialFn = func(url string) (shovelConnector, error) {
		return nil, errors.New("connection refused")
	}

	cfg := shovelConfig{
		sourceURL:   "amqp://source",
		destURL:     "amqp://source",
		sourceQueue: "src",
		destQueue:   "dst",
	}

	srcCh, destCh, cleanup, err := openShovelChannelsImpl(cfg)

	s.Require().Error(err)
	s.Contains(err.Error(), "dial source")
	s.Contains(err.Error(), "connection refused")
	s.Nil(srcCh)
	s.Nil(destCh)
	s.Nil(cleanup)
}

func (s *RabbitMQShovelChannelsTestSuite) TestOpenShovelChannelsImpl_SourceChannelFails() {
	orig := shovelDialFn
	defer func() { shovelDialFn = orig }()

	sourceConn := &mockShovelConnector{
		channelFn: func() (shovelRawChan, error) {
			return nil, errors.New("channel error")
		},
	}

	shovelDialFn = func(url string) (shovelConnector, error) {
		return sourceConn, nil
	}

	cfg := shovelConfig{
		sourceURL:   "amqp://source",
		destURL:     "amqp://source",
		sourceQueue: "src",
		destQueue:   "dst",
	}

	srcCh, destCh, cleanup, err := openShovelChannelsImpl(cfg)

	s.Require().Error(err)
	s.Contains(err.Error(), "source channel")
	s.Contains(err.Error(), "channel error")
	s.Nil(srcCh)
	s.Nil(destCh)
	s.Nil(cleanup)
	s.True(sourceConn.closed, "source connection should be closed on source channel failure")
}

func (s *RabbitMQShovelChannelsTestSuite) TestOpenShovelChannelsImpl_SameURL_DestChannelFails() {
	orig := shovelDialFn
	defer func() { shovelDialFn = orig }()

	sourceCh := &mockShovelRawChan{}
	callCount := 0
	sourceConn := &mockShovelConnector{
		channelFn: func() (shovelRawChan, error) {
			callCount++
			if callCount == 1 {
				return sourceCh, nil
			}
			return nil, errors.New("dest channel error")
		},
	}

	shovelDialFn = func(url string) (shovelConnector, error) {
		return sourceConn, nil
	}

	cfg := shovelConfig{
		sourceURL:   "amqp://same-host",
		destURL:     "amqp://same-host",
		sourceQueue: "src",
		destQueue:   "dst",
	}

	srcOut, destOut, cleanup, err := openShovelChannelsImpl(cfg)

	s.Require().Error(err)
	s.Contains(err.Error(), "dest channel")
	s.Contains(err.Error(), "dest channel error")
	s.Nil(srcOut)
	s.Nil(destOut)
	s.Nil(cleanup)
	s.True(sourceCh.closed, "source channel should be closed")
	s.True(sourceConn.closed, "source connection should be closed")
}

func (s *RabbitMQShovelChannelsTestSuite) TestOpenShovelChannelsImpl_DifferentURL_DestDialFails() {
	orig := shovelDialFn
	defer func() { shovelDialFn = orig }()

	sourceCh := &mockShovelRawChan{}
	sourceConn := &mockShovelConnector{
		channelFn: func() (shovelRawChan, error) {
			return sourceCh, nil
		},
	}

	shovelDialFn = func(url string) (shovelConnector, error) {
		if url == "amqp://source" {
			return sourceConn, nil
		}
		return nil, errors.New("dest dial refused")
	}

	cfg := shovelConfig{
		sourceURL:   "amqp://source",
		destURL:     "amqp://dest",
		sourceQueue: "src",
		destQueue:   "dst",
	}

	srcOut, destOut, cleanup, err := openShovelChannelsImpl(cfg)

	s.Require().Error(err)
	s.Contains(err.Error(), "dial dest")
	s.Contains(err.Error(), "dest dial refused")
	s.Nil(srcOut)
	s.Nil(destOut)
	s.Nil(cleanup)
	s.True(sourceCh.closed, "source channel should be closed on dest dial failure")
	s.True(sourceConn.closed, "source connection should be closed on dest dial failure")
}

func (s *RabbitMQShovelChannelsTestSuite) TestOpenShovelChannelsImpl_DifferentURL_DestChannelFails() {
	orig := shovelDialFn
	defer func() { shovelDialFn = orig }()

	sourceCh := &mockShovelRawChan{}
	sourceConn := &mockShovelConnector{
		channelFn: func() (shovelRawChan, error) {
			return sourceCh, nil
		},
	}

	destConn := &mockShovelConnector{
		channelFn: func() (shovelRawChan, error) {
			return nil, errors.New("dest channel error")
		},
	}

	shovelDialFn = func(url string) (shovelConnector, error) {
		if url == "amqp://source" {
			return sourceConn, nil
		}
		return destConn, nil
	}

	cfg := shovelConfig{
		sourceURL:   "amqp://source",
		destURL:     "amqp://dest",
		sourceQueue: "src",
		destQueue:   "dst",
	}

	srcOut, destOut, cleanup, err := openShovelChannelsImpl(cfg)

	s.Require().Error(err)
	s.Contains(err.Error(), "dest channel")
	s.Contains(err.Error(), "dest channel error")
	s.Nil(srcOut)
	s.Nil(destOut)
	s.Nil(cleanup)
	s.True(destConn.closed, "dest connection should be closed when destConn != sourceConn")
	s.True(sourceCh.closed, "source channel should be closed")
	s.True(sourceConn.closed, "source connection should be closed")
}

func (s *RabbitMQShovelChannelsTestSuite) TestOpenShovelChannelsImpl_SuccessSameURL() {
	orig := shovelDialFn
	defer func() { shovelDialFn = orig }()

	sourceCh := &mockShovelRawChan{}
	destCh := &mockShovelRawChan{}
	callCount := 0
	sourceConn := &mockShovelConnector{
		channelFn: func() (shovelRawChan, error) {
			callCount++
			if callCount == 1 {
				return sourceCh, nil
			}
			return destCh, nil
		},
	}

	dialCount := 0
	shovelDialFn = func(url string) (shovelConnector, error) {
		dialCount++
		return sourceConn, nil
	}

	cfg := shovelConfig{
		sourceURL:   "amqp://same-host",
		destURL:     "amqp://same-host",
		sourceQueue: "src",
		destQueue:   "dst",
	}

	srcOut, destOut, cleanup, err := openShovelChannelsImpl(cfg)

	s.Require().NoError(err)
	s.NotNil(srcOut)
	s.NotNil(destOut)
	s.NotNil(cleanup)
	s.Equal(1, dialCount, "should only dial once when source and dest URLs are the same")
	s.Equal(2, callCount, "should open two channels on the same connection")

	s.Equal(sourceCh, srcOut)

	rd, ok := destOut.(*realShovelDest)
	s.Require().True(ok, "destOut should be *realShovelDest")
	s.Equal(destCh, rd.ch)

	s.False(sourceCh.closed)
	s.False(destCh.closed)
	s.False(sourceConn.closed)

	cleanup()
	s.True(destCh.closed, "dest channel should be closed after cleanup")
	s.True(sourceCh.closed, "source channel should be closed after cleanup")
	s.True(sourceConn.closed, "source connection should be closed after cleanup")
}

func (s *RabbitMQShovelChannelsTestSuite) TestOpenShovelChannelsImpl_SuccessDifferentURL() {
	orig := shovelDialFn
	defer func() { shovelDialFn = orig }()

	sourceCh := &mockShovelRawChan{}
	destCh := &mockShovelRawChan{}
	sourceConn := &mockShovelConnector{
		channelFn: func() (shovelRawChan, error) {
			return sourceCh, nil
		},
	}
	destConn := &mockShovelConnector{
		channelFn: func() (shovelRawChan, error) {
			return destCh, nil
		},
	}

	dialCount := 0
	shovelDialFn = func(url string) (shovelConnector, error) {
		dialCount++
		if url == "amqp://source" {
			return sourceConn, nil
		}
		return destConn, nil
	}

	cfg := shovelConfig{
		sourceURL:   "amqp://source",
		destURL:     "amqp://dest",
		sourceQueue: "src",
		destQueue:   "dst",
	}

	srcOut, destOut, cleanup, err := openShovelChannelsImpl(cfg)

	s.Require().NoError(err)
	s.NotNil(srcOut)
	s.NotNil(destOut)
	s.NotNil(cleanup)
	s.Equal(2, dialCount, "should dial twice for different URLs")

	s.Equal(sourceCh, srcOut)

	rd, ok := destOut.(*realShovelDest)
	s.Require().True(ok, "destOut should be *realShovelDest")
	s.Equal(destCh, rd.ch)

	s.False(sourceCh.closed)
	s.False(destCh.closed)
	s.False(sourceConn.closed)
	s.False(destConn.closed)

	cleanup()
	s.True(destCh.closed, "dest channel should be closed after cleanup")
	s.True(destConn.closed, "dest connection should be closed after cleanup (different URL)")
	s.True(sourceCh.closed, "source channel should be closed after cleanup")
	s.True(sourceConn.closed, "source connection should be closed after cleanup")
}

func (s *RabbitMQShovelChannelsTestSuite) TestOpenShovelChannelsImpl_CleanupSameURLDoesNotCloseDestConn() {
	orig := shovelDialFn
	defer func() { shovelDialFn = orig }()

	sourceCh := &mockShovelRawChan{}
	destCh := &mockShovelRawChan{}
	callCount := 0
	conn := &mockShovelConnector{
		channelFn: func() (shovelRawChan, error) {
			callCount++
			if callCount == 1 {
				return sourceCh, nil
			}
			return destCh, nil
		},
	}

	closeCount := 0
	conn.closeFn = func() error {
		closeCount++
		return nil
	}

	shovelDialFn = func(url string) (shovelConnector, error) {
		return conn, nil
	}

	cfg := shovelConfig{
		sourceURL:   "amqp://same-host",
		destURL:     "amqp://same-host",
		sourceQueue: "src",
		destQueue:   "dst",
	}

	_, _, cleanup, err := openShovelChannelsImpl(cfg)
	s.Require().NoError(err)

	cleanup()

	s.Equal(1, closeCount, "connection Close should be called exactly once when source and dest use the same URL")
}

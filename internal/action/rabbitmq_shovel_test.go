package action

import (
	"context"
	"errors"
	"fmt"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/suite"
)

type mockShovelSource struct {
	getFn func(queue string, autoAck bool) (amqp.Delivery, bool, error)
}

func (m *mockShovelSource) Get(queue string, autoAck bool) (amqp.Delivery, bool, error) {
	return m.getFn(queue, autoAck)
}

type mockShovelDest struct {
	confirmFn func(noWait bool) error
	publishFn func(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (shovelConfirmation, error)
}

func (m *mockShovelDest) Confirm(noWait bool) error {
	return m.confirmFn(noWait)
}

func (m *mockShovelDest) PublishWithDeferredConfirm(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (shovelConfirmation, error) {
	return m.publishFn(exchange, key, mandatory, immediate, msg)
}

type mockShovelConfirmation struct {
	result bool
}

func (m *mockShovelConfirmation) Wait() bool { return m.result }

type mockShovelAcknowledger struct {
	ackCalled  bool
	nackCalled bool
	ackErr     error
	nackErr    error
}

func (m *mockShovelAcknowledger) Ack(tag uint64, multiple bool) error {
	m.ackCalled = true
	return m.ackErr
}

func (m *mockShovelAcknowledger) Nack(tag uint64, multiple bool, requeue bool) error {
	m.nackCalled = true
	return m.nackErr
}

func (m *mockShovelAcknowledger) Reject(tag uint64, requeue bool) error { return nil }

type RabbitMQShovelActionTestSuite struct {
	suite.Suite
}

func TestRabbitMQShovelAction(t *testing.T) {
	suite.Run(t, new(RabbitMQShovelActionTestSuite))
}

func (s *RabbitMQShovelActionTestSuite) SetupTest() {}

func (s *RabbitMQShovelActionTestSuite) TestValidateMissingURL() {
	act := NewRabbitMQShovelAction()
	ctx := newTestContext(map[string]any{
		"source_queue": "src",
		"dest_queue":   "dst",
	})

	err := act.Validate(ctx)

	s.Require().Error(err)
	s.Contains(err.Error(), "url")
}

func (s *RabbitMQShovelActionTestSuite) TestValidateMissingSourceQueue() {
	act := NewRabbitMQShovelAction()
	ctx := newTestContext(map[string]any{
		"url":        "amqp://localhost",
		"dest_queue": "dst",
	})

	err := act.Validate(ctx)

	s.Require().Error(err)
	s.Contains(err.Error(), "source_queue")
}

func (s *RabbitMQShovelActionTestSuite) TestValidateMissingDestQueue() {
	act := NewRabbitMQShovelAction()
	ctx := newTestContext(map[string]any{
		"url":          "amqp://localhost",
		"source_queue": "src",
	})

	err := act.Validate(ctx)

	s.Require().Error(err)
	s.Contains(err.Error(), "dest_queue")
}

func (s *RabbitMQShovelActionTestSuite) TestValidateOK() {
	act := NewRabbitMQShovelAction()
	ctx := newTestContext(map[string]any{
		"url":          "amqp://localhost",
		"source_queue": "src",
		"dest_queue":   "dst",
	})

	err := act.Validate(ctx)

	s.NoError(err)
}

func (s *RabbitMQShovelActionTestSuite) TestParseConfigDefaults() {
	ctx := newTestContext(map[string]any{
		"url":          "amqp://localhost",
		"source_queue": "src",
		"dest_queue":   "dst",
	})

	cfg := parseShovelConfig(ctx)

	s.Equal("amqp://localhost", cfg.sourceURL)
	s.Equal("amqp://localhost", cfg.destURL)
	s.Equal("src", cfg.sourceQueue)
	s.Equal("dst", cfg.destQueue)
	s.Equal(10, cfg.count)
	s.True(cfg.stripHeaders)
}

func (s *RabbitMQShovelActionTestSuite) TestParseConfigCustomValues() {
	ctx := newTestContext(map[string]any{
		"url":           "amqp://source",
		"dest_url":      "amqp://dest",
		"source_queue":  "src",
		"dest_queue":    "dst",
		"count":         float64(25),
		"strip_headers": false,
	})

	cfg := parseShovelConfig(ctx)

	s.Equal("amqp://source", cfg.sourceURL)
	s.Equal("amqp://dest", cfg.destURL)
	s.Equal(25, cfg.count)
	s.False(cfg.stripHeaders)
}

func (s *RabbitMQShovelActionTestSuite) TestParseConfigStripHeadersStringFalse() {
	ctx := newTestContext(map[string]any{
		"url":           "amqp://localhost",
		"source_queue":  "src",
		"dest_queue":    "dst",
		"strip_headers": "false",
	})

	cfg := parseShovelConfig(ctx)

	s.False(cfg.stripHeaders)
}

func (s *RabbitMQShovelActionTestSuite) TestParseConfigStripHeadersStringTrue() {
	ctx := newTestContext(map[string]any{
		"url":           "amqp://localhost",
		"source_queue":  "src",
		"dest_queue":    "dst",
		"strip_headers": "true",
	})

	cfg := parseShovelConfig(ctx)

	s.True(cfg.stripHeaders)
}

func (s *RabbitMQShovelActionTestSuite) TestBuildPublishingStripHeaders() {
	msg := amqp.Delivery{
		Body:          []byte("hello"),
		ContentType:   "text/plain",
		Headers:       amqp.Table{"x-retry": int32(3)},
		CorrelationId: "corr-1",
		ReplyTo:       "reply-q",
		MessageId:     "msg-1",
		Type:          "order",
		AppId:         "app-1",
	}

	pub := buildPublishing(msg, true)

	s.Equal([]byte("hello"), pub.Body)
	s.Equal("text/plain", pub.ContentType)
	s.Equal(uint8(amqp.Persistent), pub.DeliveryMode)
	s.Nil(pub.Headers)
	s.Empty(pub.CorrelationId)
	s.Empty(pub.ReplyTo)
	s.Empty(pub.MessageId)
	s.Empty(pub.Type)
	s.Empty(pub.AppId)
}

func (s *RabbitMQShovelActionTestSuite) TestBuildPublishingKeepHeaders() {
	msg := amqp.Delivery{
		Body:          []byte("hello"),
		ContentType:   "application/json",
		Headers:       amqp.Table{"x-retry": int32(3)},
		CorrelationId: "corr-1",
		ReplyTo:       "reply-q",
		MessageId:     "msg-1",
		Type:          "order",
		AppId:         "app-1",
	}

	pub := buildPublishing(msg, false)

	s.Equal([]byte("hello"), pub.Body)
	s.Equal("application/json", pub.ContentType)
	s.Equal(amqp.Table{"x-retry": int32(3)}, pub.Headers)
	s.Equal("corr-1", pub.CorrelationId)
	s.Equal("reply-q", pub.ReplyTo)
	s.Equal("msg-1", pub.MessageId)
	s.Equal("order", pub.Type)
	s.Equal("app-1", pub.AppId)
}

func (s *RabbitMQShovelActionTestSuite) TestExecuteDialFailure() {
	original := openShovelChannelsFn
	defer func() { openShovelChannelsFn = original }()

	openShovelChannelsFn = func(cfg shovelConfig) (shovelSourceChan, shovelDestChan, func(), error) {
		return nil, nil, nil, fmt.Errorf("rabbitmq.shovel: dial source: connection refused")
	}

	act := NewRabbitMQShovelAction()
	ctx := newTestContext(map[string]any{
		"url":          "amqp://invalid:9999",
		"source_queue": "src",
		"dest_queue":   "dst",
	})

	_, err := act.Execute(ctx)

	s.Require().Error(err)
	s.Contains(err.Error(), "dial source")
}

func (s *RabbitMQShovelActionTestSuite) TestExecuteConfirmFail() {
	original := openShovelChannelsFn
	defer func() { openShovelChannelsFn = original }()

	cleanedUp := false
	dest := &mockShovelDest{
		confirmFn: func(noWait bool) error {
			return errors.New("confirm failed")
		},
	}

	openShovelChannelsFn = func(cfg shovelConfig) (shovelSourceChan, shovelDestChan, func(), error) {
		return &mockShovelSource{}, dest, func() { cleanedUp = true }, nil
	}

	act := NewRabbitMQShovelAction()
	ctx := newTestContext(map[string]any{
		"url":          "amqp://localhost",
		"source_queue": "src",
		"dest_queue":   "dst",
	})

	_, err := act.Execute(ctx)

	s.Require().Error(err)
	s.Contains(err.Error(), "enable confirms")
	s.True(cleanedUp)
}

func (s *RabbitMQShovelActionTestSuite) TestExecuteSuccess() {
	original := openShovelChannelsFn
	defer func() { openShovelChannelsFn = original }()

	acker := &mockShovelAcknowledger{}
	source := &mockShovelSource{
		getFn: func(queue string, autoAck bool) (amqp.Delivery, bool, error) {
			return amqp.Delivery{
				Body:         []byte(`{"key":"val"}`),
				ContentType:  "application/json",
				Acknowledger: acker,
			}, true, nil
		},
	}
	dest := &mockShovelDest{
		confirmFn: func(noWait bool) error { return nil },
		publishFn: func(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (shovelConfirmation, error) {
			return &mockShovelConfirmation{result: true}, nil
		},
	}

	cleanedUp := false
	openShovelChannelsFn = func(cfg shovelConfig) (shovelSourceChan, shovelDestChan, func(), error) {
		return source, dest, func() { cleanedUp = true }, nil
	}

	act := NewRabbitMQShovelAction()
	ctx := newTestContext(map[string]any{
		"url":          "amqp://localhost",
		"source_queue": "src",
		"dest_queue":   "dst",
		"count":        float64(1),
	})

	result, err := act.Execute(ctx)

	s.Require().NoError(err)
	s.True(cleanedUp)

	out := result.(map[string]any)
	s.Equal(1, out["succeeded"])
	s.Equal(0, out["failed"])
	s.Equal(1, out["total"])
}

func (s *RabbitMQShovelActionTestSuite) TestRunShovelLoop_ContextCancelled() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	actCtx := &ActionContext{Context: ctx}
	cfg := shovelConfig{count: 5, sourceQueue: "q", destQueue: "d"}

	_, err := runShovelLoop(actCtx, &mockShovelSource{}, &mockShovelDest{}, cfg)

	s.Require().Error(err)
	s.ErrorIs(err, context.Canceled)
}

func (s *RabbitMQShovelActionTestSuite) TestRunShovelLoop_GetError() {
	actCtx := &ActionContext{Context: context.Background()}
	cfg := shovelConfig{count: 1, sourceQueue: "q", destQueue: "d"}

	source := &mockShovelSource{
		getFn: func(queue string, autoAck bool) (amqp.Delivery, bool, error) {
			return amqp.Delivery{}, false, errors.New("get failed")
		},
	}

	_, err := runShovelLoop(actCtx, source, &mockShovelDest{}, cfg)

	s.Require().Error(err)
	s.Contains(err.Error(), "get from source")
}

func (s *RabbitMQShovelActionTestSuite) TestRunShovelLoop_EmptyQueue() {
	actCtx := &ActionContext{Context: context.Background()}
	cfg := shovelConfig{count: 5, sourceQueue: "q", destQueue: "d"}

	source := &mockShovelSource{
		getFn: func(queue string, autoAck bool) (amqp.Delivery, bool, error) {
			return amqp.Delivery{}, false, nil // ok=false → empty queue
		},
	}

	result, err := runShovelLoop(actCtx, source, &mockShovelDest{}, cfg)

	s.Require().NoError(err)

	out := result.(map[string]any)
	s.Equal(0, out["total"])
	s.Equal(0, out["succeeded"])
	s.Equal(0, out["failed"])
}

func (s *RabbitMQShovelActionTestSuite) TestRunShovelLoop_PublishError() {
	acker := &mockShovelAcknowledger{}
	actCtx := &ActionContext{Context: context.Background()}
	cfg := shovelConfig{count: 1, sourceQueue: "q", destQueue: "d", stripHeaders: true}

	source := &mockShovelSource{
		getFn: func(queue string, autoAck bool) (amqp.Delivery, bool, error) {
			return amqp.Delivery{
				Body:         []byte("data"),
				Acknowledger: acker,
			}, true, nil
		},
	}

	dest := &mockShovelDest{
		publishFn: func(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (shovelConfirmation, error) {
			return nil, errors.New("publish failed")
		},
	}

	result, err := runShovelLoop(actCtx, source, dest, cfg)

	s.Require().NoError(err)
	s.True(acker.nackCalled)

	out := result.(map[string]any)
	s.Equal(0, out["succeeded"])
	s.Equal(1, out["failed"])
	s.Equal(1, out["total"])
	s.Contains(out["errors"].([]string)[0], "publish error")
}

func (s *RabbitMQShovelActionTestSuite) TestRunShovelLoop_BrokerNack() {
	acker := &mockShovelAcknowledger{}
	actCtx := &ActionContext{Context: context.Background()}
	cfg := shovelConfig{count: 1, sourceQueue: "q", destQueue: "d", stripHeaders: true}

	source := &mockShovelSource{
		getFn: func(queue string, autoAck bool) (amqp.Delivery, bool, error) {
			return amqp.Delivery{
				Body:         []byte("data"),
				Acknowledger: acker,
			}, true, nil
		},
	}

	dest := &mockShovelDest{
		publishFn: func(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (shovelConfirmation, error) {
			return &mockShovelConfirmation{result: false}, nil // broker nacked
		},
	}

	result, err := runShovelLoop(actCtx, source, dest, cfg)

	s.Require().NoError(err)
	s.True(acker.nackCalled)

	out := result.(map[string]any)
	s.Equal(0, out["succeeded"])
	s.Equal(1, out["failed"])
	s.Contains(out["errors"].([]string)[0], "broker nacked")
}

func (s *RabbitMQShovelActionTestSuite) TestRunShovelLoop_AckError() {
	acker := &mockShovelAcknowledger{ackErr: errors.New("ack failed")}
	actCtx := &ActionContext{Context: context.Background()}
	cfg := shovelConfig{count: 1, sourceQueue: "q", destQueue: "d", stripHeaders: true}

	source := &mockShovelSource{
		getFn: func(queue string, autoAck bool) (amqp.Delivery, bool, error) {
			return amqp.Delivery{
				Body:         []byte("data"),
				Acknowledger: acker,
			}, true, nil
		},
	}

	dest := &mockShovelDest{
		publishFn: func(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (shovelConfirmation, error) {
			return &mockShovelConfirmation{result: true}, nil
		},
	}

	result, err := runShovelLoop(actCtx, source, dest, cfg)

	s.Require().NoError(err)
	s.True(acker.ackCalled)

	out := result.(map[string]any)
	s.Equal(0, out["succeeded"])
	s.Equal(1, out["failed"])
	s.Contains(out["errors"].([]string)[0], "source ack failed")
}

func (s *RabbitMQShovelActionTestSuite) TestRunShovelLoop_SuccessWithEmitLog() {
	var logMessages []string
	acker := &mockShovelAcknowledger{}

	actCtx := &ActionContext{
		Context: context.Background(),
		EmitLog: func(msg string) { logMessages = append(logMessages, msg) },
	}
	cfg := shovelConfig{count: 2, sourceQueue: "q", destQueue: "d", stripHeaders: true}

	callCount := 0
	source := &mockShovelSource{
		getFn: func(queue string, autoAck bool) (amqp.Delivery, bool, error) {
			callCount++
			return amqp.Delivery{
				Body:         []byte(fmt.Sprintf("msg-%d", callCount)),
				Acknowledger: acker,
			}, true, nil
		},
	}

	dest := &mockShovelDest{
		publishFn: func(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (shovelConfirmation, error) {
			return &mockShovelConfirmation{result: true}, nil
		},
	}

	result, err := runShovelLoop(actCtx, source, dest, cfg)

	s.Require().NoError(err)

	out := result.(map[string]any)
	s.Equal(2, out["succeeded"])
	s.Equal(0, out["failed"])
	s.Equal(2, out["total"])
	s.Nil(out["errors"])

	s.Len(logMessages, 2)
	s.Equal("shoveled 1/2", logMessages[0])
	s.Equal("shoveled 2/2", logMessages[1])
}

func (s *RabbitMQShovelActionTestSuite) TestRunShovelLoop_MixedSuccessAndFailure() {
	actCtx := &ActionContext{Context: context.Background()}
	cfg := shovelConfig{count: 3, sourceQueue: "q", destQueue: "d", stripHeaders: true}

	callCount := 0
	source := &mockShovelSource{
		getFn: func(queue string, autoAck bool) (amqp.Delivery, bool, error) {
			callCount++
			return amqp.Delivery{
				Body:         []byte(fmt.Sprintf("msg-%d", callCount)),
				Acknowledger: &mockShovelAcknowledger{},
			}, true, nil
		},
	}

	publishCount := 0
	dest := &mockShovelDest{
		publishFn: func(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (shovelConfirmation, error) {
			publishCount++
			if publishCount == 2 {
				return nil, errors.New("publish failed")
			}
			return &mockShovelConfirmation{result: true}, nil
		},
	}

	result, err := runShovelLoop(actCtx, source, dest, cfg)

	s.Require().NoError(err)

	out := result.(map[string]any)
	s.Equal(2, out["succeeded"])
	s.Equal(1, out["failed"])
	s.Equal(3, out["total"])
	s.Len(out["errors"].([]string), 1)
}

func (s *RabbitMQShovelActionTestSuite) TestRunShovelLoop_ContextCancelledMidLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	actCtx := &ActionContext{Context: ctx}
	cfg := shovelConfig{count: 5, sourceQueue: "q", destQueue: "d", stripHeaders: true}

	callCount := 0
	source := &mockShovelSource{
		getFn: func(queue string, autoAck bool) (amqp.Delivery, bool, error) {
			callCount++
			if callCount == 2 {
				cancel() // cancel after first successful message
			}
			return amqp.Delivery{
				Body:         []byte("data"),
				Acknowledger: &mockShovelAcknowledger{},
			}, true, nil
		},
	}

	dest := &mockShovelDest{
		publishFn: func(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (shovelConfirmation, error) {
			return &mockShovelConfirmation{result: true}, nil
		},
	}

	_, err := runShovelLoop(actCtx, source, dest, cfg)

	s.Require().Error(err)
	s.ErrorIs(err, context.Canceled)
}

func (s *RabbitMQShovelActionTestSuite) TestRunShovelLoop_NoEmitLog() {
	acker := &mockShovelAcknowledger{}
	actCtx := &ActionContext{
		Context: context.Background(),
		EmitLog: nil, // no emit log function
	}
	cfg := shovelConfig{count: 1, sourceQueue: "q", destQueue: "d", stripHeaders: true}

	source := &mockShovelSource{
		getFn: func(queue string, autoAck bool) (amqp.Delivery, bool, error) {
			return amqp.Delivery{
				Body:         []byte("data"),
				Acknowledger: acker,
			}, true, nil
		},
	}

	dest := &mockShovelDest{
		publishFn: func(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (shovelConfirmation, error) {
			return &mockShovelConfirmation{result: true}, nil
		},
	}

	result, err := runShovelLoop(actCtx, source, dest, cfg)

	s.Require().NoError(err)
	out := result.(map[string]any)
	s.Equal(1, out["succeeded"])
}

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

func (m *mockShovelRawChan) PublishWithDeferredConfirm(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (*amqp.DeferredConfirmation, error) {
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

func (s *RabbitMQShovelActionTestSuite) TestOpenShovelChannelsImpl_DialSourceFails() {
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

func (s *RabbitMQShovelActionTestSuite) TestOpenShovelChannelsImpl_SourceChannelFails() {
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

func (s *RabbitMQShovelActionTestSuite) TestOpenShovelChannelsImpl_SameURL_DestChannelFails() {
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

func (s *RabbitMQShovelActionTestSuite) TestOpenShovelChannelsImpl_DifferentURL_DestDialFails() {
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

func (s *RabbitMQShovelActionTestSuite) TestOpenShovelChannelsImpl_DifferentURL_DestChannelFails() {
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

func (s *RabbitMQShovelActionTestSuite) TestOpenShovelChannelsImpl_SuccessSameURL() {
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

	// Verify the source channel is the raw channel
	s.Equal(sourceCh, srcOut)

	// Verify dest is wrapped in realShovelDest
	rd, ok := destOut.(*realShovelDest)
	s.Require().True(ok, "destOut should be *realShovelDest")
	s.Equal(destCh, rd.ch)

	// Verify nothing is closed yet
	s.False(sourceCh.closed)
	s.False(destCh.closed)
	s.False(sourceConn.closed)

	// Call cleanup and verify everything is closed
	cleanup()
	s.True(destCh.closed, "dest channel should be closed after cleanup")
	s.True(sourceCh.closed, "source channel should be closed after cleanup")
	s.True(sourceConn.closed, "source connection should be closed after cleanup")
}

func (s *RabbitMQShovelActionTestSuite) TestOpenShovelChannelsImpl_SuccessDifferentURL() {
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

	// Verify the source channel is the raw channel
	s.Equal(sourceCh, srcOut)

	// Verify dest is wrapped in realShovelDest
	rd, ok := destOut.(*realShovelDest)
	s.Require().True(ok, "destOut should be *realShovelDest")
	s.Equal(destCh, rd.ch)

	// Nothing closed yet
	s.False(sourceCh.closed)
	s.False(destCh.closed)
	s.False(sourceConn.closed)
	s.False(destConn.closed)

	// Call cleanup and verify everything is closed including the separate dest connection
	cleanup()
	s.True(destCh.closed, "dest channel should be closed after cleanup")
	s.True(destConn.closed, "dest connection should be closed after cleanup (different URL)")
	s.True(sourceCh.closed, "source channel should be closed after cleanup")
	s.True(sourceConn.closed, "source connection should be closed after cleanup")
}

func (s *RabbitMQShovelActionTestSuite) TestOpenShovelChannelsImpl_CleanupSameURLDoesNotCloseDestConn() {
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

	// When same URL, destConn == sourceConn, so Close should only be called once
	s.Equal(1, closeCount, "connection Close should be called exactly once when source and dest use the same URL")
}

func (s *RabbitMQShovelActionTestSuite) TestRealShovelDest_ConfirmDelegates() {
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

func (s *RabbitMQShovelActionTestSuite) TestRealShovelDest_ConfirmReturnsError() {
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

func (s *RabbitMQShovelActionTestSuite) TestRealShovelDest_PublishWithDeferredConfirmDelegates() {
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

func (s *RabbitMQShovelActionTestSuite) TestRealShovelDest_PublishWithDeferredConfirmReturnsError() {
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

func (s *RabbitMQShovelActionTestSuite) TestShovelDialFn_Default_DialFails() {
	_, err := shovelDialFn("amqp://localhost:59999")
	s.Require().Error(err)
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

func (s *RabbitMQShovelActionTestSuite) TestWrapShovelConn_ChannelSuccess() {
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

func (s *RabbitMQShovelActionTestSuite) TestWrapShovelConn_ChannelError() {
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

func (s *RabbitMQShovelActionTestSuite) TestWrapShovelConn_Close() {
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

func (s *RabbitMQShovelActionTestSuite) TestRealShovelConn_Channel_Success() {
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

func (s *RabbitMQShovelActionTestSuite) TestRealShovelConn_Channel_Error() {
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

func (s *RabbitMQShovelActionTestSuite) TestRealShovelConn_Close_Success() {
	conn := &realShovelConn{
		closeFn: func() error {
			return nil
		},
	}

	err := conn.Close()

	s.NoError(err)
}

func (s *RabbitMQShovelActionTestSuite) TestRealShovelConn_Close_Error() {
	conn := &realShovelConn{
		closeFn: func() error {
			return fmt.Errorf("close connection failed")
		},
	}

	err := conn.Close()

	s.Require().Error(err)
	s.Equal("close connection failed", err.Error())
}

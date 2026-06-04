package action

import (
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

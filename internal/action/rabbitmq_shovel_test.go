package action

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/suite"
)

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

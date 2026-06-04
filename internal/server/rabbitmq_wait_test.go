package server

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"testing/synctest"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/suite"
)

type RabbitMQWaitTestSuite struct {
	suite.Suite
	logger *slog.Logger
}

func TestRabbitMQWait(t *testing.T) {
	suite.Run(t, new(RabbitMQWaitTestSuite))
}

func (s *RabbitMQWaitTestSuite) SetupTest() {
	s.logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func (s *RabbitMQWaitTestSuite) TestExtractDotField_TopLevel() {
	m := map[string]any{"order_id": "abc-123"}
	s.Equal("abc-123", extractDotField(m, "order_id"))
}

func (s *RabbitMQWaitTestSuite) TestExtractDotField_Nested() {
	m := map[string]any{
		"data": map[string]any{
			"order": map[string]any{
				"id": 42,
			},
		},
	}
	s.Equal("42", extractDotField(m, "data.order.id"))
}

func (s *RabbitMQWaitTestSuite) TestExtractDotField_Missing() {
	m := map[string]any{"foo": "bar"}
	s.Equal("", extractDotField(m, "missing"))
}

func (s *RabbitMQWaitTestSuite) TestExtractDotField_MissingNested() {
	m := map[string]any{"foo": "bar"}
	s.Equal("", extractDotField(m, "foo.bar.baz"))
}

func (s *RabbitMQWaitTestSuite) TestExtractDotField_EmptyMap() {
	m := map[string]any{}
	s.Equal("", extractDotField(m, "anything"))
}

func (s *RabbitMQWaitTestSuite) TestExtractDotField_StringValue() {
	m := map[string]any{"status": "confirmed"}
	s.Equal("confirmed", extractDotField(m, "status"))
}

func (s *RabbitMQWaitTestSuite) TestRabbitMQWaitManager_ConsumeLoop_ChannelClosed() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)

		deliveries := make(chan amqp.Delivery)
		close(deliveries)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		qc := &queueConsumer{}
		exited := false
		go func() {
			mgr.consumeLoop(ctx, qc, deliveries)
			exited = true
		}()

		synctest.Wait()
		s.True(exited)
	})
}

func (s *RabbitMQWaitTestSuite) TestRabbitMQWaitManager_ConsumeLoop_ContextCancel() {
	synctest.Test(s.T(), func(t *testing.T) {
		mgr := NewRabbitMQWaitManager(s.logger)

		deliveries := make(chan amqp.Delivery)

		ctx, cancel := context.WithCancel(context.Background())

		qc := &queueConsumer{}
		exited := false
		go func() {
			mgr.consumeLoop(ctx, qc, deliveries)
			exited = true
		}()

		cancel()
		synctest.Wait()
		s.True(exited)
	})
}

func (s *RabbitMQWaitTestSuite) TestRabbitMQWaitManager_RemoveWaiter_UnknownURL() {
	mgr := NewRabbitMQWaitManager(s.logger)

	w := &rmqWaiter{ch: make(chan map[string]any, 1)}
	mgr.removeWaiter("amqp://unknown-host:5672", "some-queue", w)

	s.Len(mgr.connections, 0)
}


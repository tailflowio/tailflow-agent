package server

import (
	"context"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/tailflow/tailflow/internal/parser"
)

type RabbitMQConsumer struct {
	config *parser.RabbitMQTrigger
	logger *slog.Logger
	dial   amqpDialer
	conn   amqpConn
	ch     amqpChan
}

func NewRabbitMQConsumer(config *parser.RabbitMQTrigger, logger *slog.Logger) *RabbitMQConsumer {
	return &RabbitMQConsumer{
		config: config,
		logger: logger,
	}
}

func (c *RabbitMQConsumer) Start(ctx context.Context, onMessage func(triggerData map[string]any, ackFn func(bool))) error {
	msgs, err := c.openChannel()
	if err != nil {
		return err
	}

	c.logger.Info("rabbitmq consumer started", "queue", c.config.Queue, "ack_on_success", c.config.AckOnSuccess)

	go c.consumeLoop(ctx, msgs, onMessage)

	return nil
}

func (c *RabbitMQConsumer) openChannel() (<-chan amqp.Delivery, error) {
	dial := c.dial
	if dial == nil {
		dial = realAMQPDial
	}

	conn, err := dial(c.config.URL)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq dial: %w", err)
	}

	c.conn = conn

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq channel: %w", err)
	}

	c.ch = ch

	if c.config.Prefetch > 0 {
		err = ch.Qos(c.config.Prefetch, 0, false)
		if err != nil {
			_ = ch.Close()
			_ = conn.Close()

			return nil, fmt.Errorf("rabbitmq qos: %w", err)
		}
	}

	msgs, err := ch.Consume(c.config.Queue, "", false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()

		return nil, fmt.Errorf("rabbitmq consume: %w", err)
	}

	return msgs, nil
}

func (c *RabbitMQConsumer) consumeLoop(ctx context.Context, msgs <-chan amqp.Delivery, onMessage func(map[string]any, func(bool))) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				c.logger.Info("rabbitmq delivery channel closed")
				return
			}

			c.handleMessage(msg, onMessage)
		}
	}
}

func (c *RabbitMQConsumer) handleMessage(msg amqp.Delivery, onMessage func(map[string]any, func(bool))) {
	triggerData := map[string]any{
		"body":         string(msg.Body),
		"content_type": msg.ContentType,
		"routing_key":  msg.RoutingKey,
		"message_id":   msg.MessageId,
		"headers":      msg.Headers,
		"queue":        c.config.Queue,
	}

	if c.config.AckOnSuccess {
		ackFn := func(success bool) {
			if !success {
				err := msg.Nack(false, true)
				if err != nil {
					c.logger.Error("rabbitmq nack failed", "error", err)
				}

				return
			}

			err := msg.Ack(false)
			if err != nil {
				c.logger.Error("rabbitmq ack failed", "error", err)
			}
		}
		onMessage(triggerData, ackFn)

		return
	}

	err := msg.Ack(false)
	if err != nil {
		c.logger.Error("rabbitmq ack failed", "error", err)
	}

	onMessage(triggerData, nil)
}

func (c *RabbitMQConsumer) Stop() {
	if c.ch != nil {
		_ = c.ch.Close()
	}

	if c.conn != nil {
		_ = c.conn.Close()
	}

	c.logger.Info("rabbitmq consumer stopped")
}

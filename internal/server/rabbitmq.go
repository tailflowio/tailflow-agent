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
	conn   *amqp.Connection
	ch     *amqp.Channel
}

func NewRabbitMQConsumer(config *parser.RabbitMQTrigger, logger *slog.Logger) *RabbitMQConsumer {
	return &RabbitMQConsumer{
		config: config,
		logger: logger,
	}
}

func (c *RabbitMQConsumer) Start(ctx context.Context, onMessage func(triggerData map[string]any, ackFn func(bool))) error {
	conn, err := amqp.Dial(c.config.URL)
	if err != nil {
		return fmt.Errorf("rabbitmq dial: %w", err)
	}
	c.conn = conn

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("rabbitmq channel: %w", err)
	}
	c.ch = ch

	if c.config.Prefetch > 0 {
		err = ch.Qos(c.config.Prefetch, 0, false)
		if err != nil {
			ch.Close()
			conn.Close()
			return fmt.Errorf("rabbitmq qos: %w", err)
		}
	}

	msgs, err := ch.Consume(
		c.config.Queue,
		"",    // consumer tag (auto-generated)
		false, // auto-ack: always false, we handle ack manually
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("rabbitmq consume: %w", err)
	}

	c.logger.Info("rabbitmq consumer started", "queue", c.config.Queue, "ack_on_success", c.config.AckOnSuccess)

	go func() {
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
	}()

	return nil
}

func (c *RabbitMQConsumer) handleMessage(msg amqp.Delivery, onMessage func(map[string]any, func(bool))) {
	triggerData := map[string]any{
		"body":         string(msg.Body),
		"content_type": msg.ContentType,
		"routing_key":  msg.RoutingKey,
		"message_id":   msg.MessageId,
		"headers":      amqp.Table(msg.Headers),
		"queue":        c.config.Queue,
	}

	if c.config.AckOnSuccess {
		// Defer ack/nack to after workflow completion.
		ackFn := func(success bool) {
			if success {
				err := msg.Ack(false)
				if err != nil {
					c.logger.Error("rabbitmq ack failed", "error", err)
				}
			} else {
				err := msg.Nack(false, true)
				if err != nil {
					c.logger.Error("rabbitmq nack failed", "error", err)
				}
			}
		}
		onMessage(triggerData, ackFn)
	} else {
		// Ack immediately, then dispatch.
		err := msg.Ack(false)
		if err != nil {
			c.logger.Error("rabbitmq ack failed", "error", err)
		}
		onMessage(triggerData, nil)
	}
}

func (c *RabbitMQConsumer) Stop() {
	if c.ch != nil {
		c.ch.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
	c.logger.Info("rabbitmq consumer stopped")
}

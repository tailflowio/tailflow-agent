package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// rmqWaiter is a single pending wait.rabbitmq registration.
type rmqWaiter struct {
	matchField string
	matchValue string
	ch         chan map[string]any
	ctx        context.Context
}

// queueConsumer manages a single AMQP consumer on one queue.
type queueConsumer struct {
	mu       sync.Mutex
	waiters  []*rmqWaiter
	amqpChan *amqp.Channel
	cancel   context.CancelFunc
}

// managedConnection holds an AMQP connection and its per-queue consumers.
type managedConnection struct {
	conn      *amqp.Connection
	consumers map[string]*queueConsumer // key: queue name
}

// RabbitMQWaitManager is a shared pool of AMQP connections and consumers.
// A single consumer is created per queue; incoming messages are routed to
// matching waiters registered via Register.
type RabbitMQWaitManager struct {
	mu          sync.Mutex
	connections map[string]*managedConnection // key: AMQP URL
	logger      *slog.Logger
}

// NewRabbitMQWaitManager creates a new manager.
func NewRabbitMQWaitManager(logger *slog.Logger) *RabbitMQWaitManager {
	return &RabbitMQWaitManager{
		connections: make(map[string]*managedConnection),
		logger:      logger,
	}
}

// Register adds a waiter for the given queue. It lazily creates the AMQP
// connection and consumer goroutine when needed.
// Returns a channel that will receive the matched message, and a cleanup func.
func (m *RabbitMQWaitManager) Register(url, queue, matchField, matchValue string, ctx context.Context) (<-chan map[string]any, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	w := &rmqWaiter{
		matchField: matchField,
		matchValue: matchValue,
		ch:         make(chan map[string]any, 1),
		ctx:        ctx,
	}

	// Ensure connection exists
	mc, ok := m.connections[url]
	if !ok {
		conn, err := amqp.Dial(url)
		if err != nil {
			m.logger.Error("rabbitmq_wait: dial failed", "url", url, "error", err)
			close(w.ch)
			return w.ch, func() {}
		}

		mc = &managedConnection{
			conn:      conn,
			consumers: make(map[string]*queueConsumer),
		}
		m.connections[url] = mc
	}

	// Ensure consumer exists for this queue
	qc, ok := mc.consumers[queue]
	if !ok {
		ch, err := mc.conn.Channel()
		if err != nil {
			m.logger.Error("rabbitmq_wait: channel failed", "queue", queue, "error", err)
			close(w.ch)
			return w.ch, func() {}
		}

		if err := ch.Qos(1, 0, false); err != nil {
			m.logger.Error("rabbitmq_wait: qos failed", "queue", queue, "error", err)
			ch.Close()
			close(w.ch)
			return w.ch, func() {}
		}

		deliveries, err := ch.Consume(
			queue,
			"",    // consumer tag (auto-generated)
			false, // auto-ack
			false, // exclusive
			false, // no-local
			false, // no-wait
			nil,
		)
		if err != nil {
			m.logger.Error("rabbitmq_wait: consume failed", "queue", queue, "error", err)
			ch.Close()
			close(w.ch)
			return w.ch, func() {}
		}

		consumerCtx, cancel := context.WithCancel(context.Background())
		qc = &queueConsumer{
			amqpChan: ch,
			cancel:   cancel,
		}
		mc.consumers[queue] = qc

		go m.consumeLoop(consumerCtx, qc, deliveries)
	}

	qc.mu.Lock()
	qc.waiters = append(qc.waiters, w)
	qc.mu.Unlock()

	cleanup := func() {
		m.removeWaiter(url, queue, w)
	}

	// Auto-cleanup on context cancellation
	if ctx != nil {
		go func() {
			<-ctx.Done()
			cleanup()
		}()
	}

	return w.ch, cleanup
}

// removeWaiter removes a waiter and tears down the consumer/connection when empty.
func (m *RabbitMQWaitManager) removeWaiter(url, queue string, w *rmqWaiter) {
	m.mu.Lock()
	defer m.mu.Unlock()

	mc, ok := m.connections[url]
	if !ok {
		return
	}

	qc, ok := mc.consumers[queue]
	if !ok {
		return
	}

	qc.mu.Lock()
	for i, existing := range qc.waiters {
		if existing == w {
			qc.waiters = append(qc.waiters[:i], qc.waiters[i+1:]...)
			break
		}
	}
	empty := len(qc.waiters) == 0
	qc.mu.Unlock()

	if empty {
		qc.cancel()
		qc.amqpChan.Close()
		delete(mc.consumers, queue)

		if len(mc.consumers) == 0 {
			mc.conn.Close()
			delete(m.connections, url)
		}
	}
}

// consumeLoop reads messages from the AMQP channel and routes them to waiters.
func (m *RabbitMQWaitManager) consumeLoop(ctx context.Context, qc *queueConsumer, deliveries <-chan amqp.Delivery) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-deliveries:
			if !ok {
				return
			}

			if !m.routeMessage(qc, msg) {
				// No matching waiter — nack + requeue and throttle to avoid hot-loop
				_ = msg.Nack(false, true)

				select {
				case <-ctx.Done():
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
	}
}

// routeMessage tries to deliver a message to a matching waiter.
// Returns true if delivered (message is acked), false otherwise.
func (m *RabbitMQWaitManager) routeMessage(qc *queueConsumer, msg amqp.Delivery) bool {
	var body map[string]any
	if err := json.Unmarshal(msg.Body, &body); err != nil {
		// Non-JSON body: build a simple map
		body = map[string]any{"raw": string(msg.Body)}
	}

	qc.mu.Lock()
	defer qc.mu.Unlock()

	for i, w := range qc.waiters {
		// Skip waiters whose context is already done
		if w.ctx != nil && w.ctx.Err() != nil {
			continue
		}

		if w.matchField == "" {
			// No match field → first waiter takes the message
			return m.deliverToWaiter(qc, i, w, msg, body)
		}

		// Match on a specific field using dot-notation
		val := extractDotField(body, w.matchField)
		if val == w.matchValue {
			return m.deliverToWaiter(qc, i, w, msg, body)
		}
	}

	return false
}

// deliverToWaiter sends the parsed body to the waiter, acks the message,
// and removes the waiter from the list.
func (m *RabbitMQWaitManager) deliverToWaiter(qc *queueConsumer, idx int, w *rmqWaiter, msg amqp.Delivery, body map[string]any) bool {
	result := map[string]any{
		"body":         body,
		"content_type": msg.ContentType,
		"routing_key":  msg.RoutingKey,
		"message_id":   msg.MessageId,
		"headers":      amqpTableToMap(msg.Headers),
	}

	select {
	case w.ch <- result:
	default:
		// Channel full — should not happen with buffer=1, skip
		return false
	}

	_ = msg.Ack(false)

	// Remove waiter (order preserved)
	qc.waiters = append(qc.waiters[:idx], qc.waiters[idx+1:]...)

	return true
}

// Close shuts down all consumers and connections.
func (m *RabbitMQWaitManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for url, mc := range m.connections {
		for queue, qc := range mc.consumers {
			qc.cancel()
			qc.amqpChan.Close()
			delete(mc.consumers, queue)
		}

		mc.conn.Close()
		delete(m.connections, url)
	}
}

// extractDotField navigates a nested map using dot-notation (e.g. "data.order_id")
// and returns the value as a string. Returns "" if not found.
func extractDotField(m map[string]any, field string) string {
	parts := strings.Split(field, ".")
	var current any = m

	for _, part := range parts {
		cm, ok := current.(map[string]any)
		if !ok {
			return ""
		}

		current, ok = cm[part]
		if !ok {
			return ""
		}
	}

	return fmt.Sprintf("%v", current)
}

// amqpTableToMap converts amqp.Table to a plain map[string]any.
func amqpTableToMap(t amqp.Table) map[string]any {
	if t == nil {
		return nil
	}

	out := make(map[string]any, len(t))
	for k, v := range t {
		out[k] = v
	}

	return out
}

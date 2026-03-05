package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

var requeueThrottleDuration = 100 * time.Millisecond

type rmqWaiter struct {
	matchField string
	matchValue string
	ch         chan map[string]any
	ctx        context.Context
}

type queueConsumer struct {
	mu       sync.Mutex
	waiters  []*rmqWaiter
	amqpChan amqpChan
	cancel   context.CancelFunc
}

type managedConnection struct {
	conn      amqpConn
	consumers map[string]*queueConsumer
}

type RabbitMQWaitManager struct {
	mu          sync.Mutex
	connections map[string]*managedConnection
	logger      *slog.Logger
	dial        amqpDialer
}

func NewRabbitMQWaitManager(logger *slog.Logger) *RabbitMQWaitManager {
	return &RabbitMQWaitManager{
		connections: make(map[string]*managedConnection),
		logger:      logger,
	}
}

func (m *RabbitMQWaitManager) Register(url, queue, matchField, matchValue string, ctx context.Context) (<-chan map[string]any, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	w := &rmqWaiter{
		matchField: matchField,
		matchValue: matchValue,
		ch:         make(chan map[string]any, 1),
		ctx:        ctx,
	}

	mc, err := m.getOrCreateConnection(url)
	if err != nil {
		close(w.ch)
		return w.ch, func() {}
	}

	qc, err := m.getOrCreateConsumer(mc, queue) //nolint:contextcheck
	if err != nil {
		close(w.ch)
		return w.ch, func() {}
	}

	qc.mu.Lock()
	qc.waiters = append(qc.waiters, w)
	qc.mu.Unlock()

	cleanup := func() {
		m.removeWaiter(url, queue, w)
	}

	if ctx != nil {
		go func() {
			<-ctx.Done()
			cleanup()
		}()
	}

	return w.ch, cleanup
}

func (m *RabbitMQWaitManager) getOrCreateConnection(url string) (*managedConnection, error) {
	mc, ok := m.connections[url]
	if ok {
		return mc, nil
	}

	dial := m.dial
	if dial == nil {
		dial = realAMQPDial
	}

	conn, err := dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	mc = &managedConnection{
		conn:      conn,
		consumers: make(map[string]*queueConsumer),
	}
	m.connections[url] = mc

	return mc, nil
}

func (m *RabbitMQWaitManager) getOrCreateConsumer(mc *managedConnection, queue string) (*queueConsumer, error) {
	qc, ok := mc.consumers[queue]
	if ok {
		return qc, nil
	}

	ch, err := mc.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("channel: %w", err)
	}

	err = ch.Qos(1, 0, false)
	if err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("qos: %w", err)
	}

	deliveries, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("consume: %w", err)
	}

	consumerCtx, cancel := context.WithCancel(context.Background())
	qc = &queueConsumer{
		amqpChan: ch,
		cancel:   cancel,
	}
	mc.consumers[queue] = qc

	go m.consumeLoop(consumerCtx, qc, deliveries)

	return qc, nil
}

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

	if !empty {
		return
	}

	qc.cancel()

	_ = qc.amqpChan.Close()

	delete(mc.consumers, queue)

	if len(mc.consumers) == 0 {
		_ = mc.conn.Close()

		delete(m.connections, url)
	}
}

func (m *RabbitMQWaitManager) consumeLoop(ctx context.Context, qc *queueConsumer, deliveries <-chan amqp.Delivery) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-deliveries:
			if !ok {
				return
			}

			if m.routeMessage(qc, msg) {
				continue
			}

			nackErr := msg.Nack(false, true)
			if nackErr != nil {
				m.logger.WarnContext(ctx, "failed to nack unmatched message", "error", nackErr)
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(requeueThrottleDuration):
			}
		}
	}
}

func (m *RabbitMQWaitManager) routeMessage(qc *queueConsumer, msg amqp.Delivery) bool {
	var body map[string]any

	err := json.Unmarshal(msg.Body, &body)
	if err != nil {
		body = map[string]any{"raw": string(msg.Body)}
	}

	qc.mu.Lock()
	defer qc.mu.Unlock()

	for i, w := range qc.waiters {
		if w.ctx != nil && w.ctx.Err() != nil {
			continue
		}

		if w.matchField == "" {
			return m.deliverToWaiter(qc, i, w, msg, body)
		}

		val := extractDotField(body, w.matchField)
		if val == w.matchValue {
			return m.deliverToWaiter(qc, i, w, msg, body)
		}
	}

	return false
}

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
		return false
	}

	ackErr := msg.Ack(false)
	if ackErr != nil {
		m.logger.Warn("failed to ack delivered message", "error", ackErr)
	}

	qc.waiters = append(qc.waiters[:idx], qc.waiters[idx+1:]...)

	return true
}

func (m *RabbitMQWaitManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for url, mc := range m.connections {
		for queue, qc := range mc.consumers {
			qc.cancel()

			_ = qc.amqpChan.Close()

			delete(mc.consumers, queue)
		}

		_ = mc.conn.Close()

		delete(m.connections, url)
	}
}

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

func amqpTableToMap(t amqp.Table) map[string]any {
	if t == nil {
		return nil
	}

	out := make(map[string]any, len(t))
	maps.Copy(out, t)

	return out
}

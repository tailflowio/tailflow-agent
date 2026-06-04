package action

import (
	"errors"
	"fmt"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
)

type shovelSourceChan interface {
	Get(queue string, autoAck bool) (amqp.Delivery, bool, error)
}

type shovelConfirmation interface {
	Wait() bool
}

type shovelDestChan interface {
	Confirm(noWait bool) error
	PublishWithDeferredConfirm(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (shovelConfirmation, error)
}

type shovelConnector interface {
	Channel() (shovelRawChan, error)
	Close() error
}

type shovelRawChan interface {
	Get(queue string, autoAck bool) (amqp.Delivery, bool, error)
	Confirm(noWait bool) error
	PublishWithDeferredConfirm(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (*amqp.DeferredConfirmation, error)
	Close() error
}

type realShovelConn struct {
	channelFn func() (shovelRawChan, error)
	closeFn   func() error
}

func (r *realShovelConn) Channel() (shovelRawChan, error) { return r.channelFn() }
func (r *realShovelConn) Close() error                    { return r.closeFn() }

type shovelAMQPConn interface {
	Channel() (*amqp.Channel, error)
	Close() error
}

// shovelRawDialFn is the low-level AMQP dial — replaced in tests to avoid hitting a real broker.
var shovelRawDialFn = func(url string) (shovelAMQPConn, error) {
	return amqp.Dial(url)
}

var shovelDialFn = func(url string) (shovelConnector, error) {
	c, err := shovelRawDialFn(url)
	if err != nil {
		return nil, err
	}

	return wrapShovelConn(c), nil
}

func wrapShovelConn(c shovelAMQPConn) *realShovelConn {
	return &realShovelConn{
		channelFn: func() (shovelRawChan, error) {
			return c.Channel()
		},
		closeFn: c.Close,
	}
}

type realShovelDest struct{ ch shovelRawChan }

func (d *realShovelDest) Confirm(noWait bool) error { return d.ch.Confirm(noWait) }
func (d *realShovelDest) PublishWithDeferredConfirm(
	exchange, key string, mandatory, immediate bool, msg amqp.Publishing,
) (shovelConfirmation, error) {
	return d.ch.PublishWithDeferredConfirm(exchange, key, mandatory, immediate, msg)
}

var openShovelChannelsFn = openShovelChannelsImpl

type RabbitMQShovelAction struct{}

func NewRabbitMQShovelAction() Action { return &RabbitMQShovelAction{} }

func (a *RabbitMQShovelAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["url"]
	if !ok {
		return errors.New("rabbitmq.shovel requires 'url' in config")
	}

	_, ok = ctx.Config["source_queue"]
	if !ok {
		return errors.New("rabbitmq.shovel requires 'source_queue' in config")
	}

	_, ok = ctx.Config["dest_queue"]
	if !ok {
		return errors.New("rabbitmq.shovel requires 'dest_queue' in config")
	}

	return nil
}

func (a *RabbitMQShovelAction) Execute(ctx *ActionContext) (any, error) {
	cfg := parseShovelConfig(ctx)

	sourceCh, destCh, cleanup, err := openShovelChannelsFn(cfg)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	err = destCh.Confirm(false)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq.shovel: enable confirms: %w", err)
	}

	return runShovelLoop(ctx, sourceCh, destCh, cfg)
}

type shovelConfig struct {
	sourceURL    string
	destURL      string
	sourceQueue  string
	destQueue    string
	count        int
	stripHeaders bool
}

func parseShovelConfig(ctx *ActionContext) shovelConfig {
	cfg := shovelConfig{
		sourceURL:    fmt.Sprintf("%v", ctx.Config["url"]),
		sourceQueue:  fmt.Sprintf("%v", ctx.Config["source_queue"]),
		destQueue:    fmt.Sprintf("%v", ctx.Config["dest_queue"]),
		count:        10,
		stripHeaders: true,
	}

	cfg.destURL = cfg.sourceURL

	v, ok := ctx.Config["dest_url"]
	if ok {
		cfg.destURL = fmt.Sprintf("%v", v)
	}

	v, ok = ctx.Config["count"]
	if ok {
		n, intOK := toInt(v)
		if intOK {
			cfg.count = n
		}
	}

	v, ok = ctx.Config["strip_headers"]
	if ok {
		switch val := v.(type) {
		case bool:
			cfg.stripHeaders = val
		case string:
			cfg.stripHeaders = !strings.EqualFold(val, "false")
		}
	}

	return cfg
}

func openShovelChannelsImpl(cfg shovelConfig) (shovelSourceChan, shovelDestChan, func(), error) {
	sourceConn, err := shovelDialFn(cfg.sourceURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("rabbitmq.shovel: dial source: %w", err)
	}

	sourceCh, err := sourceConn.Channel()
	if err != nil {
		_ = sourceConn.Close()
		return nil, nil, nil, fmt.Errorf("rabbitmq.shovel: source channel: %w", err)
	}

	destConn := sourceConn
	if cfg.destURL != cfg.sourceURL {
		destConn, err = shovelDialFn(cfg.destURL)
		if err != nil {
			_ = sourceCh.Close()
			_ = sourceConn.Close()

			return nil, nil, nil, fmt.Errorf("rabbitmq.shovel: dial dest: %w", err)
		}
	}

	destCh, err := destConn.Channel()
	if err != nil {
		if destConn != sourceConn {
			_ = destConn.Close()
		}

		_ = sourceCh.Close()
		_ = sourceConn.Close()

		return nil, nil, nil, fmt.Errorf("rabbitmq.shovel: dest channel: %w", err)
	}

	cleanup := func() {
		_ = destCh.Close()

		if destConn != sourceConn {
			_ = destConn.Close()
		}

		_ = sourceCh.Close()
		_ = sourceConn.Close()
	}

	return sourceCh, &realShovelDest{ch: destCh}, cleanup, nil
}

func runShovelLoop(
	ctx *ActionContext,
	sourceCh shovelSourceChan,
	destCh shovelDestChan,
	cfg shovelConfig,
) (any, error) {
	var succeeded, failed int
	var errs []string

	for i := range cfg.count {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		msg, ok, err := sourceCh.Get(cfg.sourceQueue, false)
		if err != nil {
			return nil, fmt.Errorf("rabbitmq.shovel: get from source: %w", err)
		}

		if !ok {
			break
		}

		transferred := processMessage(ctx, destCh, msg, cfg, i, &errs)
		if transferred {
			succeeded++
		} else {
			failed++
		}

		if ctx.EmitLog != nil {
			ctx.EmitLog(fmt.Sprintf("shoveled %d/%d", succeeded+failed, cfg.count))
		}
	}

	return buildShovelOutput(succeeded, failed, errs), nil
}

func processMessage(
	_ *ActionContext,
	destCh shovelDestChan,
	msg amqp.Delivery,
	cfg shovelConfig,
	idx int,
	errs *[]string,
) bool {
	pub := buildPublishing(msg, cfg.stripHeaders)

	confirmation, err := destCh.PublishWithDeferredConfirm("", cfg.destQueue, false, false, pub)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("msg %d: publish error: %v", idx+1, err))
		nackMessage(msg, idx, errs)

		return false
	}

	if !confirmation.Wait() {
		*errs = append(*errs, fmt.Sprintf("msg %d: broker nacked", idx+1))
		nackMessage(msg, idx, errs)

		return false
	}

	err = msg.Ack(false)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("msg %d: source ack failed: %v", idx+1, err))

		return false
	}

	return true
}

func nackMessage(msg amqp.Delivery, idx int, errs *[]string) {
	nackErr := msg.Nack(false, true)
	if nackErr != nil {
		*errs = append(*errs, fmt.Sprintf("msg %d: nack failed: %v", idx+1, nackErr))
	}
}

func buildShovelOutput(succeeded, failed int, errs []string) map[string]any {
	output := map[string]any{
		"total":     succeeded + failed,
		"succeeded": succeeded,
		"failed":    failed,
	}

	if len(errs) > 0 {
		output["errors"] = errs
	}

	return output
}

func buildPublishing(msg amqp.Delivery, stripHeaders bool) amqp.Publishing {
	pub := amqp.Publishing{
		Body:         msg.Body,
		ContentType:  msg.ContentType,
		DeliveryMode: amqp.Persistent,
	}

	if !stripHeaders {
		pub.Headers = msg.Headers
		pub.CorrelationId = msg.CorrelationId
		pub.ReplyTo = msg.ReplyTo
		pub.MessageId = msg.MessageId
		pub.Type = msg.Type
		pub.AppId = msg.AppId
	}

	return pub
}

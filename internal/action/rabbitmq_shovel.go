package action

import (
	"errors"
	"fmt"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitMQShovelAction struct{}

func NewRabbitMQShovelAction() Action { return &RabbitMQShovelAction{} }

func (a *RabbitMQShovelAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["url"]; !ok {
		return errors.New("rabbitmq.shovel requires 'url' in config")
	}

	if _, ok := ctx.Config["source_queue"]; !ok {
		return errors.New("rabbitmq.shovel requires 'source_queue' in config")
	}

	if _, ok := ctx.Config["dest_queue"]; !ok {
		return errors.New("rabbitmq.shovel requires 'dest_queue' in config")
	}

	return nil
}

func (a *RabbitMQShovelAction) Execute(ctx *ActionContext) (any, error) {
	cfg := parseShovelConfig(ctx)

	sourceCh, destCh, cleanup, err := openShovelChannels(cfg)
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

	if v, ok := ctx.Config["dest_url"]; ok {
		cfg.destURL = fmt.Sprintf("%v", v)
	}

	if v, ok := ctx.Config["count"]; ok {
		if n, ok := toInt(v); ok {
			cfg.count = n
		}
	}

	if v, ok := ctx.Config["strip_headers"]; ok {
		switch val := v.(type) {
		case bool:
			cfg.stripHeaders = val
		case string:
			cfg.stripHeaders = !strings.EqualFold(val, "false")
		}
	}

	return cfg
}

func openShovelChannels(cfg shovelConfig) (*amqp.Channel, *amqp.Channel, func(), error) {
	sourceConn, err := amqp.Dial(cfg.sourceURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("rabbitmq.shovel: dial source: %w", err)
	}

	sourceCh, err := sourceConn.Channel()
	if err != nil {
		sourceConn.Close()
		return nil, nil, nil, fmt.Errorf("rabbitmq.shovel: source channel: %w", err)
	}

	destConn := sourceConn
	if cfg.destURL != cfg.sourceURL {
		destConn, err = amqp.Dial(cfg.destURL)
		if err != nil {
			sourceCh.Close()
			sourceConn.Close()
			return nil, nil, nil, fmt.Errorf("rabbitmq.shovel: dial dest: %w", err)
		}
	}

	destCh, err := destConn.Channel()
	if err != nil {
		if destConn != sourceConn {
			destConn.Close()
		}

		sourceCh.Close()
		sourceConn.Close()

		return nil, nil, nil, fmt.Errorf("rabbitmq.shovel: dest channel: %w", err)
	}

	cleanup := func() {
		destCh.Close()

		if destConn != sourceConn {
			destConn.Close()
		}

		sourceCh.Close()
		sourceConn.Close()
	}

	return sourceCh, destCh, cleanup, nil
}

func runShovelLoop(
	ctx *ActionContext,
	sourceCh *amqp.Channel,
	destCh *amqp.Channel,
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

		pub := buildPublishing(msg, cfg.stripHeaders)

		confirmation, err := destCh.PublishWithDeferredConfirm("", cfg.destQueue, false, false, pub)
		if err != nil {
			errs = append(errs, fmt.Sprintf("msg %d: publish error: %v", i+1, err))
			_ = msg.Nack(false, true)
			failed++

			continue
		}

		if !confirmation.Wait() {
			errs = append(errs, fmt.Sprintf("msg %d: broker nacked", i+1))
			_ = msg.Nack(false, true)
			failed++

			continue
		}

		err = msg.Ack(false)
		if err != nil {
			errs = append(errs, fmt.Sprintf("msg %d: source ack failed: %v", i+1, err))
			failed++

			continue
		}

		succeeded++

		if ctx.EmitLog != nil {
			ctx.EmitLog(fmt.Sprintf("shoveled %d/%d", succeeded+failed, cfg.count))
		}
	}

	output := map[string]any{
		"total":     succeeded + failed,
		"succeeded": succeeded,
		"failed":    failed,
	}

	if len(errs) > 0 {
		output["errors"] = errs
	}

	return output, nil
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

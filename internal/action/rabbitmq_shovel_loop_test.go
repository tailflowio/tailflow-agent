package action

import (
	"context"
	"errors"
	"fmt"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/suite"
)

type RabbitMQShovelLoopTestSuite struct {
	suite.Suite
}

func TestRabbitMQShovelLoop(t *testing.T) {
	suite.Run(t, new(RabbitMQShovelLoopTestSuite))
}

func (s *RabbitMQShovelLoopTestSuite) SetupTest() {}

func (s *RabbitMQShovelLoopTestSuite) TestRunShovelLoop_ContextCancelled() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	actCtx := &ActionContext{Context: ctx}
	cfg := shovelConfig{count: 5, sourceQueue: "q", destQueue: "d"}

	_, err := runShovelLoop(actCtx, &mockShovelSource{}, &mockShovelDest{}, cfg)

	s.Require().Error(err)
	s.ErrorIs(err, context.Canceled)
}

func (s *RabbitMQShovelLoopTestSuite) TestRunShovelLoop_GetError() {
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

func (s *RabbitMQShovelLoopTestSuite) TestRunShovelLoop_EmptyQueue() {
	actCtx := &ActionContext{Context: context.Background()}
	cfg := shovelConfig{count: 5, sourceQueue: "q", destQueue: "d"}

	source := &mockShovelSource{
		getFn: func(queue string, autoAck bool) (amqp.Delivery, bool, error) {
			return amqp.Delivery{}, false, nil
		},
	}

	result, err := runShovelLoop(actCtx, source, &mockShovelDest{}, cfg)

	s.Require().NoError(err)

	out := result.(map[string]any)
	s.Equal(0, out["total"])
	s.Equal(0, out["succeeded"])
	s.Equal(0, out["failed"])
}

func (s *RabbitMQShovelLoopTestSuite) TestRunShovelLoop_PublishError() {
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

func (s *RabbitMQShovelLoopTestSuite) TestRunShovelLoop_BrokerNack() {
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
			return &mockShovelConfirmation{result: false}, nil
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

func (s *RabbitMQShovelLoopTestSuite) TestRunShovelLoop_AckError() {
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

func (s *RabbitMQShovelLoopTestSuite) TestRunShovelLoop_SuccessWithEmitLog() {
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

func (s *RabbitMQShovelLoopTestSuite) TestRunShovelLoop_MixedSuccessAndFailure() {
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

func (s *RabbitMQShovelLoopTestSuite) TestRunShovelLoop_ContextCancelledMidLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	actCtx := &ActionContext{Context: ctx}
	cfg := shovelConfig{count: 5, sourceQueue: "q", destQueue: "d", stripHeaders: true}

	callCount := 0
	source := &mockShovelSource{
		getFn: func(queue string, autoAck bool) (amqp.Delivery, bool, error) {
			callCount++
			if callCount == 2 {
				cancel()
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

func (s *RabbitMQShovelLoopTestSuite) TestRunShovelLoop_NoEmitLog() {
	acker := &mockShovelAcknowledger{}
	actCtx := &ActionContext{
		Context: context.Background(),
		EmitLog: nil,
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

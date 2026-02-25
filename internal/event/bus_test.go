package event

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type BusTestSuite struct {
	suite.Suite
}

func TestBus(t *testing.T) {
	suite.Run(t, new(BusTestSuite))
}

func (s *BusTestSuite) TestPublishSubscribe() {
	bus := NewBus()
	defer bus.Close()

	ch := bus.Subscribe(10)

	e := NewEvent(StepStarted, "exec-1", "step1", "starting step1")
	bus.Publish(e)

	select {
	case received := <-ch:
		s.Equal(StepStarted, received.Type)
		s.Equal("exec-1", received.ExecutionID)
		s.Equal("step1", received.StepID)
		s.Equal("starting step1", received.Message)
	case <-time.After(time.Second):
		s.T().Fatal("timeout waiting for event")
	}
}

func (s *BusTestSuite) TestMultipleSubscribers() {
	bus := NewBus()
	defer bus.Close()

	ch1 := bus.Subscribe(10)
	ch2 := bus.Subscribe(10)

	e := NewEvent(WorkflowStarted, "exec-1", "", "workflow started")
	bus.Publish(e)

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case received := <-ch:
			s.Equal(WorkflowStarted, received.Type)
		case <-time.After(time.Second):
			s.T().Fatal("timeout")
		}
	}
}

func (s *BusTestSuite) TestUnsubscribe() {
	bus := NewBus()
	defer bus.Close()

	ch := bus.Subscribe(10)
	bus.Unsubscribe(ch)

	// Channel should be closed
	_, ok := <-ch
	s.False(ok)
}

func (s *BusTestSuite) TestDropSlowSubscribers() {
	bus := NewBus()
	defer bus.Close()

	// Buffer of 1
	ch := bus.Subscribe(1)

	// Publish 5 events
	for i := 0; i < 5; i++ {
		bus.Publish(NewEvent(StepLog, "exec-1", "step1", "log"))
	}

	// Should get at least 1 event (buffer size)
	select {
	case <-ch:
	case <-time.After(time.Second):
		s.T().Fatal("timeout")
	}
}

func (s *BusTestSuite) TestConcurrentPublish() {
	bus := NewBus()
	defer bus.Close()

	ch := bus.Subscribe(100)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				bus.Publish(NewEvent(StepLog, "exec-1", "step1", "log"))
			}
		}()
	}
	wg.Wait()

	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	s.Require().Equal(100, count)
}

func (s *BusTestSuite) TestClose() {
	bus := NewBus()
	ch := bus.Subscribe(10)
	bus.Close()

	// Channel should be closed
	_, ok := <-ch
	s.False(ok)

	// Publishing after close should not panic
	bus.Publish(NewEvent(StepLog, "exec-1", "step1", "log"))
}

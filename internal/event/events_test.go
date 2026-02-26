package event

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type EventsTestSuite struct {
	suite.Suite
}

func TestEvents(t *testing.T) {
	suite.Run(t, new(EventsTestSuite))
}

func (s *EventsTestSuite) SetupTest() {}

func (s *EventsTestSuite) TestNewEvent() {
	ev := NewEvent(StepStarted, "exec-1", "step1", "started")
	s.Equal(StepStarted, ev.Type)
	s.Equal("exec-1", ev.ExecutionID)
	s.Equal("step1", ev.StepID)
	s.Equal("started", ev.Message)
	s.False(ev.Timestamp.IsZero())
}

func (s *EventsTestSuite) TestSnapshotData() {
	original := map[string]any{
		"key":    "value",
		"nested": map[string]any{"a": 1.0, "b": "hello"},
		"list":   []any{1.0, 2.0, 3.0},
	}

	cp := SnapshotData(original)
	s.Equal(original, cp)

	// Modify original — copy should be unaffected
	original["key"] = "changed"
	s.Equal("value", cp["key"])
}

func (s *EventsTestSuite) TestSnapshotData_Nil() {
	s.Nil(SnapshotData(nil))
}

func (s *EventsTestSuite) TestSnapshotData_MarshalError() {
	// A channel cannot be marshaled to JSON, so SnapshotData should return nil
	data := map[string]any{"ch": make(chan int)}
	result := SnapshotData(data)
	s.Nil(result)
}

func (s *EventsTestSuite) TestSnapshotData_UnmarshalError() {
	orig := jsonUnmarshalFn
	jsonUnmarshalFn = func(data []byte, v any) error { return fmt.Errorf("unmarshal boom") }
	defer func() { jsonUnmarshalFn = orig }()

	data := map[string]any{"key": "value"}
	result := SnapshotData(data)
	s.Nil(result)
}

func (s *EventsTestSuite) TestPublishSnapshotsData() {
	bus := NewBus()
	defer bus.Close()

	ch := bus.Subscribe(10)

	data := map[string]any{"key": "value"}
	ev := Event{
		Type:        StepOutput,
		ExecutionID: "exec-1",
		StepID:      "step1",
		Data:        data,
	}
	bus.Publish(ev)

	// Modify original data
	data["key"] = "modified"

	received := <-ch
	// The published data should have been deep-copied
	s.Equal("value", received.Data["key"])
}

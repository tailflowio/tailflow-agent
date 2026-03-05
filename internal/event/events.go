package event

import (
	"encoding/json"
	"time"

	sharedevent "github.com/tailflow/tailflow-shared/pkg/event"
)

// timeNow is a clock function for NewEvent. Override in tests.
var timeNow = time.Now

// jsonUnmarshalFn is used by SnapshotData. Override in tests to simulate unmarshal errors.
var jsonUnmarshalFn = json.Unmarshal

type (
	EventType = sharedevent.EventType
	Event     = sharedevent.Event
)

// Re-export shared constants.
const (
	WorkflowStarted   = sharedevent.WorkflowStarted
	WorkflowCompleted = sharedevent.WorkflowCompleted
	StepStarted       = sharedevent.StepStarted
	StepCompleted     = sharedevent.StepCompleted
	StepFailed        = sharedevent.StepFailed
	StepSkipped       = sharedevent.StepSkipped
	StepLog           = sharedevent.StepLog
	StepWaiting       = sharedevent.StepWaiting
	StepInput         = sharedevent.StepInput
	StepOutput        = sharedevent.StepOutput
	StepGoto          = sharedevent.StepGoto
	Metrics           = sharedevent.Metrics
)

func NewEvent(typ EventType, executionID, stepID, message string) Event {
	return Event{
		Type:        typ,
		Timestamp:   timeNow(),
		ExecutionID: executionID,
		StepID:      stepID,
		Message:     message,
	}
}

// SnapshotData deep-copies a map via JSON roundtrip to prevent concurrent
// map access panics when the original map is later modified by goroutines.
func SnapshotData(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}

	b, err := json.Marshal(data)
	if err != nil {
		return nil
	}

	var cp map[string]any

	err = jsonUnmarshalFn(b, &cp)
	if err != nil {
		return nil
	}

	return cp
}

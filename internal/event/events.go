package event

import (
	"encoding/json"
	"time"
)

// jsonUnmarshalFn is used by SnapshotData. Override in tests to simulate unmarshal errors.
var jsonUnmarshalFn = json.Unmarshal

type EventType string

const (
	WorkflowStarted   EventType = "workflow.started"
	WorkflowCompleted EventType = "workflow.completed"
	StepStarted       EventType = "step.started"
	StepCompleted     EventType = "step.completed"
	StepFailed        EventType = "step.failed"
	StepSkipped       EventType = "step.skipped"
	StepLog           EventType = "step.log"
	StepWaiting       EventType = "step.waiting"
	StepInput         EventType = "step.input"
	StepOutput        EventType = "step.output"
	StepGoto          EventType = "step.goto"
	Metrics           EventType = "metrics"
)

type Event struct {
	Type        EventType      `json:"type"`
	Timestamp   time.Time      `json:"timestamp"`
	ExecutionID string         `json:"execution_id"`
	StepID      string         `json:"step_id,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
	Message     string         `json:"message,omitempty"`
}

func NewEvent(typ EventType, executionID, stepID, message string) Event {
	return Event{
		Type:        typ,
		Timestamp:   time.Now(),
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

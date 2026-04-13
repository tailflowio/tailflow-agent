package workflow

import "time"

// StepStatus represents the execution status of a step.
type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepRunning StepStatus = "running"
	StepSuccess StepStatus = "success"
	StepFailed  StepStatus = "failed"
	StepSkipped StepStatus = "skipped"
)

// WorkflowInfo is the public representation of a workflow.
type WorkflowInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Author      string   `json:"author,omitempty"`
	FilePath    string   `json:"file_path"`
	HasTrigger  bool     `json:"has_trigger"`
	TriggerType string   `json:"trigger_type,omitempty"`
}

// ExecutionInfo is the public representation of an execution.
type ExecutionInfo struct {
	ID           string              `json:"id"`
	WorkflowName string              `json:"workflow_name"`
	Status       string              `json:"status"`
	StartedAt    time.Time           `json:"started_at"`
	FinishedAt   *time.Time          `json:"finished_at,omitempty"`
	Params       map[string]any      `json:"params,omitempty"`
	Steps        map[string]StepInfo `json:"steps,omitempty"`
	Error        string              `json:"error,omitempty"`
}

// StepInfo is the public representation of a step's execution state.
type StepInfo struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Action     string     `json:"action"`
	Status     StepStatus `json:"status"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Output     any        `json:"output,omitempty"`
	Error      string     `json:"error,omitempty"`
	Attempt    int        `json:"attempt,omitempty"`
}

// PipelineAction describes a sub-action inside a loop pipeline.
type PipelineAction struct {
	Action string `json:"action"`
	Title  string `json:"title,omitempty"`
}

// GraphNode represents a node in the DAG visualization.
type GraphNode struct {
	ID         string           `json:"id"`
	Label      string           `json:"label"`
	Action     string           `json:"action"`                // action type (e.g. "http", "loop", "set")
	Type       string           `json:"type"`                  // "step", "trigger"
	Pipeline   []PipelineAction `json:"pipeline,omitempty"`    // loop pipeline sub-actions
	When       string           `json:"when,omitempty"`        // condition expression for conditional steps
	OnRecovery string           `json:"on_recovery,omitempty"` // recovery strategy: retry, skip, fail
}

// GraphEdge represents an edge in the DAG visualization.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type,omitempty"`  // "goto"
	Label  string `json:"label,omitempty"` // condition
}

// Graph is the DAG representation for Vue Flow.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

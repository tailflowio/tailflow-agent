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
	ID          string           `json:"id"`
	Label       string           `json:"label"`
	Action      string           `json:"action"`
	Type        string           `json:"type"`
	Pipeline    []PipelineAction `json:"pipeline,omitempty"`
	When        string           `json:"when,omitempty"`
	OnRecovery  string           `json:"on_recovery,omitempty"`
	Depth       int              `json:"depth"`
	ParentID    string           `json:"parent_id,omitempty"`
	IsLast      bool             `json:"is_last"`
	GotoTarget  string           `json:"goto_target,omitempty"`
	GotoMax     int              `json:"goto_max,omitempty"`
	InLoop      bool             `json:"in_loop,omitempty"`
	IsLoopStart bool             `json:"is_loop_start,omitempty"`
}

// GraphEdge represents an edge in the DAG visualization.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type,omitempty"`  // "goto"
	Label  string `json:"label,omitempty"` // condition
}

type TreeLine struct {
	StepID     string `json:"step_id,omitempty"`
	Prefix     string `json:"prefix"`
	Name       string `json:"name"`
	Label      string `json:"label"`
	Action     string `json:"action,omitempty"`
	Merge      string `json:"merge,omitempty"`
	Type       string `json:"type"`
	Depth      int    `json:"depth"`
	InLoop     bool   `json:"in_loop,omitempty"`
	When       string `json:"when,omitempty"`
	GotoTarget string `json:"goto_target,omitempty"`
	GotoMax    int    `json:"goto_max,omitempty"`
}

type StageInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Steps       []string `json:"steps"`
}

type Graph struct {
	Nodes  []GraphNode `json:"nodes"`
	Edges  []GraphEdge `json:"edges"`
	Tree   []TreeLine  `json:"tree,omitempty"`
	Stages []StageInfo `json:"stages,omitempty"`
}

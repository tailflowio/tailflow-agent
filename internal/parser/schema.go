package parser

import "time"

// Workflow represents a parsed v2 workflow YAML.
type Workflow struct {
	Version     string            `json:"version"               yaml:"version"`
	Name        string            `json:"name"                  yaml:"name"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"        yaml:"tags,omitempty"`
	Revision    string            `json:"revision,omitempty"    yaml:"revision,omitempty"`
	Author      string            `json:"author,omitempty"      yaml:"author,omitempty"`
	Params      []Param           `json:"params,omitempty"      yaml:"params,omitempty"`
	Env         map[string]string `json:"env,omitempty"         yaml:"env,omitempty"`
	Sensitive   []string          `json:"sensitive,omitempty"   yaml:"sensitive,omitempty"`
	Trigger     *Trigger          `json:"trigger,omitempty"     yaml:"trigger,omitempty"`
	Recovery    bool              `json:"recovery,omitempty"    yaml:"recovery,omitempty"`
	OnError     []Step            `json:"on_error,omitempty"    yaml:"on_error,omitempty"`
	Steps       []Step            `json:"steps"                 yaml:"steps"`
}

// Param defines an input parameter for a workflow.
type Param struct {
	Name     string `json:"name"               yaml:"name"`
	Type     string `json:"type"               yaml:"type"`
	Required bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Default  any    `json:"default,omitempty"  yaml:"default,omitempty"`
	Pattern  string `json:"pattern,omitempty"  yaml:"pattern,omitempty"`
}

// GotoConfig defines a conditional jump to re-execute a subset of the DAG.
type GotoConfig struct {
	Target        string `json:"target"         yaml:"target"`
	When          string `json:"when"           yaml:"when"`
	MaxIterations int    `json:"max_iterations" yaml:"max_iterations"`
}

// Step defines a single step in the workflow DAG.
type Step struct {
	ID          string         `json:"id"                     yaml:"id"`
	Action      string         `json:"action"                 yaml:"action"`
	Title       string         `json:"title,omitempty"        yaml:"title,omitempty"`
	DependsOn   []string       `json:"depends_on,omitempty"   yaml:"depends_on,omitempty"`
	When        string         `json:"when,omitempty"         yaml:"when,omitempty"`
	Config      map[string]any `json:"config,omitempty"       yaml:"config,omitempty"`
	Retry       *RetryConfig   `json:"retry,omitempty"        yaml:"retry,omitempty"`
	OnError     []Step         `json:"on_error,omitempty"     yaml:"on_error,omitempty"`
	Timeout     string         `json:"timeout,omitempty"      yaml:"timeout,omitempty"`
	Goto        *GotoConfig    `json:"goto,omitempty"         yaml:"goto,omitempty"`
	ErrorPolicy string         `json:"error_policy,omitempty" yaml:"error_policy,omitempty"`
	OnRecovery  string         `json:"on_recovery,omitempty"  yaml:"on_recovery,omitempty"`
	Testing     []TestCase     `json:"testing,omitempty"      yaml:"testing,omitempty"`
}

type TestCase struct {
	Name   string          `json:"name"             yaml:"name"`
	Output any             `json:"output,omitempty" yaml:"output,omitempty"`
	Error  *TestCaseError  `json:"error,omitempty"  yaml:"error,omitempty"`
	Expect *TestCaseExpect `json:"expect,omitempty" yaml:"expect,omitempty"`
}

type TestCaseError struct {
	Message string `json:"message"        yaml:"message"`
	Code    string `json:"code,omitempty" yaml:"code,omitempty"`
}

type TestCaseExpect struct {
	Output any            `json:"output,omitempty" yaml:"output,omitempty"`
	Status string         `json:"status,omitempty" yaml:"status,omitempty"`
	Error  *TestCaseError `json:"error,omitempty"  yaml:"error,omitempty"`
}

// RetryConfig defines retry behavior for a step.
type RetryConfig struct {
	MaxAttempts int    `json:"max_attempts"    yaml:"max_attempts"`
	Delay       string `json:"delay,omitempty" yaml:"delay,omitempty"`
}

func (r *RetryConfig) ParsedDelay() (time.Duration, error) {
	if r.Delay == "" {
		return time.Second, nil
	}

	return time.ParseDuration(r.Delay)
}

// Trigger defines how a workflow can be triggered.
type Trigger struct {
	HTTP     *HTTPTrigger     `json:"http,omitempty"     yaml:"http,omitempty"`
	Webhook  *WebhookTrigger  `json:"webhook,omitempty"  yaml:"webhook,omitempty"`
	Schedule *ScheduleTrigger `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	RabbitMQ *RabbitMQTrigger `json:"rabbitmq,omitempty" yaml:"rabbitmq,omitempty"`
}

// ScheduleTrigger triggers a workflow on a cron schedule.
type ScheduleTrigger struct {
	Cron string `json:"cron" yaml:"cron"`
}

// HTTPTrigger exposes a workflow as an HTTP endpoint.
type HTTPTrigger struct {
	Method         string `json:"method"                    yaml:"method"`
	Path           string `json:"path"                      yaml:"path"`
	Async          bool   `json:"async,omitempty"           yaml:"async,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty" yaml:"idempotency_key,omitempty"`
}

// WebhookTrigger listens for incoming webhooks.
type WebhookTrigger struct {
	Path   string `json:"path"             yaml:"path"`
	Secret string `json:"secret,omitempty" yaml:"secret,omitempty"`
	Filter string `json:"filter,omitempty" yaml:"filter,omitempty"`
}

// RabbitMQTrigger listens on a RabbitMQ queue and triggers workflow on each message.
type RabbitMQTrigger struct {
	URL          string `json:"url"                      yaml:"url"`
	Queue        string `json:"queue"                    yaml:"queue"`
	Prefetch     int    `json:"prefetch,omitempty"       yaml:"prefetch,omitempty"`
	AckOnSuccess bool   `json:"ack_on_success,omitempty" yaml:"ack_on_success,omitempty"`
}

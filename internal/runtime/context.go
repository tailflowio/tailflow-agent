package runtime

import (
	"fmt"
	"maps"
	"sync"
	"time"
)

type StepError struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`    // "timeout", "action_failed", "config_error", "cancelled"
	StepID  string `json:"step_id,omitempty"` // step that failed
}

func (e *StepError) Error() string { return e.Message }

type StepResult struct {
	Status     string     `json:"status"`           // see Status* constants
	Input      any        `json:"input,omitempty"`  // data flowing into the step (resolved config, trigger, deps)
	Output     any        `json:"output,omitempty"` // data produced by the step
	Error      *StepError `json:"error,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`  // when the step started executing
	FinishedAt *time.Time `json:"finished_at,omitempty"` // when the step finished executing
}

type ExecutionContext struct {
	mu           sync.RWMutex
	ExecutionID  string
	WorkflowName string
	Params       map[string]any
	Env          map[string]string
	Steps        map[string]*StepResult
	Variables    map[string]any  // user-defined variables (via "set" action)
	TriggerData  map[string]any  // trigger context (method, path, headers, body, query)
	Services     *ActionServices // server-side services (nil in CLI mode)
	TestCaseName string          // non-empty when running in test mode
}

func NewExecutionContext(executionID, workflowName string, params map[string]any, env map[string]string) *ExecutionContext {
	if params == nil {
		params = make(map[string]any)
	}

	if env == nil {
		env = make(map[string]string)
	}

	return &ExecutionContext{
		ExecutionID:  executionID,
		WorkflowName: workflowName,
		Params:       params,
		Env:          env,
		Steps:        make(map[string]*StepResult),
		Variables:    make(map[string]any),
		TriggerData:  make(map[string]any),
	}
}

func (c *ExecutionContext) SetStepResult(stepID string, result *StepResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Steps[stepID] = result
}

func (c *ExecutionContext) GetStepResult(stepID string) (*StepResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	r, ok := c.Steps[stepID]

	return r, ok
}

func (c *ExecutionContext) ClearStepResult(stepID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.Steps, stepID)
}

func (c *ExecutionContext) SetVariable(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Variables[key] = value
}

func (c *ExecutionContext) GetVariable(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	v, ok := c.Variables[key]

	return v, ok
}

func (c *ExecutionContext) ToMap() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stepsMap := make(map[string]any, len(c.Steps))

	for id, r := range c.Steps {
		stepMap := map[string]any{
			"status": r.Status,
			"output": deepCopyAny(r.Output),
		}

		if r.Error != nil {
			stepMap["error"] = map[string]any{
				"message": r.Error.Message,
				"code":    r.Error.Code,
				"step_id": r.Error.StepID,
			}
		}

		stepsMap[id] = stepMap
	}

	envCopy := make(map[string]string, len(c.Env))
	maps.Copy(envCopy, c.Env)

	m := map[string]any{
		"params":  deepCopyAny(c.Params),
		"env":     envCopy,
		"steps":   stepsMap,
		"vars":    deepCopyAny(c.Variables),
		"trigger": deepCopyAny(c.TriggerData),
	}

	return m
}

func deepCopyAny(v any) any {
	switch val := v.(type) {
	case map[string]any:
		cp := make(map[string]any, len(val))
		for k, item := range val {
			cp[k] = deepCopyAny(item)
		}

		return cp
	case []any:
		cp := make([]any, len(val))
		for i, item := range val {
			cp[i] = deepCopyAny(item)
		}

		return cp
	default:
		return v
	}
}

func (c *ExecutionContext) ResolveParam(name string) (any, error) {
	if v, ok := c.Params[name]; ok {
		return v, nil
	}

	return nil, fmt.Errorf("parameter %q not provided", name)
}

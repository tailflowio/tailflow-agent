package store

import (
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
)

// Execution represents a stored workflow execution.
type Execution struct {
	ID           string                         `json:"id"`
	WorkflowName string                         `json:"workflow_name"`
	Status       string                         `json:"status"` // see runtime.Status* constants
	Params       map[string]any                 `json:"params,omitempty"`
	Steps        map[string]*runtime.StepResult `json:"steps,omitempty"`
	StartedAt    time.Time                      `json:"started_at"`
	FinishedAt   *time.Time                     `json:"finished_at,omitempty"`
	Error        string                         `json:"error,omitempty"`
}

// ExecutionStore is the contract for execution persistence.
type ExecutionStore interface {
	Add(exec *Execution)
	Get(id string) (*Execution, error)
	Update(exec *Execution)
	UpdateExecution(id string, fn func(exec *Execution))
	List() []*Execution
	Count() int
	AppendEvent(executionID string, ev event.Event)
	GetEvents(executionID string) []event.Event
	UpdateStep(executionID, stepID string, fn func(step *runtime.StepResult))
	IncrStepExecCount(stepID string)
	StepExecCounts() map[string]int
	RefreshStepMetrics()
	GetStepMetrics(stepID string) *StepMetrics
	GetAllStepMetrics() map[string]*StepMetrics
}

// StepMetrics holds cached metrics for a single step.
type StepMetrics struct {
	TotalExecutions int    `json:"total_executions"`
	SuccessCount    int    `json:"success_count"`
	FailureCount    int    `json:"failure_count"`
	AvgDurationMs   int64  `json:"avg_duration_ms"`
	LastExecution   string `json:"last_execution,omitempty"`
}

// MemoryExecutionStore is an in-memory ring buffer implementation.
type MemoryExecutionStore struct {
	mu             sync.RWMutex
	buffer         []*Execution
	byID           map[string]*Execution
	events         map[string][]event.Event
	stepExecCounts map[string]int
	capacity       int
	head           int
	count          int

	metricsMu   sync.RWMutex
	stepMetrics map[string]*StepMetrics // stepID -> cached metrics
}

func NewExecutionStore(capacity int) *MemoryExecutionStore {
	if capacity <= 0 {
		capacity = 100
	}

	return &MemoryExecutionStore{
		buffer:         make([]*Execution, capacity),
		byID:           make(map[string]*Execution),
		events:         make(map[string][]event.Event),
		stepExecCounts: make(map[string]int),
		capacity:       capacity,
	}
}

func (s *MemoryExecutionStore) Add(exec *Execution) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Evict oldest if full
	if s.count >= s.capacity {
		old := s.buffer[s.head]
		if old != nil {
			delete(s.byID, old.ID)
			delete(s.events, old.ID)
		}
	}

	s.buffer[s.head] = exec
	s.byID[exec.ID] = exec
	s.head = (s.head + 1) % s.capacity

	if s.count < s.capacity {
		s.count++
	}
}

func (s *MemoryExecutionStore) Get(id string) (*Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	exec, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("execution %q not found", id)
	}

	return exec, nil
}

func (s *MemoryExecutionStore) Update(exec *Execution) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.byID[exec.ID]; ok {
		*existing = *exec
	}
}

// UpdateExecution atomically applies a mutation to the stored execution.
// The callback runs while the store lock is held, preventing concurrent
// mutations from captureEvents and finalizeExecution.
func (s *MemoryExecutionStore) UpdateExecution(id string, fn func(exec *Execution)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if exec, ok := s.byID[id]; ok {
		fn(exec)
	}
}

// List returns all executions, most recent first.
func (s *MemoryExecutionStore) List() []*Execution {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Execution, 0, s.count)

	for i := 0; i < s.count; i++ {
		idx := (s.head - 1 - i + s.capacity) % s.capacity
		if s.buffer[idx] != nil {
			result = append(result, s.buffer[idx])
		}
	}

	return result
}

// Count returns the number of stored executions.
func (s *MemoryExecutionStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.count
}

// AppendEvent stores an event for a given execution.
func (s *MemoryExecutionStore) AppendEvent(executionID string, ev event.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events[executionID] = append(s.events[executionID], ev)
}

// UpdateStep applies a partial update to a step within a running execution.
// It also automatically derives the execution-level status from step states
// (running vs waiting) while the execution is still active.
func (s *MemoryExecutionStore) UpdateStep(executionID, stepID string, fn func(step *runtime.StepResult)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	exec, ok := s.byID[executionID]
	if !ok {
		return
	}

	if exec.Steps == nil {
		exec.Steps = make(map[string]*runtime.StepResult)
	}

	if exec.Steps[stepID] == nil {
		exec.Steps[stepID] = &runtime.StepResult{}
	}

	fn(exec.Steps[stepID])

	// Auto-derive execution status from step states (only while active)
	if exec.Status != runtime.StatusRunning && exec.Status != runtime.StatusWaiting {
		return
	}

	for _, step := range exec.Steps {
		if step.Status == runtime.StatusRunning {
			exec.Status = runtime.StatusRunning
			return
		}
	}

	for _, step := range exec.Steps {
		if step.Status == runtime.StatusWaiting {
			exec.Status = runtime.StatusWaiting
			return
		}
	}
}

// GetEvents returns all stored events for an execution.
func (s *MemoryExecutionStore) GetEvents(executionID string) []event.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	evts := s.events[executionID]
	if evts == nil {
		return nil
	}
	// Return a copy
	out := make([]event.Event, len(evts))
	copy(out, evts)

	return out
}

// IncrStepExecCount increments the execution counter for a step.
func (s *MemoryExecutionStore) IncrStepExecCount(stepID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stepExecCounts[stepID]++
}

// StepExecCounts returns the execution count per step.
func (s *MemoryExecutionStore) StepExecCounts() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]int, len(s.stepExecCounts))
	maps.Copy(out, s.stepExecCounts)

	return out
}

// RefreshStepMetrics recomputes cached step metrics from all stored executions.
// Designed to be called periodically by a background goroutine.
func (s *MemoryExecutionStore) RefreshStepMetrics() {
	s.mu.RLock()

	execs := make([]*Execution, 0, s.count)

	for i := 0; i < s.count; i++ {
		idx := (s.head - 1 - i + s.capacity) % s.capacity
		if s.buffer[idx] != nil {
			execs = append(execs, s.buffer[idx])
		}
	}

	s.mu.RUnlock()

	metrics := make(map[string]*StepMetrics)

	for _, exec := range execs {
		for stepID, sr := range exec.Steps {
			m, ok := metrics[stepID]
			if !ok {
				m = &StepMetrics{}
				metrics[stepID] = m
			}

			m.TotalExecutions++

			switch sr.Status {
			case runtime.StatusSuccess:
				m.SuccessCount++
			case runtime.StatusFailed:
				m.FailureCount++
			}

			if sr.StartedAt != nil && sr.FinishedAt != nil {
				m.AvgDurationMs += sr.FinishedAt.Sub(*sr.StartedAt).Milliseconds()
			}

			ts := exec.StartedAt.Format(time.RFC3339)
			if m.LastExecution == "" || ts > m.LastExecution {
				m.LastExecution = ts
			}
		}
	}

	for _, m := range metrics {
		if m.TotalExecutions > 0 {
			m.AvgDurationMs /= int64(m.TotalExecutions)
		}
	}

	s.metricsMu.Lock()
	s.stepMetrics = metrics
	s.metricsMu.Unlock()
}

// GetStepMetrics returns cached metrics for a given step.
func (s *MemoryExecutionStore) GetStepMetrics(stepID string) *StepMetrics {
	s.metricsMu.RLock()
	defer s.metricsMu.RUnlock()

	if s.stepMetrics == nil {
		return nil
	}

	return s.stepMetrics[stepID]
}

// GetAllStepMetrics returns all cached step metrics.
func (s *MemoryExecutionStore) GetAllStepMetrics() map[string]*StepMetrics {
	s.metricsMu.RLock()
	defer s.metricsMu.RUnlock()

	out := make(map[string]*StepMetrics, len(s.stepMetrics))

	for k, v := range s.stepMetrics {
		cp := *v
		out[k] = &cp
	}

	return out
}

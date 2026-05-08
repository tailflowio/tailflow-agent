// Package store defines the persistence contract for workflow executions.
package store

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
)

// ErrNotFound is returned by ExecutionStore.Get when no execution matches the
// given id. Backends should wrap it with %w so callers can use errors.Is.
var ErrNotFound = errors.New("execution not found")

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

// ExecutionStore is the contract for execution persistence. Implementations
// may be in-memory, SQL (MariaDB/MySQL/Postgres), or columnar (ClickHouse).
// All methods accept a context for cancellation, deadlines and tracing.
//
// Writes return error so backends backed by network/disk can propagate
// failures. Implementations may also return context errors (DeadlineExceeded,
// Canceled) when the caller's context is no longer live.
type ExecutionStore interface {
	// Add inserts a new execution. Returns an error if persistence fails.
	Add(ctx context.Context, exec *Execution) error

	// Get returns the execution by id. Wraps ErrNotFound when absent.
	Get(ctx context.Context, id string) (*Execution, error)

	// Update overwrites a stored execution with the given snapshot.
	Update(ctx context.Context, exec *Execution) error

	// UpdateExecution atomically applies fn to the stored execution.
	// Implementations guarantee serial access to fn for the same id.
	// Missing executions are silently ignored.
	UpdateExecution(ctx context.Context, id string, fn func(exec *Execution)) error

	// List returns all stored executions, newest first.
	List(ctx context.Context) ([]*Execution, error)

	// Count returns the number of stored executions.
	Count(ctx context.Context) (int, error)

	// AppendEvent appends an event to the execution's event log.
	AppendEvent(ctx context.Context, executionID string, ev event.Event) error

	// GetEvents returns the full event log for the given execution.
	GetEvents(ctx context.Context, executionID string) ([]event.Event, error)

	// GetEventsPaginated returns a window of events plus the total count.
	GetEventsPaginated(ctx context.Context, executionID string, offset, limit int) ([]event.Event, int, error)

	// UpdateStep atomically applies fn to the named step within an execution.
	// Missing executions and steps are created on demand.
	UpdateStep(ctx context.Context, executionID, stepID string, fn func(step *runtime.StepResult)) error

	// IncrStepExecCount increments the lifetime execution count of a step.
	IncrStepExecCount(ctx context.Context, stepID string) error

	// StepExecCounts returns a snapshot of all step execution counts.
	StepExecCounts(ctx context.Context) (map[string]int, error)

	// GetStepMetrics returns aggregated metrics for the given step, or nil
	// if no metrics have been computed yet for that step.
	GetStepMetrics(ctx context.Context, stepID string) (*StepMetrics, error)

	// GetAllStepMetrics returns aggregated metrics for every known step.
	GetAllStepMetrics(ctx context.Context) (map[string]*StepMetrics, error)
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

// NewExecutionStore returns a new in-memory ExecutionStore with the given
// ring-buffer capacity. A non-positive capacity falls back to 100.
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

// Add inserts a new execution. Evicts the oldest entry if capacity is reached.
func (s *MemoryExecutionStore) Add(_ context.Context, exec *Execution) error {
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

	return nil
}

// Get returns a deep snapshot of the stored execution, or wraps ErrNotFound.
func (s *MemoryExecutionStore) Get(_ context.Context, id string) (*Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	exec, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, id)
	}

	return exec.snapshot(), nil
}

func (e *Execution) snapshot() *Execution {
	cp := *e
	if e.Steps != nil {
		cp.Steps = make(map[string]*runtime.StepResult, len(e.Steps))
		for k, v := range e.Steps {
			sr := *v
			cp.Steps[k] = &sr
		}
	}

	if e.Params != nil {
		cp.Params = make(map[string]any, len(e.Params))
		for k, v := range e.Params {
			cp.Params[k] = v
		}
	}

	if e.FinishedAt != nil {
		t := *e.FinishedAt
		cp.FinishedAt = &t
	}

	return &cp
}

// Update overwrites the stored execution with the given snapshot.
func (s *MemoryExecutionStore) Update(_ context.Context, exec *Execution) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.byID[exec.ID]
	if ok {
		*existing = *exec
	}

	return nil
}

// UpdateExecution atomically applies fn to the stored execution.
// The callback runs while the store lock is held, preventing concurrent
// mutations from captureEvents and finalizeExecution.
func (s *MemoryExecutionStore) UpdateExecution(_ context.Context, id string, fn func(exec *Execution)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	exec, ok := s.byID[id]
	if ok {
		fn(exec)
	}

	return nil
}

// List returns all stored executions, newest first.
func (s *MemoryExecutionStore) List(_ context.Context) ([]*Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Execution, 0, s.count)

	for i := 0; i < s.count; i++ {
		idx := (s.head - 1 - i + s.capacity) % s.capacity
		if s.buffer[idx] != nil {
			result = append(result, s.buffer[idx].snapshot())
		}
	}

	return result, nil
}

// Count returns the number of stored executions.
func (s *MemoryExecutionStore) Count(_ context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.count, nil
}

// AppendEvent appends an event to the execution's event log.
func (s *MemoryExecutionStore) AppendEvent(_ context.Context, executionID string, ev event.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events[executionID] = append(s.events[executionID], ev)

	return nil
}

// UpdateStep applies a partial update to a step within a running execution.
// It also automatically derives the execution-level status from step states
// (running vs waiting) while the execution is still active.
func (s *MemoryExecutionStore) UpdateStep(_ context.Context, executionID, stepID string, fn func(step *runtime.StepResult)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	exec, ok := s.byID[executionID]
	if !ok {
		return nil
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
		return nil
	}

	for _, step := range exec.Steps {
		if step.Status == runtime.StatusRunning {
			exec.Status = runtime.StatusRunning
			return nil
		}
	}

	for _, step := range exec.Steps {
		if step.Status == runtime.StatusWaiting {
			exec.Status = runtime.StatusWaiting
			return nil
		}
	}

	return nil
}

// GetEvents returns a defensive copy of the full event log for executionID.
func (s *MemoryExecutionStore) GetEvents(_ context.Context, executionID string) ([]event.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	evts := s.events[executionID]
	if evts == nil {
		return nil, nil
	}

	out := make([]event.Event, len(evts))
	copy(out, evts)

	return out, nil
}

// GetEventsPaginated returns a window of events with the total count.
func (s *MemoryExecutionStore) GetEventsPaginated(_ context.Context, executionID string, offset, limit int) ([]event.Event, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	evts := s.events[executionID]
	total := len(evts)

	if offset >= total {
		return nil, total, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	out := make([]event.Event, end-offset)
	copy(out, evts[offset:end])

	return out, total, nil
}

// IncrStepExecCount increments the lifetime execution count of a step.
func (s *MemoryExecutionStore) IncrStepExecCount(_ context.Context, stepID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stepExecCounts[stepID]++

	return nil
}

// StepExecCounts returns a defensive copy of every step's execution counter.
func (s *MemoryExecutionStore) StepExecCounts(_ context.Context) (map[string]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]int, len(s.stepExecCounts))
	maps.Copy(out, s.stepExecCounts)

	return out, nil
}

// RefreshStepMetrics recomputes cached step metrics from all stored executions.
// Designed to be called periodically by a background goroutine. It is not part
// of the ExecutionStore interface — backends that compute metrics on demand
// (SQL, ClickHouse) do not need it.
func (s *MemoryExecutionStore) RefreshStepMetrics() {
	s.mu.RLock()

	execs := make([]*Execution, 0, s.count)

	for i := 0; i < s.count; i++ {
		idx := (s.head - 1 - i + s.capacity) % s.capacity
		if s.buffer[idx] != nil {
			execs = append(execs, s.buffer[idx].snapshot())
		}
	}

	s.mu.RUnlock()

	metrics := computeStepMetrics(execs)

	s.metricsMu.Lock()
	s.stepMetrics = metrics
	s.metricsMu.Unlock()
}

func computeStepMetrics(execs []*Execution) map[string]*StepMetrics {
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

	return metrics
}

// GetStepMetrics returns a copy of the cached metrics for a step, or nil.
func (s *MemoryExecutionStore) GetStepMetrics(_ context.Context, stepID string) (*StepMetrics, error) {
	s.metricsMu.RLock()
	defer s.metricsMu.RUnlock()

	if s.stepMetrics == nil {
		return nil, nil
	}

	m := s.stepMetrics[stepID]
	if m == nil {
		return nil, nil
	}

	cp := *m

	return &cp, nil
}

// GetAllStepMetrics returns a defensive copy of every cached step's metrics.
func (s *MemoryExecutionStore) GetAllStepMetrics(_ context.Context) (map[string]*StepMetrics, error) {
	s.metricsMu.RLock()
	defer s.metricsMu.RUnlock()

	out := make(map[string]*StepMetrics, len(s.stepMetrics))

	for k, v := range s.stepMetrics {
		cp := *v
		out[k] = &cp
	}

	return out, nil
}

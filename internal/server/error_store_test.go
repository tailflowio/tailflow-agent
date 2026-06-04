package server

import (
	"context"
	"errors"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
	"github.com/tailflow/tailflow/internal/store"
)

// errStoreList is returned by errExecutionStore.List when configured.
var errStoreList = errors.New("list failed")

// errStoreGetEvents is returned by errExecutionStore.GetEvents when configured.
var errStoreGetEvents = errors.New("get events failed")

// errStoreGetAllStepMetrics is returned by errExecutionStore.GetAllStepMetrics.
var errStoreGetAllStepMetrics = errors.New("get all step metrics failed")

// errStoreGetStepMetrics is returned by errExecutionStore.GetStepMetrics.
var errStoreGetStepMetrics = errors.New("get step metrics failed")

// errStoreStepExecCounts is returned by errExecutionStore.StepExecCounts.
var errStoreStepExecCounts = errors.New("step exec counts failed")

// errStoreAdd is returned by errExecutionStore.Add when configured.
var errStoreAdd = errors.New("add failed")

// errStoreAppendEvent is returned by errExecutionStore.AppendEvent when configured.
var errStoreAppendEvent = errors.New("append event failed")

// errExecutionStore wraps a real store and can inject errors into specific methods.
type errExecutionStore struct {
	inner               store.ExecutionStore
	failList            bool
	failGetEvents       bool
	failAllStepMetrics  bool
	failStepMetrics     bool
	failStepExecCounts  bool
	failAdd             bool
	failAppendEvent     bool
}

func newErrStore() *errExecutionStore {
	return &errExecutionStore{inner: store.NewExecutionStore(100)}
}

func (e *errExecutionStore) Add(ctx context.Context, exec *store.Execution) error {
	if e.failAdd {
		return errStoreAdd
	}

	return e.inner.Add(ctx, exec)
}

func (e *errExecutionStore) Get(ctx context.Context, id string) (*store.Execution, error) {
	return e.inner.Get(ctx, id)
}

func (e *errExecutionStore) Update(ctx context.Context, exec *store.Execution) error {
	return e.inner.Update(ctx, exec)
}

func (e *errExecutionStore) UpdateExecution(ctx context.Context, id string, fn func(*store.Execution)) error {
	return e.inner.UpdateExecution(ctx, id, fn)
}

func (e *errExecutionStore) List(ctx context.Context) ([]*store.Execution, error) {
	if e.failList {
		return nil, errStoreList
	}

	return e.inner.List(ctx)
}

func (e *errExecutionStore) Count(ctx context.Context) (int, error) {
	return e.inner.Count(ctx)
}

func (e *errExecutionStore) AppendEvent(ctx context.Context, executionID string, ev event.Event) error {
	if e.failAppendEvent {
		return errStoreAppendEvent
	}

	return e.inner.AppendEvent(ctx, executionID, ev)
}

func (e *errExecutionStore) GetEvents(ctx context.Context, executionID string) ([]event.Event, error) {
	if e.failGetEvents {
		return nil, errStoreGetEvents
	}

	return e.inner.GetEvents(ctx, executionID)
}

func (e *errExecutionStore) GetEventsPaginated(ctx context.Context, executionID string, offset, limit int) ([]event.Event, int, error) {
	return e.inner.GetEventsPaginated(ctx, executionID, offset, limit)
}

func (e *errExecutionStore) UpdateStep(ctx context.Context, executionID, stepID string, fn func(*runtime.StepResult)) error {
	return e.inner.UpdateStep(ctx, executionID, stepID, fn)
}

func (e *errExecutionStore) IncrStepExecCount(ctx context.Context, stepID string) error {
	return e.inner.IncrStepExecCount(ctx, stepID)
}

func (e *errExecutionStore) StepExecCounts(ctx context.Context) (map[string]int, error) {
	if e.failStepExecCounts {
		return nil, errStoreStepExecCounts
	}

	return e.inner.StepExecCounts(ctx)
}

func (e *errExecutionStore) GetStepMetrics(ctx context.Context, stepID string) (*store.StepMetrics, error) {
	if e.failStepMetrics {
		return nil, errStoreGetStepMetrics
	}

	return e.inner.GetStepMetrics(ctx, stepID)
}

func (e *errExecutionStore) GetAllStepMetrics(ctx context.Context) (map[string]*store.StepMetrics, error) {
	if e.failAllStepMetrics {
		return nil, errStoreGetAllStepMetrics
	}

	return e.inner.GetAllStepMetrics(ctx)
}

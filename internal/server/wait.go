package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/tailflow/tailflow/internal/runtime"
)

// WaitRegistration represents a pending wait.webhook registration.
type WaitRegistration struct {
	ExecutionID string
	StepID      string
	Path        string
	Ch          chan runtime.WaitRequest
}

// WaitRegistry manages wait.webhook registrations.
type WaitRegistry struct {
	mu            sync.RWMutex
	registrations map[string]*WaitRegistration // key: "{executionID}/{path}"
}

func NewWaitRegistry() *WaitRegistry {
	return &WaitRegistry{
		registrations: make(map[string]*WaitRegistration),
	}
}

// Register creates a new wait registration and returns a channel and cleanup function.
func (wr *WaitRegistry) Register(executionID, stepID, path string, ctx context.Context) (<-chan runtime.WaitRequest, func()) {
	key := executionID + "/" + path
	ch := make(chan runtime.WaitRequest, 1)

	reg := &WaitRegistration{
		ExecutionID: executionID,
		StepID:      stepID,
		Path:        path,
		Ch:          ch,
	}

	wr.mu.Lock()
	wr.registrations[key] = reg
	wr.mu.Unlock()

	cleanup := func() {
		wr.mu.Lock()
		delete(wr.registrations, key)
		wr.mu.Unlock()
	}

	// Auto-cleanup on context cancellation
	if ctx != nil {
		go func() {
			<-ctx.Done()
			cleanup()
		}()
	}

	return ch, cleanup
}

// Deliver sends a WaitRequest to a registered wait.webhook.
func (wr *WaitRegistry) Deliver(executionID, path string, req runtime.WaitRequest) error {
	key := executionID + "/" + path

	wr.mu.RLock()
	reg, ok := wr.registrations[key]
	wr.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no wait registration for %s", key)
	}

	select {
	case reg.Ch <- req:
		return nil
	default:
		return fmt.Errorf("wait channel full for %s", key)
	}
}

package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type lockEntry struct {
	mu     sync.Mutex
	locked bool
	ch     chan struct{} // signaled on unlock
}

type MemoryLocker struct {
	mu      sync.Mutex
	entries map[string]*lockEntry
}

func NewMemoryLocker() *MemoryLocker {
	return &MemoryLocker{
		entries: make(map[string]*lockEntry),
	}
}

func (l *MemoryLocker) getEntry(key string) *lockEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.entries[key]
	if !ok {
		e = &lockEntry{ch: make(chan struct{}, 1)}
		l.entries[key] = e
	}

	return e
}

func (l *MemoryLocker) Lock(ctx context.Context, key string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	e := l.getEntry(key)

	for {
		e.mu.Lock()
		if !e.locked {
			e.locked = true
			e.mu.Unlock()

			return nil
		}
		e.mu.Unlock()

		select {
		case <-ctx.Done():
			return fmt.Errorf("lock %q: timeout after %s", key, timeout)
		case <-e.ch:
		}
	}
}

func (l *MemoryLocker) Unlock(_ context.Context, key string) error {
	e := l.getEntry(key)
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.locked {
		return fmt.Errorf("unlock %q: not locked", key)
	}

	e.locked = false
	// non-blocking signal
	select {
	case e.ch <- struct{}{}:
	default:
	}

	return nil
}

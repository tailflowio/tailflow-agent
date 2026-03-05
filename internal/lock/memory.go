package lock

import (
	"context"
	"sync"
)

type MemoryLocker struct {
	locks sync.Map
}

func NewMemoryLocker() *MemoryLocker {
	return &MemoryLocker{}
}

func (l *MemoryLocker) Acquire(_ context.Context, key string) (func(), bool, error) {
	_, loaded := l.locks.LoadOrStore(key, struct{}{})
	if loaded {
		return nil, false, nil
	}

	return func() { l.locks.Delete(key) }, true, nil
}

type MemoryDeduplicator struct {
	seen sync.Map
}

func NewMemoryDeduplicator() *MemoryDeduplicator {
	return &MemoryDeduplicator{}
}

func (d *MemoryDeduplicator) IsDuplicate(_ context.Context, eventID string) (bool, error) {
	_, loaded := d.seen.LoadOrStore(eventID, struct{}{})
	if loaded {
		return true, nil
	}

	return false, nil
}

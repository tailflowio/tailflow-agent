package runtime

import (
	"context"
	"sync"
	"time"
)

type kvEntry struct {
	value     any
	expiresAt time.Time // zero value means no expiration
}

type MemoryKVStore struct {
	mu    sync.Mutex
	store map[string]kvEntry
}

func NewMemoryKVStore() *MemoryKVStore {
	return &MemoryKVStore{
		store: make(map[string]kvEntry),
	}
}

func (s *MemoryKVStore) Get(_ context.Context, key string) (any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.store[key]
	if !ok {
		return nil, false
	}

	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		delete(s.store, key)
		return nil, false
	}

	return e.value, true
}

func (s *MemoryKVStore) Set(_ context.Context, key string, value any, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := kvEntry{value: value}
	if ttl > 0 {
		entry.expiresAt = time.Now().Add(ttl)
	}

	s.store[key] = entry
}

func (s *MemoryKVStore) Delete(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.store[key]
	if !ok {
		return false, nil
	}

	// Treat expired entries as non-existent
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		delete(s.store, key)
		return false, nil
	}

	delete(s.store, key)

	return true, nil
}

func (s *MemoryKVStore) Close() error {
	return nil
}

package action

import (
	"fmt"
	"sort"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	factories map[string]ActionFactory
	allowlist map[string]bool // nil = all allowed (default)
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]ActionFactory),
	}
}

func (r *Registry) Register(name string, factory ActionFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories[name] = factory
}

// Pass nil to remove the restriction (all actions allowed).
func (r *Registry) SetAllowlist(names []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if names == nil {
		r.allowlist = nil
		return
	}

	r.allowlist = make(map[string]bool, len(names))
	for _, n := range names {
		r.allowlist[n] = true
	}
}

func (r *Registry) Create(name string) (Action, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	allowed := r.allowlist == nil || r.allowlist[name]
	r.mu.RUnlock()

	if !allowed {
		return nil, fmt.Errorf("action %q is not allowed", name)
	}

	if !ok {
		return nil, fmt.Errorf("unknown action: %q", name)
	}

	return factory(), nil
}

func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.factories[name]

	return ok
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

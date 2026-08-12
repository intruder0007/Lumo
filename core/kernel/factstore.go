package kernel

import "sync"

// FactStore holds each domain's published model: the current, complete
// snapshot of what that domain knows. A domain publishes its own model
// and reads others'; per docs/architecture/domains/kernel.md, no domain
// may publish another domain's model.
type FactStore struct {
	mu     sync.RWMutex
	models map[DomainID]any
}

// NewFactStore returns an empty FactStore.
func NewFactStore() *FactStore {
	return &FactStore{models: make(map[DomainID]any)}
}

// Publish replaces domain's current model with model. Callers publish
// their whole model on every update, not a diff — diffing state changes
// is the EventBus's job, not the store's.
func (f *FactStore) Publish(domain DomainID, model any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.models[domain] = model
}

// Get returns domain's most recently published model. ok is false if
// domain has never published.
func (f *FactStore) Get(domain DomainID) (model any, ok bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	model, ok = f.models[domain]
	return model, ok
}

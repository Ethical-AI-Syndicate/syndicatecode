package models

import (
	"fmt"
	"sync"
)

// Registry maps provider names to Provider instances.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a provider under the given name.
// Calling Register with the same name overwrites the previous entry.
func (r *Registry) Register(name string, p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = p
}

// Resolve returns a LanguageModel for the given provider name and model ID.
// Returns an error if the provider is not registered.
func (r *Registry) Resolve(providerName, modelID string) (LanguageModel, error) {
	r.mu.RLock()
	p, ok := r.providers[providerName]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", providerName)
	}
	return p.Model(modelID), nil
}

// Package source defines the interface for secret sources.
package source

import (
	"fmt"
	"sync"
)

// Registry manages available secret sources.
type Registry struct {
	mu      sync.RWMutex
	sources map[string]SourceFactory
}

// NewRegistry creates a new source registry.
func NewRegistry() *Registry {
	return &Registry{
		sources: make(map[string]SourceFactory),
	}
}

// Register adds a source factory to the registry.
func (r *Registry) Register(name string, factory SourceFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sources[name] = factory
}

// Get returns a new instance of the named source.
func (r *Registry) Get(name string) (Source, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, ok := r.sources[name]
	if !ok {
		return nil, fmt.Errorf("unknown source: %s", name)
	}

	return factory(), nil
}

// List returns the names of all registered sources.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.sources))
	for name := range r.sources {
		names = append(names, name)
	}
	return names
}

// Has checks if a source is registered.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.sources[name]
	return ok
}

// DefaultRegistry is the global source registry.
var DefaultRegistry = NewRegistry()

// Register adds a source factory to the default registry.
func Register(name string, factory SourceFactory) {
	DefaultRegistry.Register(name, factory)
}

// Get returns a new instance of the named source from the default registry.
func Get(name string) (Source, error) {
	return DefaultRegistry.Get(name)
}

// List returns the names of all registered sources in the default registry.
func List() []string {
	return DefaultRegistry.List()
}

// Has checks if a source is registered in the default registry.
func Has(name string) bool {
	return DefaultRegistry.Has(name)
}

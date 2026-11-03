package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrNilModule       = errors.New("cannot register nil module")
	ErrModuleExists    = errors.New("module already registered")
	ErrModuleNotFound  = errors.New("module not found")
)

// Registry manages the lifecycle, discovery, and execution of sweep modules.
type Registry struct {
	mu      sync.RWMutex
	modules []Module
	lookup  map[string]Module
}

// NewRegistry initializes and returns a new empty module Registry.
func NewRegistry() *Registry {
	return &Registry{
		modules: make([]Module, 0),
		lookup:  make(map[string]Module),
	}
}

// Register adds a new module to the registry. Returns ErrModuleExists if the ID is already registered.
func (r *Registry) Register(m Module) error {
	if m == nil {
		return ErrNilModule
	}

	id := m.ID()
	if id == "" {
		return errors.New("module ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.lookup[id]; exists {
		return fmt.Errorf("%w: %s", ErrModuleExists, id)
	}

	r.modules = append(r.modules, m)
	r.lookup[id] = m
	return nil
}

// Unregister removes a module by its ID. Returns true if removed, false if not found.
func (r *Registry) Unregister(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.lookup[id]; !exists {
		return false
	}

	delete(r.lookup, id)
	for i, m := range r.modules {
		if m.ID() == id {
			r.modules = append(r.modules[:i], r.modules[i+1:]...)
			break
		}
	}
	return true
}

// Get retrieves a module by ID.
func (r *Registry) Get(id string) (Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.lookup[id]
	return m, ok
}

// Modules returns a copy of all registered modules in registration order.
func (r *Registry) Modules() []Module {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Module, len(r.modules))
	copy(result, r.modules)
	return result
}

// AvailableModules returns all registered modules where IsAvailable() is true.
func (r *Registry) AvailableModules() []Module {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var available []Module
	for _, m := range r.modules {
		if m.IsAvailable() {
			available = append(available, m)
		}
	}
	return available
}

// ScanAll executes Scan concurrently across all available modules.
func (r *Registry) ScanAll(ctx context.Context) (map[string]ScanResult, error) {
	return r.ScanModules(ctx, r.AvailableModules())
}

// ScanModules executes Scan concurrently across the specified subset of modules.
func (r *Registry) ScanModules(ctx context.Context, modules []Module) (map[string]ScanResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	type scanResponse struct {
		id     string
		result ScanResult
	}

	results := make(map[string]ScanResult, len(modules))
	if len(modules) == 0 {
		return results, nil
	}

	ch := make(chan scanResponse, len(modules))
	var wg sync.WaitGroup

	for _, m := range modules {
		wg.Add(1)
		go func(mod Module) {
			defer wg.Done()

			// Panic guard for module isolation
			defer func() {
				if rec := recover(); rec != nil {
					ch <- scanResponse{
						id: mod.ID(),
						result: ScanResult{
							ScannedAt: time.Now(),
							Error:     fmt.Errorf("module scan panic: %v", rec),
						},
					}
				}
			}()

			res, err := mod.Scan(ctx)
			if err != nil && res.Error == nil {
				res.Error = err
			}
			if res.ScannedAt.IsZero() {
				res.ScannedAt = time.Now()
			}
			if res.TotalSize == 0 && len(res.Items) > 0 {
				res.RecalculateTotal()
			}

			ch <- scanResponse{
				id:     mod.ID(),
				result: res,
			}
		}(m)
	}

	wg.Wait()
	close(ch)

	for resp := range ch {
		results[resp.id] = resp.result
	}

	if err := ctx.Err(); err != nil {
		return results, err
	}

	return results, nil
}

// DefaultRegistry is the global default module registry used by standard adapters.
var DefaultRegistry = NewRegistry()

// Register registers a module with the default registry.
func Register(m Module) error {
	return DefaultRegistry.Register(m)
}

// Get retrieves a module by ID from the default registry.
func Get(id string) (Module, bool) {
	return DefaultRegistry.Get(id)
}

// Modules returns all registered modules from the default registry.
func Modules() []Module {
	return DefaultRegistry.Modules()
}

// AvailableModules returns all available modules from the default registry.
func AvailableModules() []Module {
	return DefaultRegistry.AvailableModules()
}

// ScanAll scans all available modules in the default registry.
func ScanAll(ctx context.Context) (map[string]ScanResult, error) {
	return DefaultRegistry.ScanAll(ctx)
}

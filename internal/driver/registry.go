package driver

import (
	"fmt"
	"sort"
	"sync"
)

// DriverFactory creates a new Driver instance.
type DriverFactory func() Driver

// registry manages registered driver factories.
type registry struct {
	mu        sync.RWMutex
	factories map[string]DriverFactory
}

var globalRegistry = &registry{
	factories: make(map[string]DriverFactory),
}

// Register registers a driver factory for the given type name.
// Typically called from init() in the driver package.
func Register(typeName string, factory DriverFactory) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	if _, exists := globalRegistry.factories[typeName]; exists {
		panic(fmt.Sprintf("driver already registered: %s", typeName))
	}
	globalRegistry.factories[typeName] = factory
}

// NewDriver creates a new unconnected Driver instance for the given type.
func NewDriver(typeName string) (Driver, error) {
	globalRegistry.mu.RLock()
	factory, ok := globalRegistry.factories[typeName]
	globalRegistry.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported datasource type: %s", typeName)
	}
	return factory(), nil
}

// SupportedTypes returns all registered type names in sorted order.
func SupportedTypes() []string {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	types := make([]string, 0, len(globalRegistry.factories))
	for t := range globalRegistry.factories {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// IsRegistered checks whether a type name has been registered.
func IsRegistered(typeName string) bool {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	_, ok := globalRegistry.factories[typeName]
	return ok
}

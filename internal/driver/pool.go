package driver

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// poolKey uniquely identifies a connection by datasource ID.
type poolKey int64

// PoolManager manages connected Driver instances.
// It replaces the old connpool.Manager with a unified driver-based approach.
type PoolManager struct {
	mu      sync.RWMutex
	entries map[poolKey]*poolEntry
}

type poolEntry struct {
	driver  Driver
	config  *Config
	lastUse time.Time
}

// NewPoolManager creates a new PoolManager.
func NewPoolManager() *PoolManager {
	return &PoolManager{
		entries: make(map[poolKey]*poolEntry),
	}
}

// Get returns a connected Driver for the given config.
// If no cached Driver exists, it creates and connects a new one.
func (pm *PoolManager) Get(ctx context.Context, cfg *Config) (Driver, error) {
	key := poolKey(cfg.ID)

	pm.mu.RLock()
	entry, ok := pm.entries[key]
	pm.mu.RUnlock()

	if ok {
		entry.lastUse = time.Now()
		return entry.driver, nil
	}

	// Create a new driver instance
	d, err := NewDriver(cfg.Extra["_type"].(string))
	if err != nil {
		return nil, err
	}

	if err := d.Connect(ctx, cfg); err != nil {
		return nil, fmt.Errorf("driver connect: %w", err)
	}

	pm.mu.Lock()
	pm.entries[key] = &poolEntry{
		driver:  d,
		config:  cfg,
		lastUse: time.Now(),
	}
	pm.mu.Unlock()

	return d, nil
}

// GetCached returns a cached Driver without creating a new one.
// Returns nil if no cached driver exists.
func (pm *PoolManager) GetCached(dsID int64) Driver {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if entry, ok := pm.entries[poolKey(dsID)]; ok {
		entry.lastUse = time.Now()
		return entry.driver
	}
	return nil
}

// Remove closes and removes the driver for the given datasource ID.
func (pm *PoolManager) Remove(dsID int64) {
	pm.mu.Lock()
	entry, ok := pm.entries[poolKey(dsID)]
	if ok {
		delete(pm.entries, poolKey(dsID))
	}
	pm.mu.Unlock()

	if entry != nil {
		_ = entry.driver.Close()
	}
}

// Close closes all managed drivers.
func (pm *PoolManager) Close() {
	pm.mu.Lock()
	entries := pm.entries
	pm.entries = make(map[poolKey]*poolEntry)
	pm.mu.Unlock()

	for _, entry := range entries {
		_ = entry.driver.Close()
	}
}

// InjectForTest injects a pre-connected driver for testing.
func (pm *PoolManager) InjectForTest(dsID int64, d Driver, cfg *Config) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.entries[poolKey(dsID)] = &poolEntry{
		driver:  d,
		config:  cfg,
		lastUse: time.Now(),
	}
}

// ManagedIDs returns all managed datasource IDs.
func (pm *PoolManager) ManagedIDs() []int64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	ids := make([]int64, 0, len(pm.entries))
	for k := range pm.entries {
		ids = append(ids, int64(k))
	}
	return ids
}

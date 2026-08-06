package driver

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// poolKey uniquely identifies a connection by datasource ID.
type poolKey int64

// PoolManager manages connected Driver instances.
// It replaces the old connpool.Manager with a unified driver-based approach.
type PoolManager struct {
	mu      sync.RWMutex
	entries map[poolKey]*poolEntry
}

// poolEntry holds one datasource's connected driver.
//
// It carried a lastUse timestamp and a copy of the config. Neither was ever
// read — there is no idle eviction — and lastUse was written from Get after the
// read lock was released and from GetCached while only holding it, so two
// concurrent lookups of the same datasource raced. Bookkeeping nothing consults
// is not worth a race.
type poolEntry struct {
	driver Driver
}

// NewPoolManager creates a new PoolManager.
func NewPoolManager() *PoolManager {
	return &PoolManager{
		entries: make(map[poolKey]*poolEntry),
	}
}

// Get returns a connected Driver for the given config.
// If no cached Driver exists, it creates and connects a new one.
//
// Connecting happens outside the lock, so concurrent callers can race to
// connect the same datasource; the write is guarded by a second lookup and the
// loser is closed. Without that, N callers missing the cache together left N-1
// connected drivers overwritten in the map — still holding sockets, no longer
// reachable by Remove or Close, which can only walk entries that are present.
// Saving a datasource evicts the entry, which is what produces the simultaneous
// miss in the first place.
//
// Holding the write lock across Connect would make it exactly one connection,
// but a slow or hanging target would then block every other datasource's
// lookups.
func (pm *PoolManager) Get(ctx context.Context, cfg *Config) (Driver, error) {
	key := poolKey(cfg.ID)

	pm.mu.RLock()
	entry, ok := pm.entries[key]
	pm.mu.RUnlock()

	if ok {
		return entry.driver, nil
	}

	dsType, ok := cfg.Extra["_type"].(string)
	if !ok || dsType == "" {
		return nil, fmt.Errorf("config for datasource %d carries no driver type", cfg.ID)
	}

	d, err := NewDriver(dsType)
	if err != nil {
		return nil, err
	}

	if err := d.Connect(ctx, cfg); err != nil {
		return nil, fmt.Errorf("driver connect: %w", err)
	}

	pm.mu.Lock()
	if existing, raced := pm.entries[key]; raced {
		pm.mu.Unlock()
		if err := d.Close(); err != nil {
			log.Printf("driver pool: close redundant %s connection for datasource %d: %v", dsType, cfg.ID, err)
		}
		return existing.driver, nil
	}
	pm.entries[key] = &poolEntry{driver: d}
	pm.mu.Unlock()

	return d, nil
}

// GetCached returns a cached Driver without creating a new one.
// Returns nil if no cached driver exists.
func (pm *PoolManager) GetCached(dsID int64) Driver {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if entry, ok := pm.entries[poolKey(dsID)]; ok {
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
		if err := entry.driver.Close(); err != nil {
			log.Printf("driver pool: close connection for datasource %d: %v", dsID, err)
		}
	}
}

// Close closes all managed drivers.
func (pm *PoolManager) Close() {
	pm.mu.Lock()
	entries := pm.entries
	pm.entries = make(map[poolKey]*poolEntry)
	pm.mu.Unlock()

	for key, entry := range entries {
		if err := entry.driver.Close(); err != nil {
			log.Printf("driver pool: close connection for datasource %d: %v", int64(key), err)
		}
	}
}

// InjectForTest injects a pre-connected driver for testing.
//
// It takes no Config: the manager keys on datasource ID alone and never reads
// the config back, so accepting one only invited callers to believe it mattered.
func (pm *PoolManager) InjectForTest(dsID int64, d Driver) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.entries[poolKey(dsID)] = &poolEntry{driver: d}
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

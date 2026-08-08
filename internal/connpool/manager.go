// Package connpool caches the raw Elasticsearch clients that index and field
// browsing need.
//
// It is deliberately small, and it used to be much larger: MySQL and MongoDB
// pools, a PostgreSQL pool config, ping helpers, a HealthCheck that walked maps
// nothing filled, three test-injection hooks and a Pool interface with no
// implementations — roughly two thirds of the package, none of it reachable
// from main, all of it kept alive past `make deadcode` by its own tests. Every
// connection path except this one goes through internal/driver.PoolManager.
//
// What survives is one genuine gap in the driver contract: GetESIndices and
// GetESIndexFields need paginated _cat/indices and raw mappings, which
// driver.MetadataBrowser does not model — it answers with TableInfo and
// ColumnInfo, not health, doc counts, store sizes or sub-fields. That gap means
// one cluster is reached through two independently-configured clients, and
// closing it properly means extending the driver contract rather than growing
// this package.
package connpool

import (
	"sync"
)

// Manager caches Elasticsearch clients by datasource.
type Manager struct {
	mu      sync.RWMutex
	esPools sync.Map // key: string → value: *es.Client
}

// NewManager creates a new connection pool Manager.
func NewManager() *Manager {
	return &Manager{}
}

// Close drops every cached client.
//
// The Elasticsearch client has no explicit close, so this only clears the
// cache. It exists so the composition root has one shutdown call to make.
func (m *Manager) Close() {
	m.esPools.Range(func(key, _ any) bool {
		m.esPools.Delete(key)
		return true
	})
}

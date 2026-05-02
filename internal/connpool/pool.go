package connpool

import "context"

// Pool is the generic connection pool interface.
type Pool interface {
	// Close releases all connections in the pool.
	Close() error
	// Ping verifies a connection to the database is still alive.
	Ping(ctx context.Context) error
}

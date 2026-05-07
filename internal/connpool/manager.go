package connpool

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Manager manages cached connection pools for MySQL and MongoDB datasources.
// It reuses *sql.DB / *mongo.Client instances instead of creating new connections per query.
type Manager struct {
	mu         sync.RWMutex
	mysqlPools sync.Map // key: string → value: *sql.DB
	mongoPools sync.Map // key: string → value: *mongo.Client
}

// NewManager creates a new connection pool Manager.
func NewManager() *Manager {
	return &Manager{}
}

// poolKey generates a unique cache key for a MySQL connection.
func poolKey(dsID int64, host string, port int, database string) string {
	return fmt.Sprintf("mysql:%d:%s:%d:%s", dsID, host, port, database)
}

// MySQLPoolConfig holds pool configuration from datasource settings.
type MySQLPoolConfig struct {
	MaxOpen     int
	MaxIdle     int
	MaxLifetime int // seconds
	MaxIdleTime int // seconds
}

// GetMySQL returns a cached *sql.DB for the given datasource parameters,
// creating and configuring one if it doesn't exist.
func (m *Manager) GetMySQL(dsID int64, host string, port int, user, password, database string, cfg MySQLPoolConfig) (*sql.DB, error) {
	key := poolKey(dsID, host, port, database)

	// Fast path: check cache
	if v, ok := m.mysqlPools.Load(key); ok {
		return v.(*sql.DB), nil
	}

	// Slow path: create new pool
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if v, ok := m.mysqlPools.Load(key); ok {
		return v.(*sql.DB), nil
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=30s&parseTime=true", user, password, host, port, database)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	// Apply pool settings
	maxOpen := cfg.MaxOpen
	if maxOpen <= 0 {
		maxOpen = 10
	}
	maxIdle := cfg.MaxIdle
	if maxIdle <= 0 {
		maxIdle = 5
	}
	maxLifetime := cfg.MaxLifetime
	if maxLifetime <= 0 {
		maxLifetime = 3600
	}
	maxIdleTime := cfg.MaxIdleTime
	if maxIdleTime <= 0 {
		maxIdleTime = 600
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Duration(maxLifetime) * time.Second)
	db.SetConnMaxIdleTime(time.Duration(maxIdleTime) * time.Second)

	m.mysqlPools.Store(key, db)
	return db, nil
}

// Remove removes a cached connection pool for the given datasource.
// This should be called when a datasource is updated or deleted.
func (m *Manager) Remove(dsID int64, host string, port int, database string) {
	key := poolKey(dsID, host, port, database)
	if v, ok := m.mysqlPools.LoadAndDelete(key); ok {
		v.(*sql.DB).Close()
	}
}

// Close closes all cached connection pools (MySQL and MongoDB).
func (m *Manager) Close() {
	m.mysqlPools.Range(func(key, value interface{}) bool {
		value.(*sql.DB).Close()
		m.mysqlPools.Delete(key)
		return true
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m.mongoPools.Range(func(key, value interface{}) bool {
		value.(*mongo.Client).Disconnect(ctx)
		m.mongoPools.Delete(key)
		return true
	})
}

// mongoPoolKey generates a unique cache key for a MongoDB connection.
func mongoPoolKey(dsID int64, uri string) string {
	return fmt.Sprintf("mongo:%d:%s", dsID, uri)
}

// GetMongoDB returns a cached *mongo.Client for the given datasource,
// creating and pinging one if it doesn't exist.
func (m *Manager) GetMongoDB(ctx context.Context, dsID int64, uri string) (*mongo.Client, error) {
	key := mongoPoolKey(dsID, uri)

	// Fast path: check cache
	if v, ok := m.mongoPools.Load(key); ok {
		return v.(*mongo.Client), nil
	}

	// Slow path: create new client
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if v, ok := m.mongoPools.Load(key); ok {
		return v.(*mongo.Client), nil
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(connectCtx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connect mongodb: %w", err)
	}

	if err := client.Ping(connectCtx, nil); err != nil {
		client.Disconnect(connectCtx)
		return nil, fmt.Errorf("ping mongodb: %w", err)
	}

	m.mongoPools.Store(key, client)
	return client, nil
}

// GetMongoDatabaseNames returns the list of database names using a cached MongoDB client.
// It creates and caches the client if not already present.
func (m *Manager) GetMongoDatabaseNames(ctx context.Context, dsID int64, uri string) ([]string, error) {
	client, err := m.GetMongoDB(ctx, dsID, uri)
	if err != nil {
		return nil, err
	}

	listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	names, err := client.ListDatabaseNames(listCtx, map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	return names, nil
}

// RemoveMongo removes all cached MongoDB clients for the given datasource ID.
func (m *Manager) RemoveMongo(dsID int64) {
	prefix := fmt.Sprintf("mongo:%d:", dsID)
	m.mongoPools.Range(func(key, value interface{}) bool {
		if strings.HasPrefix(key.(string), prefix) {
			m.mongoPools.Delete(key)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			value.(*mongo.Client).Disconnect(ctx)
			cancel()
		}
		return true
	})
}

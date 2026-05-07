package connpool

import (
	"context"
	"sync"
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
}

func TestManager_PoolKeyUniqueness(t *testing.T) {
	tests := []struct {
		dsID     int64
		host     string
		port     int
		database string
		want     string
	}{
		{1, "localhost", 3306, "testdb", "mysql:1:localhost:3306:testdb"},
		{2, "localhost", 3306, "testdb", "mysql:2:localhost:3306:testdb"},
		{1, "10.0.0.1", 3306, "testdb", "mysql:1:10.0.0.1:3306:testdb"},
		{1, "localhost", 3307, "testdb", "mysql:1:localhost:3307:testdb"},
		{1, "localhost", 3306, "otherdb", "mysql:1:localhost:3306:otherdb"},
	}

	for _, tt := range tests {
		got := poolKey(tt.dsID, tt.host, tt.port, tt.database)
		if got != tt.want {
			t.Errorf("poolKey(%d, %s, %d, %s) = %q, want %q", tt.dsID, tt.host, tt.port, tt.database, got, tt.want)
		}
	}

	// Verify different inputs produce different keys
	keys := make(map[string]bool)
	for _, tt := range tests {
		key := poolKey(tt.dsID, tt.host, tt.port, tt.database)
		if keys[key] {
			t.Errorf("duplicate key: %s", key)
		}
		keys[key] = true
	}
}

func TestManager_RemoveNonExistent(t *testing.T) {
	m := NewManager()
	// Remove on empty manager should not panic
	m.Remove(999, "localhost", 3306, "testdb")
}

func TestManager_CloseEmpty(t *testing.T) {
	m := NewManager()
	// Close on empty manager should not panic
	m.Close()
}

func TestManager_CloseTwice(t *testing.T) {
	m := NewManager()
	m.Close()
	// Second close should not panic
	m.Close()
}

func TestManager_ConcurrentAccess(t *testing.T) {
	m := NewManager()

	var wg sync.WaitGroup
	const goroutines = 100

	// Concurrent removes on non-existent keys
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			m.Remove(int64(idx), "localhost", 3306, "testdb")
		}(i)
	}

	wg.Wait()

	// Concurrent closes should not race
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Close()
		}()
	}
	wg.Wait()
}

func TestManager_MongoPoolKeyFormat(t *testing.T) {
	tests := []struct {
		dsID int64
		uri  string
		want string
	}{
		{1, "mongodb://localhost:27017", "mongo:1:mongodb://localhost:27017"},
		{2, "mongodb://user:pass@host:27017/db", "mongo:2:mongodb://user:pass@host:27017/db"},
		{1, "mongodb://10.0.0.1:27017", "mongo:1:mongodb://10.0.0.1:27017"},
	}
	for _, tt := range tests {
		got := mongoPoolKey(tt.dsID, tt.uri)
		if got != tt.want {
			t.Errorf("mongoPoolKey(%d, %s) = %q, want %q", tt.dsID, tt.uri, got, tt.want)
		}
	}
}

func TestManager_RemoveMongoNonExistent(t *testing.T) {
	m := NewManager()
	// RemoveMongo on empty manager should not panic
	m.RemoveMongo(999)
}

func TestManager_RemoveMongoConcurrent(t *testing.T) {
	m := NewManager()

	var wg sync.WaitGroup
	const goroutines = 100

	// Concurrent RemoveMongo on non-existent keys
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			m.RemoveMongo(int64(idx))
		}(i)
	}
	wg.Wait()
}

func TestManager_CloseWithMongoEmpty(t *testing.T) {
	m := NewManager()
	// Close with no MongoDB entries should not panic
	m.Close()
}

func TestManager_GetMongoDatabaseNamesInvalidURI(t *testing.T) {
	m := NewManager()
	// Invalid URI should return an error, not panic
	_, err := m.GetMongoDatabaseNames(context.Background(), 1, "invalid-uri")
	if err == nil {
		t.Error("expected error for invalid URI, got nil")
	}
}

func TestManager_GetMongoDatabaseNamesConcurrent(t *testing.T) {
	m := NewManager()

	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrent calls with invalid URIs should not race
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// These will all fail to connect, but should not race
			m.GetMongoDatabaseNames(context.Background(), int64(idx), "invalid-uri")
		}(i)
	}
	wg.Wait()
}

func TestManager_GetMongoDatabaseNamesAfterClose(t *testing.T) {
	m := NewManager()
	m.Close()
	// After close, should still be able to attempt new connections
	// (the sync.Map is cleared but GetMongoDB will create a new one)
	_, err := m.GetMongoDatabaseNames(context.Background(), 1, "invalid-uri")
	if err == nil {
		t.Error("expected error for invalid URI, got nil")
	}
}

// --- GetMySQL concurrent safety tests ---

func TestManager_GetMySQLConcurrentInvalidDSN(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrent GetMySQL with invalid credentials should not race.
	// Note: sql.Open does not validate DSN, so GetMySQL returns *sql.DB without error.
	// We verify no race/panic occurs.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			db, err := m.GetMySQL(int64(idx), "127.0.0.1", 3306, "nouser", "nopass", "nodb", MySQLPoolConfig{})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if db == nil {
				t.Error("expected non-nil *sql.DB")
			}
		}(i)
	}
	wg.Wait()
}

func TestManager_GetMySQLConcurrentSameKey(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup
	const goroutines = 50

	// All goroutines use the same key (dsID=1), so only the first one that
	// acquires the write lock will open a connection; others hit the cache.
	// With invalid credentials this will still error, but we verify no race.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.GetMySQL(1, "127.0.0.1", 3306, "nouser", "nopass", "nodb", MySQLPoolConfig{})
		}()
	}
	wg.Wait()
}

func TestManager_GetMySQLAfterClose(t *testing.T) {
	m := NewManager()
	m.Close()
	// After Close the pool is empty; a new GetMySQL should still create a *sql.DB.
	// sql.Open does not validate DSN, so it returns successfully.
	db, err := m.GetMySQL(1, "127.0.0.1", 3306, "nouser", "nopass", "nodb", MySQLPoolConfig{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if db == nil {
		t.Error("expected non-nil *sql.DB after close")
	}
}

func TestManager_GetMySQLThenClose(t *testing.T) {
	m := NewManager()
	// Open a pool (will fail with bad creds, but the *sql.DB is still created).
	db, err := m.GetMySQL(1, "127.0.0.1", 3306, "nouser", "nopass", "nodb", MySQLPoolConfig{})
	if err != nil {
		// sql.Open itself may succeed even with bad creds; only Ping fails later.
		// We just want to ensure Close doesn't panic.
		m.Close()
		return
	}
	// If we got a db, Close should clean it up without panic.
	m.Close()
	// Using db after close is UB, but we just verify no panic from Close itself.
	_ = db
}

func TestManager_ConcurrentGetMySQLAndClose(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup
	const goroutines = 50

	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			m.GetMySQL(int64(idx), "127.0.0.1", 3306, "nouser", "nopass", "nodb", MySQLPoolConfig{})
		}(i)
		go func() {
			defer wg.Done()
			m.Close()
		}()
	}
	wg.Wait()
}

// --- GetMongoDB concurrent safety tests ---

func TestManager_GetMongoDBConcurrentInvalidURI(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup
	const goroutines = 50

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := m.GetMongoDB(context.Background(), int64(idx), "invalid-uri")
			if err == nil {
				t.Error("expected error for invalid MongoDB URI, got nil")
			}
		}(i)
	}
	wg.Wait()
}

func TestManager_GetMongoDBConcurrentSameKey(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup
	const goroutines = 50

	// All goroutines use the same key (dsID=1).
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.GetMongoDB(context.Background(), 1, "invalid-uri")
		}()
	}
	wg.Wait()
}

func TestManager_GetMongoDBAfterClose(t *testing.T) {
	m := NewManager()
	m.Close()
	_, err := m.GetMongoDB(context.Background(), 1, "invalid-uri")
	if err == nil {
		t.Error("expected error for invalid MongoDB URI after close, got nil")
	}
}

func TestManager_ConcurrentGetMongoDBAndClose(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup
	const goroutines = 50

	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			m.GetMongoDB(context.Background(), int64(idx), "invalid-uri")
		}(i)
		go func() {
			defer wg.Done()
			m.Close()
		}()
	}
	wg.Wait()
}

// --- Mixed concurrent MySQL + MongoDB + Close ---

func TestManager_ConcurrentMixedAccess(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup
	const goroutines = 30

	for i := 0; i < goroutines; i++ {
		wg.Add(3)
		go func(idx int) {
			defer wg.Done()
			m.GetMySQL(int64(idx), "127.0.0.1", 3306, "nouser", "nopass", "nodb", MySQLPoolConfig{})
		}(i)
		go func(idx int) {
			defer wg.Done()
			m.GetMongoDB(context.Background(), int64(idx), "invalid-uri")
		}(i)
		go func() {
			defer wg.Done()
			m.Close()
		}()
	}
	wg.Wait()
}

// --- Remove after failed GetMySQL ---

func TestManager_RemoveAfterGetMySQL(t *testing.T) {
	m := NewManager()
	// Even if GetMySQL failed to actually connect, Remove should not panic.
	m.Remove(1, "127.0.0.1", 3306, "nodb")
}

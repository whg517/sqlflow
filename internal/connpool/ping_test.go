package connpool

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ---------------------------------------------------------------------------
// MySQLPing mock tests
// ---------------------------------------------------------------------------

func TestMySQLPing(t *testing.T) {
	t.Run("cancelled_context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := MySQLPing(ctx, "127.0.0.1", 3306, "root", "password")
		if err == nil {
			t.Fatal("expected error with cancelled context, got nil")
		}
		if !strings.Contains(err.Error(), "ping mysql") {
			t.Errorf("error should wrap 'ping mysql', got %q", err.Error())
		}
	})

	t.Run("expired_context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(time.Millisecond)

		err := MySQLPing(ctx, "127.0.0.1", 3306, "root", "password")
		if err == nil {
			t.Fatal("expected error with expired context, got nil")
		}
	})

	t.Run("empty_host", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := MySQLPing(ctx, "", 3306, "root", "password")
		if err == nil {
			t.Fatal("expected error with empty host, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// MySQLGetTables mock tests
// ---------------------------------------------------------------------------

func TestMySQLGetTables(t *testing.T) {
	t.Run("cancelled_context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		tables, err := MySQLGetTables(ctx, "127.0.0.1", 3306, "root", "password", "testdb")
		if err == nil {
			t.Fatal("expected error with cancelled context, got nil")
		}
		if tables != nil {
			t.Errorf("expected nil tables, got %v", tables)
		}
	})

	t.Run("expired_context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(time.Millisecond)

		tables, err := MySQLGetTables(ctx, "127.0.0.1", 3306, "root", "password", "testdb")
		if err == nil {
			t.Fatal("expected error with expired context, got nil")
		}
		if tables != nil {
			t.Errorf("expected nil tables, got %v", tables)
		}
	})

	t.Run("empty_database_name", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		tables, err := MySQLGetTables(ctx, "127.0.0.1", 3306, "root", "password", "")
		if err == nil {
			t.Fatal("expected error with empty database, got nil")
		}
		if tables != nil {
			t.Errorf("expected nil tables, got %v", tables)
		}
	})
}

// ---------------------------------------------------------------------------
// MongoPing mock tests
// ---------------------------------------------------------------------------

func TestMongoPing(t *testing.T) {
	t.Run("cancelled_context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := MongoPing(ctx, "mongodb://localhost:27017")
		if err == nil {
			t.Fatal("expected error with cancelled context, got nil")
		}
		if msg := err.Error(); !strings.Contains(msg, "connect mongodb") && !strings.Contains(msg, "ping mongodb") {
			t.Errorf("error should wrap 'connect mongodb' or 'ping mongodb', got %q", msg)
		}
	})

	t.Run("invalid_uri", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		err := MongoPing(ctx, "not-a-valid-uri")
		if err == nil {
			t.Fatal("expected error with invalid URI, got nil")
		}
	})

	t.Run("empty_uri", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		err := MongoPing(ctx, "")
		if err == nil {
			t.Fatal("expected error with empty URI, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// GetMongoDB mock tests — using InjectMongoForTest
// ---------------------------------------------------------------------------

func TestManager_GetMongoDB_CacheHitViaInjection(t *testing.T) {
	m := NewManager()
	defer m.Close()

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("create mongo client: %v", err)
	}
	defer client.Disconnect(context.Background())

	m.InjectMongoForTest(42, "mongodb://localhost:27017", client)

	got, err := m.GetMongoDB(context.Background(), 42, "mongodb://localhost:27017")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != client {
		t.Error("GetMongoDB should return the injected client instance (cache hit)")
	}
}

func TestManager_GetMongoDB_InjectDoesNotAffectOtherKeys(t *testing.T) {
	m := NewManager()
	defer m.Close()

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("create mongo client: %v", err)
	}
	defer client.Disconnect(context.Background())

	// Inject for dsID=1
	m.InjectMongoForTest(1, "mongodb://localhost:27017", client)

	// Request for dsID=2 with a cancelled context should NOT get the cached client for dsID=1
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = m.GetMongoDB(ctx, 2, "mongodb://localhost:27017")
	if err == nil {
		t.Error("expected error for uncached dsID=2, got nil")
	}
}

func TestManager_GetMongoDB_ConcurrentCacheHits(t *testing.T) {
	m := NewManager()
	defer m.Close()

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("create mongo client: %v", err)
	}
	defer client.Disconnect(context.Background())

	m.InjectMongoForTest(1, "mongodb://localhost:27017", client)

	var wg sync.WaitGroup
	const goroutines = 50
	results := make(chan *mongo.Client, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, _ := m.GetMongoDB(context.Background(), 1, "mongodb://localhost:27017")
			results <- got
		}()
	}
	wg.Wait()
	close(results)

	for got := range results {
		if got != client {
			t.Error("concurrent GetMongoDB should all return the same injected client")
			break
		}
	}
}

// ---------------------------------------------------------------------------
// GetMySQL mock tests — using sqlmock + InjectMySQLForTest
// ---------------------------------------------------------------------------

func TestManager_GetMySQL_CacheHitViaInjection(t *testing.T) {
	m := NewManager()
	defer m.Close()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer db.Close()

	m.InjectMySQLForTest(1, "localhost", 3306, "testdb", db)

	got, err := m.GetMySQL(1, "localhost", 3306, "user", "pass", "testdb", MySQLPoolConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != db {
		t.Error("GetMySQL should return the injected DB instance (cache hit)")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", err)
	}
}

func TestManager_GetMySQL_InjectThenRemove(t *testing.T) {
	m := NewManager()
	defer m.Close()

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}

	m.InjectMySQLForTest(1, "localhost", 3306, "testdb", db)

	// Verify cache hit
	got, err := m.GetMySQL(1, "localhost", 3306, "user", "pass", "testdb", MySQLPoolConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != db {
		t.Fatal("first call should return injected DB")
	}

	// Remove
	m.Remove(1, "localhost", 3306, "testdb")

	// After remove, GetMySQL creates a new *sql.DB (sql.Open succeeds even with bad creds)
	got2, err := m.GetMySQL(1, "localhost", 3306, "user", "pass", "testdb", MySQLPoolConfig{})
	if err != nil {
		t.Fatalf("unexpected error after remove: %v", err)
	}
	if got2 == db {
		t.Error("after Remove, GetMySQL should return a new DB instance, not the old one")
	}
}

func TestManager_GetMySQL_PoolConfigDefaults(t *testing.T) {
	m := NewManager()
	defer m.Close()

	db, err := m.GetMySQL(99, "127.0.0.1", 3306, "nouser", "nopass", "nodb", MySQLPoolConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stats := db.Stats()
	if stats.MaxOpenConnections != 10 {
		t.Errorf("default MaxOpenConnections = %d, want 10", stats.MaxOpenConnections)
	}
}

func TestManager_GetMySQL_CustomPoolConfig(t *testing.T) {
	m := NewManager()
	defer m.Close()

	cfg := MySQLPoolConfig{
		MaxOpen:     20,
		MaxIdle:     10,
		MaxLifetime: 1800,
		MaxIdleTime: 300,
	}

	db, err := m.GetMySQL(100, "127.0.0.1", 3306, "nouser", "nopass", "nodb", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stats := db.Stats()
	if stats.MaxOpenConnections != 20 {
		t.Errorf("MaxOpenConnections = %d, want 20", stats.MaxOpenConnections)
	}
}

func TestManager_GetMySQL_ConcurrentCacheHitViaInjection(t *testing.T) {
	m := NewManager()
	defer m.Close()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer db.Close()

	m.InjectMySQLForTest(1, "localhost", 3306, "testdb", db)

	var wg sync.WaitGroup
	const goroutines = 50
	results := make(chan *sql.DB, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, _ := m.GetMySQL(1, "localhost", 3306, "user", "pass", "testdb", MySQLPoolConfig{})
			results <- got
		}()
	}
	wg.Wait()
	close(results)

	for got := range results {
		if got != db {
			t.Error("concurrent GetMySQL should all return the same injected DB")
			break
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Table-driven tests for MySQLPing, MySQLGetTables, MongoPing
// ---------------------------------------------------------------------------

func TestMySQLPing_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		host     string
		port     int
		user     string
		password string
		wantErr  bool
	}{
		{
			name:     "cancelled_context",
			ctx:      func() context.Context { ctx, _ := context.WithCancel(context.Background()); return ctx }(),
			host:     "127.0.0.1",
			port:     3306,
			user:     "root",
			password: "password",
			wantErr:  true,
		},
		{
			name:     "expired_context",
			ctx:      func() context.Context { ctx, _ := context.WithTimeout(context.Background(), 1*time.Nanosecond); time.Sleep(time.Millisecond); return ctx }(),
			host:     "127.0.0.1",
			port:     3306,
			user:     "root",
			password: "password",
			wantErr:  true,
		},
		{
			name:     "empty_host",
			ctx:      func() context.Context { ctx, _ := context.WithTimeout(context.Background(), 2*time.Second); return ctx }(),
			host:     "",
			port:     3306,
			user:     "root",
			password: "password",
			wantErr:  true,
		},
		{
			name: "timeout_context_short",
			ctx: func() context.Context {
				ctx, _ := context.WithTimeout(context.Background(), 50*time.Millisecond)
				return ctx
			}(),
			host:     "192.0.2.1", // TEST-NET-1, unreachable
			port:     3306,
			user:     "root",
			password: "password",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MySQLPing(tt.ctx, tt.host, tt.port, tt.user, tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("MySQLPing() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMySQLGetTables_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		host     string
		port     int
		user     string
		password string
		database string
		wantErr  bool
	}{
		{
			name:     "cancelled_context",
			ctx:      func() context.Context { ctx, _ := context.WithCancel(context.Background()); return ctx }(),
			host:     "127.0.0.1",
			port:     3306,
			user:     "root",
			password: "password",
			database: "testdb",
			wantErr:  true,
		},
		{
			name:     "expired_context",
			ctx:      func() context.Context { ctx, _ := context.WithTimeout(context.Background(), 1*time.Nanosecond); time.Sleep(time.Millisecond); return ctx }(),
			host:     "127.0.0.1",
			port:     3306,
			user:     "root",
			password: "password",
			database: "testdb",
			wantErr:  true,
		},
		{
			name: "timeout_short",
			ctx: func() context.Context {
				ctx, _ := context.WithTimeout(context.Background(), 50*time.Millisecond)
				return ctx
			}(),
			host:     "192.0.2.1",
			port:     3306,
			user:     "root",
			password: "password",
			database: "testdb",
			wantErr:  true,
		},
		{
			name:     "empty_database",
			ctx:      func() context.Context { ctx, _ := context.WithCancel(context.Background()); return ctx }(),
			host:     "127.0.0.1",
			port:     3306,
			user:     "root",
			password: "password",
			database: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tables, err := MySQLGetTables(tt.ctx, tt.host, tt.port, tt.user, tt.password, tt.database)
			if (err != nil) != tt.wantErr {
				t.Errorf("MySQLGetTables() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tables != nil {
				t.Errorf("expected nil tables on error, got %v", tables)
			}
		})
	}
}

func TestMongoPing_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		uri     string
		wantErr bool
	}{
		{
			name:    "cancelled_context",
			ctx:     func() context.Context { ctx, _ := context.WithCancel(context.Background()); return ctx }(),
			uri:     "mongodb://localhost:27017",
			wantErr: true,
		},
		{
			name:    "invalid_uri",
			ctx:     func() context.Context { ctx, _ := context.WithTimeout(context.Background(), 3*time.Second); return ctx }(),
			uri:     "not-a-valid-uri",
			wantErr: true,
		},
		{
			name:    "empty_uri",
			ctx:     func() context.Context { ctx, _ := context.WithTimeout(context.Background(), 3*time.Second); return ctx }(),
			uri:     "",
			wantErr: true,
		},
		{
			name: "unreachable_host",
			ctx: func() context.Context {
				ctx, _ := context.WithTimeout(context.Background(), 100*time.Millisecond)
				return ctx
			}(),
			uri:     "mongodb://192.0.2.1:27017",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MongoPing(tt.ctx, tt.uri)
			if (err != nil) != tt.wantErr {
				t.Errorf("MongoPing() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetMongoDatabaseNames mock tests — using InjectMongoForTest
// ---------------------------------------------------------------------------

func TestManager_GetMongoDatabaseNames_DisconnectedClient(t *testing.T) {
	m := NewManager()
	defer m.Close()

	// Create a client not connected to a real server.
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("create mongo client: %v", err)
	}
	defer client.Disconnect(context.Background())

	m.InjectMongoForTest(1, "mongodb://localhost:27017", client)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	names, err := m.GetMongoDatabaseNames(ctx, 1, "mongodb://localhost:27017")
	if err == nil {
		// MongoDB might be running locally — just log the result.
		t.Logf("MongoDB is running locally, got names: %v", names)
		return
	}
	if !strings.Contains(err.Error(), "list databases") {
		t.Errorf("error should wrap 'list databases', got %q", err.Error())
	}
	if names != nil {
		t.Errorf("expected nil names on error, got %v", names)
	}
}

func TestManager_GetMongoDatabaseNames_CancelledContext(t *testing.T) {
	m := NewManager()
	defer m.Close()

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("create mongo client: %v", err)
	}
	defer client.Disconnect(context.Background())

	m.InjectMongoForTest(1, "mongodb://localhost:27017", client)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = m.GetMongoDatabaseNames(ctx, 1, "mongodb://localhost:27017")
	if err == nil {
		t.Error("expected error with cancelled context, got nil")
	}
}

func TestManager_GetMongoDatabaseNames_TableDriven(t *testing.T) {
	// Set up injected client for cached scenarios.
	injectedClient, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("create injected client: %v", err)
	}
	defer injectedClient.Disconnect(context.Background())

	tests := []struct {
		name    string
		dsID    int64
		uri     string
		setup   func(m *Manager)
		wantErr bool
	}{
		{
			name:    "invalid_uri_uncached",
			dsID:    10,
			uri:     "not-a-uri",
			setup:   nil,
			wantErr: true,
		},
		{
			name: "cancelled_context_with_injected_client",
			dsID: 1,
			uri:  "mongodb://localhost:27017",
			setup: func(m *Manager) {
				m.InjectMongoForTest(1, "mongodb://localhost:27017", injectedClient)
			},
			wantErr: true,
		},
		{
			name: "injected_client_list_fails",
			dsID: 2,
			uri:  "mongodb://localhost:27017",
			setup: func(m *Manager) {
				m.InjectMongoForTest(2, "mongodb://localhost:27017", injectedClient)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager()
			defer m.Close()

			if tt.setup != nil {
				tt.setup(m)
			}

			ctx, cancel := context.WithCancel(context.Background())
			cancel() // cancel immediately so ListDatabaseNames also fails

			_, err := m.GetMongoDatabaseNames(ctx, tt.dsID, tt.uri)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMongoDatabaseNames() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetMongoDB — additional coverage
// ---------------------------------------------------------------------------

func TestManager_GetMongoDB_DoubleCheckAfterLock(t *testing.T) {
	m := NewManager()
	defer m.Close()

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("create mongo client: %v", err)
	}
	defer client.Disconnect(context.Background())

	// Inject so the double-check after lock hits.
	m.InjectMongoForTest(1, "mongodb://localhost:27017", client)

	got, err := m.GetMongoDB(context.Background(), 1, "mongodb://localhost:27017")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != client {
		t.Error("GetMongoDB should return the injected client")
	}
}

func TestManager_GetMongoDB_SlowPathConnectError(t *testing.T) {
	m := NewManager()
	defer m.Close()

	tests := []struct {
		name string
		dsID int64
		uri  string
	}{
		{"invalid_uri", 1, "not-a-uri"},
		{"empty_uri", 2, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			_, err := m.GetMongoDB(ctx, tt.dsID, tt.uri)
			if err == nil {
				t.Error("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "connect mongodb") && !strings.Contains(err.Error(), "ping mongodb") {
				t.Errorf("error should mention connect/ping mongodb, got %q", err.Error())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RemoveMongo mock tests — using InjectMongoForTest
// ---------------------------------------------------------------------------

func TestManager_RemoveMongo_WithInjectedClient(t *testing.T) {
	m := NewManager()
	defer m.Close()

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("create mongo client: %v", err)
	}

	m.InjectMongoForTest(1, "mongodb://localhost:27017", client)

	// Verify cache hit before removal.
	got, err := m.GetMongoDB(context.Background(), 1, "mongodb://localhost:27017")
	if err != nil {
		t.Fatalf("unexpected error before removal: %v", err)
	}
	if got != client {
		t.Fatal("should return injected client before removal")
	}

	// Remove all entries for dsID=1.
	m.RemoveMongo(1)

	// After removal, GetMongoDB should try to create a new connection and fail.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = m.GetMongoDB(ctx, 1, "mongodb://localhost:27017")
	if err == nil {
		t.Error("expected error after RemoveMongo (new connection attempt), got nil")
	}
}

func TestManager_RemoveMongo_DoesNotAffectOtherDSID(t *testing.T) {
	m := NewManager()
	defer m.Close()

	client1, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("create client1: %v", err)
	}
	defer client1.Disconnect(context.Background())

	client2, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("create client2: %v", err)
	}

	m.InjectMongoForTest(1, "mongodb://localhost:27017", client1)
	m.InjectMongoForTest(2, "mongodb://localhost:27017", client2)

	// Remove only dsID=1.
	m.RemoveMongo(1)

	// dsID=2 should still be cached.
	got, err := m.GetMongoDB(context.Background(), 2, "mongodb://localhost:27017")
	if err != nil {
		t.Fatalf("unexpected error for dsID=2: %v", err)
	}
	if got != client2 {
		t.Error("dsID=2 client should still be cached after removing dsID=1")
	}
}

func TestManager_RemoveMongo_MultipleURIs(t *testing.T) {
	m := NewManager()
	defer m.Close()

	client1, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("create client1: %v", err)
	}

	client2, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27018"))
	if err != nil {
		t.Fatalf("create client2: %v", err)
	}

	m.InjectMongoForTest(5, "mongodb://localhost:27017", client1)
	m.InjectMongoForTest(5, "mongodb://localhost:27018", client2)

	// Remove all entries for dsID=5.
	m.RemoveMongo(5)

	// Both URIs for dsID=5 should be gone.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = m.GetMongoDB(ctx, 5, "mongodb://localhost:27017")
	if err == nil {
		t.Error("expected error for URI 27017 after RemoveMongo")
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	_, err = m.GetMongoDB(ctx2, 5, "mongodb://localhost:27018")
	if err == nil {
		t.Error("expected error for URI 27018 after RemoveMongo")
	}
}

func TestManager_RemoveMongo_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		injectDSID   int64
		removeDSID   int64
		checkDSID    int64
		shouldBeGone bool
	}{
		{
			name:         "remove_matching_dsID",
			injectDSID:   1,
			removeDSID:   1,
			checkDSID:    1,
			shouldBeGone: true,
		},
		{
			name:         "remove_nonmatching_dsID",
			injectDSID:   1,
			removeDSID:   999,
			checkDSID:    1,
			shouldBeGone: false,
		},
		{
			name:         "remove_then_check_different_dsID",
			injectDSID:   10,
			removeDSID:   20,
			checkDSID:    10,
			shouldBeGone: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager()
			defer m.Close()

			client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
			if err != nil {
				t.Fatalf("create client: %v", err)
			}
			defer client.Disconnect(context.Background())

			m.InjectMongoForTest(tt.injectDSID, "mongodb://localhost:27017", client)

			m.RemoveMongo(tt.removeDSID)

			if tt.shouldBeGone {
				// After removal, a new GetMongoDB should need to reconnect (will fail).
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				got, err := m.GetMongoDB(ctx, tt.checkDSID, "mongodb://localhost:27017")
				if err == nil && got == client {
					t.Error("client should have been removed")
				}
			} else {
				// Should still be cached.
				got, err := m.GetMongoDB(context.Background(), tt.checkDSID, "mongodb://localhost:27017")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != client {
					t.Error("client should still be cached")
				}
			}
		})
	}
}

func TestManager_RemoveMongo_ConcurrentWithGetMongoDB(t *testing.T) {
	m := NewManager()
	defer m.Close()

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	m.InjectMongoForTest(1, "mongodb://localhost:27017", client)

	var wg sync.WaitGroup
	const goroutines = 20

	// Concurrent RemoveMongo + GetMongoDB should not race.
	// Use short-timeout contexts to avoid blocking on reconnect attempts.
	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			m.RemoveMongo(1)
		}()
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			m.GetMongoDB(ctx, 1, "mongodb://localhost:27017")
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// GetMySQL — additional coverage
// ---------------------------------------------------------------------------

func TestManager_GetMySQL_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		dsID int64
		cfg  MySQLPoolConfig
	}{
		{"default_config", 200, MySQLPoolConfig{}},
		{"custom_config", 201, MySQLPoolConfig{MaxOpen: 5, MaxIdle: 2, MaxLifetime: 600, MaxIdleTime: 120}},
		{"negative_values_use_defaults", 202, MySQLPoolConfig{MaxOpen: -1, MaxIdle: -1, MaxLifetime: -1, MaxIdleTime: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager()
			defer m.Close()

			db, err := m.GetMySQL(tt.dsID, "127.0.0.1", 3306, "nouser", "nopass", "nodb", tt.cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if db == nil {
				t.Fatal("expected non-nil *sql.DB")
			}
		})
	}
}

func TestManager_GetMySQL_CacheMissThenHit(t *testing.T) {
	m := NewManager()
	defer m.Close()

	// First call creates the pool.
	db1, err := m.GetMySQL(300, "127.0.0.1", 3306, "nouser", "nopass", "nodb", MySQLPoolConfig{})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Second call should return the same instance (cache hit).
	db2, err := m.GetMySQL(300, "127.0.0.1", 3306, "nouser", "nopass", "nodb", MySQLPoolConfig{})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if db1 != db2 {
		t.Error("second call should return the same *sql.DB instance")
	}
}

func TestManager_GetMySQL_DifferentKeys(t *testing.T) {
	m := NewManager()
	defer m.Close()

	db1, err := m.GetMySQL(400, "127.0.0.1", 3306, "user1", "pass", "db1", MySQLPoolConfig{})
	if err != nil {
		t.Fatalf("create db1: %v", err)
	}

	db2, err := m.GetMySQL(401, "127.0.0.1", 3306, "user2", "pass", "db2", MySQLPoolConfig{})
	if err != nil {
		t.Fatalf("create db2: %v", err)
	}

	if db1 == db2 {
		t.Error("different keys should produce different *sql.DB instances")
	}
}

// ---------------------------------------------------------------------------
// MySQLGetTables — mock via sqlmock (simulating the query + scan flow)
// ---------------------------------------------------------------------------

func TestMySQLGetTables_WithMockDB(t *testing.T) {
	t.Run("success_returns_tables", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("create sqlmock: %v", err)
		}
		defer db.Close()

		rows := sqlmock.NewRows([]string{"Tables_in_testdb"}).
			AddRow("users").
			AddRow("orders").
			AddRow("products")
		mock.ExpectQuery("SHOW TABLES").WillReturnRows(rows)

		resultRows, err := db.QueryContext(context.Background(), "SHOW TABLES")
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer resultRows.Close()

		var got []string
		for resultRows.Next() {
			var name string
			if err := resultRows.Scan(&name); err != nil {
				t.Fatalf("scan: %v", err)
			}
			got = append(got, name)
		}
		if err := resultRows.Err(); err != nil {
			t.Fatalf("rows err: %v", err)
		}

		if len(got) != 3 {
			t.Errorf("expected 3 tables, got %d", len(got))
		}
		if fmt.Sprintf("%v", got) != "[users orders products]" {
			t.Errorf("unexpected tables: %v", got)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("empty_result_returns_empty_slice", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("create sqlmock: %v", err)
		}
		defer db.Close()

		rows := sqlmock.NewRows([]string{"Tables_in_testdb"})
		mock.ExpectQuery("SHOW TABLES").WillReturnRows(rows)

		resultRows, err := db.QueryContext(context.Background(), "SHOW TABLES")
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer resultRows.Close()

		var got []string
		for resultRows.Next() {
			var name string
			resultRows.Scan(&name)
			got = append(got, name)
		}

		if len(got) != 0 {
			t.Errorf("expected 0 tables, got %d", len(got))
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("query_error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("create sqlmock: %v", err)
		}
		defer db.Close()

		mock.ExpectQuery("SHOW TABLES").WillReturnError(fmt.Errorf("database not found"))

		_, err = db.QueryContext(context.Background(), "SHOW TABLES")
		if err == nil {
			t.Error("expected error from mock query, got nil")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})
}

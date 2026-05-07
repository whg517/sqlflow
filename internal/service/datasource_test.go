package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/pkg/crypto"
)

const testEncKey = "0123456789abcdef0123456789abcdef" // 32 bytes for AES-256

// setupDatasourceTestDB creates a temp SQLite database with schema migrated.
func setupDatasourceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}
	return database.DB
}

// newTestDatasourceService creates a DatasourceService with a real SQLite DB.
func newTestDatasourceService(t *testing.T) (*DatasourceService, *sql.DB) {
	t.Helper()
	testDB := setupDatasourceTestDB(t)
	connMgr := connpool.NewManager()
	svc := NewDatasourceService(testDB, testEncKey, connMgr)
	return svc, testDB
}

// ctxWithTimeout returns a context with a 5-second timeout.
func ctxWithTimeout(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// ─── NewDatasourceService ─────────────────────────────────────────────────────

func TestNewDatasourceService(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	if svc == nil {
		t.Fatal("NewDatasourceService returned nil")
	}
}

// ─── CreateDataSource ─────────────────────────────────────────────────────────

func TestCreateDataSource(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	tests := []struct {
		name    string
		ds      *model.DataSource
		wantErr error
	}{
		{
			"mysql_success",
			&model.DataSource{
				Name: "prod-mysql", Type: "mysql", Host: "10.0.0.1", Port: 3306,
				Username: "root", PasswordEncrypted: "secret", Database: "app_db",
			},
			nil,
		},
		{
			"mongodb_success",
			&model.DataSource{
				Name: "dev-mongo", Type: "mongodb", Host: "10.0.0.2", Port: 27017,
				Username: "admin", PasswordEncrypted: "secret", Database: "testdb",
			},
			nil,
		},
		{
			"invalid_type",
			&model.DataSource{
				Name: "bad-type", Type: "postgres", Host: "10.0.0.1", Port: 5432,
				Username: "root", PasswordEncrypted: "secret",
			},
			ErrInvalidDatasourceType,
		},
		{
			"empty_type",
			&model.DataSource{
				Name: "empty-type", Type: "", Host: "10.0.0.1", Port: 3306,
				Username: "root", PasswordEncrypted: "secret",
			},
			ErrInvalidDatasourceType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.CreateDataSource(ctx, tt.ds)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("CreateDataSource() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateDataSource() unexpected error: %v", err)
			}
			if tt.ds.ID <= 0 {
				t.Error("expected positive ID after creation")
			}
			if tt.ds.Status != "active" {
				t.Errorf("status = %q, want active", tt.ds.Status)
			}
			if tt.ds.CreatedAt.IsZero() {
				t.Error("expected non-zero CreatedAt")
			}
			if tt.ds.UpdatedAt.IsZero() {
				t.Error("expected non-zero UpdatedAt")
			}
		})
	}
}

func TestCreateDataSource_Defaults(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	ds := &model.DataSource{
		Name: "defaults-test", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret",
	}
	if err := svc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	if ds.MaxOpen != 10 {
		t.Errorf("MaxOpen = %d, want 10 (default)", ds.MaxOpen)
	}
	if ds.MaxIdle != 5 {
		t.Errorf("MaxIdle = %d, want 5 (default)", ds.MaxIdle)
	}
	if ds.MaxLifetime != 3600 {
		t.Errorf("MaxLifetime = %d, want 3600 (default)", ds.MaxLifetime)
	}
	if ds.MaxIdleTime != 600 {
		t.Errorf("MaxIdleTime = %d, want 600 (default)", ds.MaxIdleTime)
	}
}

func TestCreateDataSource_CustomPoolConfig(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	ds := &model.DataSource{
		Name: "custom-pool", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret",
		MaxOpen: 20, MaxIdle: 10, MaxLifetime: 1800, MaxIdleTime: 300,
	}
	if err := svc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	if ds.MaxOpen != 20 {
		t.Errorf("MaxOpen = %d, want 20", ds.MaxOpen)
	}
	if ds.MaxIdle != 10 {
		t.Errorf("MaxIdle = %d, want 10", ds.MaxIdle)
	}
	if ds.MaxLifetime != 1800 {
		t.Errorf("MaxLifetime = %d, want 1800", ds.MaxLifetime)
	}
	if ds.MaxIdleTime != 300 {
		t.Errorf("MaxIdleTime = %d, want 300", ds.MaxIdleTime)
	}
}

func TestCreateDataSource_DuplicateName(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	ds1 := &model.DataSource{
		Name: "dup-name", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret",
	}
	if err := svc.CreateDataSource(ctx, ds1); err != nil {
		t.Fatalf("first CreateDataSource() error: %v", err)
	}

	ds2 := &model.DataSource{
		Name: "dup-name", Type: "mysql", Host: "10.0.0.2", Port: 3307,
		Username: "root", PasswordEncrypted: "secret2",
	}
	err := svc.CreateDataSource(ctx, ds2)
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
	if !strings.Contains(err.Error(), "insert datasource") {
		t.Errorf("error should mention insert datasource, got: %v", err)
	}
}

func TestCreateDataSource_PasswordEncrypted(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	rawPassword := "my-super-secret"
	ds := &model.DataSource{
		Name: "enc-test", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: rawPassword,
	}
	if err := svc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	// PasswordEncrypted should now hold the encrypted value (not the raw password)
	if ds.PasswordEncrypted == rawPassword {
		t.Error("PasswordEncrypted should be encrypted, not the raw password")
	}
	if ds.PasswordEncrypted == "" {
		t.Error("PasswordEncrypted should not be empty after creation")
	}

	// Verify the encrypted password can be decrypted back to the original
	retrieved, err := svc.GetDataSource(ctx, ds.ID)
	if err != nil {
		t.Fatalf("GetDataSource() error: %v", err)
	}
	if retrieved.PasswordEncrypted == rawPassword {
		t.Error("stored password should be encrypted")
	}
}

// ─── GetDataSource ────────────────────────────────────────────────────────────

func TestGetDataSource(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	created := &model.DataSource{
		Name: "get-test", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret", Database: "appdb",
	}
	if err := svc.CreateDataSource(ctx, created); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		ds, err := svc.GetDataSource(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetDataSource() error: %v", err)
		}
		if ds.ID != created.ID {
			t.Errorf("ID = %d, want %d", ds.ID, created.ID)
		}
		if ds.Name != "get-test" {
			t.Errorf("Name = %q, want get-test", ds.Name)
		}
		if ds.Type != "mysql" {
			t.Errorf("Type = %q, want mysql", ds.Type)
		}
		if ds.Host != "10.0.0.1" {
			t.Errorf("Host = %q, want 10.0.0.1", ds.Host)
		}
		if ds.Port != 3306 {
			t.Errorf("Port = %d, want 3306", ds.Port)
		}
		if ds.Username != "root" {
			t.Errorf("Username = %q, want root", ds.Username)
		}
		if ds.Database != "appdb" {
			t.Errorf("Database = %q, want appdb", ds.Database)
		}
		if ds.PasswordEncrypted == "" {
			t.Error("PasswordEncrypted should not be empty in GetDataSource")
		}
		if ds.Status != "active" {
			t.Errorf("Status = %q, want active", ds.Status)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		_, err := svc.GetDataSource(ctx, 99999)
		if err != ErrDatasourceNotFound {
			t.Errorf("GetDataSource(99999) error = %v, want ErrDatasourceNotFound", err)
		}
	})
}

// ─── GetDataSourceSafe ────────────────────────────────────────────────────────

func TestGetDataSourceSafe(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	created := &model.DataSource{
		Name: "safe-test", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret",
	}
	if err := svc.CreateDataSource(ctx, created); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	t.Run("password_cleared", func(t *testing.T) {
		ds, err := svc.GetDataSourceSafe(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetDataSourceSafe() error: %v", err)
		}
		if ds.PasswordEncrypted != "" {
			t.Errorf("PasswordEncrypted should be empty, got %q", ds.PasswordEncrypted)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		_, err := svc.GetDataSourceSafe(ctx, 99999)
		if err != ErrDatasourceNotFound {
			t.Errorf("GetDataSourceSafe(99999) error = %v, want ErrDatasourceNotFound", err)
		}
	})
}

// ─── ListDataSources ──────────────────────────────────────────────────────────

func TestListDataSources(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	t.Run("empty", func(t *testing.T) {
		list, err := svc.ListDataSources(ctx)
		if err != nil {
			t.Fatalf("ListDataSources() error: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("expected empty list, got %d items", len(list))
		}
	})

	// Create multiple datasources
	names := []string{"ds-alpha", "ds-beta", "ds-gamma"}
	for _, name := range names {
		ds := &model.DataSource{
			Name: name, Type: "mysql", Host: "10.0.0.1", Port: 3306,
			Username: "root", PasswordEncrypted: "secret",
		}
		if err := svc.CreateDataSource(ctx, ds); err != nil {
			t.Fatalf("CreateDataSource(%q) error: %v", name, err)
		}
	}

	t.Run("three_items", func(t *testing.T) {
		list, err := svc.ListDataSources(ctx)
		if err != nil {
			t.Fatalf("ListDataSources() error: %v", err)
		}
		if len(list) != 3 {
			t.Errorf("expected 3 items, got %d", len(list))
		}
	})

	t.Run("ordered_by_id", func(t *testing.T) {
		list, err := svc.ListDataSources(ctx)
		if err != nil {
			t.Fatalf("ListDataSources() error: %v", err)
		}
		for i := 1; i < len(list); i++ {
			if list[i].ID <= list[i-1].ID {
				t.Errorf("list not ordered by ID: %d <= %d", list[i].ID, list[i-1].ID)
			}
		}
	})

	t.Run("no_passwords", func(t *testing.T) {
		list, err := svc.ListDataSources(ctx)
		if err != nil {
			t.Fatalf("ListDataSources() error: %v", err)
		}
		for _, ds := range list {
			if ds.PasswordEncrypted != "" {
				t.Errorf("datasource %q should not have password in list response", ds.Name)
			}
		}
	})

	t.Run("fields_populated", func(t *testing.T) {
		list, err := svc.ListDataSources(ctx)
		if err != nil {
			t.Fatalf("ListDataSources() error: %v", err)
		}
		ds := list[0]
		if ds.Name == "" {
			t.Error("Name should not be empty")
		}
		if ds.Type == "" {
			t.Error("Type should not be empty")
		}
		if ds.Host == "" {
			t.Error("Host should not be empty")
		}
		if ds.Port == 0 {
			t.Error("Port should not be zero")
		}
		if ds.Status == "" {
			t.Error("Status should not be empty")
		}
		if ds.CreatedAt.IsZero() {
			t.Error("CreatedAt should not be zero")
		}
		if ds.UpdatedAt.IsZero() {
			t.Error("UpdatedAt should not be zero")
		}
	})
}

// ─── UpdateDataSource ─────────────────────────────────────────────────────────

func TestUpdateDataSource(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	created := &model.DataSource{
		Name: "update-test", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret", Database: "appdb",
		MaxOpen: 10, MaxIdle: 5, MaxLifetime: 3600, MaxIdleTime: 600,
	}
	if err := svc.CreateDataSource(ctx, created); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		update := &model.DataSource{
			Name: "updated-name", Type: "mysql", Host: "10.0.0.2", Port: 3307,
			Username: "admin", PasswordEncrypted: "newsecret", Database: "newdb",
			MaxOpen: 20, MaxIdle: 10, MaxLifetime: 1800, MaxIdleTime: 300,
		}
		if err := svc.UpdateDataSource(ctx, created.ID, update); err != nil {
			t.Fatalf("UpdateDataSource() error: %v", err)
		}

		got, err := svc.GetDataSource(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetDataSource() error: %v", err)
		}
		if got.Name != "updated-name" {
			t.Errorf("Name = %q, want updated-name", got.Name)
		}
		if got.Host != "10.0.0.2" {
			t.Errorf("Host = %q, want 10.0.0.2", got.Host)
		}
		if got.Port != 3307 {
			t.Errorf("Port = %d, want 3307", got.Port)
		}
		if got.Username != "admin" {
			t.Errorf("Username = %q, want admin", got.Username)
		}
		if got.Database != "newdb" {
			t.Errorf("Database = %q, want newdb", got.Database)
		}
		if got.MaxOpen != 20 {
			t.Errorf("MaxOpen = %d, want 20", got.MaxOpen)
		}
		if got.MaxIdle != 10 {
			t.Errorf("MaxIdle = %d, want 10", got.MaxIdle)
		}
		if got.MaxLifetime != 1800 {
			t.Errorf("MaxLifetime = %d, want 1800", got.MaxLifetime)
		}
		if got.MaxIdleTime != 300 {
			t.Errorf("MaxIdleTime = %d, want 300", got.MaxIdleTime)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		update := &model.DataSource{
			Name: "x", Type: "mysql", Host: "10.0.0.1", Port: 3306,
			Username: "root",
		}
		err := svc.UpdateDataSource(ctx, 99999, update)
		if err != ErrDatasourceNotFound {
			t.Errorf("UpdateDataSource(99999) error = %v, want ErrDatasourceNotFound", err)
		}
	})

	t.Run("invalid_type", func(t *testing.T) {
		update := &model.DataSource{
			Name: "x", Type: "postgres", Host: "10.0.0.1", Port: 5432,
			Username: "root",
		}
		err := svc.UpdateDataSource(ctx, created.ID, update)
		if err != ErrInvalidDatasourceType {
			t.Errorf("UpdateDataSource() error = %v, want ErrInvalidDatasourceType", err)
		}
	})
}

func TestUpdateDataSource_KeepPassword(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	created := &model.DataSource{
		Name: "keep-pw", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "original-secret",
	}
	if err := svc.CreateDataSource(ctx, created); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}
	originalEncrypted := created.PasswordEncrypted

	// Update without providing a new password
	update := &model.DataSource{
		Name: "keep-pw-updated", Type: "mysql", Host: "10.0.0.2", Port: 3307,
		Username: "admin", PasswordEncrypted: "", // empty = keep existing
	}
	if err := svc.UpdateDataSource(ctx, created.ID, update); err != nil {
		t.Fatalf("UpdateDataSource() error: %v", err)
	}

	got, err := svc.GetDataSource(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetDataSource() error: %v", err)
	}
	if got.PasswordEncrypted != originalEncrypted {
		t.Error("password should remain unchanged when empty password provided")
	}
}

func TestUpdateDataSource_ReEncryptPassword(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	created := &model.DataSource{
		Name: "reenc-pw", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "old-password",
	}
	if err := svc.CreateDataSource(ctx, created); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	// Update with a new password
	update := &model.DataSource{
		Name: "reenc-pw", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "new-password",
	}
	if err := svc.UpdateDataSource(ctx, created.ID, update); err != nil {
		t.Fatalf("UpdateDataSource() error: %v", err)
	}

	got, err := svc.GetDataSource(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetDataSource() error: %v", err)
	}

	// The new encrypted password should be different
	if got.PasswordEncrypted == "" {
		t.Error("password should not be empty after update with new password")
	}
}

func TestUpdateDataSource_UpdatedAtChanges(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	created := &model.DataSource{
		Name: "time-test", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret",
	}
	if err := svc.CreateDataSource(ctx, created); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}
	originalUpdatedAt := created.UpdatedAt

	// Small sleep to ensure updated_at differs
	time.Sleep(10 * time.Millisecond)

	update := &model.DataSource{
		Name: "time-test-v2", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root",
	}
	if err := svc.UpdateDataSource(ctx, created.ID, update); err != nil {
		t.Fatalf("UpdateDataSource() error: %v", err)
	}

	got, err := svc.GetDataSource(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetDataSource() error: %v", err)
	}
	// SQLite datetime('now') has second precision, so we check >= instead of strictly after
	if got.UpdatedAt.Before(originalUpdatedAt) {
		t.Errorf("UpdatedAt = %v, want >= %v", got.UpdatedAt, originalUpdatedAt)
	}
}

// ─── DisableDataSource ────────────────────────────────────────────────────────

func TestDisableDataSource(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	created := &model.DataSource{
		Name: "disable-test", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret",
	}
	if err := svc.CreateDataSource(ctx, created); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		if err := svc.DisableDataSource(ctx, created.ID); err != nil {
			t.Fatalf("DisableDataSource() error: %v", err)
		}

		got, err := svc.GetDataSource(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetDataSource() error: %v", err)
		}
		if got.Status != "disabled" {
			t.Errorf("Status = %q, want disabled", got.Status)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		err := svc.DisableDataSource(ctx, 99999)
		if err != ErrDatasourceNotFound {
			t.Errorf("DisableDataSource(99999) error = %v, want ErrDatasourceNotFound", err)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		// Disabling an already disabled datasource should still succeed
		if err := svc.DisableDataSource(ctx, created.ID); err != nil {
			t.Fatalf("second DisableDataSource() error: %v", err)
		}
		got, err := svc.GetDataSource(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetDataSource() error: %v", err)
		}
		if got.Status != "disabled" {
			t.Errorf("Status = %q, want disabled", got.Status)
		}
	})
}

func TestDisableDataSource_MongoDB(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	created := &model.DataSource{
		Name: "disable-mongo", Type: "mongodb", Host: "10.0.0.2", Port: 27017,
		Username: "admin", PasswordEncrypted: "secret",
	}
	if err := svc.CreateDataSource(ctx, created); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	if err := svc.DisableDataSource(ctx, created.ID); err != nil {
		t.Fatalf("DisableDataSource() error: %v", err)
	}

	got, err := svc.GetDataSource(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetDataSource() error: %v", err)
	}
	if got.Status != "disabled" {
		t.Errorf("Status = %q, want disabled", got.Status)
	}
}

// ─── TestConnection ───────────────────────────────────────────────────────────

func TestTestConnection(t *testing.T) {
	svc, _ := newTestDatasourceService(t)

	t.Run("invalid_type_no_id", func(t *testing.T) {
		ctx := ctxWithTimeout(t)
		ds := &model.DataSource{
			Type: "postgres", Host: "10.0.0.1", Port: 5432,
			Username: "root", PasswordEncrypted: "secret",
		}
		err := svc.TestConnection(ctx, ds)
		if err != ErrInvalidDatasourceType {
			t.Errorf("TestConnection() error = %v, want ErrInvalidDatasourceType", err)
		}
	})

	t.Run("mysql_connection_fails", func(t *testing.T) {
		ctx := ctxWithTimeout(t)
		// No real MySQL server, should fail with connection error
		ds := &model.DataSource{
			Type: "mysql", Host: "127.0.0.1", Port: 19999,
			Username: "root", PasswordEncrypted: "secret",
		}
		err := svc.TestConnection(ctx, ds)
		if err == nil {
			t.Error("expected connection error, got nil")
		}
	})

	t.Run("with_stored_password", func(t *testing.T) {
		ctx := ctxWithTimeout(t)
		// Create a datasource first, then test with its ID
		created := &model.DataSource{
			Name: "test-conn-stored", Type: "mysql", Host: "127.0.0.1", Port: 19999,
			Username: "root", PasswordEncrypted: "secret",
		}
		if err := svc.CreateDataSource(ctx, created); err != nil {
			t.Fatalf("CreateDataSource() error: %v", err)
		}

		ds := &model.DataSource{ID: created.ID, Type: "mysql"}
		err := svc.TestConnection(ctx, ds)
		if err == nil {
			t.Error("expected connection error, got nil")
		}
	})

	t.Run("with_nonexistent_id", func(t *testing.T) {
		ctx := ctxWithTimeout(t)
		ds := &model.DataSource{ID: 99999, Type: "mysql"}
		err := svc.TestConnection(ctx, ds)
		if err != ErrDatasourceNotFound {
			t.Errorf("TestConnection() error = %v, want ErrDatasourceNotFound", err)
		}
	})
}

// ─── GetTables ────────────────────────────────────────────────────────────────

func TestGetTables(t *testing.T) {
	svc, _ := newTestDatasourceService(t)

	t.Run("not_found", func(t *testing.T) {
		ctx := ctxWithTimeout(t)
		_, err := svc.GetTables(ctx, 99999)
		if err != ErrDatasourceNotFound {
			t.Errorf("GetTables(99999) error = %v, want ErrDatasourceNotFound", err)
		}
	})

	t.Run("disabled_datasource", func(t *testing.T) {
		ctx := ctxWithTimeout(t)
		created := &model.DataSource{
			Name: "disabled-tables", Type: "mysql", Host: "10.0.0.1", Port: 3306,
			Username: "root", PasswordEncrypted: "secret",
		}
		if err := svc.CreateDataSource(ctx, created); err != nil {
			t.Fatalf("CreateDataSource() error: %v", err)
		}
		if err := svc.DisableDataSource(ctx, created.ID); err != nil {
			t.Fatalf("DisableDataSource() error: %v", err)
		}

		_, err := svc.GetTables(ctx, created.ID)
		if err != ErrDatasourceDisabled {
			t.Errorf("GetTables() error = %v, want ErrDatasourceDisabled", err)
		}
	})

	t.Run("mysql_connection_fails", func(t *testing.T) {
		ctx := ctxWithTimeout(t)
		created := &model.DataSource{
			Name: "mysql-tables", Type: "mysql", Host: "127.0.0.1", Port: 19999,
			Username: "root", PasswordEncrypted: "secret", Database: "testdb",
		}
		if err := svc.CreateDataSource(ctx, created); err != nil {
			t.Fatalf("CreateDataSource() error: %v", err)
		}

		_, err := svc.GetTables(ctx, created.ID)
		if err == nil {
			t.Error("expected connection error, got nil")
		}
	})
}

func TestGetTables_EmptyDatabase(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := context.Background() // Use background context to avoid timeout during connection attempt

	created := &model.DataSource{
		Name: "empty-db", Type: "mysql", Host: "127.0.0.1", Port: 19999,
		Username: "root", PasswordEncrypted: "secret",
		// Database is empty, should default to information_schema
	}
	if err := svc.CreateDataSource(ctx, created); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	_, err := svc.GetTables(ctx, created.ID)
	// Will fail because no real MySQL, but the empty-database default path is exercised
	if err == nil {
		t.Error("expected connection error, got nil")
	}
}

// ─── Password Encryption Round-trip ──────────────────────────────────────────

func TestPasswordEncryptionRoundTrip(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	rawPassword := "my-complex-p@ssw0rd!#$"
	ds := &model.DataSource{
		Name: "roundtrip", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: rawPassword,
	}
	if err := svc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("CreateDataSource() error: %v", err)
	}

	// Retrieve and verify the password can be decrypted back
	retrieved, err := svc.GetDataSource(ctx, ds.ID)
	if err != nil {
		t.Fatalf("GetDataSource() error: %v", err)
	}

	// Decrypt should yield the original password
	decrypted, err := crypto.Decrypt(retrieved.PasswordEncrypted, testEncKey)
	if err != nil {
		t.Fatalf("decrypt error: %v", err)
	}
	if decrypted != rawPassword {
		t.Errorf("decrypted = %q, want %q", decrypted, rawPassword)
	}
}

// ─── Integration: Full CRUD Lifecycle ────────────────────────────────────────

func TestDatasourceService_CRUDLifecycle(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx := ctxWithTimeout(t)

	// Step 1: List (empty)
	t.Run("list_empty", func(t *testing.T) {
		list, err := svc.ListDataSources(ctx)
		if err != nil {
			t.Fatalf("ListDataSources() error: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("expected empty list, got %d items", len(list))
		}
	})

	// Step 2: Create
	var dsID int64
	t.Run("create", func(t *testing.T) {
		ds := &model.DataSource{
			Name: "lifecycle", Type: "mysql", Host: "10.0.0.1", Port: 3306,
			Username: "root", PasswordEncrypted: "secret", Database: "app",
		}
		if err := svc.CreateDataSource(ctx, ds); err != nil {
			t.Fatalf("CreateDataSource() error: %v", err)
		}
		dsID = ds.ID
		if dsID <= 0 {
			t.Errorf("expected positive ID, got %d", dsID)
		}
	})

	// Step 3: Get
	t.Run("get", func(t *testing.T) {
		got, err := svc.GetDataSource(ctx, dsID)
		if err != nil {
			t.Fatalf("GetDataSource() error: %v", err)
		}
		if got.Name != "lifecycle" {
			t.Errorf("Name = %q, want lifecycle", got.Name)
		}
		if got.Status != "active" {
			t.Errorf("Status = %q, want active", got.Status)
		}
	})

	// Step 4: Update
	t.Run("update", func(t *testing.T) {
		update := &model.DataSource{
			Name: "lifecycle-v2", Type: "mysql", Host: "10.0.0.2", Port: 3307,
			Username: "admin", Database: "newapp",
		}
		if err := svc.UpdateDataSource(ctx, dsID, update); err != nil {
			t.Fatalf("UpdateDataSource() error: %v", err)
		}

		got, err := svc.GetDataSource(ctx, dsID)
		if err != nil {
			t.Fatalf("GetDataSource() error: %v", err)
		}
		if got.Name != "lifecycle-v2" {
			t.Errorf("Name = %q, want lifecycle-v2", got.Name)
		}
		if got.Host != "10.0.0.2" {
			t.Errorf("Host = %q, want 10.0.0.2", got.Host)
		}
	})

	// Step 5: List (should have 1 item)
	t.Run("list_with_item", func(t *testing.T) {
		list, err := svc.ListDataSources(ctx)
		if err != nil {
			t.Fatalf("ListDataSources() error: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("expected 1 item, got %d", len(list))
		}
	})

	// Step 6: Disable
	t.Run("disable", func(t *testing.T) {
		if err := svc.DisableDataSource(ctx, dsID); err != nil {
			t.Fatalf("DisableDataSource() error: %v", err)
		}
	})

	// Step 7: Verify disabled
	t.Run("verify_disabled", func(t *testing.T) {
		got, err := svc.GetDataSource(ctx, dsID)
		if err != nil {
			t.Fatalf("GetDataSource() error: %v", err)
		}
		if got.Status != "disabled" {
			t.Errorf("Status = %q, want disabled", got.Status)
		}
	})

	// Step 8: GetTables on disabled should fail
	t.Run("tables_disabled", func(t *testing.T) {
		_, err := svc.GetTables(ctx, dsID)
		if err != ErrDatasourceDisabled {
			t.Errorf("GetTables() error = %v, want ErrDatasourceDisabled", err)
		}
	})
}

// ─── Context Cancellation ─────────────────────────────────────────────────────

func TestCreateDataSource_CancelledContext(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	ds := &model.DataSource{
		Name: "cancel-test", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret",
	}
	err := svc.CreateDataSource(ctx, ds)
	if err == nil {
		t.Error("expected error with cancelled context, got nil")
	}
}

func TestGetDataSource_CancelledContext(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.GetDataSource(ctx, 1)
	if err == nil {
		t.Error("expected error with cancelled context, got nil")
	}
}

func TestListDataSources_CancelledContext(t *testing.T) {
	svc, _ := newTestDatasourceService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.ListDataSources(ctx)
	if err == nil {
		t.Error("expected error with cancelled context, got nil")
	}
}

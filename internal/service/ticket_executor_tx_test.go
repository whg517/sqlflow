package service

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/pkg/crypto"
)

func setupTxTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := db.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database.DB
}

// TestExecuteSQL_PostgreSQLRoute verifies that executeSQL routes to
// executeSQLTransactional for PostgreSQL datasources.
func TestExecuteSQL_PostgreSQLRoute(t *testing.T) {
	d := setupTxTestDB(t)
	encKey := "test-encryption-key-32byte-len!!"
	connMgr := connpool.NewManager()
	t.Cleanup(func() { connMgr.Close() })

	dsSvc := NewDatasourceService(mustWrapDB(d), encKey, connMgr, nil)
	svc := &TicketService{
		db:            d,
		dsSvc:         dsSvc,
		connMgr:       connMgr,
		encryptionKey: encKey,
	}

	// Create a mock PG database
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mockDB.Close()

	// Create datasource
	encPass, _ := crypto.Encrypt("test", encKey)
	result, err := d.ExecContext(context.Background(),
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, database, sslmode) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"test-pg-route", "postgresql", "localhost", 5432, "test", encPass, "testdb", "disable")
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}
	dsID, _ := result.LastInsertId()

	// Inject the mock as a PG pool
	connMgr.InjectPGForTest(dsID, "localhost", 5432, "testdb", mockDB)

	// Expect: BeginTx → Exec("CREATE TABLE") → Exec("INSERT") → Commit
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE.*").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO.*").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ds, err := dsSvc.GetDataSource(context.Background(), dsID)
	if err != nil {
		t.Fatalf("GetDataSource: %v", err)
	}

	results, err := svc.executeSQL(context.Background(), ds, "testdb", "postgresql",
		"CREATE TABLE test (id INT); INSERT INTO test VALUES (1)")
	if err != nil {
		t.Fatalf("executeSQL: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Status != "success" {
			t.Errorf("statement status=%q, error=%s", r.Status, r.Error)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", err)
	}
}

// TestExecuteSQL_PostgreSQLRollback verifies that a failing statement triggers ROLLBACK.
func TestExecuteSQL_PostgreSQLRollback(t *testing.T) {
	d := setupTxTestDB(t)
	encKey := "test-encryption-key-32byte-len!!"
	connMgr := connpool.NewManager()
	t.Cleanup(func() { connMgr.Close() })

	dsSvc := NewDatasourceService(mustWrapDB(d), encKey, connMgr, nil)
	svc := &TicketService{
		db:            d,
		dsSvc:         dsSvc,
		connMgr:       connMgr,
		encryptionKey: encKey,
	}

	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mockDB.Close()

	encPass, _ := crypto.Encrypt("test", encKey)
	result, err := d.ExecContext(context.Background(),
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, database, sslmode) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"test-pg-rollback", "postgresql", "localhost", 5432, "test", encPass, "testdb", "disable")
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}
	dsID, _ := result.LastInsertId()

	connMgr.InjectPGForTest(dsID, "localhost", 5432, "testdb", mockDB)

	// Expect: BeginTx → Exec succeeds → Exec fails → Rollback
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE.*").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("BAD SQL.*").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	ds, err := dsSvc.GetDataSource(context.Background(), dsID)
	if err != nil {
		t.Fatalf("GetDataSource: %v", err)
	}

	results, err := svc.executeSQL(context.Background(), ds, "testdb", "postgresql",
		"CREATE TABLE test (id INT); BAD SQL SYNTAX HERE")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// First statement should be rolled_back, second should be error
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Status != "rolled_back" {
		t.Errorf("first statement status=%q, want rolled_back", results[0].Status)
	}
	if results[1].Status != "error" {
		t.Errorf("second statement status=%q, want error", results[1].Status)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", err)
	}
}

// TestExecuteSQL_MySQLNoTransaction verifies MySQL doesn't use transactions.
func TestExecuteSQL_MySQLNoTransaction(t *testing.T) {
	d := setupTxTestDB(t)
	encKey := "test-encryption-key-32byte-len!!"
	connMgr := connpool.NewManager()
	t.Cleanup(func() { connMgr.Close() })

	dsSvc := NewDatasourceService(mustWrapDB(d), encKey, connMgr, nil)
	svc := &TicketService{
		db:            d,
		dsSvc:         dsSvc,
		connMgr:       connMgr,
		encryptionKey: encKey,
	}

	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mockDB.Close()

	encPass, _ := crypto.Encrypt("test", encKey)
	result, err := d.ExecContext(context.Background(),
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, database) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"test-mysql-notx", "mysql", "localhost", 3306, "test", encPass, "testdb")
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}
	dsID, _ := result.LastInsertId()

	connMgr.InjectMySQLForTest(dsID, "localhost", 3306, "testdb", mockDB)

	// MySQL: expect direct ExecContext calls WITHOUT BeginTx/Commit/Rollback
	mock.ExpectExec("SELECT 1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("SELECT 2").WillReturnResult(sqlmock.NewResult(0, 0))

	ds, err := dsSvc.GetDataSource(context.Background(), dsID)
	if err != nil {
		t.Fatalf("GetDataSource: %v", err)
	}

	results, err := svc.executeSQL(context.Background(), ds, "testdb", "mysql",
		"SELECT 1; SELECT 2")
	if err != nil {
		t.Fatalf("executeSQL: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Status != "success" {
			t.Errorf("status=%q, want success", r.Status)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", err)
	}
}

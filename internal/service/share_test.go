package service

import (
	"database/sql"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
)

func setupShareTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return database.DB
}

func TestShareService_CreateAndGet(t *testing.T) {
	dbConn := setupShareTestDB(t)
	svc := NewShareService(mustWrapDB(dbConn))

	req := &CreateShareRequest{
		UserID:    1,
		Username:  "testuser",
		Columns:   []string{"id", "name", "status"},
		Rows: []map[string]interface{}{
			{"id": 1, "name": "Alice", "status": "active"},
			{"id": 2, "name": "Bob", "status": "inactive"},
		},
		ExpiresAt:      time.Now().Add(24 * time.Hour),
		SQLSummary:     "SELECT * FROM users",
		DatasourceName: "test-db",
	}

	result, err := svc.CreateShare(t.Context(), req)
	if err != nil {
		t.Fatalf("CreateShare: %v", err)
	}

	if result.Token == "" {
		t.Error("expected non-empty token")
	}
	if result.RowCount != 2 {
		t.Errorf("expected row_count=2, got %d", result.RowCount)
	}

	// Get the share (public)
	pub, err := svc.GetShare(t.Context(), result.Token)
	if err != nil {
		t.Fatalf("GetShare: %v", err)
	}

	if len(pub.Columns) != 3 {
		t.Errorf("expected 3 columns, got %d", len(pub.Columns))
	}
	if len(pub.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(pub.Rows))
	}
	if pub.RowCount != 2 {
		t.Errorf("expected row_count=2, got %d", pub.RowCount)
	}
	if pub.HasPassword {
		t.Error("expected no password")
	}
}

func TestShareService_ExpiredShare(t *testing.T) {
	dbConn := setupShareTestDB(t)
	svc := NewShareService(mustWrapDB(dbConn))

	req := &CreateShareRequest{
		UserID:   1,
		Username: "testuser",
		Columns:  []string{"id"},
		Rows: []map[string]interface{}{
			{"id": 1},
		},
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}

	result, err := svc.CreateShare(t.Context(), req)
	if err != nil {
		t.Fatalf("CreateShare: %v", err)
	}

	_, err = svc.GetShare(t.Context(), result.Token)
	if err != ErrShareExpired {
		t.Errorf("expected ErrShareExpired, got %v", err)
	}
}

func TestShareService_PasswordProtection(t *testing.T) {
	dbConn := setupShareTestDB(t)
	svc := NewShareService(mustWrapDB(dbConn))

	req := &CreateShareRequest{
		UserID:    1,
		Username:  "testuser",
		Columns:   []string{"id"},
		Rows:      []map[string]interface{}{{"id": 1}},
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Password:  "secret123",
	}

	result, err := svc.CreateShare(t.Context(), req)
	if err != nil {
		t.Fatalf("CreateShare: %v", err)
	}

	pub, err := svc.GetShare(t.Context(), result.Token)
	if err != nil {
		t.Fatalf("GetShare: %v", err)
	}
	if !pub.HasPassword {
		t.Error("expected HasPassword=true")
	}

	// Verify correct password
	err = svc.VerifyPassword(t.Context(), result.Token, "secret123")
	if err != nil {
		t.Errorf("VerifyPassword with correct password: %v", err)
	}

	// Verify wrong password
	err = svc.VerifyPassword(t.Context(), result.Token, "wrong")
	if err != ErrSharePassword {
		t.Errorf("expected ErrSharePassword, got %v", err)
	}
}

func TestShareService_Revoke(t *testing.T) {
	dbConn := setupShareTestDB(t)
	svc := NewShareService(mustWrapDB(dbConn))

	req := &CreateShareRequest{
		UserID:    1,
		Username:  "testuser",
		Columns:   []string{"id"},
		Rows:      []map[string]interface{}{{"id": 1}},
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	result, err := svc.CreateShare(t.Context(), req)
	if err != nil {
		t.Fatalf("CreateShare: %v", err)
	}

	err = svc.RevokeShare(t.Context(), result.ID, 1)
	if err != nil {
		t.Fatalf("RevokeShare: %v", err)
	}

	_, err = svc.GetShare(t.Context(), result.Token)
	if err != ErrShareRevoked {
		t.Errorf("expected ErrShareRevoked, got %v", err)
	}

	// Revoke by wrong user
	err = svc.RevokeShare(t.Context(), result.ID, 999)
	if err != ErrShareNotFound {
		t.Errorf("expected ErrShareNotFound for wrong user, got %v", err)
	}
}

func TestShareService_RowLimit(t *testing.T) {
	dbConn := setupShareTestDB(t)
	svc := NewShareService(mustWrapDB(dbConn))

	rows := make([]map[string]interface{}, shareMaxRows+1)
	for i := range rows {
		rows[i] = map[string]interface{}{"id": i}
	}

	req := &CreateShareRequest{
		UserID:    1,
		Username:  "testuser",
		Columns:   []string{"id"},
		Rows:      rows,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	_, err := svc.CreateShare(t.Context(), req)
	if err != ErrShareRowLimit {
		t.Errorf("expected ErrShareRowLimit, got %v", err)
	}
}

func TestShareService_TokenUniqueness(t *testing.T) {
	dbConn := setupShareTestDB(t)
	svc := NewShareService(mustWrapDB(dbConn))

	req := &CreateShareRequest{
		UserID:    1,
		Username:  "testuser",
		Columns:   []string{"id"},
		Rows:      []map[string]interface{}{{"id": 1}},
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	r1, _ := svc.CreateShare(t.Context(), req)
	r2, _ := svc.CreateShare(t.Context(), req)

	if r1.Token == r2.Token {
		t.Error("expected unique tokens for different shares")
	}

	if len(r1.Token) != 32 {
		t.Errorf("expected 32-char token, got %d", len(r1.Token))
	}
}

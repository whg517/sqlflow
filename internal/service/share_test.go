package service

import (
	"database/sql"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/testutil"
)

const shareTestAccessSecret = "share-test-access-secret-at-least-32-bytes"

func setupShareTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testutil.NewDB(t).DB
}

func TestShareService_CreateAndGet(t *testing.T) {
	dbConn := setupShareTestDB(t)
	svc := NewShareService(mustWrapDB(dbConn), shareTestAccessSecret)

	req := &CreateShareRequest{
		UserID:   1,
		Username: "testuser",
		Columns:  []string{"id", "name", "status"},
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
	pub, err := svc.GetShare(t.Context(), result.Token, "")
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
	svc := NewShareService(mustWrapDB(dbConn), shareTestAccessSecret)

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

	_, err = svc.GetShare(t.Context(), result.Token, "")
	if err != ErrShareExpired {
		t.Errorf("expected ErrShareExpired, got %v", err)
	}
}

func TestShareService_PasswordProtection(t *testing.T) {
	dbConn := setupShareTestDB(t)
	svc := NewShareService(mustWrapDB(dbConn), shareTestAccessSecret)

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

	pub, err := svc.GetShare(t.Context(), result.Token, "")
	if err != nil {
		t.Fatalf("GetShare: %v", err)
	}
	if !pub.HasPassword {
		t.Error("expected HasPassword=true")
	}

	if pub.AccessGranted {
		t.Error("expected access_granted=false before password verification")
	}
	if len(pub.Columns) != 0 || len(pub.Rows) != 0 {
		t.Fatal("password-protected share leaked result data before verification")
	}
	if pub.ID != 0 || pub.RowCount != 0 || pub.SQLSummary != "" || pub.DatasourceName != "" {
		t.Fatal("password-protected share leaked sensitive metadata before verification")
	}

	// Verify correct password
	accessProof, err := svc.VerifyPassword(t.Context(), result.Token, "secret123")
	if err != nil {
		t.Errorf("VerifyPassword with correct password: %v", err)
	}
	if accessProof == "" {
		t.Fatal("expected a short-lived access proof")
	}

	pub, err = svc.GetShare(t.Context(), result.Token, accessProof)
	if err != nil {
		t.Fatalf("GetShare with access proof: %v", err)
	}
	if !pub.AccessGranted || len(pub.Rows) != 1 {
		t.Fatal("verified access did not return shared result")
	}

	// Verify wrong password
	_, err = svc.VerifyPassword(t.Context(), result.Token, "wrong")
	if err != ErrSharePassword {
		t.Errorf("expected ErrSharePassword, got %v", err)
	}

	t.Run("tampered proof is rejected", func(t *testing.T) {
		got, getErr := svc.GetShare(t.Context(), result.Token, accessProof+"tampered")
		if getErr != nil {
			t.Fatalf("GetShare with tampered proof: %v", getErr)
		}
		if got.AccessGranted || len(got.Rows) != 0 {
			t.Fatal("tampered access proof returned protected data")
		}
	})

	t.Run("expired proof is rejected", func(t *testing.T) {
		expiredProof := svc.generateAccessProof(result.ID, result.Token, time.Now().Add(-time.Minute))
		got, getErr := svc.GetShare(t.Context(), result.Token, expiredProof)
		if getErr != nil {
			t.Fatalf("GetShare with expired proof: %v", getErr)
		}
		if got.AccessGranted || len(got.Rows) != 0 {
			t.Fatal("expired access proof returned protected data")
		}
	})

	t.Run("proof cannot be reused for another share", func(t *testing.T) {
		other, createErr := svc.CreateShare(t.Context(), &CreateShareRequest{
			UserID:    1,
			Username:  "testuser",
			Columns:   []string{"secret"},
			Rows:      []map[string]interface{}{{"secret": "other"}},
			ExpiresAt: time.Now().Add(time.Hour),
			Password:  "secret123",
		})
		if createErr != nil {
			t.Fatalf("CreateShare other: %v", createErr)
		}

		got, getErr := svc.GetShare(t.Context(), other.Token, accessProof)
		if getErr != nil {
			t.Fatalf("GetShare other with reused proof: %v", getErr)
		}
		if got.AccessGranted || len(got.Rows) != 0 {
			t.Fatal("access proof was reusable across shares")
		}
	})
}

func TestShareService_Revoke(t *testing.T) {
	dbConn := setupShareTestDB(t)
	svc := NewShareService(mustWrapDB(dbConn), shareTestAccessSecret)

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

	_, err = svc.GetShare(t.Context(), result.Token, "")
	if err != ErrShareRevoked {
		t.Errorf("expected ErrShareRevoked, got %v", err)
	}

	// Revoke by wrong user
	err = svc.RevokeShare(t.Context(), result.ID, 999)
	if err != ErrShareNotFound {
		t.Errorf("expected ErrShareNotFound for wrong user, got %v", err)
	}
}

func TestShareService_VerifyRejectsExpiredAndRevokedShares(t *testing.T) {
	dbConn := setupShareTestDB(t)
	svc := NewShareService(mustWrapDB(dbConn), shareTestAccessSecret)

	expired, err := svc.CreateShare(t.Context(), &CreateShareRequest{
		UserID:    1,
		Username:  "testuser",
		Columns:   []string{"id"},
		Rows:      []map[string]interface{}{{"id": 1}},
		ExpiresAt: time.Now().Add(-time.Hour),
		Password:  "secret123",
	})
	if err != nil {
		t.Fatalf("CreateShare expired: %v", err)
	}
	if _, err := svc.VerifyPassword(t.Context(), expired.Token, "secret123"); err != ErrShareExpired {
		t.Fatalf("VerifyPassword expired error = %v, want %v", err, ErrShareExpired)
	}

	revoked, err := svc.CreateShare(t.Context(), &CreateShareRequest{
		UserID:    1,
		Username:  "testuser",
		Columns:   []string{"id"},
		Rows:      []map[string]interface{}{{"id": 1}},
		ExpiresAt: time.Now().Add(time.Hour),
		Password:  "secret123",
	})
	if err != nil {
		t.Fatalf("CreateShare revoked: %v", err)
	}
	if err := svc.RevokeShare(t.Context(), revoked.ID, revoked.UserID); err != nil {
		t.Fatalf("RevokeShare: %v", err)
	}
	if _, err := svc.VerifyPassword(t.Context(), revoked.Token, "secret123"); err != ErrShareRevoked {
		t.Fatalf("VerifyPassword revoked error = %v, want %v", err, ErrShareRevoked)
	}
}

func TestShareService_RowLimit(t *testing.T) {
	dbConn := setupShareTestDB(t)
	svc := NewShareService(mustWrapDB(dbConn), shareTestAccessSecret)

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
	svc := NewShareService(mustWrapDB(dbConn), shareTestAccessSecret)

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

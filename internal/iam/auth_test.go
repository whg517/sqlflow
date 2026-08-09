package iam

import (
	"context"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/testutil"
)

// setupAuthTestDB creates a temp SQLite database with schema migrated.
// newTestAuthService creates a Service with a real SQLite DB and test JWT config.
func newTestAuthService(t *testing.T) (*Service, *db.DB) {
	t.Helper()
	testDB := testutil.NewDB(t)
	svc := NewService(testDB, "test-secret-key", 1*time.Hour)
	return svc, testDB
}

func TestNewAuthService(t *testing.T) {
	svc, _ := newTestAuthService(t)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}

// ---------------------------------------------------------------------------
// CreateUser
// ---------------------------------------------------------------------------

func TestAuthService_CreateUser(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		user, err := svc.CreateUser(ctx, "alice", "password123", "admin")
		if err != nil {
			t.Fatalf("CreateUser() error: %v", err)
		}
		if user.ID == 0 {
			t.Error("user ID should not be 0")
		}
		if user.Username != "alice" {
			t.Errorf("Username = %q, want %q", user.Username, "alice")
		}
		if user.Role != "admin" {
			t.Errorf("Role = %q, want %q", user.Role, "admin")
		}
		if user.PasswordHash == "" {
			t.Error("PasswordHash should be set in the returned user")
		}
		if user.CreatedAt.IsZero() {
			t.Error("CreatedAt should not be zero")
		}
	})

	t.Run("duplicate username", func(t *testing.T) {
		_, err := svc.CreateUser(ctx, "alice", "otherpass", "developer")
		if err == nil {
			t.Fatal("expected error for duplicate username, got nil")
		}
	})

	t.Run("different users", func(t *testing.T) {
		user, err := svc.CreateUser(ctx, "bob", "pass", "dba")
		if err != nil {
			t.Fatalf("CreateUser() error: %v", err)
		}
		if user.Username != "bob" {
			t.Errorf("Username = %q, want %q", user.Username, "bob")
		}
		if user.Role != "dba" {
			t.Errorf("Role = %q, want %q", user.Role, "dba")
		}
	})
}

// ---------------------------------------------------------------------------
// Authenticate
// ---------------------------------------------------------------------------

func TestAuthService_Authenticate(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	// Seed a user
	_, err := svc.CreateUser(ctx, "testuser", "mypassword", "developer")
	if err != nil {
		t.Fatalf("CreateUser() seed error: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		token, _, user, err := svc.Authenticate(ctx, "testuser", "mypassword")
		if err != nil {
			t.Fatalf("Authenticate() error: %v", err)
		}
		if token == "" {
			t.Error("token should not be empty")
		}
		if user.Username != "testuser" {
			t.Errorf("Username = %q, want %q", user.Username, "testuser")
		}
		if user.Role != "developer" {
			t.Errorf("Role = %q, want %q", user.Role, "developer")
		}

		// Verify the token can be parsed back
		claims, err := svc.ParseToken(token)
		if err != nil {
			t.Fatalf("ParseToken() error: %v", err)
		}
		if claims.UserID != user.ID {
			t.Errorf("claims.UserID = %d, want %d", claims.UserID, user.ID)
		}
		if claims.Username != "testuser" {
			t.Errorf("claims.Username = %q, want %q", claims.Username, "testuser")
		}
		if claims.Role != "developer" {
			t.Errorf("claims.Role = %q, want %q", claims.Role, "developer")
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		_, _, _, err := svc.Authenticate(ctx, "testuser", "wrongpassword")
		if err != ErrInvalidCredentials {
			t.Errorf("Authenticate() error = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("nonexistent user", func(t *testing.T) {
		_, _, _, err := svc.Authenticate(ctx, "ghost", "whatever")
		if err != ErrInvalidCredentials {
			t.Errorf("Authenticate() error = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("empty username", func(t *testing.T) {
		_, _, _, err := svc.Authenticate(ctx, "", "pass")
		if err != ErrInvalidCredentials {
			t.Errorf("Authenticate() error = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("empty password", func(t *testing.T) {
		_, _, _, err := svc.Authenticate(ctx, "testuser", "")
		if err != ErrInvalidCredentials {
			t.Errorf("Authenticate() error = %v, want ErrInvalidCredentials", err)
		}
	})
}

// ---------------------------------------------------------------------------
// ListUsers
// ---------------------------------------------------------------------------

func TestAuthService_ListUsers(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	// Seed multiple users
	roles := []struct {
		username string
		role     string
	}{
		{"admin1", "admin"},
		{"dba1", "dba"},
		{"dev1", "developer"},
		{"dev2", "developer"},
		{"dev3", "developer"},
	}
	for _, u := range roles {
		_, err := svc.CreateUser(ctx, u.username, "pass123", u.role)
		if err != nil {
			t.Fatalf("CreateUser(%s) error: %v", u.username, err)
		}
	}

	t.Run("list all", func(t *testing.T) {
		users, total, err := svc.ListUsers(ctx, 1, 50)
		if err != nil {
			t.Fatalf("ListUsers() error: %v", err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
		if len(users) != 5 {
			t.Errorf("len(users) = %d, want 5", len(users))
		}
	})

	t.Run("pagination page 1", func(t *testing.T) {
		users, total, err := svc.ListUsers(ctx, 1, 2)
		if err != nil {
			t.Fatalf("ListUsers() error: %v", err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
		if len(users) != 2 {
			t.Errorf("len(users) = %d, want 2", len(users))
		}
	})

	t.Run("pagination page 2", func(t *testing.T) {
		users, total, err := svc.ListUsers(ctx, 2, 2)
		if err != nil {
			t.Fatalf("ListUsers() error: %v", err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
		if len(users) != 2 {
			t.Errorf("len(users) = %d, want 2", len(users))
		}
	})

	t.Run("pagination last page partial", func(t *testing.T) {
		users, total, err := svc.ListUsers(ctx, 3, 2)
		if err != nil {
			t.Fatalf("ListUsers() error: %v", err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
		if len(users) != 1 {
			t.Errorf("len(users) = %d, want 1", len(users))
		}
	})

	t.Run("page beyond data", func(t *testing.T) {
		users, total, err := svc.ListUsers(ctx, 10, 10)
		if err != nil {
			t.Fatalf("ListUsers() error: %v", err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
		if len(users) != 0 {
			t.Errorf("len(users) = %d, want 0", len(users))
		}
	})

	t.Run("empty database", func(t *testing.T) {
		emptySvc, _ := newTestAuthService(t)
		users, total, err := emptySvc.ListUsers(ctx, 1, 10)
		if err != nil {
			t.Fatalf("ListUsers() error: %v", err)
		}
		if total != 0 {
			t.Errorf("total = %d, want 0", total)
		}
		if len(users) != 0 {
			t.Errorf("len(users) = %d, want 0", len(users))
		}
	})

	t.Run("default pagination values", func(t *testing.T) {
		users, total, err := svc.ListUsers(ctx, 0, 0)
		if err != nil {
			t.Fatalf("ListUsers() error: %v", err)
		}
		// sqlutil.ParsePagination defaults: page=1, pageSize=50
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
		if len(users) != 5 {
			t.Errorf("len(users) = %d, want 5", len(users))
		}
	})
}

// ---------------------------------------------------------------------------
// ChangePassword
// ---------------------------------------------------------------------------

func TestAuthService_ChangePassword(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	// Seed a user
	_, err := svc.CreateUser(ctx, "pwuser", "oldpassword", "developer")
	if err != nil {
		t.Fatalf("CreateUser() error: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		err := svc.ChangePassword(ctx, 1, "oldpassword", "newpassword")
		if err != nil {
			t.Fatalf("ChangePassword() error: %v", err)
		}

		// Verify new password works for authentication
		_, _, _, err = svc.Authenticate(ctx, "pwuser", "newpassword")
		if err != nil {
			t.Fatalf("Authenticate() with new password error: %v", err)
		}

		// Verify old password no longer works
		_, _, _, err = svc.Authenticate(ctx, "pwuser", "oldpassword")
		if err != ErrInvalidCredentials {
			t.Errorf("Authenticate() with old password error = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("wrong old password", func(t *testing.T) {
		err := svc.ChangePassword(ctx, 1, "wrongold", "newpass")
		if err != ErrInvalidCredentials {
			t.Errorf("ChangePassword() error = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("nonexistent user", func(t *testing.T) {
		err := svc.ChangePassword(ctx, 99999, "old", "new")
		if err != ErrUserNotFound {
			t.Errorf("ChangePassword() error = %v, want ErrUserNotFound", err)
		}
	})
}

// ---------------------------------------------------------------------------
// AdminCount
// ---------------------------------------------------------------------------

func TestAuthService_AdminCount(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	t.Run("no admins", func(t *testing.T) {
		count, err := svc.AdminCount(ctx)
		if err != nil {
			t.Fatalf("AdminCount() error: %v", err)
		}
		if count != 0 {
			t.Errorf("AdminCount() = %d, want 0", count)
		}
	})

	t.Run("one admin", func(t *testing.T) {
		_, err := svc.CreateUser(ctx, "admin1", "pass", "admin")
		if err != nil {
			t.Fatalf("CreateUser() error: %v", err)
		}
		count, err := svc.AdminCount(ctx)
		if err != nil {
			t.Fatalf("AdminCount() error: %v", err)
		}
		if count != 1 {
			t.Errorf("AdminCount() = %d, want 1", count)
		}
	})

	t.Run("multiple admins with mixed roles", func(t *testing.T) {
		_, err := svc.CreateUser(ctx, "admin2", "pass", "admin")
		if err != nil {
			t.Fatalf("CreateUser() error: %v", err)
		}
		_, err = svc.CreateUser(ctx, "dev1", "pass", "developer")
		if err != nil {
			t.Fatalf("CreateUser() error: %v", err)
		}
		_, err = svc.CreateUser(ctx, "dba1", "pass", "dba")
		if err != nil {
			t.Fatalf("CreateUser() error: %v", err)
		}

		count, err := svc.AdminCount(ctx)
		if err != nil {
			t.Fatalf("AdminCount() error: %v", err)
		}
		if count != 2 {
			t.Errorf("AdminCount() = %d, want 2", count)
		}
	})
}

// ---------------------------------------------------------------------------
// UserCount
// ---------------------------------------------------------------------------

func TestAuthService_UserCount(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	t.Run("empty database", func(t *testing.T) {
		count, err := svc.UserCount(ctx)
		if err != nil {
			t.Fatalf("UserCount() error: %v", err)
		}
		if count != 0 {
			t.Errorf("UserCount() = %d, want 0", count)
		}
	})

	t.Run("with users", func(t *testing.T) {
		_, _ = svc.CreateUser(ctx, "uc1", "pass1234", "admin")
		_, _ = svc.CreateUser(ctx, "uc2", "pass1234", "developer")

		count, err := svc.UserCount(ctx)
		if err != nil {
			t.Fatalf("UserCount() error: %v", err)
		}
		if count != 2 {
			t.Errorf("UserCount() = %d, want 2", count)
		}
	})

	t.Run("count decreases after delete", func(t *testing.T) {
		svc, _ := newTestAuthService(t)
		ctx := context.Background()

		user, _ := svc.CreateUser(ctx, "tempuser", "pass1234", "developer")
		svc.CreateUser(ctx, "keepuser", "pass1234", "admin")

		countBefore, _ := svc.UserCount(ctx)
		if err := svc.DeleteUser(ctx, user.ID); err != nil {
			t.Fatalf("DeleteUser() error: %v", err)
		}
		countAfter, _ := svc.UserCount(ctx)
		if countAfter != countBefore-1 {
			t.Errorf("UserCount after delete = %d, want %d", countAfter, countBefore-1)
		}
	})
}

// ---------------------------------------------------------------------------
// UpdateUserRole
// ---------------------------------------------------------------------------

func TestAuthService_UpdateUserRole(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		user, err := svc.CreateUser(ctx, "roleuser", "pass1234", "developer")
		if err != nil {
			t.Fatalf("CreateUser() error: %v", err)
		}

		updated, err := svc.UpdateUserRole(ctx, user.ID, "dba")
		if err != nil {
			t.Fatalf("UpdateUserRole() error: %v", err)
		}
		if updated.Role != "dba" {
			t.Errorf("Role = %q, want %q", updated.Role, "dba")
		}
		if updated.ID != user.ID {
			t.Errorf("ID = %d, want %d", updated.ID, user.ID)
		}
	})

	t.Run("nonexistent user", func(t *testing.T) {
		_, err := svc.UpdateUserRole(ctx, 99999, "admin")
		if err != ErrUserNotFound {
			t.Errorf("UpdateUserRole() error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("update to same role", func(t *testing.T) {
		svc, _ := newTestAuthService(t)
		ctx := context.Background()

		user, _ := svc.CreateUser(ctx, "sameuser", "pass1234", "developer")
		updated, err := svc.UpdateUserRole(ctx, user.ID, "developer")
		if err != nil {
			t.Fatalf("UpdateUserRole() error: %v", err)
		}
		if updated.Role != "developer" {
			t.Errorf("Role = %q, want %q", updated.Role, "developer")
		}
	})
}

// ---------------------------------------------------------------------------
// DeleteUser
// ---------------------------------------------------------------------------

func TestAuthService_DeleteUser(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		user, err := svc.CreateUser(ctx, "deluser", "pass1234", "developer")
		if err != nil {
			t.Fatalf("CreateUser() error: %v", err)
		}

		err = svc.DeleteUser(ctx, user.ID)
		if err != nil {
			t.Fatalf("DeleteUser() error: %v", err)
		}

		// Verify user is gone
		_, err = svc.GetUserByID(ctx, user.ID)
		if err != ErrUserNotFound {
			t.Errorf("GetUserByID() error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("nonexistent user", func(t *testing.T) {
		err := svc.DeleteUser(ctx, 99999)
		if err != ErrUserNotFound {
			t.Errorf("DeleteUser() error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("double delete same user", func(t *testing.T) {
		svc, _ := newTestAuthService(t)
		ctx := context.Background()

		user, _ := svc.CreateUser(ctx, "dblDel", "pass1234", "developer")
		if err := svc.DeleteUser(ctx, user.ID); err != nil {
			t.Fatalf("first DeleteUser() error: %v", err)
		}
		err := svc.DeleteUser(ctx, user.ID)
		if err != ErrUserNotFound {
			t.Errorf("second DeleteUser() error = %v, want ErrUserNotFound", err)
		}
	})
}

// ---------------------------------------------------------------------------
// ResetPassword
// ---------------------------------------------------------------------------

func TestAuthService_ResetPassword(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		_, err := svc.CreateUser(ctx, "resetuser", "oldpass12", "developer")
		if err != nil {
			t.Fatalf("CreateUser() error: %v", err)
		}

		err = svc.ResetPassword(ctx, 1, "newpass12")
		if err != nil {
			t.Fatalf("ResetPassword() error: %v", err)
		}

		// Verify new password works
		_, _, _, err = svc.Authenticate(ctx, "resetuser", "newpass12")
		if err != nil {
			t.Fatalf("Authenticate() with new password error: %v", err)
		}

		// Verify old password no longer works
		_, _, _, err = svc.Authenticate(ctx, "resetuser", "oldpass12")
		if err != ErrInvalidCredentials {
			t.Errorf("Authenticate() with old password error = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("nonexistent user", func(t *testing.T) {
		err := svc.ResetPassword(ctx, 99999, "newpass12")
		if err != ErrUserNotFound {
			t.Errorf("ResetPassword() error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("empty new password accepted", func(t *testing.T) {
		svc, _ := newTestAuthService(t)
		ctx := context.Background()

		user, _ := svc.CreateUser(ctx, "emptyPwUser", "oldpass12", "dba")
		err := svc.ResetPassword(ctx, user.ID, "")
		if err != nil {
			t.Fatalf("ResetPassword() with empty password error: %v", err)
		}
		// Should authenticate with empty password
		_, _, _, err = svc.Authenticate(ctx, "emptyPwUser", "")
		if err != nil {
			t.Fatalf("Authenticate() with empty password error: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// ParseToken (bonus: covers token validation edge cases)
// ---------------------------------------------------------------------------

func TestAuthService_ParseToken(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	user, _ := svc.CreateUser(ctx, "tokenuser", "pass", "admin")
	accessToken, _, _, _ := svc.Authenticate(ctx, "tokenuser", "pass")

	t.Run("valid token", func(t *testing.T) {
		claims, err := svc.ParseToken(accessToken)
		if err != nil {
			t.Fatalf("ParseToken() error: %v", err)
		}
		if claims.UserID != user.ID {
			t.Errorf("UserID = %d, want %d", claims.UserID, user.ID)
		}
	})

	t.Run("invalid token string", func(t *testing.T) {
		_, err := svc.ParseToken("not-a-valid-token")
		if err == nil {
			t.Error("expected error for invalid token, got nil")
		}
	})

	t.Run("empty token", func(t *testing.T) {
		_, err := svc.ParseToken("")
		if err == nil {
			t.Error("expected error for empty token, got nil")
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		// Create a service with a different secret and generate a token
		testDB := testutil.NewDB(t)
		otherSvc := NewService(testDB, "different-secret", 1*time.Hour)
		_, _ = otherSvc.CreateUser(ctx, "otheruser", "pass", "developer")
		otherToken, _, _, _ := otherSvc.Authenticate(ctx, "otheruser", "pass")

		_, err := svc.ParseToken(otherToken)
		if err == nil {
			t.Error("expected error for token signed with wrong secret, got nil")
		}
	})
}

// TestResetPassword_StampsPasswordChangedAt verifies that ResetPassword
// advances the user's password_changed_at timestamp. The JWT middleware uses
// this column to reject access tokens issued before the most recent reset —
// without it, "reset password" is cosmetic for anyone already holding a token.
func TestResetPassword_StampsPasswordChangedAt(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	user, err := svc.CreateUser(ctx, "stampuser", "oldpass12", "developer")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Record the initial password_changed_at (set at user creation).
	beforeReset := user.PasswordChangedAt

	// Sleep briefly so the timestamp is distinguishable.
	time.Sleep(10 * time.Millisecond)

	if err := svc.ResetPassword(ctx, user.ID, "newpass12"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	after, err := svc.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}

	if !after.PasswordChangedAt.After(beforeReset) {
		t.Errorf("password_changed_at not advanced: before=%v after=%v", beforeReset, after.PasswordChangedAt)
	}

	// A token issued now should have iat AFTER the reset, so it survives.
	// JWT iat is truncated to whole seconds (golang-jwt TimePrecision default),
	// while PostgreSQL timestamps carry microsecond precision, so we must cross
	// a full second boundary for the comparison to be unambiguous.
	time.Sleep(1100 * time.Millisecond)
	token, _, _, err := svc.Authenticate(ctx, "stampuser", "newpass12")
	if err != nil {
		t.Fatalf("Authenticate after reset: %v", err)
	}
	claims, err := svc.ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.IssuedAt != nil && after.PasswordChangedAt.After(claims.IssuedAt.Time) {
		t.Error("a token issued AFTER the reset should not be older than password_changed_at")
	}
}

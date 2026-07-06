package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func createPermTestUser(t *testing.T, db *sql.DB, username string) int64 {
	t.Helper()
	_, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?, 'hashed', 'admin')`, username)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	var id int64
	err = db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&id)
	if err != nil {
		t.Fatalf("get test user id: %v", err)
	}
	return id
}

func TestPermReqService_CreateRequest(t *testing.T) {
	db := setupTestDB(t)
	// Create a PermissionService (needed for dependency)
	permSvc, err := NewPermissionService(mustWrapDB(db))
	if err != nil {
		t.Fatalf("create PermissionService: %v", err)
	}
	svc := NewPermissionRequestService(mustWrapDB(db), permSvc, nil)
	ctx := context.Background()

	userID := createPermTestUser(t, db, "perm_create")

	pr, err := svc.CreateRequest(ctx, userID, 1, "testdb", "users", "select", "need read access", 2*time.Hour)
	if err != nil {
		t.Fatalf("CreateRequest failed: %v", err)
	}
	if pr.Database != "testdb" {
		t.Errorf("expected database 'testdb', got '%s'", pr.Database)
	}
	if pr.TableName != "users" {
		t.Errorf("expected table 'users', got '%s'", pr.TableName)
	}
	if pr.Status != "PENDING" {
		t.Errorf("expected status PENDING, got '%s'", pr.Status)
	}
	if pr.Actions != "select" {
		t.Errorf("expected actions 'select', got '%s'", pr.Actions)
	}
}

func TestPermReqService_InvalidActions(t *testing.T) {
	db := setupTestDB(t)
	permSvc, err := NewPermissionService(mustWrapDB(db))
	if err != nil {
		t.Fatalf("create PermissionService: %v", err)
	}
	svc := NewPermissionRequestService(mustWrapDB(db), permSvc, nil)
	ctx := context.Background()
	userID := createPermTestUser(t, db, "perm_invalid")

	_, err = svc.CreateRequest(ctx, userID, 1, "testdb", "users", "invalid_action", "test", 1*time.Hour)
	if !errors.Is(err, ErrInvalidAction) {
		t.Errorf("expected ErrInvalidAction, got %v", err)
	}
}

func TestPermReqService_InvalidDuration(t *testing.T) {
	db := setupTestDB(t)
	permSvc, err := NewPermissionService(mustWrapDB(db))
	if err != nil {
		t.Fatalf("create PermissionService: %v", err)
	}
	svc := NewPermissionRequestService(mustWrapDB(db), permSvc, nil)
	ctx := context.Background()
	userID := createPermTestUser(t, db, "perm_dur")

	// Too short
	_, err = svc.CreateRequest(ctx, userID, 1, "testdb", "users", "select", "test", 30*time.Second)
	if !errors.Is(err, ErrInvalidDuration) {
		t.Errorf("expected ErrInvalidDuration for too short, got %v", err)
	}

	// Too long
	_, err = svc.CreateRequest(ctx, userID, 1, "testdb", "users", "select", "test", 100*time.Hour)
	if !errors.Is(err, ErrInvalidDuration) {
		t.Errorf("expected ErrInvalidDuration for too long, got %v", err)
	}
}

func TestPermReqService_ApproveAndReject(t *testing.T) {
	db := setupTestDB(t)
	permSvc, err := NewPermissionService(mustWrapDB(db))
	if err != nil {
		t.Fatalf("create PermissionService: %v", err)
	}
	svc := NewPermissionRequestService(mustWrapDB(db), permSvc, nil)
	ctx := context.Background()
	userID := createPermTestUser(t, db, "perm_approve")
	adminID := createPermTestUser(t, db, "perm_admin")

	pr, err := svc.CreateRequest(ctx, userID, 1, "testdb", "users", "select,update", "need access", 24*time.Hour)
	if err != nil {
		t.Fatalf("CreateRequest failed: %v", err)
	}

	// Approve
	approved, err := svc.ApproveRequest(ctx, pr.ID, adminID, "looks good")
	if err != nil {
		t.Fatalf("ApproveRequest failed: %v", err)
	}
	if approved.Status != "APPROVED" {
		t.Errorf("expected APPROVED, got '%s'", approved.Status)
	}
	if approved.ApproverID != adminID {
		t.Errorf("expected approver %d, got %d", adminID, approved.ApproverID)
	}

	// Cannot approve again
	_, err = svc.ApproveRequest(ctx, pr.ID, adminID, "dup")
	if !errors.Is(err, ErrPermReqAlreadyDone) {
		t.Errorf("expected ErrPermReqAlreadyDone, got %v", err)
	}

	// Cannot reject approved
	_, err = svc.RejectRequest(ctx, pr.ID, adminID, "nope")
	if !errors.Is(err, ErrPermReqAlreadyDone) {
		t.Errorf("expected ErrPermReqAlreadyDone, got %v", err)
	}
}

func TestPermReqService_RejectPending(t *testing.T) {
	db := setupTestDB(t)
	permSvc, err := NewPermissionService(mustWrapDB(db))
	if err != nil {
		t.Fatalf("create PermissionService: %v", err)
	}
	svc := NewPermissionRequestService(mustWrapDB(db), permSvc, nil)
	ctx := context.Background()
	userID := createPermTestUser(t, db, "perm_reject")
	adminID := createPermTestUser(t, db, "perm_reject_admin")

	pr, err := svc.CreateRequest(ctx, userID, 1, "testdb", "orders", "select", "need access", 1*time.Hour)
	if err != nil {
		t.Fatalf("CreateRequest failed: %v", err)
	}

	rejected, err := svc.RejectRequest(ctx, pr.ID, adminID, "security concern")
	if err != nil {
		t.Fatalf("RejectRequest failed: %v", err)
	}
	if rejected.Status != "REJECTED" {
		t.Errorf("expected REJECTED, got '%s'", rejected.Status)
	}
}

func TestPermReqService_RevokeApproved(t *testing.T) {
	db := setupTestDB(t)
	permSvc, err := NewPermissionService(mustWrapDB(db))
	if err != nil {
		t.Fatalf("create PermissionService: %v", err)
	}
	svc := NewPermissionRequestService(mustWrapDB(db), permSvc, nil)
	ctx := context.Background()
	userID := createPermTestUser(t, db, "perm_revoke")
	adminID := createPermTestUser(t, db, "perm_revoke_admin")

	pr, err := svc.CreateRequest(ctx, userID, 1, "testdb", "users", "select", "need access", 24*time.Hour)
	if err != nil {
		t.Fatalf("CreateRequest failed: %v", err)
	}

	_, err = svc.ApproveRequest(ctx, pr.ID, adminID, "ok")
	if err != nil {
		t.Fatalf("ApproveRequest failed: %v", err)
	}

	revoked, err := svc.RevokeRequest(ctx, pr.ID, adminID, "no longer needed")
	if err != nil {
		t.Fatalf("RevokeRequest failed: %v", err)
	}
	if revoked.Status != "REVOKED" {
		t.Errorf("expected REVOKED, got '%s'", revoked.Status)
	}
}

func TestPermReqService_ListRequests(t *testing.T) {
	db := setupTestDB(t)
	permSvc, err := NewPermissionService(mustWrapDB(db))
	if err != nil {
		t.Fatalf("create PermissionService: %v", err)
	}
	svc := NewPermissionRequestService(mustWrapDB(db), permSvc, nil)
	ctx := context.Background()
	userID := createPermTestUser(t, db, "perm_list")

	// Create 3 requests
	for i := 0; i < 3; i++ {
		_, err := svc.CreateRequest(ctx, userID, 1, "testdb", "table1", "select", "test", 1*time.Hour)
		if err != nil {
			t.Fatalf("CreateRequest %d failed: %v", i, err)
		}
	}

	requests, total, err := svc.ListRequests(ctx, 1, 20, "", "")
	if err != nil {
		t.Fatalf("ListRequests failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 total, got %d", total)
	}
	if len(requests) != 3 {
		t.Errorf("expected 3 requests, got %d", len(requests))
	}

	// Filter by status
	_, pendingTotal, _ := svc.ListRequests(ctx, 1, 20, "PENDING", "")
	if pendingTotal != 3 {
		t.Errorf("expected 3 pending, got %d", pendingTotal)
	}
	_, approvedTotal, _ := svc.ListRequests(ctx, 1, 20, "APPROVED", "")
	if approvedTotal != 0 {
		t.Errorf("expected 0 approved, got %d", approvedTotal)
	}
}

func TestPermReqService_ExpireOverdue(t *testing.T) {
	db := setupTestDB(t)
	permSvc, err := NewPermissionService(mustWrapDB(db))
	if err != nil {
		t.Fatalf("create PermissionService: %v", err)
	}
	svc := NewPermissionRequestService(mustWrapDB(db), permSvc, nil)
	ctx := context.Background()

	userID := createPermTestUser(t, db, "perm_expire")

	// Create two requests and approve them.
	req1, err := svc.CreateRequest(ctx, userID, 1, "testdb", "table1", "select", "test", 1*time.Hour)
	if err != nil {
		t.Fatalf("CreateRequest 1: %v", err)
	}
	req2, err := svc.CreateRequest(ctx, userID, 1, "testdb", "table2", "select", "test", 1*time.Hour)
	if err != nil {
		t.Fatalf("CreateRequest 2: %v", err)
	}
	if _, err := svc.ApproveRequest(ctx, req1.ID, userID, "approved"); err != nil {
		t.Fatalf("ApproveRequest 1: %v", err)
	}
	if _, err := svc.ApproveRequest(ctx, req2.ID, userID, "approved"); err != nil {
		t.Fatalf("ApproveRequest 2: %v", err)
	}

	// Force req1 into the past (expired), leave req2 still valid.
	if _, err := db.ExecContext(ctx,
		`UPDATE permission_requests SET expires_at = datetime('now','-1 hour') WHERE id = ?`,
		req1.ID); err != nil {
		t.Fatalf("force expiry: %v", err)
	}

	// Run the expiry sweep — only the overdue one should be marked EXPIRED.
	// (Previously skipped due to a suspected SQLite single-connection deadlock
	// that no longer reproduces; verified safe under -race.)
	count, err := svc.ExpireOverdue(ctx)
	if err != nil {
		t.Fatalf("ExpireOverdue: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 expired, got %d", count)
	}

	// Verify the expired one is now EXPIRED and the still-valid one is APPROVED.
	got1, err := svc.GetRequestByID(ctx, req1.ID)
	if err != nil {
		t.Fatalf("GetRequestByID 1: %v", err)
	}
	if got1.Status != "EXPIRED" {
		t.Errorf("req1 status = %s, want EXPIRED", got1.Status)
	}
	got2, err := svc.GetRequestByID(ctx, req2.ID)
	if err != nil {
		t.Fatalf("GetRequestByID 2: %v", err)
	}
	if got2.Status != "APPROVED" {
		t.Errorf("req2 status = %s, want APPROVED (should not be expired)", got2.Status)
	}
}

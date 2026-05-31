package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
)

// setupTicketTestDB creates an in-memory SQLite database with the required schema.
func setupTicketTestDB(t *testing.T) *sql.DB {
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

// seedTestUser creates a test user and returns the user ID.
func seedTestUser(t *testing.T, testDB *sql.DB, username, role string) int64 {
	t.Helper()
	result, err := testDB.Exec(
		`INSERT INTO users (username, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		username, "$2a$10$testhash", role,
	)
	if err != nil {
		t.Fatalf("failed to seed user %s: %v", username, err)
	}
	id, _ := result.LastInsertId()
	return id
}

// seedTestDatasource creates a test datasource and returns the ID.
func seedTestDatasource(t *testing.T, testDB *sql.DB, name string) int64 {
	t.Helper()
	result, err := testDB.Exec(
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, status, created_at, updated_at) VALUES (?, 'mysql', 'localhost', 3306, 'root', '', 'active', datetime('now'), datetime('now'))`,
		name,
	)
	if err != nil {
		t.Fatalf("failed to seed datasource %s: %v", name, err)
	}
	id, _ := result.LastInsertId()
	return id
}

func TestNewTicketService(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	if svc == nil {
		t.Fatal("NewTicketService returned nil")
	}
}

func TestCreateTicket(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	userID := seedTestUser(t, testDB, "dev1", "developer")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	tests := []struct {
		name         string
		submitterID  int64
		datasourceID int64
		database     string
		sqlContent   string
		dbType       string
		changeReason string
		riskLevel    string
		aiReview     string
		wantErr      error
	}{
		{
			name:         "success - basic ticket",
			submitterID:  userID,
			datasourceID: dsID,
			database:     "mydb",
			sqlContent:   "ALTER TABLE users ADD COLUMN phone VARCHAR(20)",
			dbType:       "mysql",
			changeReason: "add phone field",
			riskLevel:    "medium",
			aiReview:     `{"risk":"medium","suggestion":"low impact"}`,
			wantErr:      nil,
		},
		{
			name:         "success - default db type",
			submitterID:  userID,
			datasourceID: dsID,
			sqlContent:   "UPDATE users SET status = 1 WHERE id = 1",
			dbType:       "",
			riskLevel:    "high",
			wantErr:      nil,
		},
		{
			name:         "error - empty SQL",
			submitterID:  userID,
			datasourceID: dsID,
			sqlContent:   "",
			wantErr:      ErrTicketSQLRequired,
		},
		{
			name:         "error - whitespace SQL",
			submitterID:  userID,
			datasourceID: dsID,
			sqlContent:   "   ",
			wantErr:      ErrTicketSQLRequired,
		},
		{
			name:         "error - no datasource",
			submitterID:  userID,
			datasourceID: 0,
			sqlContent:   "SELECT 1",
			wantErr:      ErrTicketDatasourceRequired,
		},
		{
			name:         "success - long SQL truncated in summary",
			submitterID:  userID,
			datasourceID: dsID,
			sqlContent:   string(make([]byte, 200)), // will be all zeros, but non-empty
			wantErr:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ticket, err := svc.CreateTicket(context.Background(),
				tt.submitterID, "developer", tt.datasourceID, tt.database,
				tt.sqlContent, tt.dbType, tt.changeReason,
				tt.riskLevel, tt.aiReview,
			)

			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("CreateTicket() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("CreateTicket() unexpected error: %v", err)
			}

			if ticket.ID == 0 {
				t.Error("CreateTicket() returned ticket with ID 0")
			}
			if ticket.SubmitterID != tt.submitterID {
				t.Errorf("SubmitterID = %d, want %d", ticket.SubmitterID, tt.submitterID)
			}
			if ticket.DatasourceID != tt.datasourceID {
				t.Errorf("DatasourceID = %d, want %d", ticket.DatasourceID, tt.datasourceID)
			}
			if ticket.Status != model.TicketStatusSubmitted {
				t.Errorf("Status = %s, want SUBMITTED", ticket.Status)
			}
			if ticket.CreatedAt.IsZero() {
				t.Error("CreatedAt is zero")
			}
			if ticket.UpdatedAt.IsZero() {
				t.Error("UpdatedAt is zero")
			}
		})
	}
}

func TestGetTicket(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	userID := seedTestUser(t, testDB, "dev1", "developer")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	// Create a ticket first
	created, err := svc.CreateTicket(context.Background(), userID, "developer", dsID, "mydb", "ALTER TABLE t ADD c INT", "mysql", "test", "low", "")
	if err != nil {
		t.Fatalf("CreateTicket() error: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		ticket, err := svc.GetTicket(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("GetTicket() error: %v", err)
		}
		if ticket.ID != created.ID {
			t.Errorf("ID = %d, want %d", ticket.ID, created.ID)
		}
		if ticket.SubmitterName != "dev1" {
			t.Errorf("SubmitterName = %s, want dev1", ticket.SubmitterName)
		}
		if ticket.Status != model.TicketStatusSubmitted {
			t.Errorf("Status = %s, want SUBMITTED", ticket.Status)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := svc.GetTicket(context.Background(), 99999)
		if err != ErrTicketNotFound {
			t.Errorf("GetTicket(99999) error = %v, want ErrTicketNotFound", err)
		}
	})
}

func TestListTickets(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	userID := seedTestUser(t, testDB, "dev1", "developer")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	// Create multiple tickets
	for i := 0; i < 5; i++ {
		_, err := svc.CreateTicket(context.Background(), userID, "developer", dsID, "mydb",
			fmt.Sprintf("ALTER TABLE t%d ADD c INT", i), "mysql",
			fmt.Sprintf("reason %d", i), "low", "")
		if err != nil {
			t.Fatalf("CreateTicket() error: %v", err)
		}
	}

	t.Run("list all", func(t *testing.T) {
		tickets, total, err := svc.ListTickets(context.Background(), 1, 10, "", "", "", "", "", "", userID, "developer")
		if err != nil {
			t.Fatalf("ListTickets() error: %v", err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
		if len(tickets) != 5 {
			t.Errorf("len(tickets) = %d, want 5", len(tickets))
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		tickets, total, err := svc.ListTickets(context.Background(), 1, 10, "SUBMITTED", "", "", "", "", "", userID, "developer")
		if err != nil {
			t.Fatalf("ListTickets() error: %v", err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
		if len(tickets) != 5 {
			t.Errorf("len(tickets) = %d, want 5", len(tickets))
		}
	})

	t.Run("filter by keyword", func(t *testing.T) {
		tickets, total, err := svc.ListTickets(context.Background(), 1, 10, "", "", "", "", "ALTER TABLE t0", "", userID, "developer")
		if err != nil {
			t.Fatalf("ListTickets() error: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(tickets) != 1 {
			t.Errorf("len(tickets) = %d, want 1", len(tickets))
		}
	})

	t.Run("scope mine", func(t *testing.T) {
		tickets, total, err := svc.ListTickets(context.Background(), 1, 10, "", "", "", "", "", "mine", userID, "developer")
		if err != nil {
			t.Fatalf("ListTickets() error: %v", err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
		if len(tickets) != 5 {
			t.Errorf("len(tickets) = %d, want 5", len(tickets))
		}
	})

	t.Run("scope pending", func(t *testing.T) {
		_, total, err := svc.ListTickets(context.Background(), 1, 10, "", "", "", "", "", "pending", userID, "developer")
		if err != nil {
			t.Fatalf("ListTickets() error: %v", err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		tickets, total, err := svc.ListTickets(context.Background(), 2, 2, "", "", "", "", "", "", userID, "developer")
		if err != nil {
			t.Fatalf("ListTickets() error: %v", err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
		if len(tickets) != 2 {
			t.Errorf("len(tickets) = %d, want 2", len(tickets))
		}
	})

	t.Run("empty result", func(t *testing.T) {
		tickets, total, err := svc.ListTickets(context.Background(), 1, 10, "", "", "", "", "", "mine", 99999, "developer")
		if err != nil {
			t.Fatalf("ListTickets() error: %v", err)
		}
		if total != 0 {
			t.Errorf("total = %d, want 0", total)
		}
		if tickets == nil {
			t.Error("tickets should be non-nil empty slice")
		}
		if len(tickets) != 0 {
			t.Errorf("len(tickets) = %d, want 0", len(tickets))
		}
	})
}

func TestApproveTicket(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	devID := seedTestUser(t, testDB, "dev1", "developer")
	dbaID := seedTestUser(t, testDB, "dba1", "dba")
	adminID := seedTestUser(t, testDB, "admin1", "admin")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	t.Run("developer cannot approve", func(t *testing.T) {
		// Create ticket and set it to PENDING_APPROVAL manually
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)

		_, err := svc.ApproveTicket(context.Background(), ticket.ID, devID, "developer", "ok")
		if err != ErrNoPermission {
			t.Errorf("ApproveTicket() error = %v, want ErrNoPermission", err)
		}
	})

	t.Run("dba can approve", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)

		result, err := svc.ApproveTicket(context.Background(), ticket.ID, dbaID, "dba", "approved by dba")
		if err != nil {
			t.Fatalf("ApproveTicket() error: %v", err)
		}
		if result.Status != model.TicketStatusApproved {
			t.Errorf("Status = %s, want APPROVED", result.Status)
		}
		if result.ReviewerID != dbaID {
			t.Errorf("ReviewerID = %d, want %d", result.ReviewerID, dbaID)
		}
		if result.ReviewerName != "dba1" {
			t.Errorf("ReviewerName = %s, want dba1", result.ReviewerName)
		}
	})

	t.Run("admin can approve", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)

		result, err := svc.ApproveTicket(context.Background(), ticket.ID, adminID, "admin", "approved")
		if err != nil {
			t.Fatalf("ApproveTicket() error: %v", err)
		}
		if result.Status != model.TicketStatusApproved {
			t.Errorf("Status = %s, want APPROVED", result.Status)
		}
	})

	t.Run("cannot approve non-pending ticket", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusSubmitted)

		_, err := svc.ApproveTicket(context.Background(), ticket.ID, dbaID, "dba", "ok")
		if err != ErrInvalidStatusTransition {
			t.Errorf("ApproveTicket() error = %v, want ErrInvalidStatusTransition", err)
		}
	})

	t.Run("cannot approve twice", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)

		_, err := svc.ApproveTicket(context.Background(), ticket.ID, dbaID, "dba", "ok")
		if err != nil {
			t.Fatalf("first ApproveTicket() error: %v", err)
		}

		_, err = svc.ApproveTicket(context.Background(), ticket.ID, dbaID, "dba", "ok again")
		if err != ErrInvalidStatusTransition {
			t.Errorf("second ApproveTicket() error = %v, want ErrInvalidStatusTransition", err)
		}
	})
}

func TestRejectTicket(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	devID := seedTestUser(t, testDB, "dev1", "developer")
	dbaID := seedTestUser(t, testDB, "dba1", "dba")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	t.Run("reject without reason fails", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)

		_, err := svc.RejectTicket(context.Background(), ticket.ID, dbaID, "dba", "")
		if err != ErrRejectReasonRequired {
			t.Errorf("RejectTicket() error = %v, want ErrRejectReasonRequired", err)
		}
	})

	t.Run("reject with reason succeeds", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)

		result, err := svc.RejectTicket(context.Background(), ticket.ID, dbaID, "dba", "too risky")
		if err != nil {
			t.Fatalf("RejectTicket() error: %v", err)
		}
		if result.Status != model.TicketStatusRejected {
			t.Errorf("Status = %s, want REJECTED", result.Status)
		}
		if result.ReviewComment != "too risky" {
			t.Errorf("ReviewComment = %s, want 'too risky'", result.ReviewComment)
		}
	})

	t.Run("developer cannot reject", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)

		_, err := svc.RejectTicket(context.Background(), ticket.ID, devID, "developer", "no good")
		if err != ErrNoPermission {
			t.Errorf("RejectTicket() error = %v, want ErrNoPermission", err)
		}
	})
}

func TestCancelTicket(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	devID := seedTestUser(t, testDB, "dev1", "developer")
	dbaID := seedTestUser(t, testDB, "dba1", "dba")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	t.Run("submitter can cancel submitted", func(t *testing.T) {
		ticket, _ := svc.CreateTicket(context.Background(), devID, "developer", dsID, "mydb", "ALTER TABLE t ADD c INT", "mysql", "test", "low", "")

		result, err := svc.CancelTicket(context.Background(), ticket.ID, devID, "developer", "changed my mind")
		if err != nil {
			t.Fatalf("CancelTicket() error: %v", err)
		}
		if result.Status != model.TicketStatusCancelled {
			t.Errorf("Status = %s, want CANCELLED", result.Status)
		}
	})

	t.Run("dba can cancel", func(t *testing.T) {
		ticket, _ := svc.CreateTicket(context.Background(), devID, "developer", dsID, "mydb", "ALTER TABLE t ADD c INT", "mysql", "test", "low", "")

		result, err := svc.CancelTicket(context.Background(), ticket.ID, dbaID, "dba", "not needed")
		if err != nil {
			t.Fatalf("CancelTicket() error: %v", err)
		}
		if result.Status != model.TicketStatusCancelled {
			t.Errorf("Status = %s, want CANCELLED", result.Status)
		}
	})

	t.Run("cancel without reason fails", func(t *testing.T) {
		ticket, _ := svc.CreateTicket(context.Background(), devID, "developer", dsID, "mydb", "ALTER TABLE t ADD c INT", "mysql", "test", "low", "")

		_, err := svc.CancelTicket(context.Background(), ticket.ID, devID, "developer", "")
		if err != ErrCancelReasonRequired {
			t.Errorf("CancelTicket() error = %v, want ErrCancelReasonRequired", err)
		}
	})

	t.Run("other user cannot cancel", func(t *testing.T) {
		ticket, _ := svc.CreateTicket(context.Background(), devID, "developer", dsID, "mydb", "ALTER TABLE t ADD c INT", "mysql", "test", "low", "")

		otherID := seedTestUser(t, testDB, "dev2", "developer")
		_, err := svc.CancelTicket(context.Background(), ticket.ID, otherID, "developer", "cancel it")
		if err != ErrNoPermission {
			t.Errorf("CancelTicket() error = %v, want ErrNoPermission", err)
		}
	})

	t.Run("cannot cancel done ticket", func(t *testing.T) {
		// Create ticket and set it to DONE manually
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusDone)

		_, err := svc.CancelTicket(context.Background(), ticket.ID, devID, "developer", "cancel")
		if err != ErrTicketNotCancellable {
			t.Errorf("CancelTicket() error = %v, want ErrTicketNotCancellable", err)
		}
	})

	t.Run("cannot cancel rejected ticket", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusRejected)

		_, err := svc.CancelTicket(context.Background(), ticket.ID, devID, "developer", "cancel")
		if err != ErrTicketNotCancellable {
			t.Errorf("CancelTicket() error = %v, want ErrTicketNotCancellable", err)
		}
	})
}

func TestExecuteTicket(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	devID := seedTestUser(t, testDB, "dev1", "developer")
	dbaID := seedTestUser(t, testDB, "dba1", "dba")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	t.Run("cannot execute non-approved ticket", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusSubmitted)

		_, err := svc.ExecuteTicket(context.Background(), ticket.ID, devID, "developer", "dev1")
		if err != ErrTicketNotExecutable {
			t.Errorf("ExecuteTicket() error = %v, want ErrTicketNotExecutable", err)
		}
	})

	t.Run("other user cannot execute", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusApproved)

		otherID := seedTestUser(t, testDB, "dev2", "developer")
		_, err := svc.ExecuteTicket(context.Background(), ticket.ID, otherID, "developer", "dev2")
		if err != ErrNoPermission {
			t.Errorf("ExecuteTicket() error = %v, want ErrNoPermission", err)
		}
	})

	t.Run("execute fails without datasource service", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusApproved)

		_, err := svc.ExecuteTicket(context.Background(), ticket.ID, devID, "developer", "dev1")
		if err == nil {
			t.Error("expected error when dsSvc is nil")
		}
		// Should remain APPROVED (not transitioned to EXECUTING since dsSvc is nil)
		updated, _ := svc.GetTicket(context.Background(), ticket.ID)
		if updated.Status != model.TicketStatusApproved {
			t.Errorf("Status = %s, want APPROVED (execution service not configured, no state change)", updated.Status)
		}
	})

	t.Run("dba can pass permission check", func(t *testing.T) {
		// Test that dba CAN reach the execute phase (permission passes)
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusApproved)

		_, err := svc.ExecuteTicket(context.Background(), ticket.ID, dbaID, "dba", "dba1")
		if err == nil {
			t.Error("expected error when dsSvc is nil")
		}
		updated, _ := svc.GetTicket(context.Background(), ticket.ID)
		if updated.Status != model.TicketStatusApproved {
			t.Errorf("Status = %s, want APPROVED", updated.Status)
		}
	})
}

func TestStateMachine(t *testing.T) {
	t.Run("valid transitions", func(t *testing.T) {
		valid := []struct {
			from model.TicketStatus
			to   model.TicketStatus
		}{
			{model.TicketStatusSubmitted, model.TicketStatusAIReviewed},
			{model.TicketStatusSubmitted, model.TicketStatusCancelled},
			{model.TicketStatusAIReviewed, model.TicketStatusPendingApproval},
			{model.TicketStatusAIReviewed, model.TicketStatusCancelled},
			{model.TicketStatusPendingApproval, model.TicketStatusApproved},
			{model.TicketStatusPendingApproval, model.TicketStatusRejected},
			{model.TicketStatusPendingApproval, model.TicketStatusCancelled},
			{model.TicketStatusApproved, model.TicketStatusExecuting},
			{model.TicketStatusApproved, model.TicketStatusCancelled},
			{model.TicketStatusExecuting, model.TicketStatusDone},
			{model.TicketStatusExecuting, model.TicketStatusFailed},
			{model.TicketStatusRejected, model.TicketStatusSubmitted},
		}

		for _, tt := range valid {
			t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
				if !CanTransition(tt.from, tt.to) {
					t.Errorf("CanTransition(%s, %s) = false, want true", tt.from, tt.to)
				}
			})
		}
	})

	t.Run("invalid transitions", func(t *testing.T) {
		invalid := []struct {
			from model.TicketStatus
			to   model.TicketStatus
		}{
			{model.TicketStatusSubmitted, model.TicketStatusApproved},
			{model.TicketStatusSubmitted, model.TicketStatusDone},
			{model.TicketStatusSubmitted, model.TicketStatusRejected},
			{model.TicketStatusApproved, model.TicketStatusSubmitted},
			{model.TicketStatusDone, model.TicketStatusSubmitted},
			{model.TicketStatusDone, model.TicketStatusCancelled},
			{model.TicketStatusRejected, model.TicketStatusApproved},
			{model.TicketStatusCancelled, model.TicketStatusSubmitted},
			{model.TicketStatusExecuting, model.TicketStatusCancelled},
		}

		for _, tt := range invalid {
			t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
				if CanTransition(tt.from, tt.to) {
					t.Errorf("CanTransition(%s, %s) = true, want false", tt.from, tt.to)
				}
			})
		}
	})

	t.Run("terminal states have no outgoing transitions", func(t *testing.T) {
		terminals := []model.TicketStatus{
			model.TicketStatusDone,
			model.TicketStatusCancelled,
		}
		for _, terminal := range terminals {
			transitions := validTransitions[terminal]
			if len(transitions) != 0 {
				t.Errorf("terminal state %s should have no transitions, got %v", terminal, transitions)
			}
		}
	})
}

func TestFullWorkflow(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	devID := seedTestUser(t, testDB, "dev1", "developer")
	dbaID := seedTestUser(t, testDB, "dba1", "dba")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	// Step 1: Create ticket
	ticket, err := svc.CreateTicket(context.Background(), devID, "developer", dsID, "mydb",
		"ALTER TABLE users ADD COLUMN phone VARCHAR(20)",
		"mysql", "add phone column", "medium", `{"risk":"medium"}`)
	if err != nil {
		t.Fatalf("CreateTicket() error: %v", err)
	}
	if ticket.Status != model.TicketStatusSubmitted {
		t.Fatalf("initial status = %s, want SUBMITTED", ticket.Status)
	}

	// Step 2: Simulate AI review -> set to PENDING_APPROVAL
	setTicketStatus(t, testDB, ticket.ID, model.TicketStatusPendingApproval)

	// Step 3: DBA approves
	ticket, err = svc.ApproveTicket(context.Background(), ticket.ID, dbaID, "dba", "looks good")
	if err != nil {
		t.Fatalf("ApproveTicket() error: %v", err)
	}
	if ticket.Status != model.TicketStatusApproved {
		t.Fatalf("approved status = %s, want APPROVED", ticket.Status)
	}
	if ticket.ReviewerName != "dba1" {
		t.Errorf("ReviewerName = %s, want dba1", ticket.ReviewerName)
	}

	// Step 4: Developer executes
	ticket, err = svc.ExecuteTicket(context.Background(), ticket.ID, devID, "developer", "dev1")
	if err != nil {
		t.Fatalf("ExecuteTicket() error: %v", err)
	}
	if ticket.Status != model.TicketStatusDone {
		t.Fatalf("final status = %s, want DONE", ticket.Status)
	}
	if ticket.ExecutedAt == nil {
		t.Error("ExecutedAt should not be nil after execution")
	}
}

func TestRejectWorkflow(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	devID := seedTestUser(t, testDB, "dev1", "developer")
	dbaID := seedTestUser(t, testDB, "dba1", "dba")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	ticket, _ := svc.CreateTicket(context.Background(), devID, "developer", dsID, "mydb",
		"DELETE FROM users", "mysql", "cleanup", "high", `{"risk":"high"}`)

	setTicketStatus(t, testDB, ticket.ID, model.TicketStatusPendingApproval)

	ticket, err := svc.RejectTicket(context.Background(), ticket.ID, dbaID, "dba", "too dangerous without WHERE")
	if err != nil {
		t.Fatalf("RejectTicket() error: %v", err)
	}
	if ticket.Status != model.TicketStatusRejected {
		t.Fatalf("status = %s, want REJECTED", ticket.Status)
	}
}

func TestCancelWorkflow(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	devID := seedTestUser(t, testDB, "dev1", "developer")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	ticket, _ := svc.CreateTicket(context.Background(), devID, "developer", dsID, "mydb",
		"UPDATE users SET name = 'test'", "mysql", "fix data", "high", "")

	ticket, err := svc.CancelTicket(context.Background(), ticket.ID, devID, "developer", "no longer needed")
	if err != nil {
		t.Fatalf("CancelTicket() error: %v", err)
	}
	if ticket.Status != model.TicketStatusCancelled {
		t.Fatalf("status = %s, want CANCELLED", ticket.Status)
	}
}

func TestAuditLogWritten(t *testing.T) {
	testDB := setupTicketTestDB(t)
	auditSvc := NewAuditService(mustWrapDB(testDB), 100, 50*time.Millisecond)
	defer auditSvc.Close()
	svc := NewTicketService(testDB, auditSvc, nil)
	devID := seedTestUser(t, testDB, "dev1", "developer")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	// Create a ticket - this should write an audit log
	_, err := svc.CreateTicket(context.Background(), devID, "developer", dsID, "mydb", "ALTER TABLE t ADD c INT", "mysql", "test", "low", "")
	if err != nil {
		t.Fatalf("CreateTicket() error: %v", err)
	}

	// Give the async audit log writer time to complete
	time.Sleep(200 * time.Millisecond)

	var count int
	err = testDB.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE action = 'ticket_create'").Scan(&count)
	if err != nil {
		t.Fatalf("query audit_logs: %v", err)
	}
	if count != 1 {
		t.Errorf("audit log count = %d, want 1", count)
	}
}

// Helper: create a ticket and set its status directly in the DB for testing.
func createTicketAtStatus(t *testing.T, testDB *sql.DB, svc *TicketService, userID, dsID int64, status model.TicketStatus) *model.Ticket {
	t.Helper()
	ticket, err := svc.CreateTicket(context.Background(), userID, "developer", dsID, "mydb", "ALTER TABLE t ADD c INT", "mysql", "test reason", "medium", "")
	if err != nil {
		t.Fatalf("CreateTicket() error: %v", err)
	}
	setTicketStatus(t, testDB, ticket.ID, status)

	ticket, err = svc.GetTicket(context.Background(), ticket.ID)
	if err != nil {
		t.Fatalf("GetTicket() error: %v", err)
	}
	return ticket
}

// Helper: directly set a ticket's status in the DB.
func setTicketStatus(t *testing.T, testDB *sql.DB, ticketID int64, status model.TicketStatus) {
	t.Helper()
	_, err := testDB.Exec(`UPDATE tickets SET status = ?, updated_at = datetime('now') WHERE id = ?`, status, ticketID)
	if err != nil {
		t.Fatalf("setTicketStatus(%d, %s) error: %v", ticketID, status, err)
	}
}

func TestMain(m *testing.M) {
	// Ensure the SQLite driver is imported
	_ = os.TempDir()
	code := m.Run()
	os.Exit(code)
}

func TestScheduleTicket(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	userID := seedTestUser(t, testDB, "dev1", "developer")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	// Create a ticket
	ticket := createTicketAtStatus(t, testDB, svc, userID, dsID, model.TicketStatusApproved)

	// Schedule it
	scheduledAt := time.Now().Add(24 * time.Hour)
	result, err := svc.ScheduleTicket(context.Background(), ticket.ID, userID, "developer", scheduledAt)
	if err != nil {
		t.Fatalf("ScheduleTicket: %v", err)
	}
	if result.Status != model.TicketStatusScheduled {
		t.Errorf("status = %v, want %v", result.Status, model.TicketStatusScheduled)
	}
	if result.ScheduledAt == nil {
		t.Error("ScheduledAt should not be nil")
	}
}

func TestScheduleTicket_NotApproved(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	userID := seedTestUser(t, testDB, "dev1", "developer")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	ticket := createTicketAtStatus(t, testDB, svc, userID, dsID, model.TicketStatusSubmitted)
	// Keep as SUBMITTED, not APPROVED

	_, err := svc.ScheduleTicket(context.Background(), ticket.ID, userID, "developer", time.Now().Add(24*time.Hour))
	if err != ErrTicketNotSchedulable {
		t.Errorf("err = %v, want ErrTicketNotSchedulable", err)
	}
}

func TestScheduleTicket_NoPermission(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	userID := seedTestUser(t, testDB, "dev1", "developer")
	userID2 := seedTestUser(t, testDB, "dev2", "developer")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	ticket := createTicketAtStatus(t, testDB, svc, userID, dsID, model.TicketStatusApproved)

	// Different user tries to schedule
	_, err := svc.ScheduleTicket(context.Background(), ticket.ID, userID2, "developer", time.Now().Add(24*time.Hour))
	if err != ErrNoPermission {
		t.Errorf("err = %v, want ErrNoPermission", err)
	}
}

func TestScheduleTicket_DBACanSchedule(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	userID := seedTestUser(t, testDB, "dev1", "developer")
	dbaID := seedTestUser(t, testDB, "dba1", "dba")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	ticket := createTicketAtStatus(t, testDB, svc, userID, dsID, model.TicketStatusApproved)

	// DBA can schedule anyone's ticket
	result, err := svc.ScheduleTicket(context.Background(), ticket.ID, dbaID, "dba", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("ScheduleTicket: %v", err)
	}
	if result.Status != model.TicketStatusScheduled {
		t.Errorf("status = %v, want %v", result.Status, model.TicketStatusScheduled)
	}
}

func TestCancelSchedule(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	userID := seedTestUser(t, testDB, "dev1", "developer")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	ticket := createTicketAtStatus(t, testDB, svc, userID, dsID, model.TicketStatusApproved)

	// Schedule first
	scheduledAt := time.Now().Add(24 * time.Hour)
	svc.ScheduleTicket(context.Background(), ticket.ID, userID, "developer", scheduledAt)

	// Cancel the schedule
	result, err := svc.CancelSchedule(context.Background(), ticket.ID, userID, "developer")
	if err != nil {
		t.Fatalf("CancelSchedule: %v", err)
	}
	if result.Status != model.TicketStatusApproved {
		t.Errorf("status = %v, want %v", result.Status, model.TicketStatusApproved)
	}
	if result.ScheduledAt != nil {
		t.Error("ScheduledAt should be nil after cancel")
	}
}

func TestCancelSchedule_NotScheduled(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	userID := seedTestUser(t, testDB, "dev1", "developer")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	ticket := createTicketAtStatus(t, testDB, svc, userID, dsID, model.TicketStatusSubmitted)
	// Keep as APPROVED, not SCHEDULED

	_, err := svc.CancelSchedule(context.Background(), ticket.ID, userID, "developer")
	if err != ErrTicketNotScheduled {
		t.Errorf("err = %v, want ErrTicketNotScheduled", err)
	}
}

func TestCancelSchedule_NoPermission(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)
	userID := seedTestUser(t, testDB, "dev1", "developer")
	userID2 := seedTestUser(t, testDB, "dev2", "developer")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	ticket := createTicketAtStatus(t, testDB, svc, userID, dsID, model.TicketStatusApproved)
	svc.ScheduleTicket(context.Background(), ticket.ID, userID, "developer", time.Now().Add(24*time.Hour))

	// Different user tries to cancel
	_, err := svc.CancelSchedule(context.Background(), ticket.ID, userID2, "developer")
	if err != ErrNoPermission {
		t.Errorf("err = %v, want ErrNoPermission", err)
	}
}

func TestSetNotifyService(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)

	// Initially nil
	svc.SetNotifyService(nil)
	// Should not panic
}

// ---------------------------------------------------------------------------
// Batch operations tests
// ---------------------------------------------------------------------------

func TestBatchApprove(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)

	devID := seedTestUser(t, testDB, "batchdev", "developer")
	dbaID := seedTestUser(t, testDB, "batchdba", "dba")
	dsID := seedTestDatasource(t, testDB, "batchds")

	t.Run("successful batch approve", func(t *testing.T) {
		t1 := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)
		t2 := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)
		t3 := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)

		result, err := svc.BatchApprove(context.Background(), []int64{t1.ID, t2.ID, t3.ID}, dbaID, "dba", "batch approved")
		if err != nil {
			t.Fatalf("BatchApprove() error: %v", err)
		}

		if result.Total != 3 {
			t.Errorf("Total = %d, want 3", result.Total)
		}
		if result.Succeeded != 3 {
			t.Errorf("Succeeded = %d, want 3", result.Succeeded)
		}
		if result.Failed != 0 {
			t.Errorf("Failed = %d, want 0", result.Failed)
		}
	})

	t.Run("partial failure with wrong status", func(t *testing.T) {
		pending := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)
		approved := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusApproved)
		pending2 := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)

		result, err := svc.BatchApprove(context.Background(), []int64{pending.ID, approved.ID, pending2.ID}, dbaID, "dba", "batch")
		if err != nil {
			t.Fatalf("BatchApprove() error: %v", err)
		}

		if result.Total != 3 {
			t.Errorf("Total = %d, want 3", result.Total)
		}
		if result.Succeeded != 2 {
			t.Errorf("Succeeded = %d, want 2", result.Succeeded)
		}
		if result.Failed != 1 {
			t.Errorf("Failed = %d, want 1", result.Failed)
		}

		// Find the failed one
		for _, r := range result.Results {
			if r.TicketID == approved.ID {
				if r.Success {
					t.Error("approved ticket should have failed")
				}
				if r.Error == "" {
					t.Error("expected error message for failed ticket")
				}
			}
		}
	})

	t.Run("developer cannot batch approve", func(t *testing.T) {
		_, err := svc.BatchApprove(context.Background(), []int64{1, 2}, devID, "developer", "nope")
		if err != ErrNoPermission {
			t.Errorf("BatchApprove() error = %v, want ErrNoPermission", err)
		}
	})

	t.Run("nonexistent ticket returns error", func(t *testing.T) {
		pending := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)

		result, err := svc.BatchApprove(context.Background(), []int64{pending.ID, 99999}, dbaID, "dba", "ok")
		if err != nil {
			t.Fatalf("BatchApprove() error: %v", err)
		}

		if result.Succeeded != 1 {
			t.Errorf("Succeeded = %d, want 1", result.Succeeded)
		}
		if result.Failed != 1 {
			t.Errorf("Failed = %d, want 1", result.Failed)
		}
	})
}

func TestBatchReject(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(testDB, nil, nil)

	devID := seedTestUser(t, testDB, "batchdev2", "developer")
	dbaID := seedTestUser(t, testDB, "batchdba2", "dba")
	dsID := seedTestDatasource(t, testDB, "batchds2")

	t.Run("successful batch reject", func(t *testing.T) {
		t1 := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)
		t2 := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)

		result, err := svc.BatchReject(context.Background(), []int64{t1.ID, t2.ID}, dbaID, "dba", "不符合规范")
		if err != nil {
			t.Fatalf("BatchReject() error: %v", err)
		}

		if result.Total != 2 {
			t.Errorf("Total = %d, want 2", result.Total)
		}
		if result.Succeeded != 2 {
			t.Errorf("Succeeded = %d, want 2", result.Succeeded)
		}
		if result.Failed != 0 {
			t.Errorf("Failed = %d, want 0", result.Failed)
		}
	})

	t.Run("empty reason returns error", func(t *testing.T) {
		_, err := svc.BatchReject(context.Background(), []int64{1, 2}, dbaID, "dba", "")
		if err != ErrRejectReasonRequired {
			t.Errorf("BatchReject() error = %v, want ErrRejectReasonRequired", err)
		}
	})

	t.Run("whitespace-only reason returns error", func(t *testing.T) {
		_, err := svc.BatchReject(context.Background(), []int64{1, 2}, dbaID, "dba", "   ")
		if err != ErrRejectReasonRequired {
			t.Errorf("BatchReject() error = %v, want ErrRejectReasonRequired", err)
		}
	})

	t.Run("developer cannot batch reject", func(t *testing.T) {
		_, err := svc.BatchReject(context.Background(), []int64{1, 2}, devID, "developer", "nope")
		if err != ErrNoPermission {
			t.Errorf("BatchReject() error = %v, want ErrNoPermission", err)
		}
	})

	t.Run("partial failure with wrong status", func(t *testing.T) {
		pending := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)
		rejected := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusRejected)

		result, err := svc.BatchReject(context.Background(), []int64{pending.ID, rejected.ID}, dbaID, "dba", "批量驳回")
		if err != nil {
			t.Fatalf("BatchReject() error: %v", err)
		}

		if result.Succeeded != 1 {
			t.Errorf("Succeeded = %d, want 1", result.Succeeded)
		}
		if result.Failed != 1 {
			t.Errorf("Failed = %d, want 1", result.Failed)
		}
	})
}

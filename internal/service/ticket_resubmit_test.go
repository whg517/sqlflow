package service

import (
	"context"
	"testing"

	"github.com/whg517/sqlflow/internal/model"
)

func TestResubmitTicket(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(mustWrapDB(testDB), nil, nil)
	devID := seedTestUser(t, testDB, "dev1", "developer")
	dbaID := seedTestUser(t, testDB, "dba1", "dba")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	t.Run("success - resubmit rejected ticket", func(t *testing.T) {
		// Create ticket, move to PENDING_APPROVAL, then reject
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)
		_, err := svc.RejectTicket(context.Background(), ticket.ID, dbaID, "dba", "SQL有问题，加WHERE条件")
		if err != nil {
			t.Fatalf("RejectTicket() error: %v", err)
		}

		// Resubmit with fixed SQL
		result, err := svc.ResubmitTicket(context.Background(), ticket.ID, devID,
			"DELETE FROM users WHERE status = 0", "加了WHERE条件")
		if err != nil {
			t.Fatalf("ResubmitTicket() error: %v", err)
		}

		if result.Status != model.TicketStatusSubmitted {
			t.Errorf("Status = %s, want SUBMITTED", result.Status)
		}
		if result.Revision != 2 {
			t.Errorf("Revision = %d, want 2", result.Revision)
		}
		if result.SQLContent != "DELETE FROM users WHERE status = 0" {
			t.Errorf("SQLContent = %s, want fixed SQL", result.SQLContent)
		}
		if result.ChangeReason != "加了WHERE条件" {
			t.Errorf("ChangeReason = %s, want '加了WHERE条件'", result.ChangeReason)
		}
		if result.RiskLevel != "" {
			t.Errorf("RiskLevel should be cleared, got %s", result.RiskLevel)
		}
		if result.ReviewerID != 0 {
			t.Errorf("ReviewerID should be 0, got %d", result.ReviewerID)
		}
	})

	t.Run("cannot resubmit non-rejected ticket", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)

		_, err := svc.ResubmitTicket(context.Background(), ticket.ID, devID, "SELECT 1", "test")
		if err != ErrTicketNotResubmittable {
			t.Errorf("ResubmitTicket() error = %v, want ErrTicketNotResubmittable", err)
		}
	})

	t.Run("non-submitter cannot resubmit", func(t *testing.T) {
		otherID := seedTestUser(t, testDB, "other_dev", "developer")
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)
		svc.RejectTicket(context.Background(), ticket.ID, dbaID, "dba", "reject")

		_, err := svc.ResubmitTicket(context.Background(), ticket.ID, otherID, "SELECT 1", "test")
		if err != ErrNoPermission {
			t.Errorf("ResubmitTicket() error = %v, want ErrNoPermission", err)
		}
	})

	t.Run("empty SQL fails", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)
		svc.RejectTicket(context.Background(), ticket.ID, dbaID, "dba", "reject")

		_, err := svc.ResubmitTicket(context.Background(), ticket.ID, devID, "", "test")
		if err != ErrTicketSQLRequired {
			t.Errorf("ResubmitTicket() error = %v, want ErrTicketSQLRequired", err)
		}
	})

	t.Run("revision increments on multiple resubmits", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)
		svc.RejectTicket(context.Background(), ticket.ID, dbaID, "dba", "reject 1")

		result, err := svc.ResubmitTicket(context.Background(), ticket.ID, devID, "DELETE FROM t WHERE id = 1", "fix 1")
		if err != nil {
			t.Fatalf("first resubmit: %v", err)
		}
		if result.Revision != 2 {
			t.Errorf("first resubmit revision = %d, want 2", result.Revision)
		}

		// Reject again and resubmit again
		setTicketStatus(t, testDB, ticket.ID, model.TicketStatusPendingApproval)
		svc.RejectTicket(context.Background(), ticket.ID, dbaID, "dba", "reject 2")

		result, err = svc.ResubmitTicket(context.Background(), ticket.ID, devID, "DELETE FROM t WHERE id = 1 LIMIT 10", "fix 2")
		if err != nil {
			t.Fatalf("second resubmit: %v", err)
		}
		if result.Revision != 3 {
			t.Errorf("second resubmit revision = %d, want 3", result.Revision)
		}
	})

	t.Run("not found ticket", func(t *testing.T) {
		_, err := svc.ResubmitTicket(context.Background(), 99999, devID, "SELECT 1", "test")
		if err != ErrTicketNotFound {
			t.Errorf("ResubmitTicket() error = %v, want ErrTicketNotFound", err)
		}
	})
}

func TestListRevisions(t *testing.T) {
	testDB := setupTicketTestDB(t)
	svc := NewTicketService(mustWrapDB(testDB), nil, nil)
	devID := seedTestUser(t, testDB, "dev1", "developer")
	dbaID := seedTestUser(t, testDB, "dba1", "dba")
	dsID := seedTestDatasource(t, testDB, "test-mysql")

	t.Run("no revisions for new ticket", func(t *testing.T) {
		ticket, _ := svc.CreateTicket(context.Background(), devID, "developer", dsID, "mydb", "ALTER TABLE t ADD c INT", "mysql", "test", "low", "")

		revisions, err := svc.ListRevisions(context.Background(), ticket.ID)
		if err != nil {
			t.Fatalf("ListRevisions() error: %v", err)
		}
		if len(revisions) != 0 {
			t.Errorf("expected 0 revisions, got %d", len(revisions))
		}
	})

	t.Run("revisions after resubmit", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)
		svc.RejectTicket(context.Background(), ticket.ID, dbaID, "dba", "需要加WHERE")

		// Resubmit
		svc.ResubmitTicket(context.Background(), ticket.ID, devID, "DELETE FROM t WHERE id = 1", "加了WHERE")

		revisions, err := svc.ListRevisions(context.Background(), ticket.ID)
		if err != nil {
			t.Fatalf("ListRevisions() error: %v", err)
		}
		if len(revisions) != 1 {
			t.Fatalf("expected 1 revision, got %d", len(revisions))
		}

		rev := revisions[0]
		if rev.TicketID != ticket.ID {
			t.Errorf("TicketID = %d, want %d", rev.TicketID, ticket.ID)
		}
		if rev.Revision != 1 {
			t.Errorf("Revision = %d, want 1", rev.Revision)
		}
		if rev.Status != model.TicketStatusRejected {
			t.Errorf("Status = %s, want REJECTED", rev.Status)
		}
		if rev.ReviewComment != "需要加WHERE" {
			t.Errorf("ReviewComment = %s, want '需要加WHERE'", rev.ReviewComment)
		}
	})

	t.Run("multiple revisions in order", func(t *testing.T) {
		ticket := createTicketAtStatus(t, testDB, svc, devID, dsID, model.TicketStatusPendingApproval)
		svc.RejectTicket(context.Background(), ticket.ID, dbaID, "dba", "reject 1")
		svc.ResubmitTicket(context.Background(), ticket.ID, devID, "DELETE FROM t WHERE id = 1", "fix 1")

		setTicketStatus(t, testDB, ticket.ID, model.TicketStatusPendingApproval)
		svc.RejectTicket(context.Background(), ticket.ID, dbaID, "dba", "reject 2")
		svc.ResubmitTicket(context.Background(), ticket.ID, devID, "DELETE FROM t WHERE id = 1 LIMIT 10", "fix 2")

		revisions, err := svc.ListRevisions(context.Background(), ticket.ID)
		if err != nil {
			t.Fatalf("ListRevisions() error: %v", err)
		}
		if len(revisions) != 2 {
			t.Fatalf("expected 2 revisions, got %d", len(revisions))
		}

		if revisions[0].Revision != 1 {
			t.Errorf("first revision = %d, want 1", revisions[0].Revision)
		}
		if revisions[1].Revision != 2 {
			t.Errorf("second revision = %d, want 2", revisions[1].Revision)
		}
	})
}

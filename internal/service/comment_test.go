package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
)

// setupCommentTest creates a test DB with a user and a ticket for comment tests.
func setupCommentTest(t *testing.T) (*sql.DB, int64, int64) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Insert a test user
	userRes, err := database.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)", "commenter", "hash", "developer")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	userID, _ := userRes.LastInsertId()

	// Insert a test datasource
	dsRes, err := database.Exec("INSERT INTO datasources (name, type, host, port, username, password_encrypted, status) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"test-ds", "mysql", "10.0.0.1", 3306, "root", "enc", "active")
	if err != nil {
		t.Fatalf("insert datasource: %v", err)
	}
	dsID, _ := dsRes.LastInsertId()

	// Insert a test ticket
	ticketRes, err := database.Exec(
		`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, risk_level, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, dsID, "testdb", "SELECT 1", "SELECT 1", "mysql", "test", "low", model.TicketStatusDone,
	)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	ticketID, _ := ticketRes.LastInsertId()

	return database.DB, userID, ticketID
}

func TestCommentService_CreateComment(t *testing.T) {
	testDB, userID, ticketID := setupCommentTest(t)
	svc := NewCommentService(testDB)
	ctx := context.Background()

	comment, err := svc.CreateComment(ctx, ticketID, userID, "Nice work!", 0)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if comment.ID == 0 {
		t.Error("expected non-zero comment ID")
	}
	if comment.Content != "Nice work!" {
		t.Errorf("content = %q, want %q", comment.Content, "Nice work!")
	}
	if comment.OrderID != ticketID {
		t.Errorf("orderID = %d, want %d", comment.OrderID, ticketID)
	}
	if comment.UserID != userID {
		t.Errorf("userID = %d, want %d", comment.UserID, userID)
	}
	if comment.ParentID != 0 {
		t.Errorf("parentID = %d, want 0", comment.ParentID)
	}
}

func TestCommentService_CreateComment_EmptyContent(t *testing.T) {
	testDB, userID, ticketID := setupCommentTest(t)
	svc := NewCommentService(testDB)

	_, err := svc.CreateComment(context.Background(), ticketID, userID, "", 0)
	if err != ErrCommentContentEmpty {
		t.Errorf("err = %v, want ErrCommentContentEmpty", err)
	}

	_, err = svc.CreateComment(context.Background(), ticketID, userID, "   ", 0)
	if err != ErrCommentContentEmpty {
		t.Errorf("whitespace-only: err = %v, want ErrCommentContentEmpty", err)
	}
}

func TestCommentService_CreateComment_NonexistentTicket(t *testing.T) {
	testDB, userID, _ := setupCommentTest(t)
	svc := NewCommentService(testDB)

	_, err := svc.CreateComment(context.Background(), 99999, userID, "Hello", 0)
	if err != ErrOrderNotFound {
		t.Errorf("err = %v, want ErrOrderNotFound", err)
	}
}

func TestCommentService_CreateComment_ReplyToComment(t *testing.T) {
	testDB, userID, ticketID := setupCommentTest(t)
	svc := NewCommentService(testDB)
	ctx := context.Background()

	// Create parent comment
	parent, err := svc.CreateComment(ctx, ticketID, userID, "Parent comment", 0)
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	// Reply to parent
	reply, err := svc.CreateComment(ctx, ticketID, userID, "Reply", parent.ID)
	if err != nil {
		t.Fatalf("create reply: %v", err)
	}
	if reply.ParentID != parent.ID {
		t.Errorf("reply parentID = %d, want %d", reply.ParentID, parent.ID)
	}
}

func TestCommentService_CreateComment_ReplyToNonexistentComment(t *testing.T) {
	testDB, userID, ticketID := setupCommentTest(t)
	svc := NewCommentService(testDB)

	_, err := svc.CreateComment(context.Background(), ticketID, userID, "Reply", 99999)
	if err != ErrCommentNotFound {
		t.Errorf("err = %v, want ErrCommentNotFound", err)
	}
}

func TestCommentService_ListComments(t *testing.T) {
	testDB, userID, ticketID := setupCommentTest(t)
	svc := NewCommentService(testDB)
	ctx := context.Background()

	// Create multiple comments
	for _, content := range []string{"First", "Second", "Third"} {
		_, err := svc.CreateComment(ctx, ticketID, userID, content, 0)
		if err != nil {
			t.Fatalf("create comment %q: %v", content, err)
		}
	}

	comments, err := svc.ListComments(ctx, ticketID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 3 {
		t.Errorf("len(comments) = %d, want 3", len(comments))
	}

	// Comments should be ordered by created_at ASC
	if comments[0].Content != "First" {
		t.Errorf("first comment = %q, want %q", comments[0].Content, "First")
	}
}

func TestCommentService_ListComments_Empty(t *testing.T) {
	testDB, _, ticketID := setupCommentTest(t)
	svc := NewCommentService(testDB)

	comments, err := svc.ListComments(context.Background(), ticketID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("expected 0 comments, got %d", len(comments))
	}
}

func TestCommentService_DeleteComment_AsOwner(t *testing.T) {
	testDB, userID, ticketID := setupCommentTest(t)
	svc := NewCommentService(testDB)
	ctx := context.Background()

	comment, err := svc.CreateComment(ctx, ticketID, userID, "To delete", 0)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	err = svc.DeleteComment(ctx, comment.ID, userID, "developer")
	if err != nil {
		t.Fatalf("DeleteComment as owner: %v", err)
	}

	// Verify deleted
	comments, _ := svc.ListComments(ctx, ticketID)
	if len(comments) != 0 {
		t.Errorf("expected 0 comments after delete, got %d", len(comments))
	}
}

func TestCommentService_DeleteComment_AsAdmin(t *testing.T) {
	testDB, userID, ticketID := setupCommentTest(t)
	svc := NewCommentService(testDB)
	ctx := context.Background()

	comment, err := svc.CreateComment(ctx, ticketID, userID, "Admin delete", 0)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	// Different user but admin role
	err = svc.DeleteComment(ctx, comment.ID, userID+999, "admin")
	if err != nil {
		t.Fatalf("DeleteComment as admin: %v", err)
	}
}

func TestCommentService_DeleteComment_AsDBA(t *testing.T) {
	testDB, userID, ticketID := setupCommentTest(t)
	svc := NewCommentService(testDB)
	ctx := context.Background()

	comment, err := svc.CreateComment(ctx, ticketID, userID, "DBA delete", 0)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	err = svc.DeleteComment(ctx, comment.ID, userID+999, "dba")
	if err != nil {
		t.Fatalf("DeleteComment as dba: %v", err)
	}
}

func TestCommentService_DeleteComment_NotOwner(t *testing.T) {
	testDB, userID, ticketID := setupCommentTest(t)
	svc := NewCommentService(testDB)
	ctx := context.Background()

	comment, err := svc.CreateComment(ctx, ticketID, userID, "Protected", 0)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	// Different user, non-admin role
	err = svc.DeleteComment(ctx, comment.ID, userID+999, "developer")
	if err != ErrCommentNotOwner {
		t.Errorf("err = %v, want ErrCommentNotOwner", err)
	}
}

func TestCommentService_DeleteComment_NotFound(t *testing.T) {
	testDB, userID, _ := setupCommentTest(t)
	svc := NewCommentService(testDB)

	err := svc.DeleteComment(context.Background(), 99999, userID, "admin")
	if err != ErrCommentNotFound {
		t.Errorf("err = %v, want ErrCommentNotFound", err)
	}
}

func TestCommentService_DeleteComment_WithReplies(t *testing.T) {
	testDB, userID, ticketID := setupCommentTest(t)
	svc := NewCommentService(testDB)
	ctx := context.Background()

	// Create parent and reply
	parent, err := svc.CreateComment(ctx, ticketID, userID, "Parent", 0)
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	_, err = svc.CreateComment(ctx, ticketID, userID, "Reply", parent.ID)
	if err != nil {
		t.Fatalf("create reply: %v", err)
	}

	// Delete parent — should also delete replies
	err = svc.DeleteComment(ctx, parent.ID, userID, "admin")
	if err != nil {
		t.Fatalf("DeleteComment with replies: %v", err)
	}

	comments, _ := svc.ListComments(ctx, ticketID)
	if len(comments) != 0 {
		t.Errorf("expected 0 comments after deleting parent with reply, got %d", len(comments))
	}
}

package service

import (
	"context"
	"database/sql"
	"testing"

	"github.com/whg517/sqlflow/internal/model"
)

func TestGitService_CreateAndList(t *testing.T) {
	db := setupTestDB(t)
	gitSvc := NewGitService(mustWrapDB(db))

	ctx := context.Background()

	// Create a test user for created_by
	userID := createTestUser(t, db, "git_tester")

	input := CreateGitLinkInput{
		EntityType:  "ticket",
		EntityID:    100,
		LinkType:    model.GitLinkTypeCommit,
		CommitHash:  "abc1234",
		CommitMsg:   "feat: add user table",
		AuthorName:  "Test Author",
		AuthorEmail: "test@example.com",
		RepoURL:     "https://github.com/example/repo",
		Branch:      "main",
		CreatedBy:   userID,
	}

	link, err := gitSvc.CreateGitLink(ctx, input)
	if err != nil {
		t.Fatalf("CreateGitLink failed: %v", err)
	}

	if link.ID <= 0 {
		t.Errorf("expected positive ID, got %d", link.ID)
	}
	if link.EntityType != "ticket" {
		t.Errorf("expected entity_type ticket, got %s", link.EntityType)
	}
	if link.CommitHash != "abc1234" {
		t.Errorf("expected commit_hash abc1234, got %s", link.CommitHash)
	}
	if link.LinkType != model.GitLinkTypeCommit {
		t.Errorf("expected link_type commit, got %s", link.LinkType)
	}

	// List links
	links, err := gitSvc.ListGitLinks(ctx, "ticket", 100)
	if err != nil {
		t.Fatalf("ListGitLinks failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].CommitHash != "abc1234" {
		t.Errorf("expected commit_hash abc1234, got %s", links[0].CommitHash)
	}
}

func TestGitService_PRCreation(t *testing.T) {
	db := setupTestDB(t)
	gitSvc := NewGitService(mustWrapDB(db))

	ctx := context.Background()
	userID := createTestUser(t, db, "git_pr_tester")

	input := CreateGitLinkInput{
		EntityType: "ticket",
		EntityID:   200,
		LinkType:   model.GitLinkTypePR,
		CommitHash: "def5678",
		CommitMsg:  "Merge pull request #42",
		AuthorName: "PR Author",
		PRNumber:   42,
		PRTitle:    "Add user table",
		PRURL:      "https://github.com/example/repo/pull/42",
		RepoURL:    "https://github.com/example/repo",
		Branch:     "feature/user-table",
		CreatedBy:  userID,
	}

	link, err := gitSvc.CreateGitLink(ctx, input)
	if err != nil {
		t.Fatalf("CreateGitLink (PR) failed: %v", err)
	}

	if link.PRNumber != 42 {
		t.Errorf("expected PR number 42, got %d", link.PRNumber)
	}
	if link.PRTitle != "Add user table" {
		t.Errorf("expected PR title 'Add user table', got %s", link.PRTitle)
	}
}

func TestGitService_Delete(t *testing.T) {
	db := setupTestDB(t)
	gitSvc := NewGitService(mustWrapDB(db))

	ctx := context.Background()
	userID := createTestUser(t, db, "git_delete_tester")

	input := CreateGitLinkInput{
		EntityType: "audit_log",
		EntityID:   300,
		LinkType:   model.GitLinkTypeCommit,
		CommitHash: "del1234",
		AuthorName: "Delete Tester",
		CreatedBy:  userID,
	}

	link, err := gitSvc.CreateGitLink(ctx, input)
	if err != nil {
		t.Fatalf("CreateGitLink failed: %v", err)
	}

	// Delete the link
	err = gitSvc.DeleteGitLink(ctx, link.ID)
	if err != nil {
		t.Fatalf("DeleteGitLink failed: %v", err)
	}

	// Verify it's gone
	links, err := gitSvc.ListGitLinks(ctx, "audit_log", 300)
	if err != nil {
		t.Fatalf("ListGitLinks after delete failed: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected 0 links after delete, got %d", len(links))
	}

	// Delete non-existent should return error
	err = gitSvc.DeleteGitLink(ctx, 99999)
	if err != ErrGitLinkNotFound {
		t.Errorf("expected ErrGitLinkNotFound, got %v", err)
	}
}

func TestGitService_ValidationErrors(t *testing.T) {
	db := setupTestDB(t)
	gitSvc := NewGitService(mustWrapDB(db))
	ctx := context.Background()
	userID := createTestUser(t, db, "git_valid_tester")

	tests := []struct {
		name  string
		input CreateGitLinkInput
		want  error
	}{
		{
			name: "invalid entity type",
			input: CreateGitLinkInput{
				EntityType: "invalid",
				EntityID:   1,
				LinkType:   model.GitLinkTypeCommit,
				CommitHash: "abc",
				CreatedBy:  userID,
			},
			want: ErrGitLinkEntityTypeInvalid,
		},
		{
			name: "invalid link type",
			input: CreateGitLinkInput{
				EntityType: "ticket",
				EntityID:   1,
				LinkType:   "invalid",
				CommitHash: "abc",
				CreatedBy:  userID,
			},
			want: ErrGitLinkTypeInvalid,
		},
		{
			name: "missing commit hash and PR number",
			input: CreateGitLinkInput{
				EntityType: "ticket",
				EntityID:   1,
				LinkType:   model.GitLinkTypeCommit,
				CreatedBy:  userID,
			},
			want: ErrGitLinkCommitHashRequired,
		},
		{
			name: "invalid list entity type",
			// We test this via ListGitLinks directly
		},
	}

	for _, tt := range tests {
		if tt.name == "invalid list entity type" {
			_, err := gitSvc.ListGitLinks(ctx, "invalid", 1)
			if err != ErrGitLinkEntityTypeInvalid {
				t.Errorf("%s: expected %v, got %v", tt.name, ErrGitLinkEntityTypeInvalid, err)
			}
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			_, err := gitSvc.CreateGitLink(ctx, tt.input)
			if err != tt.want {
				t.Errorf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestGitService_GetByID(t *testing.T) {
	db := setupTestDB(t)
	gitSvc := NewGitService(mustWrapDB(db))

	ctx := context.Background()
	userID := createTestUser(t, db, "git_get_tester")

	input := CreateGitLinkInput{
		EntityType: "ticket",
		EntityID:   400,
		LinkType:   model.GitLinkTypeCommit,
		CommitHash: "get1234",
		AuthorName: "Get Tester",
		CreatedBy:  userID,
	}

	link, err := gitSvc.CreateGitLink(ctx, input)
	if err != nil {
		t.Fatalf("CreateGitLink failed: %v", err)
	}

	// Get by ID
	fetched, err := gitSvc.GetGitLink(ctx, link.ID)
	if err != nil {
		t.Fatalf("GetGitLink failed: %v", err)
	}
	if fetched.CommitHash != "get1234" {
		t.Errorf("expected commit_hash get1234, got %s", fetched.CommitHash)
	}

	// Get non-existent
	_, err = gitSvc.GetGitLink(ctx, 99999)
	if err != ErrGitLinkNotFound {
		t.Errorf("expected ErrGitLinkNotFound, got %v", err)
	}
}

func TestGitService_PopulateGitLinks(t *testing.T) {
	db := setupTestDB(t)
	gitSvc := NewGitService(mustWrapDB(db))

	ctx := context.Background()
	userID := createTestUser(t, db, "git_populate_tester")

	// Create a ticket-like model
	ticket := &model.Ticket{
		ID:            500,
		SubmitterName: "Test User",
	}

	// Populate on empty should return empty slice
	gitSvc.PopulateGitLinks(ctx, ticket)
	if len(ticket.GitLinks) != 0 {
		t.Errorf("expected 0 links, got %d", len(ticket.GitLinks))
	}

	// Create a link
	_, err := gitSvc.CreateGitLink(ctx, CreateGitLinkInput{
		EntityType: "ticket",
		EntityID:   500,
		LinkType:   model.GitLinkTypeCommit,
		CommitHash: "pop1234",
		AuthorName: "Populate Tester",
		CreatedBy:  userID,
	})
	if err != nil {
		t.Fatalf("CreateGitLink failed: %v", err)
	}

	// Populate again
	gitSvc.PopulateGitLinks(ctx, ticket)
	if len(ticket.GitLinks) != 1 {
		t.Errorf("expected 1 link, got %d", len(ticket.GitLinks))
	}

	// Nil ticket should not panic
	gitSvc.PopulateGitLinks(ctx, nil)
}

// Helper: create a test user and return the ID.
func createTestUser(t *testing.T, db *sql.DB, username string) int64 {
	t.Helper()
	_, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?, 'hashed', 'developer')`, username)
	if err != nil {
		t.Fatalf("create test user failed: %v", err)
	}
	var id int64
	err = db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&id)
	if err != nil {
		t.Fatalf("get test user id failed: %v", err)
	}
	return id
}

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

var (
	// ErrGitLinkNotFound indicates the git link does not exist.
	ErrGitLinkNotFound = errors.New("Git关联记录不存在")
	// ErrGitLinkEntityTypeInvalid indicates an invalid entity type.
	ErrGitLinkEntityTypeInvalid = errors.New("无效的实体类型，必须是 ticket 或 audit_log")
	// ErrGitLinkTypeInvalid indicates an invalid link type.
	ErrGitLinkTypeInvalid = errors.New("无效的关联类型，必须是 commit 或 pr")
	// ErrGitLinkCommitHashRequired indicates commit hash is required.
	ErrGitLinkCommitHashRequired = errors.New("commit hash 不能为空")
	// ErrGitLinkPRNumberRequired indicates PR number is required for PR type.
	ErrGitLinkPRNumberRequired = errors.New("PR 编号不能为空")
	// ErrGitLinkDuplicate indicates a duplicate git link.
	ErrGitLinkDuplicate = errors.New("该 Git 关联已存在")
)

// GitService handles git link management — associating commits/PRs with tickets and audit logs.
type GitService struct {
	db *sql.DB
}

// NewGitService creates a new GitService.
func NewGitService(db *sql.DB) *GitService {
	return &GitService{db: db}
}

// CreateGitLinkInput holds the input parameters for creating a git link.
type CreateGitLinkInput struct {
	EntityType  string          // "ticket" or "audit_log"
	EntityID    int64
	LinkType    model.GitLinkType // "commit" or "pr"
	CommitHash  string
	CommitMsg   string
	AuthorName  string
	AuthorEmail string
	PRNumber    int
	PRTitle     string
	PRURL       string
	RepoURL     string
	Branch      string
	CreatedBy   int64 // user ID who creates the link
}

// validateInput validates the input for creating a git link.
func validateInput(input CreateGitLinkInput) error {
	if input.EntityType != "ticket" && input.EntityType != "audit_log" {
		return ErrGitLinkEntityTypeInvalid
	}
	if input.LinkType != model.GitLinkTypeCommit && input.LinkType != model.GitLinkTypePR {
		return ErrGitLinkTypeInvalid
	}
	if input.CommitHash == "" && input.PRNumber == 0 {
		return ErrGitLinkCommitHashRequired
	}
	return nil
}

// CreateGitLink creates a new git link association.
func (s *GitService) CreateGitLink(ctx context.Context, input CreateGitLinkInput) (*model.GitLink, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}

	// Check for duplicates: same entity + link_type + commit_hash
	var existingID int64
	checkQuery := `SELECT id FROM git_links WHERE entity_type = ? AND entity_id = ? AND link_type = ? AND commit_hash = ? LIMIT 1`
	checkArgs := []interface{}{input.EntityType, input.EntityID, string(input.LinkType), input.CommitHash}
	if input.LinkType == model.GitLinkTypePR {
		checkQuery = `SELECT id FROM git_links WHERE entity_type = ? AND entity_id = ? AND link_type = ? AND pr_number = ? LIMIT 1`
		checkArgs = []interface{}{input.EntityType, input.EntityID, string(input.LinkType), input.PRNumber}
	}
	err := s.db.QueryRowContext(ctx, checkQuery, checkArgs...).Scan(&existingID)
	if err == nil {
		// Duplicate found
		return nil, ErrGitLinkDuplicate
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("check duplicate git link: %w", err)
	}

	now := time.Now()
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO git_links (entity_type, entity_id, link_type, commit_hash, commit_msg, author_name, author_email, pr_number, pr_title, pr_url, repo_url, branch, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.EntityType, input.EntityID, string(input.LinkType),
		input.CommitHash, input.CommitMsg, input.AuthorName, input.AuthorEmail,
		input.PRNumber, input.PRTitle, input.PRURL, input.RepoURL, input.Branch,
		input.CreatedBy, now,
	)
	if err != nil {
		// Check for SQLite unique constraint violation (if any) — currently no unique constraint,
		// but we check for duplicates at the application level.
		return nil, fmt.Errorf("创建Git关联失败: %w", err)
	}

	id, _ := result.LastInsertId()

	link := &model.GitLink{
		ID:          id,
		EntityType:  input.EntityType,
		EntityID:    input.EntityID,
		LinkType:    input.LinkType,
		CommitHash:  input.CommitHash,
		CommitMsg:   input.CommitMsg,
		AuthorName:  input.AuthorName,
		AuthorEmail: input.AuthorEmail,
		PRNumber:    input.PRNumber,
		PRTitle:     input.PRTitle,
		PRURL:       input.PRURL,
		RepoURL:     input.RepoURL,
		Branch:      input.Branch,
		CreatedBy:   input.CreatedBy,
		CreatedAt:   now,
	}

	return link, nil
}

// GetGitLink retrieves a git link by ID.
func (s *GitService) GetGitLink(ctx context.Context, id int64) (*model.GitLink, error) {
	link := &model.GitLink{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, entity_type, entity_id, link_type, commit_hash, commit_msg, author_name, author_email, pr_number, pr_title, pr_url, repo_url, branch, created_by, created_at
		 FROM git_links WHERE id = ?`, id,
	).Scan(
		&link.ID, &link.EntityType, &link.EntityID, &link.LinkType,
		&link.CommitHash, &link.CommitMsg, &link.AuthorName, &link.AuthorEmail,
		&link.PRNumber, &link.PRTitle, &link.PRURL, &link.RepoURL, &link.Branch,
		&link.CreatedBy, &link.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrGitLinkNotFound
		}
		return nil, fmt.Errorf("获取Git关联失败: %w", err)
	}
	return link, nil
}

// ListGitLinks retrieves all git links for a given entity (ticket or audit_log).
func (s *GitService) ListGitLinks(ctx context.Context, entityType string, entityID int64) ([]model.GitLink, error) {
	if entityType != "ticket" && entityType != "audit_log" {
		return nil, ErrGitLinkEntityTypeInvalid
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, entity_type, entity_id, link_type, commit_hash, commit_msg, author_name, author_email, pr_number, pr_title, pr_url, repo_url, branch, created_by, created_at
		 FROM git_links WHERE entity_type = ? AND entity_id = ? ORDER BY created_at DESC`,
		entityType, entityID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询Git关联列表失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	links := make([]model.GitLink, 0)
	for rows.Next() {
		var link model.GitLink
		if err := rows.Scan(
			&link.ID, &link.EntityType, &link.EntityID, &link.LinkType,
			&link.CommitHash, &link.CommitMsg, &link.AuthorName, &link.AuthorEmail,
			&link.PRNumber, &link.PRTitle, &link.PRURL, &link.RepoURL, &link.Branch,
			&link.CreatedBy, &link.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan git link row: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历Git关联列表失败: %w", err)
	}

	return links, nil
}

// DeleteGitLink deletes a git link by ID.
func (s *GitService) DeleteGitLink(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM git_links WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除Git关联失败: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrGitLinkNotFound
	}
	return nil
}

// ToGitLinkType converts a string to GitLinkType.
func ToGitLinkType(s string) model.GitLinkType {
	switch s {
	case "pr":
		return model.GitLinkTypePR
	default:
		return model.GitLinkTypeCommit
	}
}

// PopulateGitLinks fills in the GitLinks field of a ticket model.
// This is a convenience method used by TicketService.
func (s *GitService) PopulateGitLinks(ctx context.Context, ticket *model.Ticket) {
	if ticket == nil {
		return
	}
	links, err := s.ListGitLinks(ctx, "ticket", ticket.ID)
	if err != nil {
		return
	}
	ticket.GitLinks = links
}

// BatchPopulateGitLinks fetches git links for multiple tickets in a single query.
func (s *GitService) BatchPopulateGitLinks(ctx context.Context, tickets []model.Ticket) {
	if len(tickets) == 0 {
		return
	}
	ids := make([]string, len(tickets))
	idMap := make(map[int64]int, len(tickets))
	for i, t := range tickets {
		ids[i] = strconv.FormatInt(t.ID, 10)
		idMap[t.ID] = i
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, entity_type, entity_id, link_type, commit_hash, commit_msg, author_name, author_email, pr_number, pr_title, pr_url, repo_url, branch, created_by, created_at
		 FROM git_links WHERE entity_type = 'ticket' AND entity_id IN (`+strings.Join(ids, ",")+`) ORDER BY created_at DESC`,
	)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var link model.GitLink
		var created sql.NullTime
		if err := rows.Scan(&link.ID, &link.EntityType, &link.EntityID, &link.LinkType, &link.CommitHash, &link.CommitMsg, &link.AuthorName, &link.AuthorEmail, &link.PRNumber, &link.PRTitle, &link.PRURL, &link.RepoURL, &link.Branch, &link.CreatedBy, &created); err != nil {
			continue // batch populate: skip bad rows to avoid failing entire batch
		}
		if created.Valid {
			link.CreatedAt = created.Time
		}
		if idx, ok := idMap[link.EntityID]; ok {
			tickets[idx].GitLinks = append(tickets[idx].GitLinks, link)
		}
	}
}

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	esql "entgo.io/ent/dialect/sql"
	"github.com/whg517/sqlflow/internal/db/ent/gitlink"
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
	database *db.DB
	client   *ent.Client
}

// NewGitService creates a new GitService.
func NewGitService(database *db.DB) *GitService {
	return &GitService{database: database, client: database.Client()}
}

// CreateGitLinkInput holds the input parameters for creating a git link.
type CreateGitLinkInput struct {
	EntityType  string            // "ticket" or "audit_log"
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
	dupQuery := s.client.GitLink.Query().
		Where(
			gitlink.EntityType(input.EntityType),
			gitlink.EntityID(input.EntityID),
			gitlink.LinkType(string(input.LinkType)),
		)
	if input.LinkType == model.GitLinkTypeCommit {
		dupQuery = dupQuery.Where(gitlink.CommitHash(input.CommitHash))
	} else {
		dupQuery = dupQuery.Where(gitlink.PrNumber(input.PRNumber))
	}
	exists, err := dupQuery.Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("check duplicate git link: %w", err)
	}
	if exists {
		return nil, ErrGitLinkDuplicate
	}

	now := time.Now()
	saved, err := s.client.GitLink.Create().
		SetEntityType(input.EntityType).
		SetEntityID(input.EntityID).
		SetLinkType(string(input.LinkType)).
		SetCommitHash(input.CommitHash).
		SetCommitMsg(input.CommitMsg).
		SetAuthorName(input.AuthorName).
		SetAuthorEmail(input.AuthorEmail).
		SetPrNumber(input.PRNumber).
		SetPrTitle(input.PRTitle).
		SetPrURL(input.PRURL).
		SetRepoURL(input.RepoURL).
		SetBranch(input.Branch).
		SetCreatedBy(input.CreatedBy).
		SetCreatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建Git关联失败: %w", err)
	}

	link := &model.GitLink{
		ID:          int64(saved.ID),
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
	gl, err := s.client.GitLink.Get(ctx, int(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrGitLinkNotFound
		}
		return nil, fmt.Errorf("获取Git关联失败: %w", err)
	}
	return entGitLinkToModel(gl), nil
}

// ListGitLinks retrieves all git links for a given entity (ticket or audit_log).
func (s *GitService) ListGitLinks(ctx context.Context, entityType string, entityID int64) ([]model.GitLink, error) {
	if entityType != "ticket" && entityType != "audit_log" {
		return nil, ErrGitLinkEntityTypeInvalid
	}

	links, err := s.client.GitLink.Query().
		Where(gitlink.EntityType(entityType), gitlink.EntityID(entityID)).
		Order(gitlink.ByCreatedAt(esql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询Git关联列表失败: %w", err)
	}

	var result []model.GitLink
	for _, gl := range links {
		result = append(result, *entGitLinkToModel(gl))
	}
	return result, nil
}

// DeleteGitLink deletes a git link by ID.
func (s *GitService) DeleteGitLink(ctx context.Context, id int64) error {
	n, err := s.client.GitLink.Delete().Where(gitlink.ID(int(id))).Exec(ctx)
	if err != nil {
		return fmt.Errorf("删除Git关联失败: %w", err)
	}
	if n == 0 {
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
// Uses raw SQL for the IN clause — same pattern as Phase 3 ListTickets.
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

	rows, err := s.database.QueryContext(ctx,
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

// entGitLinkToModel converts an ent GitLink to a model.GitLink.
func entGitLinkToModel(gl *ent.GitLink) *model.GitLink {
	return &model.GitLink{
		ID:          int64(gl.ID),
		EntityType:  gl.EntityType,
		EntityID:    gl.EntityID,
		LinkType:    model.GitLinkType(gl.LinkType),
		CommitHash:  gl.CommitHash,
		CommitMsg:   gl.CommitMsg,
		AuthorName:  gl.AuthorName,
		AuthorEmail: gl.AuthorEmail,
		PRNumber:    gl.PrNumber,
		PRTitle:     gl.PrTitle,
		PRURL:       gl.PrURL,
		RepoURL:     gl.RepoURL,
		Branch:      gl.Branch,
		CreatedBy:   gl.CreatedBy,
		CreatedAt:   gl.CreatedAt,
	}
}

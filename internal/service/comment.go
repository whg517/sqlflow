package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	"github.com/whg517/sqlflow/internal/db/ent/comment"
	"github.com/whg517/sqlflow/internal/db/ent/ticket"
	"github.com/whg517/sqlflow/internal/model"
)

var (
	// ErrCommentNotFound indicates the comment does not exist.
	ErrCommentNotFound = errors.New("评论不存在")
	// ErrCommentContentEmpty indicates the comment content is empty.
	ErrCommentContentEmpty = errors.New("评论内容不能为空")
	// ErrCommentNotOwner indicates the user is not the comment owner.
	ErrCommentNotOwner = errors.New("只能删除自己的评论")
	// ErrOrderNotFound indicates the ticket/order does not exist.
	ErrOrderNotFound = errors.New("工单不存在")
)

// CommentService handles comment CRUD logic.
type CommentService struct {
	database *db.DB
	client   *ent.Client
}

// NewCommentService creates a new CommentService.
func NewCommentService(database *db.DB) *CommentService {
	return &CommentService{database: database, client: database.Client()}
}

// CreateComment adds a new comment to a ticket.
func (s *CommentService) CreateComment(ctx context.Context, orderID, userID int64, content string, parentID int64) (*model.Comment, error) {
	if strings.TrimSpace(content) == "" {
		return nil, ErrCommentContentEmpty
	}

	// Verify the ticket exists
	exists, err := s.client.Ticket.Query().Where(ticket.ID(int(orderID))).Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("验证工单失败: %w", err)
	}
	if !exists {
		return nil, ErrOrderNotFound
	}

	// If replying to a comment, verify the parent comment exists
	if parentID != 0 {
		parentExists, err := s.client.Comment.Query().Where(comment.ID(int(parentID))).Exist(ctx)
		if err != nil {
			return nil, fmt.Errorf("验证父评论失败: %w", err)
		}
		if !parentExists {
			return nil, ErrCommentNotFound
		}
	}

	now := time.Now()
	saved, err := s.client.Comment.Create().
		SetOrderID(orderID).
		SetUserID(userID).
		SetContent(content).
		SetParentID(parentID).
		SetCreatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建评论失败: %w", err)
	}

	c := &model.Comment{
		ID:        int64(saved.ID),
		OrderID:   orderID,
		UserID:    userID,
		Content:   content,
		ParentID:  parentID,
		CreatedAt: now,
	}

	s.populateCommentUsername(ctx, c)
	return c, nil
}

// ListComments retrieves all comments for a ticket ordered by created_at.
func (s *CommentService) ListComments(ctx context.Context, orderID int64) ([]model.Comment, error) {
	comments, err := s.client.Comment.Query().
		Where(comment.OrderID(orderID)).
		Order(comment.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询评论失败: %w", err)
	}

	var result []model.Comment
	for _, c := range comments {
		mc := model.Comment{
			ID:        int64(c.ID),
			OrderID:   c.OrderID,
			UserID:    c.UserID,
			Content:   c.Content,
			ParentID:  c.ParentID,
			CreatedAt: c.CreatedAt,
		}
		s.populateCommentUsername(ctx, &mc)
		result = append(result, mc)
	}

	return result, nil
}

// DeleteComment removes a comment. Only the comment owner can delete it.
func (s *CommentService) DeleteComment(ctx context.Context, commentID, userID int64, userRole string) error {
	c, err := s.client.Comment.Get(ctx, int(commentID))
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrCommentNotFound
		}
		return fmt.Errorf("查询评论失败: %w", err)
	}

	// Only the owner or admin/dba can delete
	if c.UserID != userID && userRole != "admin" && userRole != "dba" {
		return ErrCommentNotOwner
	}

	// Delete the comment and any child replies
	_, err = s.client.Comment.Delete().
		Where(comment.ID(int(commentID))).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("删除评论失败: %w", err)
	}

	// Delete child replies
	_, _ = s.client.Comment.Delete().
		Where(comment.ParentID(commentID)).
		Exec(ctx)

	return nil
}

// populateCommentUsername fetches the username for a comment.
func (s *CommentService) populateCommentUsername(ctx context.Context, c *model.Comment) {
	u, err := s.client.User.Get(ctx, int(c.UserID))
	if err == nil {
		c.Username = u.Username
	}
}

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

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
	db *sql.DB
}

// NewCommentService creates a new CommentService.
func NewCommentService(db *sql.DB) *CommentService {
	return &CommentService{db: db}
}

// CreateComment adds a new comment to a ticket.
func (s *CommentService) CreateComment(ctx context.Context, orderID, userID int64, content string, parentID int64) (*model.Comment, error) {
	if strings.TrimSpace(content) == "" {
		return nil, ErrCommentContentEmpty
	}

	// Verify the ticket exists
	var ticketExists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tickets WHERE id = ?)`, orderID).Scan(&ticketExists)
	if err != nil {
		return nil, fmt.Errorf("验证工单失败: %w", err)
	}
	if !ticketExists {
		return nil, ErrOrderNotFound
	}

	// If replying to a comment, verify the parent comment exists
	if parentID != 0 {
		var parentExists bool
		err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM comments WHERE id = ?)`, parentID).Scan(&parentExists)
		if err != nil {
			return nil, fmt.Errorf("验证父评论失败: %w", err)
		}
		if !parentExists {
			return nil, ErrCommentNotFound
		}
	}

	now := time.Now()
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO comments (order_id, user_id, content, parent_id, created_at) VALUES (?, ?, ?, ?, ?)`,
		orderID, userID, content, parentID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("创建评论失败: %w", err)
	}

	id, _ := result.LastInsertId()

	comment := &model.Comment{
		ID:        id,
		OrderID:   orderID,
		UserID:    userID,
		Content:   content,
		ParentID:  parentID,
		CreatedAt: now,
	}

	s.populateCommentUsername(ctx, comment)
	return comment, nil
}

// ListComments retrieves all comments for a ticket ordered by created_at.
func (s *CommentService) ListComments(ctx context.Context, orderID int64) ([]model.Comment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, order_id, user_id, content, parent_id, created_at
		 FROM comments WHERE order_id = ? ORDER BY created_at ASC`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询评论失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var comments []model.Comment
	for rows.Next() {
		var c model.Comment
		if err := rows.Scan(&c.ID, &c.OrderID, &c.UserID, &c.Content, &c.ParentID, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("解析评论失败: %w", err)
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历评论失败: %w", err)
	}

	// Populate usernames
	for i := range comments {
		s.populateCommentUsername(ctx, &comments[i])
	}

	return comments, nil
}

// DeleteComment removes a comment. Only the comment owner can delete it.
func (s *CommentService) DeleteComment(ctx context.Context, commentID, userID int64, userRole string) error {
	var c model.Comment
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id FROM comments WHERE id = ?`, commentID,
	).Scan(&c.ID, &c.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCommentNotFound
		}
		return fmt.Errorf("查询评论失败: %w", err)
	}

	// Only the owner or admin/dba can delete
	if c.UserID != userID && userRole != "admin" && userRole != "dba" {
		return ErrCommentNotOwner
	}

	// Delete the comment and any child replies
	_, err = s.db.ExecContext(ctx, `DELETE FROM comments WHERE id = ? OR parent_id = ?`, commentID, commentID)
	if err != nil {
		return fmt.Errorf("删除评论失败: %w", err)
	}

	return nil
}

// populateCommentUsername fetches the username for a comment.
func (s *CommentService) populateCommentUsername(ctx context.Context, c *model.Comment) {
	var username string
	if err := s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, c.UserID).Scan(&username); err == nil {
		c.Username = username
	}
}

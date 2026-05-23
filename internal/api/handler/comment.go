package handler

import (
	"log"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// CommentHandler handles comment-related requests.
type CommentHandler struct {
	commentSvc *service.CommentService
}

// NewCommentHandler creates a new CommentHandler.
func NewCommentHandler(commentSvc *service.CommentService) *CommentHandler {
	return &CommentHandler{commentSvc: commentSvc}
}

type createCommentRequest struct {
	Content  string `json:"content"`
	ParentID int64  `json:"parent_id"`
}

// ListComments handles GET /api/tickets/:id/comments.
func (h *CommentHandler) ListComments(c echo.Context) error {
	orderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的工单ID")
	}

	comments, err := h.commentSvc.ListComments(c.Request().Context(), orderID)
	if err != nil {
		log.Printf("ListComments failed: %v", err)
		return resp.InternalError(c, "获取评论失败")
	}

	return resp.OK(c, comments)
}

// CreateComment handles POST /api/tickets/:id/comments.
func (h *CommentHandler) CreateComment(c echo.Context) error {
	orderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的工单ID")
	}

	var req createCommentRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.Content == "" {
		return resp.BadRequest(c, "评论内容不能为空")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)

	comment, err := h.commentSvc.CreateComment(c.Request().Context(), orderID, userID, req.Content, req.ParentID)
	if err != nil {
		switch err {
		case service.ErrCommentContentEmpty:
			return resp.BadRequest(c, err.Error())
		case service.ErrOrderNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrCommentNotFound:
			return resp.BadRequest(c, "回复的评论不存在")
		default:
			log.Printf("CreateComment failed: %v", err)
			return resp.InternalError(c, "创建评论失败")
		}
	}

	return resp.Created(c, comment)
}

// DeleteComment handles DELETE /api/comments/:id.
func (h *CommentHandler) DeleteComment(c echo.Context) error {
	commentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的评论ID")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)
	userRole := c.Get(middleware.ContextKeyRole).(string)

	err = h.commentSvc.DeleteComment(c.Request().Context(), commentID, userID, userRole)
	if err != nil {
		switch err {
		case service.ErrCommentNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrCommentNotOwner:
			return resp.Forbidden(c, err.Error())
		default:
			log.Printf("DeleteComment failed: %v", err)
			return resp.InternalError(c, "删除评论失败")
		}
	}

	return resp.OKWithMessage(c, "评论已删除", nil)
}

package handler

import (
	"log"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// GitHandler handles git link related requests.
type GitHandler struct {
	gitSvc *service.GitService
}

// NewGitHandler creates a new GitHandler.
func NewGitHandler(gitSvc *service.GitService) *GitHandler {
	return &GitHandler{gitSvc: gitSvc}
}

type createGitLinkRequest struct {
	EntityType  string `json:"entity_type"`  // "ticket" or "audit_log"
	EntityID    int64  `json:"entity_id"`
	LinkType    string `json:"link_type"`    // "commit" or "pr"
	CommitHash  string `json:"commit_hash"`
	CommitMsg   string `json:"commit_message"`
	AuthorName  string `json:"author_name"`
	AuthorEmail string `json:"author_email"`
	PRNumber    int    `json:"pr_number"`
	PRTitle     string `json:"pr_title"`
	PRURL       string `json:"pr_url"`
	RepoURL     string `json:"repo_url"`
	Branch      string `json:"branch"`
}

// CreateGitLink handles POST /api/git-links.
//
// @Summary 创建Git关联
// @Description 关联一个Git commit或PR到工单/审计日志
// @Tags Git
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body createGitLinkRequest true "创建Git关联请求"
// @Success 201 {object} resp.SuccessResponse "创建成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 500 {object} resp.ErrorResponse "创建失败"
// @Router /git-links [post]
func (h *GitHandler) CreateGitLink(c echo.Context) error {
	var req createGitLinkRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.EntityType != "ticket" && req.EntityType != "audit_log" {
		return resp.BadRequest(c, "entity_type 必须是 ticket 或 audit_log")
	}
	if req.EntityID <= 0 {
		return resp.BadRequest(c, "entity_id 必须大于 0")
	}
	if req.LinkType != "commit" && req.LinkType != "pr" {
		return resp.BadRequest(c, "link_type 必须是 commit 或 pr")
	}
	if req.CommitHash == "" && req.PRNumber == 0 {
		return resp.BadRequest(c, "commit_hash 和 pr_number 不能同时为空")
	}

	userID := getContextUserID(c)

	input := service.CreateGitLinkInput{
		EntityType:  req.EntityType,
		EntityID:    req.EntityID,
		LinkType:    service.ToGitLinkType(req.LinkType),
		CommitHash:  req.CommitHash,
		CommitMsg:   req.CommitMsg,
		AuthorName:  req.AuthorName,
		AuthorEmail: req.AuthorEmail,
		PRNumber:    req.PRNumber,
		PRTitle:     req.PRTitle,
		PRURL:       req.PRURL,
		RepoURL:     req.RepoURL,
		Branch:      req.Branch,
		CreatedBy:   userID,
	}

	link, err := h.gitSvc.CreateGitLink(c.Request().Context(), input)
	if err != nil {
		switch err {
		case service.ErrGitLinkEntityTypeInvalid,
			service.ErrGitLinkTypeInvalid,
			service.ErrGitLinkCommitHashRequired:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("CreateGitLink failed: %v", err)
			return resp.InternalError(c, "创建Git关联失败")
		}
	}

	return resp.Created(c, link)
}

// ListGitLinks handles GET /api/git-links.
//
// @Summary 获取Git关联列表
// @Description 获取指定实体的Git关联列表
// @Tags Git
// @Produce json
// @Security BearerAuth
// @Param entity_type query string true "实体类型 (ticket/audit_log)"
// @Param entity_id query int true "实体ID"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 500 {object} resp.ErrorResponse "查询失败"
// @Router /git-links [get]
func (h *GitHandler) ListGitLinks(c echo.Context) error {
	entityType := c.QueryParam("entity_type")
	entityIDStr := c.QueryParam("entity_id")

	if entityType == "" {
		return resp.BadRequest(c, "entity_type 参数不能为空")
	}
	if entityIDStr == "" {
		return resp.BadRequest(c, "entity_id 参数不能为空")
	}

	entityID, err := strconv.ParseInt(entityIDStr, 10, 64)
	if err != nil || entityID <= 0 {
		return resp.BadRequest(c, "entity_id 必须是正整数")
	}

	links, err := h.gitSvc.ListGitLinks(c.Request().Context(), entityType, entityID)
	if err != nil {
		switch err {
		case service.ErrGitLinkEntityTypeInvalid:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("ListGitLinks failed: %v", err)
			return resp.InternalError(c, "查询Git关联列表失败")
		}
	}

	return resp.OK(c, links)
}

// DeleteGitLink handles DELETE /api/git-links/:id.
//
// @Summary 删除Git关联
// @Description 删除指定Git关联记录
// @Tags Git
// @Produce json
// @Security BearerAuth
// @Param id path int true "Git关联ID"
// @Success 200 {object} resp.SuccessResponse "删除成功"
// @Failure 404 {object} resp.ErrorResponse "记录不存在"
// @Failure 500 {object} resp.ErrorResponse "删除失败"
// @Router /git-links/{id} [delete]
func (h *GitHandler) DeleteGitLink(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的ID")
	}

	err = h.gitSvc.DeleteGitLink(c.Request().Context(), id)
	if err != nil {
		switch err {
		case service.ErrGitLinkNotFound:
			return resp.NotFound(c, err.Error())
		default:
			log.Printf("DeleteGitLink failed: %v", err)
			return resp.InternalError(c, "删除Git关联失败")
		}
	}

	return resp.OK(c, nil)
}

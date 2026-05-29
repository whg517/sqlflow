package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// TokenHandler handles API token management endpoints.
type TokenHandler struct {
	tokenSvc *service.TokenService
}

// NewTokenHandler creates a new TokenHandler.
func NewTokenHandler(tokenSvc *service.TokenService) *TokenHandler {
	return &TokenHandler{tokenSvc: tokenSvc}
}

type createTokenRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Scopes      []string `json:"scopes"`
	ExpiresDays int      `json:"expires_days"` // defaults to 365
}

// CreateToken generates a new API token.
// CreateToken godoc
// @Summary 创建API令牌
// @Description 认证用户创建新的API访问令牌
// @Tags API令牌
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body object true "令牌配置"
// @Success 201 {object} resp.SuccessResponse "创建成功"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Router /tokens [post]

func (h *TokenHandler) CreateToken(c echo.Context) error {
	userID := getContextUserID(c)

	var req createTokenRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求参数无效")
	}

	if req.Name == "" {
		return resp.BadRequest(c, "Token 名称不能为空")
	}
	if len(req.Name) > 50 {
		return resp.BadRequest(c, "Token 名称不能超过 50 个字符")
	}
	if len(req.Scopes) == 0 {
		return resp.BadRequest(c, "至少选择一个权限范围")
	}

	expiresDays := req.ExpiresDays
	if expiresDays <= 0 {
		expiresDays = 365
	}
	if expiresDays > 3650 {
		expiresDays = 3650
	}

	expiresAt := time.Now().Add(time.Duration(expiresDays) * 24 * time.Hour)

	plainToken, token, err := h.tokenSvc.CreateToken(c.Request().Context(), userID, req.Name, req.Description, req.Scopes, expiresAt)
	if err != nil {
		if errors.Is(err, service.ErrTokenNameExists) {
			return c.JSON(http.StatusConflict, resp.ErrorResponse{
				Code:    409,
				Message: "同名 Token 已存在",
			})
		}
		return resp.InternalError(c, "创建 Token 失败")
	}

	return c.JSON(http.StatusCreated, resp.SuccessResponse{
		Code:    0,
		Message: "created",
		Data: map[string]interface{}{
			"id":           token.ID,
			"name":         token.Name,
			"token":        plainToken, // only returned at creation
			"token_prefix": token.TokenPrefix,
			"scopes":       token.Scopes,
			"expires_at":   token.ExpiresAt,
			"created_at":   token.CreatedAt,
		},
	})
}

// ListMyTokens returns tokens for the authenticated user.
// ListMyTokens godoc
// @Summary 我的令牌列表
// @Description 获取当前用户的API令牌列表
// @Tags API令牌
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /tokens [get]

func (h *TokenHandler) ListMyTokens(c echo.Context) error {
	userID := getContextUserID(c)

	tokens, err := h.tokenSvc.ListTokens(c.Request().Context(), userID)
	if err != nil {
		return resp.InternalError(c, "查询 Token 列表失败")
	}

	return resp.OK(c, tokens)
}

// ListAllTokens returns all tokens (admin only).
// ListAllTokens godoc
// @Summary 所有令牌列表（管理员）
// @Description 管理员获取所有用户的API令牌列表
// @Tags API令牌
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /admin/tokens [get]

func (h *TokenHandler) ListAllTokens(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	tokens, total, err := h.tokenSvc.ListAllTokens(c.Request().Context(), page, pageSize)
	if err != nil {
		return resp.InternalError(c, "查询 Token 列表失败")
	}

	return resp.OKPage(c, tokens, int64(page), int64(pageSize), total)
}

// RevokeMyToken revokes a token owned by the authenticated user.
// RevokeMyToken godoc
// @Summary 撤销我的令牌
// @Description 撤销当前用户自己的API令牌
// @Tags API令牌
// @Produce json
// @Security BearerAuth
// @Param id path int true "令牌ID"
// @Success 200 {object} resp.SuccessResponse "撤销成功"
// @Failure 404 {object} resp.ErrorResponse "令牌不存在"
// @Router /tokens/{id} [delete]

func (h *TokenHandler) RevokeMyToken(c echo.Context) error {
	userID := getContextUserID(c)
	tokenID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的 Token ID")
	}

	if err := h.tokenSvc.RevokeToken(c.Request().Context(), tokenID, userID); err != nil {
		if errors.Is(err, service.ErrTokenNotFound) {
			return resp.NotFound(c, "Token 不存在或无权操作")
		}
		return resp.InternalError(c, "撤销 Token 失败")
	}

	return resp.OKWithMessage(c, "Token 已撤销", nil)
}

// RevokeAnyToken revokes any token (admin only).
// RevokeAnyToken godoc
// @Summary 撤销任意令牌（管理员）
// @Description 管理员撤销任意用户的API令牌
// @Tags API令牌
// @Produce json
// @Security BearerAuth
// @Param id path int true "令牌ID"
// @Success 200 {object} resp.SuccessResponse "撤销成功"
// @Failure 404 {object} resp.ErrorResponse "令牌不存在"
// @Router /admin/tokens/{id} [delete]

func (h *TokenHandler) RevokeAnyToken(c echo.Context) error {
	tokenID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的 Token ID")
	}

	if err := h.tokenSvc.RevokeTokenAdmin(c.Request().Context(), tokenID); err != nil {
		if errors.Is(err, service.ErrTokenNotFound) {
			return resp.NotFound(c, "Token 不存在")
		}
		return resp.InternalError(c, "撤销 Token 失败")
	}

	return resp.OKWithMessage(c, "Token 已撤销", nil)
}

// GetTokenStats returns token usage statistics.
// GetTokenStats godoc
// @Summary 令牌使用统计
// @Description 获取当前用户API令牌的使用统计
// @Tags API令牌
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /tokens/stats [get]

func (h *TokenHandler) GetTokenStats(c echo.Context) error {
	userID := getContextUserID(c)

	totalCount, activeCount, totalUsage, err := h.tokenSvc.GetTokenStats(c.Request().Context(), userID)
	if err != nil {
		return resp.InternalError(c, "查询失败")
	}

	return resp.OK(c, map[string]interface{}{
		"total_tokens":  totalCount,
		"active_tokens": activeCount,
		"total_usage":   totalUsage,
	})
}

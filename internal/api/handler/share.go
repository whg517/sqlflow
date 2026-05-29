package handler

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// ShareHandler handles shared query result requests.
type ShareHandler struct {
	shareSvc *service.ShareService
}

// NewShareHandler creates a new ShareHandler.
func NewShareHandler(shareSvc *service.ShareService) *ShareHandler {
	return &ShareHandler{shareSvc: shareSvc}
}

type createShareRequest struct {
	Columns        []string                 `json:"columns"`
	Rows           []map[string]interface{} `json:"rows"`
	ExpiresInHours int                     `json:"expires_in_hours"` // default 24, max 168
	Password       string                  `json:"password,omitempty"`
	SQLSummary     string                  `json:"sql_summary,omitempty"`
	DatasourceName string                  `json:"datasource_name,omitempty"`
}

type verifyPasswordRequest struct {
	Password string `json:"password"`
}

// CreateShare handles POST /api/query/share.
//
// @Summary 创建查询结果共享链接
// @Description 将查询结果创建为临时共享链接
// @Tags 共享
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body createShareRequest true "共享请求"
// @Success 200 {object} resp.SuccessResponse{data=service.SharedResult} "创建成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Router /query/share [post]
func (h *ShareHandler) CreateShare(c echo.Context) error {
	var req createShareRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if len(req.Columns) == 0 {
		return resp.BadRequest(c, "列数据不能为空")
	}

	userID := getContextUserID(c)
	username := getContextUsername(c)

	expiresInHours := 24
	if req.ExpiresInHours > 0 {
		expiresInHours = req.ExpiresInHours
	}
	expiresAt := time.Now().Add(time.Duration(expiresInHours) * time.Hour)

	shareReq := &service.CreateShareRequest{
		UserID:         userID,
		Username:       username,
		Columns:        req.Columns,
		Rows:           req.Rows,
		ExpiresAt:      expiresAt,
		Password:       req.Password,
		SQLSummary:     req.SQLSummary,
		DatasourceName: req.DatasourceName,
	}

	result, err := h.shareSvc.CreateShare(c.Request().Context(), shareReq)
	if err != nil {
		switch err {
		case service.ErrShareRowLimit:
			return resp.BadRequest(c, "共享数据超过 10000 行上限")
		case service.ErrShareExpiryTooLong:
			return resp.BadRequest(c, "过期时间不能超过 7 天")
		default:
			log.Printf("CreateShare failed: %v", err)
			return resp.InternalError(c, "创建共享链接失败")
		}
	}

	return resp.OK(c, result)
}

// GetShare handles GET /s/:token (public, no auth required).
//
// @Summary 获取共享查询结果
// @Description 通过 token 获取共享的查询结果（无需认证）
// @Tags 共享
// @Produce json
// @Param token path string true "共享 token"
// @Success 200 {object} resp.SuccessResponse{data=model.SharedResultPublic} "获取成功"
// @Failure 404 {object} resp.ErrorResponse "链接不存在"
// @Failure 410 {object} resp.ErrorResponse "链接已过期"
// @Router /s/{token} [get]
func (h *ShareHandler) GetShare(c echo.Context) error {
	token := c.Param("token")
	if token == "" {
		return resp.BadRequest(c, "token 不能为空")
	}

	result, err := h.shareSvc.GetShare(c.Request().Context(), token)
	if err != nil {
		switch err {
		case service.ErrShareNotFound:
			return c.JSON(http.StatusNotFound, map[string]interface{}{
				"code":    404,
				"message": "共享链接不存在",
			})
		case service.ErrShareExpired:
			return c.JSON(http.StatusGone, map[string]interface{}{
				"code":    410,
				"message": "共享链接已过期",
			})
		case service.ErrShareRevoked:
			return c.JSON(http.StatusGone, map[string]interface{}{
				"code":    410,
				"message": "共享链接已被撤销",
			})
		default:
			log.Printf("GetShare failed: %v", err)
			return resp.InternalError(c, "获取共享链接失败")
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "success",
		"data":    result,
	})
}

// VerifySharePassword handles POST /s/:token/verify (public).
//
// @Summary 验证共享链接密码
// @Description 验证共享链接的访问密码
// @Tags 共享
// @Accept json
// @Produce json
// @Param token path string true "共享 token"
// @Param body body verifyPasswordRequest true "密码验证请求"
// @Success 200 {object} resp.SuccessResponse "验证成功"
// @Failure 401 {object} resp.ErrorResponse "密码错误"
// @Router /s/{token}/verify [post]
func (h *ShareHandler) VerifySharePassword(c echo.Context) error {
	token := c.Param("token")
	if token == "" {
		return resp.BadRequest(c, "token 不能为空")
	}

	var req verifyPasswordRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if err := h.shareSvc.VerifyPassword(c.Request().Context(), token, req.Password); err != nil {
		switch err {
		case service.ErrShareNotFound:
			return resp.BadRequest(c, "共享链接不存在")
		case service.ErrSharePassword:
			return c.JSON(http.StatusUnauthorized, map[string]interface{}{
				"code":    401,
				"message": "密码错误",
			})
		default:
			return resp.InternalError(c, "验证密码失败")
		}
	}

	return resp.OKWithMessage(c, "密码验证成功", nil)
}

// ListMyShares handles GET /api/query/share (authenticated).
//
// @Summary 获取我的共享链接列表
// @Description 获取当前用户创建的所有共享链接
// @Tags 共享
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /query/share [get]
func (h *ShareHandler) ListMyShares(c echo.Context) error {
	userID := getContextUserID(c)

	list, err := h.shareSvc.ListMyShares(c.Request().Context(), userID)
	if err != nil {
		return resp.InternalError(c, "获取共享列表失败")
	}

	if list == nil {
		list = make([]model.SharedResult, 0)
	}

	return resp.OK(c, list)
}

// RevokeShare handles DELETE /api/query/share/:id.
//
// @Summary 撤销共享链接
// @Description 撤销指定的共享链接
// @Tags 共享
// @Produce json
// @Security BearerAuth
// @Param id path int true "共享记录ID"
// @Success 200 {object} resp.SuccessResponse "撤销成功"
// @Failure 400 {object} resp.ErrorResponse "无效的ID"
// @Router /query/share/{id} [delete]
func (h *ShareHandler) RevokeShare(c echo.Context) error {
	userID := getContextUserID(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的ID")
	}

	if err := h.shareSvc.RevokeShare(c.Request().Context(), id, userID); err != nil {
		if err == service.ErrShareNotFound {
			return resp.BadRequest(c, "共享链接不存在或已被撤销")
		}
		log.Printf("RevokeShare failed: %v", err)
		return resp.InternalError(c, "撤销共享链接失败")
	}

	return resp.OKWithMessage(c, "撤销成功", nil)
}

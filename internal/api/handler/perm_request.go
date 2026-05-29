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

// PermReqHandler handles permission request endpoints.
type PermReqHandler struct {
	svc *service.PermissionRequestService
}

// NewPermReqHandler creates a new PermReqHandler.
func NewPermReqHandler(svc *service.PermissionRequestService) *PermReqHandler {
	return &PermReqHandler{svc: svc}
}

type createPermReqRequest struct {
	DatasourceID int64  `json:"datasource_id"`
	Database     string `json:"database"`
	TableName    string `json:"table_name"`
	Actions      string `json:"actions"`
	Reason       string `json:"reason"`
	DurationH    int    `json:"duration_hours"` // default 24
}

// CreateRequest creates a new permission request.
// CreateRequest godoc
// @Summary 创建权限申请
// @Description 认证用户申请数据源访问权限
// @Tags 权限申请
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body object true "权限申请信息"
// @Success 201 {object} resp.SuccessResponse "申请成功"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Router /permission-requests [post]

func (h *PermReqHandler) CreateRequest(c echo.Context) error {
	userID := getContextUserID(c)

	var req createPermReqRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求参数无效")
	}
	if req.DatasourceID == 0 {
		return resp.BadRequest(c, "数据源 ID 不能为空")
	}
	if req.Database == "" {
		return resp.BadRequest(c, "数据库名不能为空")
	}
	if req.Actions == "" {
		return resp.BadRequest(c, "操作类型不能为空")
	}

	durationH := req.DurationH
	if durationH <= 0 {
		durationH = 24
	}
	if durationH > 72 {
		durationH = 72
	}

	pr, err := h.svc.CreateRequest(c.Request().Context(), userID, req.DatasourceID, req.Database, req.TableName, req.Actions, req.Reason, time.Duration(durationH)*time.Hour)
	if err != nil {
		if errors.Is(err, service.ErrInvalidAction) || errors.Is(err, service.ErrInvalidDuration) {
			return resp.BadRequest(c, err.Error())
		}
		return resp.InternalError(c, "创建权限申请失败")
	}

	return c.JSON(http.StatusCreated, resp.SuccessResponse{
		Code:    0,
		Message: "created",
		Data:    pr,
	})
}

// ListRequests returns permission requests (admin view, with filters).
// ListRequests godoc
// @Summary 权限申请列表（管理员）
// @Description 管理员获取所有权限申请列表
// @Tags 权限申请
// @Produce json
// @Security BearerAuth
// @Param status query string false "状态筛选"
// @Param page query int false "页码"
// @Param page_size query int false "每页条数"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /permission-requests [get]

func (h *PermReqHandler) ListRequests(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	requests, total, err := h.svc.ListRequests(c.Request().Context(), page, pageSize,
		c.QueryParam("status"), c.QueryParam("applicant_id"))
	if err != nil {
		return resp.InternalError(c, "查询权限申请失败")
	}

	return resp.OKPage(c, requests, int64(page), int64(pageSize), total)
}

// MyRequests returns the current user's permission requests.
// MyRequests godoc
// @Summary 我的权限申请
// @Description 获取当前用户的权限申请列表
// @Tags 权限申请
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /permission-requests/mine [get]

func (h *PermReqHandler) MyRequests(c echo.Context) error {
	userID := getContextUserID(c)

	requests, total, err := h.svc.ListRequests(c.Request().Context(), 1, 100,
		"", strconv.FormatInt(userID, 10))
	if err != nil {
		return resp.InternalError(c, "查询我的申请失败")
	}

	return resp.OK(c, map[string]interface{}{
		"items": requests,
		"total": total,
	})
}

// MyActiveRequests returns current user's active permissions.
// MyActiveRequests godoc
// @Summary 我的活跃权限
// @Description 获取当前用户仍有效的权限列表
// @Tags 权限申请
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /permission-requests/active [get]

func (h *PermReqHandler) MyActiveRequests(c echo.Context) error {
	userID := getContextUserID(c)

	requests, _, err := h.svc.MyActiveRequests(c.Request().Context(), userID)
	if err != nil {
		return resp.InternalError(c, "查询有效权限失败")
	}

	return resp.OK(c, requests)
}

// GetRequest returns a single permission request.
// GetRequest godoc
// @Summary 获取权限申请详情
// @Description 获取指定权限申请的详细信息
// @Tags 权限申请
// @Produce json
// @Security BearerAuth
// @Param id path int true "申请ID"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 404 {object} resp.ErrorResponse "申请不存在"
// @Router /permission-requests/{id} [get]

func (h *PermReqHandler) GetRequest(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	pr, err := h.svc.GetRequestByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrPermReqNotFound) {
			return resp.NotFound(c, "权限申请不存在")
		}
		return resp.InternalError(c, "查询失败")
	}

	return resp.OK(c, pr)
}

type approveRejectReq struct {
	Comment string `json:"comment"`
}

// ApproveRequest approves a permission request (admin/dba).
// ApproveRequest godoc
// @Summary 审批通过权限申请
// @Description 管理员审批通过权限申请
// @Tags 权限申请
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "申请ID"
// @Param body body object true "审批信息"
// @Success 200 {object} resp.SuccessResponse "审批成功"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 403 {object} resp.ErrorResponse "无权限"
// @Router /permission-requests/{id}/approve [post]

func (h *PermReqHandler) ApproveRequest(c echo.Context) error {
	approverID := getContextUserID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	var req approveRejectReq
	_ = c.Bind(&req) // optional comment

	pr, err := h.svc.ApproveRequest(c.Request().Context(), id, approverID, req.Comment)
	if err != nil {
		if errors.Is(err, service.ErrPermReqNotFound) {
			return resp.NotFound(c, "权限申请不存在")
		}
		if errors.Is(err, service.ErrPermReqAlreadyDone) {
			return c.JSON(http.StatusConflict, resp.ErrorResponse{Code: 409, Message: err.Error()})
		}
		return resp.InternalError(c, "审批失败")
	}

	return resp.OKWithMessage(c, "审批通过", pr)
}

// RejectRequest rejects a permission request (admin/dba).
// RejectRequest godoc
// @Summary 驳回权限申请
// @Description 管理员驳回权限申请
// @Tags 权限申请
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "申请ID"
// @Param body body object true "驳回信息"
// @Success 200 {object} resp.SuccessResponse "驳回成功"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 403 {object} resp.ErrorResponse "无权限"
// @Router /permission-requests/{id}/reject [post]

func (h *PermReqHandler) RejectRequest(c echo.Context) error {
	approverID := getContextUserID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	var req approveRejectReq
	_ = c.Bind(&req)

	pr, err := h.svc.RejectRequest(c.Request().Context(), id, approverID, req.Comment)
	if err != nil {
		if errors.Is(err, service.ErrPermReqNotFound) {
			return resp.NotFound(c, "权限申请不存在")
		}
		if errors.Is(err, service.ErrPermReqAlreadyDone) {
			return c.JSON(http.StatusConflict, resp.ErrorResponse{Code: 409, Message: err.Error()})
		}
		return resp.InternalError(c, "拒绝失败")
	}

	return resp.OKWithMessage(c, "已拒绝", pr)
}

type revokeReq struct {
	Reason string `json:"reason"`
}

// RevokeRequest revokes an approved permission (admin/dba).
// RevokeRequest godoc
// @Summary 撤销权限
// @Description 管理员撤销已批准的权限
// @Tags 权限申请
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "申请ID"
// @Param body body object true "撤销原因"
// @Success 200 {object} resp.SuccessResponse "撤销成功"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Router /permission-requests/{id}/revoke [post]

func (h *PermReqHandler) RevokeRequest(c echo.Context) error {
	revokerID := getContextUserID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	var req revokeReq
	_ = c.Bind(&req)

	pr, err := h.svc.RevokeRequest(c.Request().Context(), id, revokerID, req.Reason)
	if err != nil {
		if errors.Is(err, service.ErrPermReqNotFound) {
			return resp.NotFound(c, "权限申请不存在")
		}
		if errors.Is(err, service.ErrPermReqAlreadyDone) {
			return c.JSON(http.StatusConflict, resp.ErrorResponse{Code: 409, Message: err.Error()})
		}
		return resp.InternalError(c, "撤销失败")
	}

	return resp.OKWithMessage(c, "已撤销", pr)
}

// ExpireOverdue triggers manual expiry cleanup (admin).
// ExpireOverdue godoc
// @Summary 过期逾期权限
// @Description 管理员手动触发逾期权限过期
// @Tags 权限申请
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "处理成功"
// @Router /permission-requests/expire [post]

func (h *PermReqHandler) ExpireOverdue(c echo.Context) error {
	count, err := h.svc.ExpireOverdue(c.Request().Context())
	if err != nil {
		return resp.InternalError(c, "清理过期权限失败")
	}

	return resp.OKWithMessage(c, "清理完成", map[string]interface{}{
		"expired_count": count,
	})
}

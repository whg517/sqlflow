package handler

import (
	"log"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// SnapshotHandler handles query snapshot endpoints.
type SnapshotHandler struct {
	snapshotSvc *service.SnapshotService
}

// NewSnapshotHandler creates a new SnapshotHandler.
func NewSnapshotHandler(snapshotSvc *service.SnapshotService) *SnapshotHandler {
	return &SnapshotHandler{snapshotSvc: snapshotSvc}
}

type createSnapshotRequest struct {
	QueryHistoryID int64 `json:"query_history_id"`
}

// CreateSnapshot handles POST /api/query/snapshots.
//
// @Summary 保存查询快照
// @Description 根据查询历史ID重新执行SQL并保存结果为快照，用于后续对比
// @Tags 快照
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body createSnapshotRequest true "查询历史ID"
// @Success 200 {object} resp.SuccessResponse "保存成功"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 403 {object} resp.ErrorResponse "无权限"
// @Failure 404 {object} resp.ErrorResponse "查询历史不存在"
// @Router /query/snapshots [post]
func (h *SnapshotHandler) CreateSnapshot(c echo.Context) error {
	var req createSnapshotRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.QueryHistoryID == 0 {
		return resp.BadRequest(c, "query_history_id 不能为空")
	}

	userID := getContextUserID(c)
	username := getContextUsername(c)
	role := getContextRole(c)

	snapshot, err := h.snapshotSvc.CreateSnapshotFromHistory(c.Request().Context(), userID, username, role, req.QueryHistoryID)
	if err != nil {
		switch err {
		case service.ErrHistoryNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrSnapshotForbidden:
			return resp.Forbidden(c, err.Error())
		default:
			log.Printf("CreateSnapshot failed: %v", err)
			return resp.InternalError(c, "保存快照失败")
		}
	}

	return resp.OK(c, snapshot)
}

// ListSnapshots handles GET /api/query/snapshots.
//
// @Summary 快照列表
// @Description 获取当前用户的查询快照列表
// @Tags 快照
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页条数" default(20)
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /query/snapshots [get]
func (h *SnapshotHandler) ListSnapshots(c echo.Context) error {
	userID := getContextUserID(c)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))

	snapshots, total, err := h.snapshotSvc.ListSnapshots(c.Request().Context(), userID, page, pageSize)
	if err != nil {
		log.Printf("ListSnapshots failed: %v", err)
		return resp.InternalError(c, "获取快照列表失败")
	}

	if snapshots == nil {
		snapshots = []*model.QuerySnapshot{}
	}

	return resp.OKPage(c, snapshots, int64(page), int64(pageSize), int64(total))
}

// GetSnapshot handles GET /api/query/snapshots/:id.
//
// @Summary 获取快照详情
// @Description 获取指定快照的完整数据
// @Tags 快照
// @Produce json
// @Security BearerAuth
// @Param id path int true "快照ID"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 403 {object} resp.ErrorResponse "无权限"
// @Failure 404 {object} resp.ErrorResponse "快照不存在"
// @Router /query/snapshots/{id} [get]
func (h *SnapshotHandler) GetSnapshot(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的快照ID")
	}

	userID := getContextUserID(c)

	snapshot, err := h.snapshotSvc.GetSnapshot(c.Request().Context(), id, userID)
	if err != nil {
		switch err {
		case service.ErrSnapshotNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrSnapshotForbidden:
			return resp.Forbidden(c, err.Error())
		default:
			log.Printf("GetSnapshot failed: %v", err)
			return resp.InternalError(c, "获取快照失败")
		}
	}

	return resp.OK(c, snapshot)
}

// DeleteSnapshot handles DELETE /api/query/snapshots/:id.
//
// @Summary 删除快照
// @Description 删除指定快照
// @Tags 快照
// @Produce json
// @Security BearerAuth
// @Param id path int true "快照ID"
// @Success 200 {object} resp.SuccessResponse "删除成功"
// @Failure 403 {object} resp.ErrorResponse "无权限"
// @Failure 404 {object} resp.ErrorResponse "快照不存在"
// @Router /query/snapshots/{id} [delete]
func (h *SnapshotHandler) DeleteSnapshot(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的快照ID")
	}

	userID := getContextUserID(c)

	if err := h.snapshotSvc.DeleteSnapshot(c.Request().Context(), id, userID); err != nil {
		switch err {
		case service.ErrSnapshotNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrSnapshotForbidden:
			return resp.Forbidden(c, err.Error())
		default:
			log.Printf("DeleteSnapshot failed: %v", err)
			return resp.InternalError(c, "删除快照失败")
		}
	}

	return resp.OKWithMessage(c, "删除成功", nil)
}

type compareRequest struct {
	SnapshotAID int64 `json:"left_snapshot_id"`
	SnapshotBID int64 `json:"right_snapshot_id"`
}

// CompareSnapshots handles POST /api/query/compare.
//
// @Summary 对比快照
// @Description 对比两个查询快照的行列差异
// @Tags 快照
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body compareRequest true "对比请求"
// @Success 200 {object} resp.SuccessResponse "对比结果"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 403 {object} resp.ErrorResponse "无权限"
// @Failure 404 {object} resp.ErrorResponse "快照不存在"
// @Router /query/compare [post]
func (h *SnapshotHandler) CompareSnapshots(c echo.Context) error {
	var req compareRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.SnapshotAID == 0 || req.SnapshotBID == 0 {
		return resp.BadRequest(c, "请选择两个快照进行对比")
	}

	userID := getContextUserID(c)

	result, err := h.snapshotSvc.CompareSnapshots(c.Request().Context(), req.SnapshotAID, req.SnapshotBID, userID)
	if err != nil {
		switch err {
		case service.ErrSnapshotNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrSnapshotForbidden:
			return resp.Forbidden(c, err.Error())
		case service.ErrSchemaMismatch:
			return resp.BadRequest(c, err.Error())
		case service.ErrSameSnapshot:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("CompareSnapshots failed: %v", err)
			return resp.InternalError(c, "对比快照失败")
		}
	}

	return resp.OK(c, result)
}

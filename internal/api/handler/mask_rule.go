package handler

import (
	"log"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// MaskRuleHandler handles mask rule related requests.
type MaskRuleHandler struct {
	maskRuleSvc *service.MaskRuleService
}

// NewMaskRuleHandler creates a new MaskRuleHandler.
func NewMaskRuleHandler(maskRuleSvc *service.MaskRuleService) *MaskRuleHandler {
	return &MaskRuleHandler{maskRuleSvc: maskRuleSvc}
}

// --- Mask Rules API ---

type createMaskRuleRequest struct {
	DatasourceID   int64  `json:"datasource_id"`
	Database       string `json:"database"`
	TableName      string `json:"table_name"`
	Field          string `json:"field"`
	MaskType       string `json:"mask_type"`
	CustomRegex    string `json:"custom_regex"`
	CustomTemplate string `json:"custom_template"`
}

// CreateMaskRule handles POST /api/mask-rules.
//
// @Summary 创建脱敏规则
// @Description 管理员创建数据脱敏规则
// @Tags 脱敏规则
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body createMaskRuleRequest true "创建脱敏规则请求"
// @Success 201 {object} resp.SuccessResponse "创建成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 500 {object} resp.ErrorResponse "创建脱敏规则失败"
// @Router /mask-rules [post]
func (h *MaskRuleHandler) CreateMaskRule(c echo.Context) error {
	var req createMaskRuleRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.DatasourceID == 0 {
		return resp.BadRequest(c, "数据源ID不能为空")
	}
	if req.TableName == "" {
		return resp.BadRequest(c, "表名不能为空")
	}
	if req.Field == "" {
		return resp.BadRequest(c, "字段名不能为空")
	}
	if req.MaskType == "" {
		return resp.BadRequest(c, "脱敏类型不能为空")
	}

	userID := getContextUserID(c)

	rule, err := h.maskRuleSvc.CreateMaskRule(
		c.Request().Context(), userID, req.DatasourceID, req.Database, req.TableName, req.Field,
		req.MaskType, req.CustomRegex, req.CustomTemplate,
	)
	if err != nil {
		switch err {
		case service.ErrMaskRuleFieldRequired:
			return resp.BadRequest(c, err.Error())
		case service.ErrMaskRuleTableRequired:
			return resp.BadRequest(c, err.Error())
		case service.ErrMaskRuleTypeInvalid:
			return resp.BadRequest(c, err.Error())
		case service.ErrMaskRuleCustomRegexRequired:
			return resp.BadRequest(c, err.Error())
		case service.ErrMaskRuleDuplicate:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("CreateMaskRule failed: %v", err)
			return resp.InternalError(c, "创建脱敏规则失败")
		}
	}

	return resp.Created(c, rule)
}

// GetMaskRule handles GET /api/mask-rules/:id.
//
// @Summary 获取脱敏规则详情
// @Description 管理员获取指定脱敏规则详情
// @Tags 脱敏规则
// @Produce json
// @Security BearerAuth
// @Param id path int true "规则ID"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 400 {object} resp.ErrorResponse "无效的规则ID"
// @Failure 404 {object} resp.ErrorResponse "规则不存在"
// @Router /mask-rules/{id} [get]
func (h *MaskRuleHandler) GetMaskRule(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的规则ID")
	}

	rule, err := h.maskRuleSvc.GetMaskRule(c.Request().Context(), id)
	if err != nil {
		switch err {
		case service.ErrMaskRuleNotFound:
			return resp.NotFound(c, err.Error())
		default:
			log.Printf("GetMaskRule failed: %v", err)
			return resp.InternalError(c, "获取脱敏规则失败")
		}
	}

	return resp.OK(c, rule)
}

// ListMaskRules handles GET /api/mask-rules.
//
// @Summary 获取脱敏规则列表
// @Description 管理员获取脱敏规则列表，支持分页和筛选
// @Tags 脱敏规则
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(50)
// @Param datasource_id query string false "数据源ID"
// @Param database query string false "数据库名"
// @Param table_name query string false "表名"
// @Success 200 {object} resp.PageResponse "成功"
// @Failure 500 {object} resp.ErrorResponse "获取脱敏规则列表失败"
// @Router /mask-rules [get]
func (h *MaskRuleHandler) ListMaskRules(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}

	rules, total, err := h.maskRuleSvc.ListMaskRules(
		c.Request().Context(), page, pageSize,
		c.QueryParam("datasource_id"),
		c.QueryParam("database"),
		c.QueryParam("table_name"),
	)
	if err != nil {
		log.Printf("ListMaskRules failed: %v", err)
		return resp.InternalError(c, "获取脱敏规则列表失败")
	}

	return resp.OKPage(c, rules, int64(page), int64(pageSize), total)
}

type updateMaskRuleRequest struct {
	TableName      string `json:"table_name"`
	Field          string `json:"field"`
	MaskType       string `json:"mask_type"`
	CustomRegex    string `json:"custom_regex"`
	CustomTemplate string `json:"custom_template"`
}

// UpdateMaskRule handles PUT /api/mask-rules/:id.
//
// @Summary 更新脱敏规则
// @Description 管理员更新指定脱敏规则
// @Tags 脱敏规则
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "规则ID"
// @Param body body updateMaskRuleRequest true "更新脱敏规则请求"
// @Success 200 {object} resp.SuccessResponse "更新成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 404 {object} resp.ErrorResponse "规则不存在"
// @Router /mask-rules/{id} [put]
func (h *MaskRuleHandler) UpdateMaskRule(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的规则ID")
	}

	var req updateMaskRuleRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	userID := getContextUserID(c)

	rule, err := h.maskRuleSvc.UpdateMaskRule(
		c.Request().Context(), userID, id, req.TableName, req.Field, req.MaskType,
		req.CustomRegex, req.CustomTemplate,
	)
	if err != nil {
		switch err {
		case service.ErrMaskRuleNotFound:
			return resp.NotFound(c, err.Error())
		case service.ErrMaskRuleTypeInvalid:
			return resp.BadRequest(c, err.Error())
		case service.ErrMaskRuleCustomRegexRequired:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("UpdateMaskRule failed: %v", err)
			return resp.InternalError(c, "更新脱敏规则失败")
		}
	}

	return resp.OK(c, rule)
}

// DeleteMaskRule handles DELETE /api/mask-rules/:id.
//
// @Summary 删除脱敏规则
// @Description 管理员删除指定脱敏规则
// @Tags 脱敏规则
// @Produce json
// @Security BearerAuth
// @Param id path int true "规则ID"
// @Success 200 {object} resp.SuccessResponse "删除成功"
// @Failure 400 {object} resp.ErrorResponse "无效的规则ID"
// @Failure 404 {object} resp.ErrorResponse "规则不存在"
// @Router /mask-rules/{id} [delete]
func (h *MaskRuleHandler) DeleteMaskRule(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的规则ID")
	}

	userID := getContextUserID(c)

	if err := h.maskRuleSvc.DeleteMaskRule(c.Request().Context(), userID, id); err != nil {
		switch err {
		case service.ErrMaskRuleNotFound:
			return resp.NotFound(c, err.Error())
		default:
			log.Printf("DeleteMaskRule failed: %v", err)
			return resp.InternalError(c, "删除脱敏规则失败")
		}
	}

	return resp.OK(c, map[string]string{"message": "删除成功"})
}

// --- Sensitive Tables API ---

type createSensitiveTableRequest struct {
	DatasourceID     int64  `json:"datasource_id"`
	Database         string `json:"database"`
	TableName        string `json:"table_name"`
	SensitivityLevel string `json:"sensitivity_level"`
}

// CreateSensitiveTable handles POST /api/sensitive-tables.
//
// @Summary 创建敏感表
// @Description 管理员标记指定表为敏感表
// @Tags 敏感表
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body createSensitiveTableRequest true "创建敏感表请求"
// @Success 201 {object} resp.SuccessResponse "创建成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 500 {object} resp.ErrorResponse "创建敏感表失败"
// @Router /sensitive-tables [post]
func (h *MaskRuleHandler) CreateSensitiveTable(c echo.Context) error {
	var req createSensitiveTableRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.DatasourceID == 0 {
		return resp.BadRequest(c, "数据源ID不能为空")
	}
	if req.TableName == "" {
		return resp.BadRequest(c, "表名不能为空")
	}
	if req.SensitivityLevel == "" {
		req.SensitivityLevel = "medium"
	}

	userID := getContextUserID(c)

	st, err := h.maskRuleSvc.CreateSensitiveTable(
		c.Request().Context(), userID, req.DatasourceID, req.Database, req.TableName, req.SensitivityLevel,
	)
	if err != nil {
		switch err {
		case service.ErrSensitiveTableRequired:
			return resp.BadRequest(c, err.Error())
		case service.ErrSensitiveTableDuplicate:
			return resp.BadRequest(c, err.Error())
		case service.ErrInvalidSensitivityLevel:
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("CreateSensitiveTable failed: %v", err)
			return resp.InternalError(c, "创建敏感表失败")
		}
	}

	return resp.Created(c, st)
}

// ListSensitiveTables handles GET /api/sensitive-tables.
//
// @Summary 获取敏感表列表
// @Description 管理员获取敏感表列表，支持分页和筛选
// @Tags 敏感表
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(50)
// @Param datasource_id query string false "数据源ID"
// @Param database query string false "数据库名"
// @Param table_name query string false "表名"
// @Success 200 {object} resp.PageResponse "成功"
// @Failure 500 {object} resp.ErrorResponse "获取敏感表列表失败"
// @Router /sensitive-tables [get]
func (h *MaskRuleHandler) ListSensitiveTables(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}

	tables, total, err := h.maskRuleSvc.ListSensitiveTables(
		c.Request().Context(), page, pageSize,
		c.QueryParam("datasource_id"),
		c.QueryParam("database"),
		c.QueryParam("table_name"),
	)
	if err != nil {
		log.Printf("ListSensitiveTables failed: %v", err)
		return resp.InternalError(c, "获取敏感表列表失败")
	}

	return resp.OKPage(c, tables, int64(page), int64(pageSize), total)
}

// DeleteSensitiveTable handles DELETE /api/sensitive-tables/:id.
//
// @Summary 删除敏感表
// @Description 管理员删除指定敏感表标记
// @Tags 敏感表
// @Produce json
// @Security BearerAuth
// @Param id path int true "记录ID"
// @Success 200 {object} resp.SuccessResponse "删除成功"
// @Failure 400 {object} resp.ErrorResponse "无效的记录ID"
// @Failure 404 {object} resp.ErrorResponse "记录不存在"
// @Router /sensitive-tables/{id} [delete]
func (h *MaskRuleHandler) DeleteSensitiveTable(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的记录ID")
	}

	userID := getContextUserID(c)

	if err := h.maskRuleSvc.DeleteSensitiveTable(c.Request().Context(), userID, id); err != nil {
		switch err {
		case service.ErrSensitiveTableNotFound:
			return resp.NotFound(c, err.Error())
		default:
			log.Printf("DeleteSensitiveTable failed: %v", err)
			return resp.InternalError(c, "删除敏感表失败")
		}
	}

	return resp.OK(c, map[string]string{"message": "删除成功"})
}

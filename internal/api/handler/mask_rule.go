package handler

import (
	"log"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
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

	userID := c.Get(middleware.ContextKeyUserID).(int64)

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
func (h *MaskRuleHandler) UpdateMaskRule(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的规则ID")
	}

	var req updateMaskRuleRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)

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
func (h *MaskRuleHandler) DeleteMaskRule(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的规则ID")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)

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

	userID := c.Get(middleware.ContextKeyUserID).(int64)

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
func (h *MaskRuleHandler) DeleteSensitiveTable(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的记录ID")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)

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

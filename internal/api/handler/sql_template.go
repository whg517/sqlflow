package handler

import (
	"errors"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// SQLTemplateHandler handles SQL template management endpoints.
type SQLTemplateHandler struct {
	svc *service.TemplateService
}

// NewSQLTemplateHandler creates a new SQLTemplateHandler.
func NewSQLTemplateHandler(svc *service.TemplateService) *SQLTemplateHandler {
	return &SQLTemplateHandler{svc: svc}
}

type createOrUpdateTemplateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SQLContent  string `json:"sql_content"`
	DBType      string `json:"db_type"`
	Category    string `json:"category"`
	IsPublic    bool   `json:"is_public"`
}

type renderTemplateRequest struct {
	Params map[string]string `json:"params"`
}

// CreateTemplate handles POST /api/sql-templates.
func (h *SQLTemplateHandler) CreateTemplate(c echo.Context) error {
	userID := getContextUserID(c)

	var req createOrUpdateTemplateRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求参数无效")
	}

	if req.Name == "" {
		return resp.BadRequest(c, "模板名称不能为空")
	}
	if len(req.Name) > 100 {
		return resp.BadRequest(c, "模板名称不能超过 100 个字符")
	}
	if req.SQLContent == "" {
		return resp.BadRequest(c, "SQL 内容不能为空")
	}
	if req.DBType == "" {
		req.DBType = "mysql"
	}
	if req.Category == "" {
		req.Category = "general"
	}

	tpl, err := h.svc.CreateTemplate(c.Request().Context(), userID, req.Name, req.Description, req.SQLContent, req.DBType, req.Category, req.IsPublic)
	if err != nil {
		if errors.Is(err, service.ErrTemplateNameExists) {
			return c.JSON(409, resp.ErrorResponse{Code: 409, Message: "同名模板已存在"})
		}
		if errors.Is(err, service.ErrSQLContentTooLarge) {
			return resp.BadRequest(c, "SQL 内容不能超过 10KB")
		}
		return resp.InternalError(c, "创建模板失败")
	}

	return resp.Created(c, tpl)
}

// GetTemplate handles GET /api/sql-templates/:id.
func (h *SQLTemplateHandler) GetTemplate(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的模板 ID")
	}

	tpl, err := h.svc.GetTemplate(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrTemplateNotFound) {
			return resp.NotFound(c, "模板不存在")
		}
		return resp.InternalError(c, "查询模板失败")
	}

	return resp.OK(c, tpl)
}

// ListTemplates handles GET /api/sql-templates.
func (h *SQLTemplateHandler) ListTemplates(c echo.Context) error {
	userID := getContextUserID(c)

	category := c.QueryParam("category")
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	templates, total, err := h.svc.ListTemplates(c.Request().Context(), userID, category, page, pageSize)
	if err != nil {
		return resp.InternalError(c, "查询模板列表失败")
	}

	return resp.OKPage(c, templates, int64(page), int64(pageSize), total)
}

// UpdateTemplate handles PUT /api/sql-templates/:id.
func (h *SQLTemplateHandler) UpdateTemplate(c echo.Context) error {
	userID := getContextUserID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的模板 ID")
	}

	var req createOrUpdateTemplateRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求参数无效")
	}

	if req.Name == "" {
		return resp.BadRequest(c, "模板名称不能为空")
	}
	if len(req.Name) > 100 {
		return resp.BadRequest(c, "模板名称不能超过 100 个字符")
	}
	if req.SQLContent == "" {
		return resp.BadRequest(c, "SQL 内容不能为空")
	}
	if req.DBType == "" {
		req.DBType = "mysql"
	}
	if req.Category == "" {
		req.Category = "general"
	}

	err = h.svc.UpdateTemplate(c.Request().Context(), id, userID, req.Name, req.Description, req.SQLContent, req.DBType, req.Category, req.IsPublic)
	if err != nil {
		if errors.Is(err, service.ErrTemplateNotFound) {
			return resp.NotFound(c, "模板不存在或无权修改")
		}
		if errors.Is(err, service.ErrTemplateNameExists) {
			return c.JSON(409, resp.ErrorResponse{Code: 409, Message: "同名模板已存在"})
		}
		if errors.Is(err, service.ErrSQLContentTooLarge) {
			return resp.BadRequest(c, "SQL 内容不能超过 10KB")
		}
		return resp.InternalError(c, "更新模板失败")
	}

	return resp.OK(c, nil)
}

// DeleteTemplate handles DELETE /api/sql-templates/:id.
func (h *SQLTemplateHandler) DeleteTemplate(c echo.Context) error {
	userID := getContextUserID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的模板 ID")
	}

	err = h.svc.DeleteTemplate(c.Request().Context(), id, userID)
	if err != nil {
		if errors.Is(err, service.ErrTemplateNotFound) {
			return resp.NotFound(c, "模板不存在或无权删除")
		}
		return resp.InternalError(c, "删除模板失败")
	}

	return resp.OKWithMessage(c, "模板已删除", nil)
}

// RenderTemplate handles POST /api/sql-templates/:id/render.
func (h *SQLTemplateHandler) RenderTemplate(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的模板 ID")
	}

	var req renderTemplateRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求参数无效")
	}
	if req.Params == nil {
		req.Params = make(map[string]string)
	}

	result, err := h.svc.RenderTemplate(c.Request().Context(), id, req.Params)
	if err != nil {
		if errors.Is(err, service.ErrTemplateNotFound) {
			return resp.NotFound(c, "模板不存在")
		}
		return resp.InternalError(c, "渲染模板失败")
	}

	return resp.OK(c, result)
}

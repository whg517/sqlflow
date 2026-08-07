package query

import (
	"errors"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/platform/httpx"
	"github.com/whg517/sqlflow/internal/resp"
)

// TemplateHandler handles SQL template management endpoints.
type TemplateHandler struct {
	svc *TemplateService
}

// NewTemplateHandler creates a new TemplateHandler.
func NewTemplateHandler(svc *TemplateService) *TemplateHandler {
	return &TemplateHandler{svc: svc}
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
// CreateTemplate godoc
// @Summary 创建SQL模板
// @Description 认证用户创建SQL查询模板
// @Tags SQL模板
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body object true "模板信息"
// @Success 201 {object} resp.SuccessResponse "创建成功"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 409 {object} resp.ErrorResponse "模板名称已存在"
// @Router /sql-templates [post]

func (h *TemplateHandler) CreateTemplate(c echo.Context) error {
	userID := httpx.UserID(c)

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
	// db_type is required, not defaulted.
	//
	// It selects the placeholder dialect the renderer emits, so guessing "mysql"
	// gave a PostgreSQL template `?` markers that PostgreSQL rejects — and on
	// update it silently rewrote the dialect of a template that already had one.
	// The browser always sends it; a server that only holds when the caller is
	// well behaved is not holding.
	if req.DBType == "" {
		return resp.BadRequest(c, "必须指定数据库类型")
	}
	if req.Category == "" {
		req.Category = "general"
	}

	tpl, err := h.svc.CreateTemplate(c.Request().Context(), userID, req.Name, req.Description, req.SQLContent, req.DBType, req.Category, req.IsPublic)
	if err != nil {
		if errors.Is(err, ErrTemplateNameExists) {
			return c.JSON(409, resp.ErrorResponse{Code: 409, Message: "同名模板已存在"})
		}
		if errors.Is(err, ErrSQLContentTooLarge) {
			return resp.BadRequest(c, "SQL 内容不能超过 10KB")
		}
		if errors.Is(err, ErrTemplateDBType) {
			return resp.BadRequest(c, "仅支持 MySQL、PostgreSQL 和 MongoDB 模板")
		}
		return resp.InternalError(c, "创建模板失败")
	}

	return resp.Created(c, tpl)
}

// GetTemplate handles GET /api/sql-templates/:id.
// GetTemplate godoc
// @Summary 获取SQL模板
// @Description 获取指定SQL模板详情
// @Tags SQL模板
// @Produce json
// @Security BearerAuth
// @Param id path int true "模板ID"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 404 {object} resp.ErrorResponse "模板不存在"
// @Router /sql-templates/{id} [get]

func (h *TemplateHandler) GetTemplate(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的模板 ID")
	}

	tpl, err := h.svc.GetTemplateForUser(c.Request().Context(), id, httpx.UserID(c))
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			return resp.NotFound(c, "模板不存在")
		}
		return resp.InternalError(c, "查询模板失败")
	}

	return resp.OK(c, tpl)
}

// ListTemplates handles GET /api/sql-templates.
// ListTemplates godoc
// @Summary SQL模板列表
// @Description 获取SQL模板列表（支持筛选）
// @Tags SQL模板
// @Produce json
// @Security BearerAuth
// @Param category query string false "分类筛选"
// @Param db_type query string false "数据库类型筛选"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /sql-templates [get]

func (h *TemplateHandler) ListTemplates(c echo.Context) error {
	userID := httpx.UserID(c)

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
// UpdateTemplate godoc
// @Summary 更新SQL模板
// @Description 更新指定SQL模板
// @Tags SQL模板
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "模板ID"
// @Param body body object true "模板信息"
// @Success 200 {object} resp.SuccessResponse "更新成功"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 404 {object} resp.ErrorResponse "模板不存在"
// @Router /sql-templates/{id} [put]

func (h *TemplateHandler) UpdateTemplate(c echo.Context) error {
	userID := httpx.UserID(c)
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
	// db_type is required, not defaulted.
	//
	// It selects the placeholder dialect the renderer emits, so guessing "mysql"
	// gave a PostgreSQL template `?` markers that PostgreSQL rejects — and on
	// update it silently rewrote the dialect of a template that already had one.
	// The browser always sends it; a server that only holds when the caller is
	// well behaved is not holding.
	if req.DBType == "" {
		return resp.BadRequest(c, "必须指定数据库类型")
	}
	if req.Category == "" {
		req.Category = "general"
	}

	err = h.svc.UpdateTemplate(c.Request().Context(), id, userID, req.Name, req.Description, req.SQLContent, req.DBType, req.Category, req.IsPublic)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			return resp.NotFound(c, "模板不存在或无权修改")
		}
		if errors.Is(err, ErrTemplateNameExists) {
			return c.JSON(409, resp.ErrorResponse{Code: 409, Message: "同名模板已存在"})
		}
		if errors.Is(err, ErrSQLContentTooLarge) {
			return resp.BadRequest(c, "SQL 内容不能超过 10KB")
		}
		if errors.Is(err, ErrTemplateDBType) {
			return resp.BadRequest(c, "仅支持 MySQL、PostgreSQL 和 MongoDB 模板")
		}
		return resp.InternalError(c, "更新模板失败")
	}

	return resp.OK(c, nil)
}

// DeleteTemplate handles DELETE /api/sql-templates/:id.
// DeleteTemplate godoc
// @Summary 删除SQL模板
// @Description 删除指定SQL模板
// @Tags SQL模板
// @Produce json
// @Security BearerAuth
// @Param id path int true "模板ID"
// @Success 200 {object} resp.SuccessResponse "删除成功"
// @Failure 404 {object} resp.ErrorResponse "模板不存在"
// @Router /sql-templates/{id} [delete]

func (h *TemplateHandler) DeleteTemplate(c echo.Context) error {
	userID := httpx.UserID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的模板 ID")
	}

	err = h.svc.DeleteTemplate(c.Request().Context(), id, userID)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			return resp.NotFound(c, "模板不存在或无权删除")
		}
		return resp.InternalError(c, "删除模板失败")
	}

	return resp.OKWithMessage(c, "模板已删除", nil)
}

// RenderTemplate handles POST /api/sql-templates/:id/render.
// RenderTemplate godoc
// @Summary 渲染SQL模板
// @Description 用参数渲染SQL模板，返回完整SQL
// @Tags SQL模板
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "模板ID"
// @Param body body object true "模板参数"
// @Success 200 {object} resp.SuccessResponse "渲染成功"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 404 {object} resp.ErrorResponse "模板不存在"
// @Router /sql-templates/{id}/render [post]

func (h *TemplateHandler) RenderTemplate(c echo.Context) error {
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

	result, err := h.svc.RenderTemplateForUser(c.Request().Context(), id, httpx.UserID(c), req.Params)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			return resp.NotFound(c, "模板不存在")
		}
		if errors.Is(err, ErrTemplateParamMissing) {
			paramName := strings.TrimSpace(strings.TrimPrefix(err.Error(), ErrTemplateParamMissing.Error()+":"))
			return resp.BadRequest(c, "缺少必填参数："+paramName)
		}
		return resp.InternalError(c, "渲染模板失败")
	}

	return resp.OK(c, result)
}

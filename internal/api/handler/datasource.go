package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// DatasourceHandler handles datasource related requests.
type DatasourceHandler struct {
	dsSvc   *service.DatasourceService
	permSvc *service.PermissionService
}

// NewDatasourceHandler creates a new DatasourceHandler.
func NewDatasourceHandler(dsSvc *service.DatasourceService, permission ...*service.PermissionService) *DatasourceHandler {
	h := &DatasourceHandler{dsSvc: dsSvc}
	if len(permission) > 0 {
		h.permSvc = permission[0]
	}
	return h
}

type createDatasourceRequest struct {
	ID          int64  `json:"id,omitempty"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Database    string `json:"database"`
	SSLMode     string `json:"sslmode"`     // PostgreSQL: disable, prefer, require, verify-ca, verify-full
	SchemaName  string `json:"schema_name"` // PostgreSQL schema (default: public)
	MaxOpen     int    `json:"max_open"`
	MaxIdle     int    `json:"max_idle"`
	MaxLifetime int    `json:"max_lifetime"`
	MaxIdleTime int    `json:"max_idle_time"`
	// Elasticsearch 特有字段
	ESUrls         string `json:"es_urls"`
	ESVersion      string `json:"es_version"`
	ESAuthType     string `json:"es_auth_type"`
	ESApiKey       string `json:"es_api_key"`
	ESIndexPattern string `json:"es_index_pattern"`
	ESVerifyCerts  bool   `json:"es_verify_certs"`
}

type updateDatasourceRequest struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Database    string `json:"database"`
	SSLMode     string `json:"sslmode"`     // PostgreSQL: disable, prefer, require, verify-ca, verify-full
	SchemaName  string `json:"schema_name"` // PostgreSQL schema (default: public)
	MaxOpen     int    `json:"max_open"`
	MaxIdle     int    `json:"max_idle"`
	MaxLifetime int    `json:"max_lifetime"`
	MaxIdleTime int    `json:"max_idle_time"`
	// Elasticsearch 特有字段
	ESUrls         string `json:"es_urls"`
	ESVersion      string `json:"es_version"`
	ESAuthType     string `json:"es_auth_type"`
	ESApiKey       string `json:"es_api_key"`
	ESIndexPattern string `json:"es_index_pattern"`
	ESVerifyCerts  bool   `json:"es_verify_certs"`
}

type datasourceResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Database    string `json:"database"`
	SSLMode     string `json:"sslmode,omitempty"`     // PostgreSQL SSL mode
	SchemaName  string `json:"schema_name,omitempty"` // PostgreSQL schema
	MaxOpen     int    `json:"max_open"`
	MaxIdle     int    `json:"max_idle"`
	MaxLifetime int    `json:"max_lifetime"`
	MaxIdleTime int    `json:"max_idle_time"`
	Status      string `json:"status"`
	System      bool   `json:"system"`
	// Elasticsearch 特有字段
	ESUrls         string `json:"es_urls,omitempty"`
	ESVersion      string `json:"es_version,omitempty"`
	ESAuthType     string `json:"es_auth_type,omitempty"`
	ESIndexPattern string `json:"es_index_pattern,omitempty"`
	ESVerifyCerts  bool   `json:"es_verify_certs"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type datasourceOptionResponse struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

func validateDatasourceConnectionRequest(req createDatasourceRequest, allowStoredCredentials bool) string {
	switch req.Type {
	case "sqlite":
		if req.Database == "" {
			return "SQLite 文件路径不能为空"
		}
	case "elasticsearch":
		if req.ESUrls == "" {
			return "Elasticsearch 节点地址不能为空"
		}
		switch req.ESAuthType {
		case "", "basic":
			if req.Username == "" {
				return "Elasticsearch 用户名不能为空"
			}
			if req.Password == "" && !allowStoredCredentials {
				return "密码不能为空"
			}
		case "api_key":
			if req.ESApiKey == "" && !allowStoredCredentials {
				return "Elasticsearch API Key 不能为空"
			}
		case "none":
			// No credentials required.
		default:
			return "Elasticsearch 认证方式无效"
		}
	default:
		if req.Host == "" {
			return "主机地址不能为空"
		}
		if req.Port == 0 {
			return "端口不能为空"
		}
		if req.Password == "" && !allowStoredCredentials {
			return "密码不能为空"
		}
	}
	return ""
}

func normalizeDatasourceStorageFields(req *createDatasourceRequest) {
	// host is a legacy non-null storage column. SQLite and Elasticsearch use
	// database/es_urls as their actual connection endpoint.
	if req.Type == "sqlite" && req.Host == "" {
		req.Host = "localhost"
	}
	if req.Type == "elasticsearch" && req.Host == "" {
		req.Host = "elasticsearch"
	}
}

func toDatasourceResponse(ds *model.DataSource) datasourceResponse {
	return datasourceResponse{
		ID:             ds.ID,
		Name:           ds.Name,
		Type:           ds.Type,
		Host:           ds.Host,
		Port:           ds.Port,
		Username:       ds.Username,
		Database:       ds.Database,
		SSLMode:        ds.SSLMode,
		SchemaName:     ds.SchemaName,
		MaxOpen:        ds.MaxOpen,
		MaxIdle:        ds.MaxIdle,
		MaxLifetime:    ds.MaxLifetime,
		MaxIdleTime:    ds.MaxIdleTime,
		Status:         ds.Status,
		System:         service.IsInternalDataSource(ds),
		ESUrls:         ds.ESUrls,
		ESVersion:      ds.ESVersion,
		ESAuthType:     ds.ESAuthType,
		ESIndexPattern: ds.ESIndexPattern,
		ESVerifyCerts:  ds.ESVerifyCerts,
		CreatedAt:      ds.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:      ds.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

var validDatasourceTypesMsg = "数据源类型必须是 mysql、postgresql、sqlite、mongodb 或 elasticsearch"

// CreateDatasource handles POST /api/datasources (admin).
//
// @Summary 创建数据源
// @Description 管理员创建新的数据源连接
// @Tags 数据源
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body createDatasourceRequest true "创建数据源请求"
// @Success 201 {object} resp.SuccessResponse{data=datasourceResponse} "创建成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 500 {object} resp.ErrorResponse "创建数据源失败"
// @Router /datasources [post]
func (h *DatasourceHandler) CreateDatasource(c echo.Context) error {
	var req createDatasourceRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.Name == "" {
		return resp.BadRequest(c, "数据源名称不能为空")
	}
	if !service.ValidDatasourceTypes[req.Type] {
		return resp.BadRequest(c, validDatasourceTypesMsg)
	}
	if message := validateDatasourceConnectionRequest(req, false); message != "" {
		return resp.BadRequest(c, message)
	}

	normalizeDatasourceStorageFields(&req)

	ds := &model.DataSource{
		Name:              req.Name,
		Type:              req.Type,
		Host:              req.Host,
		Port:              req.Port,
		Username:          req.Username,
		PasswordEncrypted: req.Password,
		Database:          req.Database,
		SSLMode:           req.SSLMode,
		SchemaName:        req.SchemaName,
		MaxOpen:           req.MaxOpen,
		MaxIdle:           req.MaxIdle,
		MaxLifetime:       req.MaxLifetime,
		MaxIdleTime:       req.MaxIdleTime,
		ESUrls:            req.ESUrls,
		ESVersion:         req.ESVersion,
		ESAuthType:        req.ESAuthType,
		ESApiKey:          req.ESApiKey,
		ESIndexPattern:    req.ESIndexPattern,
		ESVerifyCerts:     req.ESVerifyCerts,
	}

	if err := h.dsSvc.CreateDataSource(c.Request().Context(), ds); err != nil {
		return resp.InternalError(c, "创建数据源失败")
	}

	return resp.Created(c, toDatasourceResponse(ds))
}

// ListDatasources handles GET /api/datasources (admin).
//
// @Summary 获取数据源列表
// @Description 管理员获取所有数据源列表
// @Tags 数据源
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse{data=[]datasourceResponse} "成功"
// @Failure 500 {object} resp.ErrorResponse "获取数据源列表失败"
// @Router /datasources [get]
func (h *DatasourceHandler) ListDatasources(c echo.Context) error {
	list, err := h.dsSvc.ListDataSources(c.Request().Context())
	if err != nil {
		return resp.InternalError(c, "获取数据源列表失败")
	}

	items := make([]datasourceResponse, 0, len(list))
	for i := range list {
		items = append(items, toDatasourceResponse(&list[i]))
	}

	return resp.OK(c, items)
}

// ListAvailableDatasources handles GET /api/datasources/available.
//
// @Summary 获取可发现的数据源
// @Description 已认证用户获取活动数据源的安全摘要；该列表不授予查询权限
// @Tags 数据源
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse{data=[]datasourceOptionResponse} "成功"
// @Failure 401 {object} resp.ErrorResponse "未认证"
// @Failure 500 {object} resp.ErrorResponse "获取数据源列表失败"
// @Router /datasources/available [get]
func (h *DatasourceHandler) ListAvailableDatasources(c echo.Context) error {
	list, err := h.dsSvc.ListAvailableDataSources(c.Request().Context())
	if err != nil {
		return resp.InternalError(c, "获取数据源列表失败")
	}

	items := make([]datasourceOptionResponse, 0, len(list))
	for i := range list {
		if service.IsInternalDataSource(&list[i]) && getContextRole(c) != "admin" {
			continue
		}
		items = append(items, datasourceOptionResponse{
			ID:     list[i].ID,
			Name:   list[i].Name,
			Type:   list[i].Type,
			Status: list[i].Status,
		})
	}
	return resp.OK(c, items)
}

// GetDatasource handles GET /api/datasources/:id (admin).
//
// @Summary 获取数据源详情
// @Description 管理员获取指定数据源详细信息
// @Tags 数据源
// @Produce json
// @Security BearerAuth
// @Param id path int true "数据源ID"
// @Success 200 {object} resp.SuccessResponse{data=datasourceResponse} "成功"
// @Failure 400 {object} resp.ErrorResponse "无效的数据源ID"
// @Failure 404 {object} resp.ErrorResponse "数据源不存在"
// @Router /datasources/{id} [get]
func (h *DatasourceHandler) GetDatasource(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	ds, err := h.dsSvc.GetDataSourceSafe(c.Request().Context(), id)
	if err != nil {
		if err == service.ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		return resp.InternalError(c, "获取数据源失败")
	}

	return resp.OK(c, toDatasourceResponse(ds))
}

// UpdateDatasource handles PUT /api/datasources/:id (admin).
//
// @Summary 更新数据源
// @Description 管理员更新指定数据源配置
// @Tags 数据源
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "数据源ID"
// @Param body body updateDatasourceRequest true "更新数据源请求"
// @Success 200 {object} resp.SuccessResponse{data=datasourceResponse} "更新成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Failure 404 {object} resp.ErrorResponse "数据源不存在"
// @Router /datasources/{id} [put]
func (h *DatasourceHandler) UpdateDatasource(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	var req updateDatasourceRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if !service.ValidDatasourceTypes[req.Type] {
		return resp.BadRequest(c, validDatasourceTypesMsg)
	}
	connectionReq := createDatasourceRequest{
		ID:             id,
		Name:           req.Name,
		Type:           req.Type,
		Host:           req.Host,
		Port:           req.Port,
		Username:       req.Username,
		Password:       req.Password,
		Database:       req.Database,
		SSLMode:        req.SSLMode,
		SchemaName:     req.SchemaName,
		MaxOpen:        req.MaxOpen,
		MaxIdle:        req.MaxIdle,
		MaxLifetime:    req.MaxLifetime,
		MaxIdleTime:    req.MaxIdleTime,
		ESUrls:         req.ESUrls,
		ESVersion:      req.ESVersion,
		ESAuthType:     req.ESAuthType,
		ESApiKey:       req.ESApiKey,
		ESIndexPattern: req.ESIndexPattern,
		ESVerifyCerts:  req.ESVerifyCerts,
	}
	if message := validateDatasourceConnectionRequest(connectionReq, true); message != "" {
		return resp.BadRequest(c, message)
	}
	normalizeDatasourceStorageFields(&connectionReq)
	req.Host = connectionReq.Host

	ds := &model.DataSource{
		Name:              req.Name,
		Type:              req.Type,
		Host:              req.Host,
		Port:              req.Port,
		Username:          req.Username,
		PasswordEncrypted: req.Password, // empty = keep existing
		Database:          req.Database,
		SSLMode:           req.SSLMode,
		SchemaName:        req.SchemaName,
		MaxOpen:           req.MaxOpen,
		MaxIdle:           req.MaxIdle,
		MaxLifetime:       req.MaxLifetime,
		MaxIdleTime:       req.MaxIdleTime,
		ESUrls:            req.ESUrls,
		ESVersion:         req.ESVersion,
		ESAuthType:        req.ESAuthType,
		ESApiKey:          req.ESApiKey,
		ESIndexPattern:    req.ESIndexPattern,
		ESVerifyCerts:     req.ESVerifyCerts,
	}

	if err := h.dsSvc.UpdateDataSource(c.Request().Context(), id, ds); err != nil {
		if err == service.ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		if errors.Is(err, service.ErrSystemDatasource) {
			return resp.BadRequest(c, err.Error())
		}
		return resp.InternalError(c, "更新数据源失败")
	}

	updated, err := h.dsSvc.GetDataSourceSafe(c.Request().Context(), id)
	if err != nil {
		return resp.InternalError(c, "获取数据源失败")
	}

	return resp.OK(c, toDatasourceResponse(updated))
}

// DisableDatasource handles DELETE /api/datasources/:id (admin).
//
// @Summary 禁用数据源
// @Description 管理员禁用指定数据源
// @Tags 数据源
// @Produce json
// @Security BearerAuth
// @Param id path int true "数据源ID"
// @Success 200 {object} resp.SuccessResponse "数据源已禁用"
// @Failure 400 {object} resp.ErrorResponse "无效的数据源ID"
// @Failure 404 {object} resp.ErrorResponse "数据源不存在"
func (h *DatasourceHandler) DisableDatasource(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	if err := h.dsSvc.DisableDataSource(c.Request().Context(), id); err != nil {
		if err == service.ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		if errors.Is(err, service.ErrSystemDatasource) {
			return resp.BadRequest(c, err.Error())
		}
		return resp.InternalError(c, "禁用数据源失败")
	}

	return resp.OKWithMessage(c, "数据源已禁用", nil)
}

type updateDatasourceStatusRequest struct {
	Status string `json:"status"`
}

// UpdateDatasourceStatus handles PUT /api/datasources/:id/status (admin).
//
// @Summary 更新数据源状态
// @Description 启用或禁用数据源；启用前会验证连接
// @Tags 数据源
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "数据源ID"
// @Param body body updateDatasourceStatusRequest true "目标状态"
// @Success 200 {object} resp.SuccessResponse "状态已更新"
// @Failure 400 {object} resp.ErrorResponse "状态无效或连接验证失败"
// @Failure 404 {object} resp.ErrorResponse "数据源不存在"
// @Router /datasources/{id}/status [put]
func (h *DatasourceHandler) UpdateDatasourceStatus(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}
	var req updateDatasourceStatusRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}
	if req.Status != "active" && req.Status != "disabled" {
		return resp.BadRequest(c, "状态必须是 active 或 disabled")
	}

	if req.Status == "active" {
		ds, err := h.dsSvc.GetDataSource(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrDatasourceNotFound) {
				return resp.NotFound(c, "数据源不存在")
			}
			return resp.InternalError(c, "获取数据源失败")
		}
		ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
		defer cancel()
		if err := h.dsSvc.TestConnection(ctx, ds); err != nil {
			log.Printf("Enable datasource connection test failed for %d: %v", id, err)
			return resp.BadRequest(c, "连接验证失败："+err.Error())
		}
		if err := h.dsSvc.EnableDataSource(c.Request().Context(), id); err != nil {
			return h.handleDatasourceLifecycleError(c, err, "启用数据源失败")
		}
		return resp.OKWithMessage(c, "数据源已启用", nil)
	}

	if err := h.dsSvc.DisableDataSource(c.Request().Context(), id); err != nil {
		return h.handleDatasourceLifecycleError(c, err, "禁用数据源失败")
	}
	return resp.OKWithMessage(c, "数据源已禁用", nil)
}

// DeleteDatasource handles DELETE /api/datasources/:id (admin).
//
// @Summary 永久删除数据源
// @Description 仅允许删除已禁用、无业务或权限引用的普通数据源
// @Tags 数据源
// @Produce json
// @Security BearerAuth
// @Param id path int true "数据源ID"
// @Success 200 {object} resp.SuccessResponse "数据源已删除"
// @Failure 400 {object} resp.ErrorResponse "数据源未禁用或属于系统数据源"
// @Failure 404 {object} resp.ErrorResponse "数据源不存在"
// @Failure 409 {object} resp.ErrorResponse "数据源仍被业务或权限数据引用"
// @Router /datasources/{id} [delete]
func (h *DatasourceHandler) DeleteDatasource(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}
	if err := h.dsSvc.DeleteDataSource(c.Request().Context(), id); err != nil {
		var inUse *service.DatasourceInUseError
		if errors.As(err, &inUse) {
			return c.JSON(http.StatusConflict, resp.ErrorResponse{
				Code:    http.StatusConflict,
				Message: inUse.Error(),
			})
		}
		return h.handleDatasourceLifecycleError(c, err, "删除数据源失败")
	}
	return resp.OKWithMessage(c, "数据源已删除", nil)
}

func (h *DatasourceHandler) handleDatasourceLifecycleError(c echo.Context, err error, fallback string) error {
	switch {
	case errors.Is(err, service.ErrDatasourceNotFound):
		return resp.NotFound(c, "数据源不存在")
	case errors.Is(err, service.ErrSystemDatasource),
		errors.Is(err, service.ErrDatasourceMustDisable):
		return resp.BadRequest(c, err.Error())
	default:
		return resp.InternalError(c, fallback)
	}
}

// TestConnection handles POST /api/datasources/:id/test (admin).
//
// @Summary 测试数据源连接
// @Description 管理员测试指定数据源的连接状态
// @Tags 数据源
// @Produce json
// @Security BearerAuth
// @Param id path int true "数据源ID"
// @Success 200 {object} resp.SuccessResponse{data=map[string]interface{}} "测试结果"
// @Failure 400 {object} resp.ErrorResponse "无效的数据源ID"
// @Failure 404 {object} resp.ErrorResponse "数据源不存在"
// @Router /datasources/{id}/test [post]
func (h *DatasourceHandler) TestConnection(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	ds, err := h.dsSvc.GetDataSource(c.Request().Context(), id)
	if err != nil {
		if err == service.ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		return resp.InternalError(c, "获取数据源失败")
	}

	if err := h.dsSvc.TestConnection(c.Request().Context(), ds); err != nil {
		log.Printf("TestConnection failed for datasource %d: %v", id, err)
		return resp.OK(c, map[string]interface{}{
			"success": false,
			"message": "连接失败",
		})
	}

	return resp.OK(c, map[string]interface{}{
		"success": true,
		"message": "连接成功",
	})
}

// TestConnectionConfig handles POST /api/datasources/test-config.
// It tests an unsaved configuration and never writes credentials or datasource
// metadata to the database.
func (h *DatasourceHandler) TestConnectionConfig(c echo.Context) error {
	var req createDatasourceRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}
	if !service.ValidDatasourceTypes[req.Type] {
		return resp.BadRequest(c, validDatasourceTypesMsg)
	}
	if message := validateDatasourceConnectionRequest(req, req.ID > 0); message != "" {
		return resp.BadRequest(c, message)
	}
	normalizeDatasourceStorageFields(&req)

	ds := &model.DataSource{
		ID:                req.ID,
		Type:              req.Type,
		Host:              req.Host,
		Port:              req.Port,
		Username:          req.Username,
		PasswordEncrypted: req.Password,
		Database:          req.Database,
		SSLMode:           req.SSLMode,
		SchemaName:        req.SchemaName,
		MaxOpen:           req.MaxOpen,
		ESUrls:            req.ESUrls,
		ESAuthType:        req.ESAuthType,
		ESApiKey:          req.ESApiKey,
		ESIndexPattern:    req.ESIndexPattern,
		ESVerifyCerts:     req.ESVerifyCerts,
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()
	if err := h.dsSvc.TestConnection(ctx, ds); err != nil {
		log.Printf("TestConnectionConfig failed for type %s: %v", req.Type, err)
		return resp.OK(c, map[string]interface{}{
			"success": false,
			"message": "连接失败：" + err.Error(),
		})
	}
	return resp.OK(c, map[string]interface{}{
		"success": true,
		"message": "连接成功，配置可用",
	})
}

// GetTableColumns handles GET /api/datasources/:id/tables/:name/columns (authenticated).
//
// @Summary 获取表字段列表
// @Description 获取指定数据源中指定表的字段列表
// @Tags 数据源
// @Produce json
// @Security BearerAuth
// @Param id path int true "数据源ID"
// @Param name path string true "表名"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 400 {object} resp.ErrorResponse "无效的数据源ID或表名"
// @Failure 404 {object} resp.ErrorResponse "数据源不存在"
// @Router /datasources/{id}/tables/{name}/columns [get]
func (h *DatasourceHandler) GetTableColumns(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	tableName := c.Param("name")
	if tableName == "" {
		return resp.BadRequest(c, "表名不能为空")
	}
	if allowed, err := h.canAccessDatasource(c, id); err != nil {
		return resp.InternalError(c, "权限校验失败")
	} else if !allowed {
		return resp.Forbidden(c, "SQLFlow 元数据库仅允许管理员访问")
	}
	if allowed, err := h.canViewObject(c, id, tableName); err != nil {
		return resp.InternalError(c, "权限校验失败")
	} else if !allowed {
		return resp.Forbidden(c, "无权查看该表的字段")
	}

	columns, err := h.dsSvc.GetTableColumns(c.Request().Context(), id, tableName)
	if err != nil {
		if err == service.ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		if err == service.ErrDatasourceDisabled {
			return resp.BadRequest(c, "数据源已禁用")
		}
		log.Printf("GetTableColumns failed for datasource %d table %s: %v", id, tableName, err)
		return resp.InternalError(c, "获取字段列表失败")
	}

	return resp.OK(c, columns)
}

// GetTables handles GET /api/datasources/:id/tables (authenticated).
//
// @Summary 获取表列表
// @Description 获取指定数据源中的所有表列表
// @Tags 数据源
// @Produce json
// @Security BearerAuth
// @Param id path int true "数据源ID"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 400 {object} resp.ErrorResponse "无效的数据源ID"
// @Failure 404 {object} resp.ErrorResponse "数据源不存在"
// @Router /datasources/{id}/tables [get]
func (h *DatasourceHandler) GetTables(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}
	if allowed, err := h.canAccessDatasource(c, id); err != nil {
		return resp.InternalError(c, "权限校验失败")
	} else if !allowed {
		return resp.Forbidden(c, "SQLFlow 元数据库仅允许管理员访问")
	}

	tables, err := h.dsSvc.GetTables(c.Request().Context(), id)
	if err != nil {
		if err == service.ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		if err == service.ErrDatasourceDisabled {
			return resp.BadRequest(c, "数据源已禁用")
		}
		log.Printf("GetTables failed for datasource %d: %v", id, err)
		return resp.InternalError(c, "获取表列表失败")
	}

	visible := make([]string, 0, len(tables))
	for _, table := range tables {
		allowed, authErr := h.canViewObject(c, id, table)
		if authErr != nil {
			return resp.InternalError(c, "权限校验失败")
		}
		if allowed {
			visible = append(visible, table)
		}
	}
	return resp.OK(c, visible)
}

func (h *DatasourceHandler) canAccessDatasource(c echo.Context, datasourceID int64) (bool, error) {
	if getContextRole(c) == "admin" {
		return true, nil
	}
	ds, err := h.dsSvc.GetDataSource(c.Request().Context(), datasourceID)
	if err != nil {
		if err == service.ErrDatasourceNotFound {
			// Preserve the endpoint's normal 404 handling.
			return true, nil
		}
		return false, err
	}
	return !service.IsInternalDataSource(ds), nil
}

func parseDatasourceID(c echo.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

func (h *DatasourceHandler) canViewObject(c echo.Context, datasourceID int64, object string) (bool, error) {
	if h.permSvc == nil {
		return true, nil
	}
	return h.permSvc.CanViewObject(
		c.Request().Context(),
		getContextUserID(c),
		getContextRole(c),
		authz.DatasourceDomain(datasourceID),
		object,
	)
}

// GetESIndices returns the list of Elasticsearch indices for a datasource.
func (h *DatasourceHandler) GetESIndices(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	// Parse query parameters
	query := c.QueryParam("q")
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))

	indices, _, err := h.dsSvc.GetESIndices(c.Request().Context(), id, query, page, pageSize)
	if err != nil {
		if err == service.ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		if err == service.ErrDatasourceDisabled {
			return resp.BadRequest(c, "数据源已禁用")
		}
		log.Printf("GetESIndices failed for datasource %d: %v", id, err)
		return resp.InternalError(c, "获取 ES 索引列表失败")
	}

	visible := indices[:0]
	for _, index := range indices {
		allowed, authErr := h.canViewObject(c, id, index.Name)
		if authErr != nil {
			return resp.InternalError(c, "权限校验失败")
		}
		if allowed {
			visible = append(visible, index)
		}
	}
	return resp.OK(c, map[string]interface{}{
		"items": visible,
		"total": len(visible),
	})
}

// GetESIndexFields returns field mapping for a specific Elasticsearch index.
func (h *DatasourceHandler) GetESIndexFields(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	indexName := c.Param("index")
	if indexName == "" {
		return resp.BadRequest(c, "索引名称不能为空")
	}
	if allowed, err := h.canViewObject(c, id, indexName); err != nil {
		return resp.InternalError(c, "权限校验失败")
	} else if !allowed {
		return resp.Forbidden(c, "无权查看该索引的字段")
	}

	fields, err := h.dsSvc.GetESIndexFields(c.Request().Context(), id, indexName)
	if err != nil {
		if err == service.ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		if err == service.ErrDatasourceDisabled {
			return resp.BadRequest(c, "数据源已禁用")
		}
		log.Printf("GetESIndexFields failed for datasource %d index %q: %v", id, indexName, err)
		return resp.InternalError(c, "获取 ES 索引字段失败")
	}

	return resp.OK(c, fields)
}

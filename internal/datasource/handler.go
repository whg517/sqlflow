package datasource

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/httpx"
	"github.com/whg517/sqlflow/internal/resp"
)

// ObjectViewChecker reports whether a caller may see a named object — a table,
// a collection, an index — inside a datasource.
//
// It is declared here, at the point of use, rather than imported from the
// permission service: metadata listing is the only thing this package asks of
// authorization, and naming just that keeps the datasource domain from
// depending on the security domain in either direction.
type ObjectViewChecker interface {
	CanViewObject(ctx context.Context, userID int64, role, domain, object string) (bool, error)
}

// Handler handles datasource related requests.
type Handler struct {
	dsSvc   *Service
	permSvc ObjectViewChecker
}

// NewHandler creates a new Handler.
//
// permission is variadic because metadata browsing predates object-level
// permissions; when it is absent every object is visible.
func NewHandler(dsSvc *Service, permission ...ObjectViewChecker) *Handler {
	h := &Handler{dsSvc: dsSvc}
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
	// ExtraConfig carries whatever the driver needs beyond the fields above.
	//
	// Five Elasticsearch fields used to sit here by name — es_urls, es_version,
	// es_auth_type, es_index_pattern, es_verify_certs — which meant this
	// transport struct, the model, the adapter and five database columns all had
	// to learn a setting before any driver could use it. The driver decodes them
	// now (driver.ConfigDecoder), so adding a type touches neither this file nor
	// the schema.
	ExtraConfig map[string]interface{} `json:"extra_config"`
	// ESApiKey stays named because it is a credential, not configuration: it is
	// encrypted at rest and never travels inside extra_config, which holds what
	// was written verbatim.
	ESApiKey string `json:"es_api_key"`
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
	// ExtraConfig carries whatever the driver needs beyond the fields above.
	//
	// Five Elasticsearch fields used to sit here by name — es_urls, es_version,
	// es_auth_type, es_index_pattern, es_verify_certs — which meant this
	// transport struct, the model, the adapter and five database columns all had
	// to learn a setting before any driver could use it. The driver decodes them
	// now (driver.ConfigDecoder), so adding a type touches neither this file nor
	// the schema.
	ExtraConfig map[string]interface{} `json:"extra_config"`
	// ESApiKey stays named because it is a credential, not configuration: it is
	// encrypted at rest and never travels inside extra_config, which holds what
	// was written verbatim.
	ESApiKey string `json:"es_api_key"`
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
	// ExtraConfig is echoed back as the driver stored it. It never contains
	// credentials.
	ExtraConfig map[string]interface{} `json:"extra_config,omitempty"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

type datasourceOptionResponse struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// validateDatasourceConnectionRequest asks the driver whether the configuration
// is well formed.
//
// It used to be a `switch req.Type` naming sqlite and elasticsearch, with a
// default branch for everything else — so a new data source type had to be
// added to this file before it could be created, and the Elasticsearch rules
// were maintained here and in the driver's own ValidateConfig at the same time.
//
// allowStoredCredentials is accepted for the update path, where an omitted
// password means "keep the one on file". Nothing here inspects credentials —
// whether a secret is accepted is the target's answer, and reporting "密码不能
// 为空" for a MySQL instance that has no password was this layer guessing.
func validateDatasourceConnectionRequest(req createDatasourceRequest, _ bool) string {
	cfg, err := driver.BuildConfigFromDataSource(
		requestAdapter{req}, driver.Secrets{Password: req.Password, APIKey: req.ESApiKey},
	)
	if err != nil {
		return err.Error()
	}
	if err := driver.ValidateConfigFor(req.Type, cfg); err != nil {
		return err.Error()
	}
	return ""
}

// encodeExtraConfig stores the driver's settings as they arrived. An empty map
// is stored as empty rather than "{}", so "not configured" stays distinct from
// "configured with nothing".
func encodeExtraConfig(extra map[string]interface{}) string {
	if len(extra) == 0 {
		return ""
	}
	raw, err := json.Marshal(extra)
	if err != nil {
		return ""
	}
	return string(raw)
}

// decodeExtraConfig turns stored settings back into a JSON object for the API.
// Malformed content is reported as absent: it is the driver's business to
// complain about its own keys, and a list endpoint should not fail because one
// row is bad.
func decodeExtraConfig(raw string) map[string]interface{} {
	if raw == "" {
		return nil
	}
	var extra map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &extra); err != nil {
		return nil
	}
	return extra
}

// requestAdapter presents an unsaved request as a DataSourceInfo so the same
// decoding runs for validation as for connecting. Validating a hand-built
// struct instead was how the handler and the driver came to disagree about
// Elasticsearch.
type requestAdapter struct{ req createDatasourceRequest }

func (a requestAdapter) GetID() int64          { return a.req.ID }
func (a requestAdapter) GetType() string       { return a.req.Type }
func (a requestAdapter) GetHost() string       { return a.req.Host }
func (a requestAdapter) GetPort() int          { return a.req.Port }
func (a requestAdapter) GetUsername() string   { return a.req.Username }
func (a requestAdapter) GetDatabase() string   { return a.req.Database }
func (a requestAdapter) GetSSLMode() string    { return a.req.SSLMode }
func (a requestAdapter) GetSchemaName() string { return a.req.SchemaName }
func (a requestAdapter) GetMaxOpen() int       { return a.req.MaxOpen }
func (a requestAdapter) GetMaxIdle() int       { return a.req.MaxIdle }
func (a requestAdapter) GetMaxLifetime() int   { return a.req.MaxLifetime }
func (a requestAdapter) GetMaxIdleTime() int   { return a.req.MaxIdleTime }

func (a requestAdapter) GetExtraConfig() string {
	if len(a.req.ExtraConfig) == 0 {
		return ""
	}
	raw, err := json.Marshal(a.req.ExtraConfig)
	if err != nil {
		return ""
	}
	return string(raw)
}

// normalizeDatasourceStorageFields fills the non-null host column for types
// that connect by some other means.
//
// The column predates SQLite and Elasticsearch, both of which carry their real
// endpoint elsewhere — a file path and a url list. The placeholder is storage
// bookkeeping, not configuration, which is why it is confined to this function:
// it once leaked into the connection path, where the literal "elasticsearch"
// became a hostname the driver tried to resolve.
func normalizeDatasourceStorageFields(req *createDatasourceRequest) {
	if req.Host == "" {
		req.Host = "-"
	}
}

func toDatasourceResponse(ds *model.DataSource) datasourceResponse {
	return datasourceResponse{
		ID:          ds.ID,
		Name:        ds.Name,
		Type:        ds.Type,
		Host:        ds.Host,
		Port:        ds.Port,
		Username:    ds.Username,
		Database:    ds.Database,
		SSLMode:     ds.SSLMode,
		SchemaName:  ds.SchemaName,
		MaxOpen:     ds.MaxOpen,
		MaxIdle:     ds.MaxIdle,
		MaxLifetime: ds.MaxLifetime,
		MaxIdleTime: ds.MaxIdleTime,
		Status:      ds.Status,
		System:      IsInternal(ds),
		ExtraConfig: decodeExtraConfig(ds.ExtraConfig),
		CreatedAt:   ds.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   ds.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

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
func (h *Handler) CreateDatasource(c echo.Context) error {
	var req createDatasourceRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.Name == "" {
		return resp.BadRequest(c, "数据源名称不能为空")
	}
	if !IsValidDatasourceType(req.Type) {
		return resp.BadRequest(c, ErrInvalidDatasourceType.Error())
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
		ExtraConfig:       encodeExtraConfig(req.ExtraConfig),
		ESApiKey:          req.ESApiKey,
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
func (h *Handler) ListDatasources(c echo.Context) error {
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
func (h *Handler) ListAvailableDatasources(c echo.Context) error {
	list, err := h.dsSvc.ListAvailableDataSources(c.Request().Context())
	if err != nil {
		return resp.InternalError(c, "获取数据源列表失败")
	}

	items := make([]datasourceOptionResponse, 0, len(list))
	for i := range list {
		if IsInternal(&list[i]) && httpx.Role(c) != "admin" {
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

// GetDatasourceCapabilities handles GET /api/datasources/:id/capabilities.
//
// @Summary 获取数据源能力
// @Description 返回该数据源类型的驱动能力与查询形态，供前端决定编辑器、可用操作和结果渲染方式
// @Tags 数据源
// @Produce json
// @Security BearerAuth
// @Param id path int true "数据源ID"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 400 {object} resp.ErrorResponse "无效的数据源ID"
// @Failure 404 {object} resp.ErrorResponse "数据源不存在"
// @Router /datasources/{id}/capabilities [get]
func (h *Handler) GetDatasourceCapabilities(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	desc, err := h.dsSvc.DescribeDataSource(c.Request().Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, ErrDatasourceNotFound):
			return resp.NotFound(c, err.Error())
		case errors.Is(err, ErrDatasourceType):
			return resp.BadRequest(c, err.Error())
		default:
			log.Printf("GetDatasourceCapabilities failed: %v", err)
			return resp.InternalError(c, "获取数据源能力失败")
		}
	}
	return resp.OK(c, desc)
}

// ListDatasourceTypes handles GET /api/datasource-types.
//
// @Summary 列出可用的数据源类型及其连接表单
// @Description 返回每种已注册数据源类型的连接字段声明，供前端渲染表单
// @Tags 数据源
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "获取成功"
// @Router /datasource-types [get]
func (h *Handler) ListDatasourceTypes(c echo.Context) error {
	types, err := driver.DatasourceTypes()
	if err != nil {
		log.Printf("ListDatasourceTypes failed: %v", err)
		return resp.InternalError(c, "获取数据源类型失败")
	}
	return resp.OK(c, types)
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
func (h *Handler) GetDatasource(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	ds, err := h.dsSvc.GetDataSourceSafe(c.Request().Context(), id)
	if err != nil {
		if err == ErrDatasourceNotFound {
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
func (h *Handler) UpdateDatasource(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	var req updateDatasourceRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if !IsValidDatasourceType(req.Type) {
		return resp.BadRequest(c, ErrInvalidDatasourceType.Error())
	}
	connectionReq := createDatasourceRequest{
		ID:          id,
		Name:        req.Name,
		Type:        req.Type,
		Host:        req.Host,
		Port:        req.Port,
		Username:    req.Username,
		Password:    req.Password,
		Database:    req.Database,
		SSLMode:     req.SSLMode,
		SchemaName:  req.SchemaName,
		MaxOpen:     req.MaxOpen,
		MaxIdle:     req.MaxIdle,
		MaxLifetime: req.MaxLifetime,
		MaxIdleTime: req.MaxIdleTime,
		ExtraConfig: req.ExtraConfig,
		ESApiKey:    req.ESApiKey,
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
		ExtraConfig:       encodeExtraConfig(req.ExtraConfig),
		ESApiKey:          req.ESApiKey,
	}

	if err := h.dsSvc.UpdateDataSource(c.Request().Context(), id, ds); err != nil {
		if err == ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		if errors.Is(err, ErrSystemDatasource) {
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
func (h *Handler) DisableDatasource(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	if err := h.dsSvc.DisableDataSource(c.Request().Context(), id); err != nil {
		if err == ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		if errors.Is(err, ErrSystemDatasource) {
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
func (h *Handler) UpdateDatasourceStatus(c echo.Context) error {
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
			if errors.Is(err, ErrDatasourceNotFound) {
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
func (h *Handler) DeleteDatasource(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}
	if err := h.dsSvc.DeleteDataSource(c.Request().Context(), id); err != nil {
		var inUse *DatasourceInUseError
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

func (h *Handler) handleDatasourceLifecycleError(c echo.Context, err error, fallback string) error {
	switch {
	case errors.Is(err, ErrDatasourceNotFound):
		return resp.NotFound(c, "数据源不存在")
	case errors.Is(err, ErrSystemDatasource),
		errors.Is(err, ErrDatasourceMustDisable):
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
func (h *Handler) TestConnection(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	ds, err := h.dsSvc.GetDataSource(c.Request().Context(), id)
	if err != nil {
		if err == ErrDatasourceNotFound {
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
func (h *Handler) TestConnectionConfig(c echo.Context) error {
	var req createDatasourceRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}
	if !IsValidDatasourceType(req.Type) {
		return resp.BadRequest(c, ErrInvalidDatasourceType.Error())
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
		ExtraConfig:       encodeExtraConfig(req.ExtraConfig),
		ESApiKey:          req.ESApiKey,
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
func (h *Handler) GetTableColumns(c echo.Context) error {
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
		if err == ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		if err == ErrDatasourceDisabled {
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
func (h *Handler) GetTables(c echo.Context) error {
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
		if err == ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		if err == ErrDatasourceDisabled {
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

func (h *Handler) canAccessDatasource(c echo.Context, datasourceID int64) (bool, error) {
	if httpx.Role(c) == "admin" {
		return true, nil
	}
	ds, err := h.dsSvc.GetDataSource(c.Request().Context(), datasourceID)
	if err != nil {
		if err == ErrDatasourceNotFound {
			// Preserve the endpoint's normal 404 handling.
			return true, nil
		}
		return false, err
	}
	return !IsInternal(ds), nil
}

func parseDatasourceID(c echo.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

func (h *Handler) canViewObject(c echo.Context, datasourceID int64, object string) (bool, error) {
	if h.permSvc == nil {
		return true, nil
	}
	return h.permSvc.CanViewObject(
		c.Request().Context(),
		httpx.UserID(c),
		httpx.Role(c),
		authz.DatasourceDomain(datasourceID),
		object,
	)
}

// GetESIndices returns the list of Elasticsearch indices for a datasource.
func (h *Handler) GetESIndices(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	// Parse query parameters
	query := c.QueryParam("q")
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))

	indices, err := h.dsSvc.GetESIndices(c.Request().Context(), id, query)
	if err != nil {
		if err == ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		if err == ErrDatasourceDisabled {
			return resp.BadRequest(c, "数据源已禁用")
		}
		log.Printf("GetESIndices failed for datasource %d: %v", id, err)
		return resp.InternalError(c, "获取 ES 索引列表失败")
	}

	// Filter before paging. The service used to cut the page and this loop
	// filtered what came back, so a caller permitted to see three indices out of
	// two hundred got twenty rows with three filled and then nineteen empty
	// pages — and the reported total was the length of that one page, so the UI
	// drew a single page and the rest were unreachable.
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
		"items": paginate(visible, page, pageSize),
		"total": len(visible),
	})
}

// esIndexPageSize bounds a page of indices; the default matches what the UI asks for.
const (
	esIndexDefaultPageSize = 20
	esIndexMaxPageSize     = 100
)

// paginate returns one page of items, clamping the request to the slice.
func paginate(items []ESIndexInfo, page, pageSize int) []ESIndexInfo {
	if page < 1 {
		page = 1
	}
	switch {
	case pageSize < 1:
		pageSize = esIndexDefaultPageSize
	case pageSize > esIndexMaxPageSize:
		pageSize = esIndexMaxPageSize
	}

	start := min((page-1)*pageSize, len(items))
	end := min(start+pageSize, len(items))
	return items[start:end]
}

// GetESIndexFields returns field mapping for a specific Elasticsearch index.
func (h *Handler) GetESIndexFields(c echo.Context) error {
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
		if err == ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		if err == ErrDatasourceDisabled {
			return resp.BadRequest(c, "数据源已禁用")
		}
		log.Printf("GetESIndexFields failed for datasource %d index %q: %v", id, indexName, err)
		return resp.InternalError(c, "获取 ES 索引字段失败")
	}

	return resp.OK(c, fields)
}

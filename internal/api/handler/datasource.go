package handler

import (
	"log"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// DatasourceHandler handles datasource related requests.
type DatasourceHandler struct {
	dsSvc *service.DatasourceService
}

// NewDatasourceHandler creates a new DatasourceHandler.
func NewDatasourceHandler(dsSvc *service.DatasourceService) *DatasourceHandler {
	return &DatasourceHandler{dsSvc: dsSvc}
}

type createDatasourceRequest struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Database    string `json:"database"`
	MaxOpen     int    `json:"max_open"`
	MaxIdle     int    `json:"max_idle"`
	MaxLifetime int    `json:"max_lifetime"`
	MaxIdleTime int    `json:"max_idle_time"`
}

type updateDatasourceRequest struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Database    string `json:"database"`
	MaxOpen     int    `json:"max_open"`
	MaxIdle     int    `json:"max_idle"`
	MaxLifetime int    `json:"max_lifetime"`
	MaxIdleTime int    `json:"max_idle_time"`
}

type datasourceResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Database    string `json:"database"`
	MaxOpen     int    `json:"max_open"`
	MaxIdle     int    `json:"max_idle"`
	MaxLifetime int    `json:"max_lifetime"`
	MaxIdleTime int    `json:"max_idle_time"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
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
		MaxOpen:     ds.MaxOpen,
		MaxIdle:     ds.MaxIdle,
		MaxLifetime: ds.MaxLifetime,
		MaxIdleTime: ds.MaxIdleTime,
		Status:      ds.Status,
		CreatedAt:   ds.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   ds.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// CreateDatasource handles POST /api/datasources (admin).
func (h *DatasourceHandler) CreateDatasource(c echo.Context) error {
	var req createDatasourceRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.Name == "" {
		return resp.BadRequest(c, "数据源名称不能为空")
	}
	if !service.ValidDatasourceTypes[req.Type] {
		return resp.BadRequest(c, "数据源类型必须是 mysql 或 mongodb")
	}
	if req.Host == "" {
		return resp.BadRequest(c, "主机地址不能为空")
	}
	if req.Port == 0 {
		return resp.BadRequest(c, "端口不能为空")
	}
	if req.Password == "" {
		return resp.BadRequest(c, "密码不能为空")
	}

	ds := &model.DataSource{
		Name:              req.Name,
		Type:              req.Type,
		Host:              req.Host,
		Port:              req.Port,
		Username:          req.Username,
		PasswordEncrypted: req.Password,
		Database:          req.Database,
		MaxOpen:           req.MaxOpen,
		MaxIdle:           req.MaxIdle,
		MaxLifetime:       req.MaxLifetime,
		MaxIdleTime:       req.MaxIdleTime,
	}

	if err := h.dsSvc.CreateDataSource(c.Request().Context(), ds); err != nil {
		return resp.InternalError(c, "创建数据源失败")
	}

	return resp.Created(c, toDatasourceResponse(ds))
}

// ListDatasources handles GET /api/datasources (admin).
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

// GetDatasource handles GET /api/datasources/:id (admin).
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
		return resp.BadRequest(c, "数据源类型必须是 mysql 或 mongodb")
	}

	ds := &model.DataSource{
		Name:              req.Name,
		Type:              req.Type,
		Host:              req.Host,
		Port:              req.Port,
		Username:          req.Username,
		PasswordEncrypted: req.Password, // empty = keep existing
		Database:          req.Database,
		MaxOpen:           req.MaxOpen,
		MaxIdle:           req.MaxIdle,
		MaxLifetime:       req.MaxLifetime,
		MaxIdleTime:       req.MaxIdleTime,
	}

	if err := h.dsSvc.UpdateDataSource(c.Request().Context(), id, ds); err != nil {
		if err == service.ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
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
func (h *DatasourceHandler) DisableDatasource(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
	}

	if err := h.dsSvc.DisableDataSource(c.Request().Context(), id); err != nil {
		if err == service.ErrDatasourceNotFound {
			return resp.NotFound(c, "数据源不存在")
		}
		return resp.InternalError(c, "禁用数据源失败")
	}

	return resp.OKWithMessage(c, "数据源已禁用", nil)
}

// TestConnection handles POST /api/datasources/:id/test (admin).
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

// GetTables handles GET /api/datasources/:id/tables (authenticated).
func (h *DatasourceHandler) GetTables(c echo.Context) error {
	id, err := parseDatasourceID(c)
	if err != nil {
		return resp.BadRequest(c, "无效的数据源ID")
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

	return resp.OK(c, tables)
}

func parseDatasourceID(c echo.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

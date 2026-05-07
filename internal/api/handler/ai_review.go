package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/pkg/sqlparser"
	"github.com/whg517/sqlflow/internal/service"
)

// AIReviewHandler handles AI review SSE streaming requests.
type AIReviewHandler struct {
	aiReviewSvc *service.AIReviewService
	dsSvc       *service.DatasourceService
}

// NewAIReviewHandler creates a new AIReviewHandler.
func NewAIReviewHandler(aiReviewSvc *service.AIReviewService, dsSvc *service.DatasourceService) *AIReviewHandler {
	return &AIReviewHandler{
		aiReviewSvc: aiReviewSvc,
		dsSvc:       dsSvc,
	}
}

type reviewRequest struct {
	DatasourceID int64  `json:"datasource_id"`
	Database     string `json:"database"`
	SQL          string `json:"sql"`
}

// ReviewStream handles POST /api/query/review — SSE streaming AI review.
func (h *AIReviewHandler) ReviewStream(c echo.Context) error {
	var req reviewRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(400, "请求格式错误")
	}
	if req.DatasourceID == 0 {
		return echo.NewHTTPError(400, "数据源ID不能为空")
	}
	if req.SQL == "" {
		return echo.NewHTTPError(400, "SQL不能为空")
	}

	userID := c.Get(middleware.ContextKeyUserID).(int64)
	username := c.Get(middleware.ContextKeyUsername).(string)
	role := c.Get(middleware.ContextKeyRole).(string)

	// Get datasource to determine db type
	ds, err := h.dsSvc.GetDataSource(c.Request().Context(), req.DatasourceID)
	if err != nil {
		return echo.NewHTTPError(500, "获取数据源失败")
	}
	dbType := ds.Type

	// Parse SQL
	parseResult, err := sqlparser.ParseSQL(req.SQL, dbType)
	if err != nil {
		return echo.NewHTTPError(400, fmt.Sprintf("SQL解析失败: %v", err))
	}

	aiReq := &service.AIReviewRequest{
		SQL:          req.SQL,
		DBType:       dbType,
		DatasourceID: req.DatasourceID,
		Database:     req.Database,
		UserID:       userID,
		Username:     username,
		Role:         role,
		Operation:    parseResult.Operation,
		Tables:       parseResult.Tables,
		ParseResult:  parseResult,
	}

	// Start SSE stream
	eventCh, err := h.aiReviewSvc.ReviewStream(c.Request().Context(), aiReq)
	if err != nil {
		return echo.NewHTTPError(500, fmt.Sprintf("AI评审启动失败: %v", err))
	}

	// Set SSE headers
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(200)

	flusher, canFlush := c.Response().Writer.(http.Flusher)

	for event := range eventCh {
		data, err := json.Marshal(event.Data)
		if err != nil {
			log.Printf("marshal SSE event: %v", err)
			continue
		}

		fmt.Fprintf(c.Response().Writer, "event: %s\ndata: %s\n\n", event.Type, data)
		if canFlush {
			flusher.Flush()
		}
	}

	// Send final [DONE] marker
	fmt.Fprint(c.Response().Writer, "event: done\ndata: {}\n\n")
	if canFlush {
		flusher.Flush()
	}

	c.Response().Writer.(io.Closer).Close()
	return nil
}

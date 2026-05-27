package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

const (
	// AsyncExportThreshold is the row count above which exports become async.
	AsyncExportThreshold = 5000
	// ExportFileTTL is how long async export files are kept before cleanup.
	ExportFileTTL = 24 * time.Hour
	// ExportDir is the subdirectory under the data path for export files.
	ExportDir = "exports"
)

var (
	// ErrExportNotFound indicates the export task does not exist.
	ErrExportNotFound = errors.New("导出任务不存在")
	// ErrExportNotReady indicates the export task is not yet completed.
	ErrExportNotReady = errors.New("导出任务尚未完成")
	// ErrExportFileGone indicates the export file has been cleaned up.
	ErrExportFileGone = errors.New("导出文件已过期或已清理")
)

// ExportAsyncService handles asynchronous export task lifecycle.
type ExportAsyncService struct {
	db          *sql.DB
	exportSvc   *ExportService
	auditSvc    *AuditService
	exportDir   string
	tasks       sync.Map // taskID -> *model.ExportTask (in-memory cache for active tasks)
	stopCleanup chan struct{}
}

// NewExportAsyncService creates a new ExportAsyncService.
func NewExportAsyncService(db *sql.DB, exportSvc *ExportService, auditSvc *AuditService, dataDir string) *ExportAsyncService {
	dir := filepath.Join(dataDir, ExportDir)
	_ = os.MkdirAll(dir, 0755)

	svc := &ExportAsyncService{
		db:          db,
		exportSvc:   exportSvc,
		auditSvc:    auditSvc,
		exportDir:   dir,
		stopCleanup: make(chan struct{}),
	}

	// Load any incomplete tasks from DB on startup
	svc.recoverPendingTasks()

	// Start background cleanup goroutine
	go svc.cleanupLoop()

	return svc
}

// Close stops the background cleanup goroutine.
func (s *ExportAsyncService) Close() {
	close(s.stopCleanup)
}

// CreateAsyncExport creates an async export task and starts it in a goroutine.
// It returns the task ID immediately.
func (s *ExportAsyncService) CreateAsyncExport(ctx context.Context, userID int64, username, role, exportType string, filtersJSON string) (*model.ExportTask, error) {
	if !s.exportSvc.hasExportPermission(role, ExportType(exportType)) {
		return nil, ErrExportNoPermission
	}

	filename := generateExportFilename(exportType)
	filePath := filepath.Join(s.exportDir, filename)

	now := time.Now()
	task := &model.ExportTask{
		UserID:      userID,
		Username:    username,
		ExportType:  exportType,
		Status:      model.ExportTaskStatusPending,
		Filename:    filename,
		FilePath:    filePath,
		FiltersJSON: filtersJSON,
		CreatedAt:   now,
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO export_tasks (user_id, username, export_type, status, filename, file_path, filters_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		task.UserID, task.Username, task.ExportType, task.Status,
		task.Filename, task.FilePath, task.FiltersJSON, task.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("创建导出任务失败: %w", err)
	}

	id, _ := result.LastInsertId()
	task.ID = id
	s.tasks.Store(id, task)

	// Launch async export in goroutine
	go s.executeExport(task, username, role)

	return task, nil
}

// GetTask retrieves an export task by ID.
func (s *ExportAsyncService) GetTask(ctx context.Context, taskID int64, userID int64) (*model.ExportTask, error) {
	var task model.ExportTask
	var completedAt sql.NullTime

	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, username, export_type, status, filename, file_path,
		        total_rows, file_bytes, filters_json, error_msg, created_at, completed_at
		 FROM export_tasks WHERE id = ? AND user_id = ?`,
		taskID, userID,
	).Scan(
		&task.ID, &task.UserID, &task.Username, &task.ExportType, &task.Status,
		&task.Filename, &task.FilePath,
		&task.TotalRows, &task.FileBytes, &task.FiltersJSON, &task.ErrorMsg,
		&task.CreatedAt, &completedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrExportNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询导出任务失败: %w", err)
	}

	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}

	return &task, nil
}

// ListTasks lists export tasks for a user.
func (s *ExportAsyncService) ListTasks(ctx context.Context, userID int64) ([]model.ExportTaskSlim, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, export_type, status, filename, total_rows, file_bytes, error_msg, created_at, completed_at
		 FROM export_tasks WHERE user_id = ? ORDER BY created_at DESC LIMIT 50`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询导出任务列表失败: %w", err)
	}
	defer rows.Close()

	var tasks []model.ExportTaskSlim
	for rows.Next() {
		var t model.ExportTaskSlim
		var completedAt sql.NullTime
		if err := rows.Scan(
			&t.ID, &t.ExportType, &t.Status, &t.Filename,
			&t.TotalRows, &t.FileBytes, &t.ErrorMsg,
			&t.CreatedAt, &completedAt,
		); err != nil {
			continue
		}
		if completedAt.Valid {
			t.CompletedAt = &completedAt.Time
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// DownloadFile returns the file content for a completed export task.
func (s *ExportAsyncService) DownloadFile(ctx context.Context, taskID int64, userID int64) (io.ReadCloser, string, error) {
	task, err := s.GetTask(ctx, taskID, userID)
	if err != nil {
		return nil, "", err
	}

	if task.Status != model.ExportTaskStatusCompleted {
		return nil, "", ErrExportNotReady
	}

	f, err := os.Open(task.FilePath)
	if os.IsNotExist(err) {
		return nil, "", ErrExportFileGone
	}
	if err != nil {
		return nil, "", fmt.Errorf("打开导出文件失败: %w", err)
	}

	return f, task.Filename, nil
}

// executeExport runs the actual export in a background goroutine.
func (s *ExportAsyncService) executeExport(task *model.ExportTask, username, role string) {
	ctx := context.Background()

	// Mark as processing
	s.updateTaskStatus(task.ID, model.ExportTaskStatusProcessing, "", 0, 0, nil)

	// Create the export file
	f, err := os.Create(task.FilePath)
	if err != nil {
		errMsg := fmt.Sprintf("创建导出文件失败: %v", err)
		s.updateTaskStatus(task.ID, model.ExportTaskStatusFailed, errMsg, 0, 0, nil)
		return
	}
	defer f.Close()

	var totalRows int64

	// Write BOM header
	_, _ = f.Write([]byte{0xEF, 0xBB, 0xBF})

	// Use streaming CSV writer
	w := csv.NewWriter(f)

	switch ExportType(task.ExportType) {
	case ExportTypeAudit:
		var filters AuditExportFilters
		_ = json.Unmarshal([]byte(task.FiltersJSON), &filters)
		totalRows, err = s.streamAuditCSV(ctx, w, username, role, filters)
	case ExportTypeTicket:
		var filters TicketExportFilters
		_ = json.Unmarshal([]byte(task.FiltersJSON), &filters)
		totalRows, err = s.streamTicketCSV(ctx, w, username, role, filters)
	default:
		err = ErrExportTypeInvalid
	}

	// Flush remaining data
	w.Flush()
	if wErr := w.Error(); wErr != nil && err == nil {
		err = wErr
	}

	if err != nil {
		errMsg := fmt.Sprintf("导出失败: %v", err)
		_ = os.Remove(task.FilePath)
		s.updateTaskStatus(task.ID, model.ExportTaskStatusFailed, errMsg, 0, 0, nil)
		return
	}

	// Write watermark
	_, _ = fmt.Fprintf(f, "\n# 导出水印: 导出人=%s | 导出时间=%s | 仅限内部使用\n",
		username,
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)

	_ = f.Sync()

	// Get file size
	info, _ := os.Stat(task.FilePath)
	var fileSize int64
	if info != nil {
		fileSize = info.Size()
	}

	now := time.Now()
	s.updateTaskStatus(task.ID, model.ExportTaskStatusCompleted, "", totalRows, fileSize, &now)
}

// streamAuditCSV streams audit logs directly to the CSV writer without buffering all rows.
func (s *ExportAsyncService) streamAuditCSV(ctx context.Context, w *csv.Writer, username, role string, filters AuditExportFilters) (int64, error) {
	var filterClauses []FilterClause
	if filters.UserID != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "a.user_id = ?", Args: []interface{}{filters.UserID}})
	}
	if filters.Action != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "a.action = ?", Args: []interface{}{filters.Action}})
	}
	if filters.DatasourceID != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "a.datasource_id = ?", Args: []interface{}{filters.DatasourceID}})
	}
	if filters.Start != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "a.created_at >= ?", Args: []interface{}{filters.Start}})
	}
	if filters.End != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "a.created_at <= ?", Args: []interface{}{filters.End}})
	}
	if filters.Keyword != "" {
		keywordLike := "%" + escapeLike(filters.Keyword) + "%"
		filterClauses = append(filterClauses, FilterClause{
			Condition: "(a.sql_content LIKE ? ESCAPE '\\' OR a.sql_summary LIKE ? ESCAPE '\\' OR u.username LIKE ? ESCAPE '\\' OR a.action LIKE ? ESCAPE '\\' OR a.error_message LIKE ? ESCAPE '\\' OR a.database LIKE ? ESCAPE '\\' OR a.ip_address LIKE ? ESCAPE '\\')",
			Args:      []interface{}{keywordLike, keywordLike, keywordLike, keywordLike, keywordLike, keywordLike, keywordLike},
		})
	}

	whereClause, args := BuildWhereClause(filterClauses)

	// Get total count
	countTable := "audit_logs a"
	if filters.Keyword != "" {
		countTable = "audit_logs a LEFT JOIN users u ON a.user_id = u.id"
	}
	countSQL := PaginatedCountSQL(countTable, whereClause)

	var total int64
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("统计审计日志失败: %w", err)
	}

	// Write header
	_ = w.Write([]string{
		"ID", "时间", "用户", "操作", "数据源ID", "数据库",
		"SQL内容", "SQL摘要", "返回行数", "影响行数", "耗时(ms)",
		"错误信息", "脱敏字段", "IP地址", "AI评审", "工单ID",
	})

	querySQL := fmt.Sprintf(
		`SELECT a.id, a.user_id, a.action, a.datasource_id, a.database, a.sql_content, a.sql_summary,
		        a.result_rows, a.affected_rows, a.execution_time_ms, a.error_message,
		        a.desensitized_fields, a.ip_address, a.ai_review_result, a.ticket_id, a.created_at,
		        COALESCE(u.username, '') AS username
		 FROM audit_logs a
		 LEFT JOIN users u ON a.user_id = u.id
		 %s ORDER BY a.created_at DESC`,
		whereClause,
	)

	rows, err := s.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return 0, fmt.Errorf("查询审计日志失败: %w", err)
	}
	defer rows.Close()

	var written int64
	for rows.Next() {
		var a auditCSVRow
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.Action, &a.DatasourceID, &a.Database,
			&a.SQLContent, &a.SQLSummary,
			&a.ResultRows, &a.AffectedRows, &a.ExecutionTimeMs,
			&a.ErrorMessage, &a.DesensitizedFields, &a.IPAddress,
			&a.AIReviewResult, &a.TicketID, &a.CreatedAt,
			&a.Username,
		); err != nil {
			continue
		}

		createdAtStr := ""
		if !a.CreatedAt.IsZero() {
			createdAtStr = a.CreatedAt.Format("2006-01-02 15:04:05")
		}

		_ = w.Write([]string{
			fmt.Sprintf("%d", a.ID),
			createdAtStr,
			a.Username,
			a.Action,
			fmt.Sprintf("%d", a.DatasourceID),
			a.Database,
			a.SQLContent,
			a.SQLSummary,
			fmt.Sprintf("%d", a.ResultRows),
			fmt.Sprintf("%d", a.AffectedRows),
			fmt.Sprintf("%d", a.ExecutionTimeMs),
			a.ErrorMessage,
			a.DesensitizedFields,
			a.IPAddress,
			a.AIReviewResult,
			fmt.Sprintf("%d", a.TicketID),
		})
		written++

		// Flush every 1000 rows to keep memory bounded
		if written%1000 == 0 {
			w.Flush()
		}
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate audit rows: %w", err)
	}

	return total, nil
}

// streamTicketCSV streams tickets directly to the CSV writer.
func (s *ExportAsyncService) streamTicketCSV(ctx context.Context, w *csv.Writer, username, role string, filters TicketExportFilters) (int64, error) {
	var filterClauses []FilterClause
	if filters.Status != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "status = ?", Args: []interface{}{filters.Status}})
	}
	if filters.DatasourceID != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "datasource_id = ?", Args: []interface{}{filters.DatasourceID}})
	}
	if filters.RiskLevel != "" {
		filterClauses = append(filterClauses, FilterClause{Condition: "risk_level = ?", Args: []interface{}{filters.RiskLevel}})
	}
	if filters.Keyword != "" {
		like := "%" + escapeLike(filters.Keyword) + "%"
		filterClauses = append(filterClauses, FilterClause{Condition: "(sql_content LIKE ? OR change_reason LIKE ? OR sql_summary LIKE ?)", Args: []interface{}{like, like, like}})
	}

	whereClause, args := BuildWhereClause(filterClauses)

	var total int64
	countSQL := PaginatedCountSQL("tickets", whereClause)
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("统计工单失败: %w", err)
	}

	// Write header
	_ = w.Write([]string{
		"ID", "提交人", "提交人ID", "数据源ID", "数据库",
		"SQL内容", "SQL摘要", "数据库类型", "变更原因",
		"状态", "风险等级", "审批人", "审批意见",
		"定时执行时间", "实际执行时间", "创建时间", "更新时间",
	})

	querySQL := fmt.Sprintf(
		`SELECT t.id, t.submitter_id, COALESCE(su.username, '') AS submitter_name,
		        t.datasource_id, t.database, t.sql_content, t.sql_summary,
		        t.db_type, t.change_reason, t.status, t.risk_level,
		        t.reviewer_id, COALESCE(rev.username, '') AS reviewer_name,
		        t.review_comment, t.scheduled_at, t.executed_at,
		        t.created_at, t.updated_at
		 FROM tickets t
		 LEFT JOIN users su ON t.submitter_id = su.id
		 LEFT JOIN users rev ON t.reviewer_id = rev.id
		 %s ORDER BY t.created_at DESC`,
		whereClause,
	)

	rows, err := s.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return 0, fmt.Errorf("查询工单失败: %w", err)
	}
	defer rows.Close()

	var written int64
	for rows.Next() {
		var t ticketCSVRow
		if err := rows.Scan(
			&t.ID, &t.SubmitterID, &t.SubmitterName,
			&t.DatasourceID, &t.Database,
			&t.SQLContent, &t.SQLSummary,
			&t.DBType, &t.ChangeReason,
			&t.Status, &t.RiskLevel,
			&t.ReviewerID, &t.ReviewerName,
			&t.ReviewComment,
			&t.ScheduledAt, &t.ExecutedAt,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			continue
		}

		scheduledAtStr := ""
		if t.ScheduledAt.Valid {
			scheduledAtStr = t.ScheduledAt.Time.Format("2006-01-02 15:04:05")
		}
		executedAtStr := ""
		if t.ExecutedAt.Valid {
			executedAtStr = t.ExecutedAt.Time.Format("2006-01-02 15:04:05")
		}

		_ = w.Write([]string{
			fmt.Sprintf("%d", t.ID),
			t.SubmitterName,
			fmt.Sprintf("%d", t.SubmitterID),
			fmt.Sprintf("%d", t.DatasourceID),
			t.Database,
			t.SQLContent,
			t.SQLSummary,
			t.DBType,
			t.ChangeReason,
			t.Status,
			t.RiskLevel,
			t.ReviewerName,
			t.ReviewComment,
			scheduledAtStr,
			executedAtStr,
			t.CreatedAt.Format("2006-01-02 15:04:05"),
			t.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
		written++

		if written%1000 == 0 {
			w.Flush()
		}
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate ticket rows: %w", err)
	}

	return total, nil
}

// updateTaskStatus updates the task status in both DB and in-memory cache.
func (s *ExportAsyncService) updateTaskStatus(taskID int64, status model.ExportTaskStatus, errMsg string, totalRows, fileBytes int64, completedAt *time.Time) {
	_, err := s.db.Exec(
		`UPDATE export_tasks SET status = ?, error_msg = ?, total_rows = ?, file_bytes = ?, completed_at = ? WHERE id = ?`,
		string(status), errMsg, totalRows, fileBytes, completedAt, taskID,
	)
	if err != nil {
		log.Printf("updateTaskStatus error (task %d): %v", taskID, err)
	}

	// Update in-memory cache
	if v, ok := s.tasks.Load(taskID); ok {
		task := v.(*model.ExportTask)
		task.Status = status
		task.ErrorMsg = errMsg
		task.TotalRows = totalRows
		task.FileBytes = fileBytes
		task.CompletedAt = completedAt
	}
}

// recoverPendingTasks re-marks any tasks left in processing state as failed (server restart recovery).
func (s *ExportAsyncService) recoverPendingTasks() {
	_, err := s.db.Exec(
		`UPDATE export_tasks SET status = ?, error_msg = ? WHERE status IN (?, ?)`,
		string(model.ExportTaskStatusFailed), "服务器重启，任务中断", string(model.ExportTaskStatusPending), string(model.ExportTaskStatusProcessing),
	)
	if err != nil {
		log.Printf("recoverPendingTasks error: %v", err)
	}
}

// cleanupLoop periodically removes expired export files and cleans up DB records.
func (s *ExportAsyncService) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanupExpiredFiles()
		case <-s.stopCleanup:
			return
		}
	}
}

// cleanupExpiredFiles removes export files older than ExportFileTTL.
func (s *ExportAsyncService) cleanupExpiredFiles() {
	cutoff := time.Now().Add(-ExportFileTTL)

	// Find expired completed tasks
	rows, err := s.db.Query(
		`SELECT id, file_path FROM export_tasks WHERE status = ? AND completed_at < ?`,
		string(model.ExportTaskStatusCompleted), cutoff,
	)
	if err != nil {
		log.Printf("cleanupExpiredFiles query error: %v", err)
		return
	}

	var taskIDs []int64
	for rows.Next() {
		var id int64
		var fp string
		if err := rows.Scan(&id, &fp); err != nil {
			continue
		}
		// Remove the file
		if err := os.Remove(fp); err != nil && !os.IsNotExist(err) {
			log.Printf("cleanupExpiredFiles: remove %s error: %v", fp, err)
		}
		taskIDs = append(taskIDs, id)
	}
	rows.Close()

	// Mark as failed (archived) to indicate file no longer available
	for _, id := range taskIDs {
		_, _ = s.db.Exec(
			`UPDATE export_tasks SET status = ?, error_msg = ? WHERE id = ?`,
			string(model.ExportTaskStatusFailed), "导出文件已过期清理（24小时）", id,
		)
		s.tasks.Delete(id)
	}

	if len(taskIDs) > 0 {
		log.Printf("cleanupExpiredFiles: cleaned up %d expired export files", len(taskIDs))
	}
}

// generateExportFilename creates a unique filename for an export file.
func generateExportFilename(exportType string) string {
	randBytes := make([]byte, 8)
	_, _ = rand.Read(randBytes)
	suffix := hex.EncodeToString(randBytes)
	return strings.ToLower(exportType) + "_" + time.Now().Format("20060102_150405") + "_" + suffix + ".csv"
}

package service

import (
	"context"
	"crypto/rand"
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

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	"entgo.io/ent/dialect/sql"
	entexporttask "github.com/whg517/sqlflow/internal/db/ent/exporttask"
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
	database   *db.DB
	client     *ent.Client
	exportSvc  *ExportService
	auditSvc   *AuditService
	exportDir  string
	tasks      sync.Map // taskID -> *model.ExportTask (in-memory cache for active tasks)
	stopCleanup chan struct{}
}

// NewExportAsyncService creates a new ExportAsyncService.
func NewExportAsyncService(database *db.DB, exportSvc *ExportService, auditSvc *AuditService, dataDir string) *ExportAsyncService {
	dir := filepath.Join(dataDir, ExportDir)
	_ = os.MkdirAll(dir, 0755)

	svc := &ExportAsyncService{
		database:    database,
		client:      database.Client(),
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

	saved, err := s.client.ExportTask.Create().
		SetUserID(userID).
		SetUsername(username).
		SetExportType(exportType).
		SetStatus(string(model.ExportTaskStatusPending)).
		SetFilename(filename).
		SetFilePath(filePath).
		SetFiltersJSON(filtersJSON).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建导出任务失败: %w", err)
	}

	task := &model.ExportTask{
		ID:          int64(saved.ID),
		UserID:      userID,
		Username:    username,
		ExportType:  exportType,
		Status:      model.ExportTaskStatusPending,
		Filename:    filename,
		FilePath:    filePath,
		FiltersJSON: filtersJSON,
		CreatedAt:   saved.CreatedAt,
	}

	s.tasks.Store(task.ID, task)

	// Launch async export in goroutine
	go s.executeExport(task, username, role)

	return task, nil
}

// GetTask retrieves an export task by ID.
func (s *ExportAsyncService) GetTask(ctx context.Context, taskID int64, userID int64) (*model.ExportTask, error) {
	t, err := s.client.ExportTask.Query().
		Where(entexporttask.ID(int(taskID)), entexporttask.UserID(userID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrExportNotFound
		}
		return nil, fmt.Errorf("查询导出任务失败: %w", err)
	}

	task := &model.ExportTask{
		ID:          int64(t.ID),
		UserID:      t.UserID,
		Username:    t.Username,
		ExportType:  t.ExportType,
		Status:      model.ExportTaskStatus(t.Status),
		Filename:    t.Filename,
		FilePath:    t.FilePath,
		TotalRows:   t.TotalRows,
		FileBytes:   t.FileBytes,
		FiltersJSON: t.FiltersJSON,
		ErrorMsg:    t.ErrorMsg,
		CreatedAt:   t.CreatedAt,
		CompletedAt: t.CompletedAt,
	}

	return task, nil
}

// ListTasks lists export tasks for a user.
func (s *ExportAsyncService) ListTasks(ctx context.Context, userID int64) ([]model.ExportTaskSlim, error) {
	tasks, err := s.client.ExportTask.Query().
		Where(entexporttask.UserID(userID)).
		Order(entexporttask.ByCreatedAt(sql.OrderDesc())).
		Limit(50).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询导出任务列表失败: %w", err)
	}

	var result []model.ExportTaskSlim
	for _, t := range tasks {
		slim := model.ExportTaskSlim{
			ID:          int64(t.ID),
			ExportType:  t.ExportType,
			Status:      model.ExportTaskStatus(t.Status),
			Filename:    t.Filename,
			TotalRows:   t.TotalRows,
			FileBytes:   t.FileBytes,
			ErrorMsg:    t.ErrorMsg,
			CreatedAt:   t.CreatedAt,
			CompletedAt: t.CompletedAt,
		}
		result = append(result, slim)
	}
	return result, nil
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

	// Write BOM header
	_, _ = f.Write([]byte{0xEF, 0xBB, 0xBF})

	var totalRows int64

	switch ExportType(task.ExportType) {
	case ExportTypeAudit:
		var filters AuditExportFilters
		_ = json.Unmarshal([]byte(task.FiltersJSON), &filters)
		totalRows, err = s.exportSvc.StreamExportAuditLogs(ctx, f, username, filters)
	case ExportTypeTicket:
		var filters TicketExportFilters
		_ = json.Unmarshal([]byte(task.FiltersJSON), &filters)
		totalRows, err = s.exportSvc.StreamExportTickets(ctx, f, username, filters)
	default:
		err = ErrExportTypeInvalid
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

// updateTaskStatus updates the task status in both DB and in-memory cache.
// Uses raw SQL (no context) to match Phase 3 Scheduler pattern for background goroutines.
func (s *ExportAsyncService) updateTaskStatus(taskID int64, status model.ExportTaskStatus, errMsg string, totalRows, fileBytes int64, completedAt *time.Time) {
	_, err := s.database.Exec(
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
// Uses raw SQL — same pattern as Phase 3 Scheduler.
func (s *ExportAsyncService) recoverPendingTasks() {
	_, err := s.database.Exec(
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
// Uses raw SQL for the file path query — no ent equivalent for this pattern.
func (s *ExportAsyncService) cleanupExpiredFiles() {
	cutoff := time.Now().Add(-ExportFileTTL)

	// Find expired completed tasks
	rows, err := s.database.Query(
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
		_, _ = s.database.Exec(
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

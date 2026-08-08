package query

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

	"entgo.io/ent/dialect/sql"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entexporttask "github.com/whg517/sqlflow/internal/db/ent/exporttask"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
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

// AsyncExportService handles asynchronous export task lifecycle.
type AsyncExportService struct {
	database    *db.DB
	client      *ent.Client
	exportSvc   *ExportService
	auditSvc    auditlog.Writer
	exportDir   string
	tasks       sync.Map // taskID -> *model.ExportTask (in-memory cache for active tasks)
	stopCleanup chan struct{}
}

// NewAsyncExportService creates a new AsyncExportService.
func NewAsyncExportService(database *db.DB, exportSvc *ExportService, auditSvc auditlog.Writer, dataDir string) *AsyncExportService {
	dir := filepath.Join(dataDir, ExportDir)
	_ = os.MkdirAll(dir, 0755)

	svc := &AsyncExportService{
		database:    database,
		client:      database.Client(),
		exportSvc:   exportSvc,
		auditSvc:    auditlog.OrDiscard(auditSvc),
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
func (s *AsyncExportService) Close() {
	close(s.stopCleanup)
}

// CreateAsyncExport creates an async export task and starts it in a goroutine.
// It returns the task ID immediately.
// CreateAsyncExport queues an export and starts it.
//
// columns is the caller's column selection, nil meaning every column. It
// travels to the worker the same way fileFormat does — as a parameter rather
// than on the task row — because the goroutine starts here and nothing resumes
// a task across a restart. It used to stop at the handler, so an export that
// crossed the sync row limit and switched to this path silently widened back
// to every column.
//
// actor travels the same way, and for the same reason: the worker needs it to
// scope the rows it reads. recoverPendingTasks fails anything left in flight by
// a restart, so no resumed task can arrive without one.
func (s *AsyncExportService) CreateAsyncExport(ctx context.Context, actor ExportActor, exportType string, filtersJSON string, fileFormat string, columns map[string]int) (*model.ExportTask, error) {
	if err := s.exportSvc.authorizeExportRequest(ctx, actor, ExportType(exportType)); err != nil {
		return nil, err
	}

	filename := generateExportFilename(exportType, fileFormat)
	filePath := filepath.Join(s.exportDir, filename)

	saved, err := s.client.ExportTask.Create().
		SetUserID(actor.UserID).
		SetUsername(actor.Username).
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
		UserID:      actor.UserID,
		Username:    actor.Username,
		ExportType:  exportType,
		Status:      model.ExportTaskStatusPending,
		Filename:    filename,
		FilePath:    filePath,
		FiltersJSON: filtersJSON,
		CreatedAt:   saved.CreatedAt,
	}

	s.tasks.Store(task.ID, task)

	// Launch async export in goroutine
	go s.executeExport(task, actor, fileFormat, columns)

	return task, nil
}

// GetTask retrieves an export task by ID.
func (s *AsyncExportService) GetTask(ctx context.Context, taskID int64, userID int64) (*model.ExportTask, error) {
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
func (s *AsyncExportService) ListTasks(ctx context.Context, userID int64) ([]model.ExportTaskSlim, error) {
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
//
// Two boundaries meet here. GetTask restricts the task row to its owner, and the
// rows inside the file are the ones that owner could see when the worker wrote
// it — a file is a snapshot, so nothing at download time can narrow it further.
// What the re-authorization below does add is revocation: an actor whose export
// grant was withdrawn after the file was generated no longer gets to pull it.
func (s *AsyncExportService) DownloadFile(ctx context.Context, taskID int64, actor ExportActor) (io.ReadCloser, string, error) {
	task, err := s.GetTask(ctx, taskID, actor.UserID)
	if err != nil {
		return nil, "", err
	}

	if err := s.exportSvc.authorizeExportRequest(ctx, actor, ExportType(task.ExportType)); err != nil {
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
//
// It carries the actor rather than just a name: the streams below derive their
// row boundary from it. The worker used to take only a username and so wrote
// whatever the unscoped query returned, which turned a request the handler had
// narrowed into a file containing everyone's rows.
func (s *AsyncExportService) executeExport(task *model.ExportTask, actor ExportActor, fileFormat string, columns map[string]int) {
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

	// Write BOM header (CSV only)
	if fileFormat != string(ExportFormatExcel) {
		_, _ = f.Write([]byte{0xEF, 0xBB, 0xBF})
	}

	var totalRows int64

	switch ExportType(task.ExportType) {
	case ExportTypeAudit:
		var filters AuditExportFilters
		_ = json.Unmarshal([]byte(task.FiltersJSON), &filters)
		if fileFormat == string(ExportFormatExcel) {
			totalRows, err = s.exportSvc.StreamExportAuditLogsExcel(ctx, f, actor, filters, columns)
		} else {
			totalRows, err = s.exportSvc.StreamExportAuditLogs(ctx, f, actor, filters)
		}
	case ExportTypeTicket:
		var filters TicketExportFilters
		_ = json.Unmarshal([]byte(task.FiltersJSON), &filters)
		if fileFormat == string(ExportFormatExcel) {
			totalRows, err = s.exportSvc.StreamExportTicketsExcel(ctx, f, actor, filters, columns)
		} else {
			totalRows, err = s.exportSvc.StreamExportTickets(ctx, f, actor, filters)
		}
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
		actor.Username,
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
//
// It runs on a background goroutine with no request context of its own, so it
// takes context.Background(): a caller's cancelled context must not leave the
// task stuck in processing forever.
func (s *AsyncExportService) updateTaskStatus(taskID int64, status model.ExportTaskStatus, errMsg string, totalRows, fileBytes int64, completedAt *time.Time) {
	upd := s.client.ExportTask.UpdateOneID(int(taskID)).
		SetStatus(string(status)).
		SetErrorMsg(errMsg).
		SetTotalRows(totalRows).
		SetFileBytes(fileBytes)
	if completedAt != nil {
		upd = upd.SetCompletedAt(*completedAt)
	} else {
		upd = upd.ClearCompletedAt()
	}
	if err := upd.Exec(context.Background()); err != nil {
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

// recoverPendingTasks re-marks any tasks left in processing state as failed.
//
// A task in flight when the process died has no worker any more, so leaving it
// pending would show the user a job that never finishes.
func (s *AsyncExportService) recoverPendingTasks() {
	_, err := s.client.ExportTask.Update().
		Where(entexporttask.StatusIn(
			string(model.ExportTaskStatusPending),
			string(model.ExportTaskStatusProcessing),
		)).
		SetStatus(string(model.ExportTaskStatusFailed)).
		SetErrorMsg("服务器重启，任务中断").
		Save(context.Background())
	if err != nil {
		log.Printf("recoverPendingTasks error: %v", err)
	}
}

// cleanupLoop periodically removes expired export files and cleans up DB records.
func (s *AsyncExportService) cleanupLoop() {
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
func (s *AsyncExportService) cleanupExpiredFiles() {
	ctx := context.Background()
	cutoff := time.Now().Add(-ExportFileTTL)

	expired, err := s.client.ExportTask.Query().
		Where(
			entexporttask.StatusEQ(string(model.ExportTaskStatusCompleted)),
			entexporttask.CompletedAtLT(cutoff),
		).
		All(ctx)
	if err != nil {
		log.Printf("cleanupExpiredFiles query error: %v", err)
		return
	}

	taskIDs := make([]int, 0, len(expired))
	for _, t := range expired {
		if err := os.Remove(t.FilePath); err != nil && !os.IsNotExist(err) {
			// The row is still marked expired: leaving it completed would offer
			// the user a download for a file that may already be gone.
			log.Printf("cleanupExpiredFiles: remove %s error: %v", t.FilePath, err)
		}
		taskIDs = append(taskIDs, t.ID)
	}
	if len(taskIDs) == 0 {
		return
	}

	// One statement rather than one per task: the previous loop issued an
	// update per row, and this runs hourly over whatever accumulated.
	if _, err := s.client.ExportTask.Update().
		Where(entexporttask.IDIn(taskIDs...)).
		SetStatus(string(model.ExportTaskStatusFailed)).
		SetErrorMsg("导出文件已过期清理（24小时）").
		Save(ctx); err != nil {
		log.Printf("cleanupExpiredFiles update error: %v", err)
		return
	}
	for _, id := range taskIDs {
		s.tasks.Delete(int64(id))
	}

	log.Printf("cleanupExpiredFiles: cleaned up %d expired export files", len(taskIDs))
}

// generateExportFilename creates a unique filename for an export file.
func generateExportFilename(exportType string, fileFormat string) string {
	randBytes := make([]byte, 8)
	_, _ = rand.Read(randBytes)
	suffix := hex.EncodeToString(randBytes)
	ext := "csv"
	if fileFormat == string(ExportFormatExcel) {
		ext = "xlsx"
	}
	return strings.ToLower(exportType) + "_" + time.Now().Format("20060102_150405") + "_" + suffix + "." + ext
}

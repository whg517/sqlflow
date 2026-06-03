package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/config"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/service"
)

func setupBackupHandlerTest(t *testing.T) (*echo.Echo, *service.BackupService, *BackupHandler, string) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := config.BackupConfig{
		Enabled:  true,
		Dir:      backupDir,
		Interval: 999 * 24 * 60 * 60 * 1000000000, // very long to prevent auto-start
		Keep:     5,
		Compress: false,
	}

	wrapped, _ := db.WrapSQL(database.DB)
	backupSvc := service.NewBackupService(wrapped, dbPath, cfg)
	handler := NewBackupHandler(backupSvc)
	e := echo.New()

	return e, backupSvc, handler, backupDir
}

func TestBackupHandler_TriggerBackup(t *testing.T) {
	e, _, h, _ := setupBackupHandlerTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/backups", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.TriggerBackup(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// resp.OK wraps in {code:0, message:"ok", data:...}
	if result["message"] != "ok" {
		t.Errorf("message = %v, want %q", result["message"], "ok")
	}
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object; body=%s", rec.Body.String())
	}
	if data["message"] != "备份已创建" {
		t.Errorf("data.message = %v, want %q", data["message"], "备份已创建")
	}
}

func TestBackupHandler_ListBackups(t *testing.T) {
	e, backupSvc, h, backupDir := setupBackupHandlerTest(t)

	// Create a backup manually
	if err := backupSvc.RunBackup(); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/backups", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ListBackups(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Verify backup dir exists
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least 1 backup file")
	}
}

func TestBackupHandler_ListBackups_Empty(t *testing.T) {
	e, _, h, _ := setupBackupHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/backups", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ListBackups(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Empty list returns null in JSON
	data := result["data"]
	if data != nil {
		t.Errorf("expected null data for empty list, got %v", data)
	}
}

func TestBackupHandler_DeleteBackup(t *testing.T) {
	e, backupSvc, h, _ := setupBackupHandlerTest(t)

	// Create a backup first
	if err := backupSvc.RunBackup(); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}

	// Find the backup filename
	backups, err := backupSvc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) == 0 {
		t.Fatal("expected at least 1 backup")
	}

	filename := backups[0].Filename

	req := httptest.NewRequest(http.MethodDelete, "/api/backups/"+filename, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("filename")
	c.SetParamValues(filename)

	if err := h.DeleteBackup(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// resp.OK wraps in {code:0, message:"ok", data:...}
	if result["message"] != "ok" {
		t.Errorf("message = %v, want %q", result["message"], "ok")
	}
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object; body=%s", rec.Body.String())
	}
	if data["message"] != "备份已删除" {
		t.Errorf("data.message = %v, want %q", data["message"], "备份已删除")
	}
}

func TestBackupHandler_DeleteBackup_EmptyFilename(t *testing.T) {
	e, _, h, _ := setupBackupHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/backups/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("filename")
	c.SetParamValues("")

	if err := h.DeleteBackup(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestBackupHandler_DeleteBackup_NotFound(t *testing.T) {
	e, _, h, _ := setupBackupHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/backups/sqlflow-nonexistent.db", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("filename")
	c.SetParamValues("sqlflow-nonexistent.db")

	if err := h.DeleteBackup(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestBackupHandler_DownloadBackup(t *testing.T) {
	e, backupSvc, h, _ := setupBackupHandlerTest(t)

	// Create a backup first
	if err := backupSvc.RunBackup(); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}

	backups, err := backupSvc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) == 0 {
		t.Fatal("expected at least 1 backup")
	}

	filename := backups[0].Filename

	req := httptest.NewRequest(http.MethodGet, "/api/backups/"+filename+"/download", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("filename")
	c.SetParamValues(filename)

	if err := h.DownloadBackup(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Verify file content was sent
	body := rec.Body.String()
	if len(body) == 0 {
		t.Error("download response should not be empty")
	}
	// SQLite files start with "SQLite format 3"
	if !strings.HasPrefix(body, "SQLite format 3") {
		t.Errorf("expected SQLite file header, got: %s", body[:min(50, len(body))])
	}
}

func TestBackupHandler_DownloadBackup_NotFound(t *testing.T) {
	e, _, h, _ := setupBackupHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/backups/sqlflow-nonexistent.db/download", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("filename")
	c.SetParamValues("sqlflow-nonexistent.db")

	if err := h.DownloadBackup(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestBackupHandler_DownloadBackup_EmptyFilename(t *testing.T) {
	e, _, h, _ := setupBackupHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/backups//download", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("filename")
	c.SetParamValues("")

	if err := h.DownloadBackup(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// Suppress unused import
var _ = strings.NewReader

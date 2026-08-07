package query

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/whg517/sqlflow/internal/audit"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/testutil"
)

// waitForExportFile polls until the background goroutine has written the file.
func waitForExportFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			// The writer may still be flushing; a stable size means it is done.
			time.Sleep(100 * time.Millisecond)
			if again, err := os.Stat(path); err == nil && again.Size() == info.Size() {
				return
			}
			continue
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("export file %s was never written", path)
}

// TestAsyncExcelExportHonorsTheColumnSelection pins what the caller asked for.
//
// The column selection reached createAsyncExport and stopped there: the task
// carried filters and format but not columns, and the background worker passed
// nil, which auditColumnIndices reads as "every column".
//
// The consequence is not a cosmetic difference. Async is not something the user
// opts into — the handler switches to it automatically once the row count
// exceeds the sync limit. So the same request, with the same column selection,
// returned the chosen columns on a small date range and every column on a large
// one, including SQL内容 and 错误信息, which is exactly what a caller narrowing
// the column list is usually trying to leave out.
func TestAsyncExcelExportHonorsTheColumnSelection(t *testing.T) {
	db, dataDir := newExportAsyncTestDB(t)
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	exportSvc := NewExportService(testutil.WrapSQL(t, db), auditSvc)
	asyncSvc := NewAsyncExportService(testutil.WrapSQL(t, db), exportSvc, auditSvc, dataDir)
	defer asyncSvc.Close()

	if _, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	auditSvc.Write(context.Background(), auditlog.Record{
		UserID: 1, Action: "query_execute",
		SQLContent: "SELECT secret FROM payroll", SQLSummary: "SELECT secret",
	})

	filtersJSON, _ := json.Marshal(AuditExportFilters{})
	chosen, err := ValidateExportColumns([]string{"ID", "时间", "用户"}, ExportTypeAudit)
	if err != nil {
		t.Fatalf("ValidateExportColumns: %v", err)
	}

	task, err := asyncSvc.CreateAsyncExport(context.Background(), 1, "admin", "admin",
		"audit", string(filtersJSON), "xlsx", chosen)
	if err != nil {
		t.Fatalf("CreateAsyncExport: %v", err)
	}

	path := filepath.Join(dataDir, ExportDir, task.Filename)
	waitForExportFile(t, path)

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer func() { _ = f.Close() }()

	rows, err := f.GetRows("Sheet1")
	if err != nil || len(rows) == 0 {
		t.Fatalf("read rows: %v", err)
	}
	header := rows[0]

	if len(header) != 3 {
		t.Errorf("header has %d columns %v, want the 3 that were selected", len(header), header)
	}
	for _, unwanted := range []string{"SQL内容", "错误信息"} {
		for _, got := range header {
			if got == unwanted {
				t.Errorf("column %q was exported although the caller did not select it", unwanted)
			}
		}
	}
}

// TestAsyncExcelExportWithoutSelectionKeepsEveryColumn is the other direction:
// no selection still means everything, which is what an empty columns parameter
// has always meant.
func TestAsyncExcelExportWithoutSelectionKeepsEveryColumn(t *testing.T) {
	db, dataDir := newExportAsyncTestDB(t)
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	exportSvc := NewExportService(testutil.WrapSQL(t, db), auditSvc)
	asyncSvc := NewAsyncExportService(testutil.WrapSQL(t, db), exportSvc, auditSvc, dataDir)
	defer asyncSvc.Close()

	if _, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	auditSvc.Write(context.Background(), auditlog.Record{UserID: 1, Action: "query_execute"})

	filtersJSON, _ := json.Marshal(AuditExportFilters{})
	task, err := asyncSvc.CreateAsyncExport(context.Background(), 1, "admin", "admin",
		"audit", string(filtersJSON), "xlsx", nil)
	if err != nil {
		t.Fatalf("CreateAsyncExport: %v", err)
	}

	path := filepath.Join(dataDir, ExportDir, task.Filename)
	waitForExportFile(t, path)

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer func() { _ = f.Close() }()

	rows, err := f.GetRows("Sheet1")
	if err != nil || len(rows) == 0 {
		t.Fatalf("read rows: %v", err)
	}
	if len(rows[0]) != len(auditColumnNames) {
		t.Errorf("header has %d columns, want all %d", len(rows[0]), len(auditColumnNames))
	}
}

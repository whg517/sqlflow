package service

import (
	"bytes"
	"context"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestStreamExportAuditLogsExcel_Basic(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(mustWrapDB(db), 0, 0)
	svc := NewExportService(mustWrapDB(db), auditSvc)

	seedAuditLogs(t, db, 5)

	var buf bytes.Buffer
	written, err := svc.StreamExportAuditLogsExcel(context.Background(), &buf, "admin", AuditExportFilters{}, nil)
	if err != nil {
		t.Fatalf("StreamExportAuditLogsExcel: %v", err)
	}
	if written != 5 {
		t.Errorf("written = %d, want 5", written)
	}

	// Verify the output is a valid xlsx file
	f, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open generated xlsx: %v", err)
	}
	defer f.Close()

	// Check header row
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least 2 rows (header + data), got %d", len(rows))
	}

	// Verify header contains expected columns
	header := rows[0]
	if header[0] != "ID" {
		t.Errorf("expected header[0] = 'ID', got %q", header[0])
	}
	if header[2] != "用户" {
		t.Errorf("expected header[2] = '用户', got %q", header[2])
	}

	// Verify data rows exist (1 header + 5 data rows)
	if len(rows) != 6 {
		t.Errorf("expected 6 rows (1 header + 5 data), got %d", len(rows))
	}
}

func TestStreamExportAuditLogsExcel_WithColumnSelection(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(mustWrapDB(db), 0, 0)
	svc := NewExportService(mustWrapDB(db), auditSvc)

	seedAuditLogs(t, db, 3)

	// Select only ID, 用户, 操作 columns
	columns := map[string]int{
		"ID": 0, "用户": 2, "操作": 3,
	}

	var buf bytes.Buffer
	written, err := svc.StreamExportAuditLogsExcel(context.Background(), &buf, "admin", AuditExportFilters{}, columns)
	if err != nil {
		t.Fatalf("StreamExportAuditLogsExcel with columns: %v", err)
	}
	if written != 3 {
		t.Errorf("written = %d, want 3", written)
	}

	f, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open generated xlsx: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}

	// Header should have only 3 columns
	if len(rows[0]) != 3 {
		t.Errorf("expected 3 columns in header, got %d: %v", len(rows[0]), rows[0])
	}
	if rows[0][0] != "ID" || rows[0][1] != "用户" || rows[0][2] != "操作" {
		t.Errorf("unexpected header columns: %v", rows[0])
	}

	// Each data row should also have 3 columns
	for i := 1; i < len(rows); i++ {
		if len(rows[i]) != 3 {
			t.Errorf("data row %d: expected 3 columns, got %d", i, len(rows[i]))
		}
	}
}

func TestStreamExportTicketsExcel_Basic(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(mustWrapDB(db), 0, 0)
	svc := NewExportService(mustWrapDB(db), auditSvc)

	seedTickets(t, db, 4)

	var buf bytes.Buffer
	written, err := svc.StreamExportTicketsExcel(context.Background(), &buf, "admin", TicketExportFilters{}, nil)
	if err != nil {
		t.Fatalf("StreamExportTicketsExcel: %v", err)
	}
	if written != 4 {
		t.Errorf("written = %d, want 4", written)
	}

	f, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open generated xlsx: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) != 5 { // 1 header + 4 data
		t.Errorf("expected 5 rows, got %d", len(rows))
	}

	// Verify ticket header
	header := rows[0]
	if header[0] != "ID" {
		t.Errorf("expected header[0] = 'ID', got %q", header[0])
	}
	if header[1] != "提交人" {
		t.Errorf("expected header[1] = '提交人', got %q", header[1])
	}
}

func TestStreamExportTicketsExcel_WithColumnSelection(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(mustWrapDB(db), 0, 0)
	svc := NewExportService(mustWrapDB(db), auditSvc)

	seedTickets(t, db, 2)

	// Select only ID, 状态, 风险等级 columns
	columns := map[string]int{
		"ID": 0, "状态": 9, "风险等级": 10,
	}

	var buf bytes.Buffer
	written, err := svc.StreamExportTicketsExcel(context.Background(), &buf, "admin", TicketExportFilters{}, columns)
	if err != nil {
		t.Fatalf("StreamExportTicketsExcel with columns: %v", err)
	}
	if written != 2 {
		t.Errorf("written = %d, want 2", written)
	}

	f, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open generated xlsx: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}

	if len(rows[0]) != 3 {
		t.Errorf("expected 3 columns, got %d: %v", len(rows[0]), rows[0])
	}
	if rows[0][0] != "ID" || rows[0][1] != "状态" || rows[0][2] != "风险等级" {
		t.Errorf("unexpected header: %v", rows[0])
	}
}

func TestStreamExportAuditLogsExcel_Empty(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(mustWrapDB(db), 0, 0)
	svc := NewExportService(mustWrapDB(db), auditSvc)

	var buf bytes.Buffer
	written, err := svc.StreamExportAuditLogsExcel(context.Background(), &buf, "admin", AuditExportFilters{}, nil)
	if err != nil {
		t.Fatalf("StreamExportAuditLogsExcel empty: %v", err)
	}
	if written != 0 {
		t.Errorf("written = %d, want 0", written)
	}

	// Should still be valid xlsx with header
	f, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open generated xlsx: %v", err)
	}
	defer f.Close()
}

// ---------------------------------------------------------------------------
// Format and column validation tests
// ---------------------------------------------------------------------------

func TestParseExportFormat(t *testing.T) {
	tests := []struct {
		input string
		want  ExportFormat
	}{
		{"csv", ExportFormatCSV},
		{"xlsx", ExportFormatExcel},
		{"CSV", ExportFormatCSV}, // case-sensitive, fallback to CSV
		{"", ExportFormatCSV},
		{"json", ExportFormatCSV}, // unknown, fallback to CSV
	}

	for _, tt := range tests {
		got := ParseExportFormat(tt.input)
		if got != tt.want {
			t.Errorf("ParseExportFormat(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidateExportColumns(t *testing.T) {
	t.Run("nil columns means all", func(t *testing.T) {
		result, err := ValidateExportColumns(nil, ExportTypeAudit)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil for empty columns, got %v", result)
		}
	})

	t.Run("valid audit columns", func(t *testing.T) {
		columns := []string{"ID", "用户", "操作"}
		result, err := ValidateExportColumns(columns, ExportTypeAudit)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Errorf("expected 3 columns, got %d", len(result))
		}
		if result["ID"] != 0 || result["用户"] != 2 || result["操作"] != 3 {
			t.Errorf("unexpected column indices: %v", result)
		}
	})

	t.Run("invalid audit column name", func(t *testing.T) {
		columns := []string{"ID", "NOT_A_COLUMN"}
		_, err := ValidateExportColumns(columns, ExportTypeAudit)
		if err == nil {
			t.Error("expected error for invalid column")
		}
	})

	t.Run("valid ticket columns", func(t *testing.T) {
		columns := []string{"ID", "状态", "风险等级"}
		result, err := ValidateExportColumns(columns, ExportTypeTicket)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Errorf("expected 3 columns, got %d", len(result))
		}
	})

	t.Run("ticket column used for audit export", func(t *testing.T) {
		columns := []string{"ID", "数据库类型"} // valid ticket column, not valid for audit
		_, err := ValidateExportColumns(columns, ExportTypeAudit)
		if err == nil {
			t.Error("expected error for ticket column in audit export")
		}
	})
}

func TestEscapeExcelFormula(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"=SUM(A1:A10)", "'=SUM(A1:A10)"},
		{"+cmd|/c calc", "'+cmd|/c calc"},
		{"-1+1", "'-1+1"},
		{"@SUM", "'@SUM"},
		{"\ttab", "'\ttab"},
		{"\rcarriage", "'\rcarriage"},
		{"normal text", "normal text"},
		{"", ""},
		{"SELECT * FROM users", "SELECT * FROM users"},
		{"=CMD('something')", "'=CMD('something')"},
	}

	for _, tt := range tests {
		got := escapeExcelFormula(tt.input)
		if got != tt.want {
			t.Errorf("escapeExcelFormula(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSortColumnIndices(t *testing.T) {
	t.Run("preserves original order", func(t *testing.T) {
		columns := map[string]int{
			"操作": 3, "ID": 0, "用户": 2,
		}
		indices := sortColumnIndices(columns, 16)
		if len(indices) != 3 {
			t.Fatalf("expected 3 indices, got %d", len(indices))
		}
		// Should maintain original column order (0, 2, 3)
		if indices[0] != 0 || indices[1] != 2 || indices[2] != 3 {
			t.Errorf("expected [0, 2, 3], got %v", indices)
		}
	})

	t.Run("nil returns all", func(t *testing.T) {
		indices := sortColumnIndices(nil, 5)
		if indices != nil {
			t.Errorf("expected nil, got %v", indices)
		}
	})
}

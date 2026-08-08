package query

import (
	"bytes"
	"testing"

	"github.com/whg517/sqlflow/internal/audit"
	"github.com/whg517/sqlflow/internal/testutil"
	"github.com/xuri/excelize/v2"
)

// TestExportAuditLogsWithFilters covers every audit export path with a filter
// actually applied.
//
// The existing cases all passed empty filters, which is the one shape that
// hides a whole class of defect: with no filters the WHERE clause is empty, so
// a statement that mixes placeholder styles still has exactly one placeholder
// and runs. The Excel path did exactly that — it interpolated a WHERE clause
// written with ? and then hard-coded $1 for the row limit — and was broken for
// any filtered export.
func TestExportAuditLogsWithFilters(t *testing.T) {
	database := testutil.NewDB(t)
	auditSvc := audit.NewService(database, 0, 0)
	svc := newExportServiceForTest(t, database, auditSvc)

	seedAuditLogs(t, database.DB, 6)

	// Half the seeded records belong to user 1.
	filters := AuditExportFilters{UserID: "1"}

	t.Run("count", func(t *testing.T) {
		total, err := svc.countAuditLogs(t.Context(), adminActor, filters)
		if err != nil {
			t.Fatalf("countAuditLogs: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
	})

	t.Run("csv", func(t *testing.T) {
		var buf bytes.Buffer
		written, err := svc.StreamExportAuditLogs(t.Context(), &buf, adminActor, filters, nil)
		if err != nil {
			t.Fatalf("StreamExportAuditLogs: %v", err)
		}
		if written != 3 {
			t.Errorf("written = %d, want 3", written)
		}
	})

	t.Run("excel", func(t *testing.T) {
		var buf bytes.Buffer
		written, err := svc.StreamExportAuditLogsExcel(t.Context(), &buf, adminActor, filters, nil)
		if err != nil {
			t.Fatalf("StreamExportAuditLogsExcel: %v", err)
		}
		if written != 3 {
			t.Errorf("written = %d, want 3", written)
		}
		f, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("open generated xlsx: %v", err)
		}
		defer func() { _ = f.Close() }()
		rows, err := f.GetRows("Sheet1")
		if err != nil {
			t.Fatalf("GetRows: %v", err)
		}
		if len(rows) != 4 {
			t.Errorf("sheet has %d rows, want 4 (header + 3)", len(rows))
		}
	})
}

// TestExportAuditLogsKeywordIsNotAPattern checks that a keyword of LIKE
// metacharacters does not widen the export to the whole table.
//
// An export is the one place where over-matching hands the caller the data
// rather than merely showing it on screen.
func TestExportAuditLogsKeywordIsNotAPattern(t *testing.T) {
	database := testutil.NewDB(t)
	auditSvc := audit.NewService(database, 0, 0)
	svc := newExportServiceForTest(t, database, auditSvc)

	seedAuditLogs(t, database.DB, 6)

	total, err := svc.countAuditLogs(t.Context(), adminActor, AuditExportFilters{Keyword: "%"})
	if err != nil {
		t.Fatalf("countAuditLogs: %v", err)
	}
	if total != 0 {
		t.Errorf("keyword %%%% matched %d records, want 0", total)
	}
}

// TestExportTicketsWithFilters is the ticket-side equivalent.
func TestExportTicketsWithFilters(t *testing.T) {
	database := testutil.NewDB(t)
	auditSvc := audit.NewService(database, 0, 0)
	svc := newExportServiceForTest(t, database, auditSvc)

	seedTickets(t, database.DB, 6)

	filters := TicketExportFilters{Status: "SUBMITTED"}

	total, err := svc.countTickets(t.Context(), adminActor, filters)
	if err != nil {
		t.Fatalf("countTickets: %v", err)
	}
	if total == 0 {
		t.Fatal("no tickets matched status=SUBMITTED; the fixture no longer seeds any")
	}

	var csvBuf bytes.Buffer
	csvWritten, err := svc.StreamExportTickets(t.Context(), &csvBuf, adminActor, filters, nil)
	if err != nil {
		t.Fatalf("StreamExportTickets: %v", err)
	}
	if csvWritten != total {
		t.Errorf("csv wrote %d rows, want %d", csvWritten, total)
	}

	var xlsxBuf bytes.Buffer
	xlsxWritten, err := svc.StreamExportTicketsExcel(t.Context(), &xlsxBuf, adminActor, filters, nil)
	if err != nil {
		t.Fatalf("StreamExportTicketsExcel: %v", err)
	}
	if xlsxWritten != total {
		t.Errorf("xlsx wrote %d rows, want %d", xlsxWritten, total)
	}
}

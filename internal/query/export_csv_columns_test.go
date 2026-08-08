package query

import (
	"bytes"
	"context"
	"encoding/csv"
	"testing"

	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/testutil"
)

// TestStreamExportCSVHonorsTheColumnSelection closes the half of a fix whose
// comment says it already landed.
//
// CreateAsyncExport carries the column selection to the worker precisely so
// that "an export that crossed the sync row limit and switched to this path"
// could not silently widen back to every column. That was implemented for
// Excel only: the CSV writers took no column argument at all, so
// ?format=csv&columns=["ID"] validated, was accepted, and returned all sixteen.
//
// Parsing with encoding/csv rather than scanning the text, because a header
// that is present but quoted is still present.
func TestStreamExportCSVHonorsTheColumnSelection(t *testing.T) {
	testDB := newExportTestDB(t)
	seedAuditLogs(t, testDB, 3)
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, testDB), auditlog.Discard)
	ctx := context.Background()
	actor := adminActor

	want := []string{"ID", "用户", "SQL摘要"}
	cols, err := ValidateExportColumns(want, ExportTypeAudit)
	if err != nil {
		t.Fatalf("ValidateExportColumns: %v", err)
	}

	var buf bytes.Buffer
	if _, err := svc.StreamExportAuditLogs(ctx, &buf, actor, AuditExportFilters{}, cols); err != nil {
		t.Fatalf("StreamExportAuditLogs: %v", err)
	}

	records, err := csv.NewReader(bytes.NewReader(buf.Bytes())).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("no CSV output")
	}

	header := records[0]
	if len(header) != len(want) {
		t.Fatalf("header has %d columns %v, want %d %v", len(header), header, len(want), want)
	}
	for i, name := range want {
		if header[i] != name {
			t.Errorf("column %d = %q, want %q", i, header[i], name)
		}
	}
	for _, row := range records[1:] {
		if len(row) != len(want) {
			t.Errorf("data row has %d cells, want %d", len(row), len(want))
		}
	}
}

// TestStreamExportCSVDefaultsToEveryColumn is the control: asking for nothing
// still exports everything, which is what the sync path relies on.
func TestStreamExportCSVDefaultsToEveryColumn(t *testing.T) {
	testDB := newExportTestDB(t)
	seedAuditLogs(t, testDB, 2)
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, testDB), auditlog.Discard)
	ctx := context.Background()
	actor := adminActor

	var buf bytes.Buffer
	if _, err := svc.StreamExportAuditLogs(ctx, &buf, actor, AuditExportFilters{}, nil); err != nil {
		t.Fatalf("StreamExportAuditLogs: %v", err)
	}
	records, err := csv.NewReader(bytes.NewReader(buf.Bytes())).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records[0]) != len(auditColumnNames) {
		t.Errorf("header has %d columns, want all %d", len(records[0]), len(auditColumnNames))
	}
}

// TestCSVHeaderMatchesTheDeclaredColumnNames keeps the two lists from drifting.
//
// The header used to be a second copy of auditColumnNames written out as a
// literal, with nothing checking that the writer's field order still matched
// the index order ValidateExportColumns hands to the writers. That is the
// four-places-one-list shape this repo has been bitten by before.
func TestCSVHeaderMatchesTheDeclaredColumnNames(t *testing.T) {
	testDB := newExportTestDB(t)
	seedAuditLogs(t, testDB, 1)
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, testDB), auditlog.Discard)
	actor := adminActor

	var buf bytes.Buffer
	if _, err := svc.StreamExportTickets(context.Background(), &buf, actor, TicketExportFilters{}, nil); err != nil {
		t.Fatalf("StreamExportTickets: %v", err)
	}
	records, err := csv.NewReader(bytes.NewReader(buf.Bytes())).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	for i, name := range ticketColumnNames {
		if records[0][i] != name {
			t.Errorf("ticket CSV column %d = %q, want %q", i, records[0][i], name)
		}
	}
}

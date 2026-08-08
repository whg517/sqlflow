package query

import (
	"encoding/csv"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestWriteCSV_NeutralizesFormulas closes the gap between this package's two
// CSV writers.
//
// export.go escapes cells that begin with = + - @ before they reach a
// spreadsheet, and export_excel.go does the same; writeCSV — which serves the
// query result grid, the widest attacker-controlled surface of the three — did
// not. A value written into the target database by anyone at all lands in the
// downloaded file as a live formula.
//
// Second-order, and export is dba/admin-scoped by the seed policy, so this is
// not a privilege escalation. It is the same package having fixed the problem
// for SQL text and left the actual data unprotected.
func TestWriteCSV_NeutralizesFormulas(t *testing.T) {
	dangerous := []string{
		`=cmd|'/c calc'!A1`,
		`+1+1`,
		`-1+1`,
		`@SUM(A1)`,
	}

	rows := make([]map[string]interface{}, 0, len(dangerous))
	for _, v := range dangerous {
		rows = append(rows, map[string]interface{}{"note": v})
	}

	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest("GET", "/", nil), rec)
	if err := writeCSV(c, &QueryResult{Columns: []string{"note"}, Rows: rows}); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	// Parse the file the way a spreadsheet would rather than scanning the raw
	// bytes. Quoting is not neutralization: `"=cmd|'/c calc'!A1"` is quoted
	// because it contains a comma, and unquotes straight back into a formula —
	// a raw-substring assertion misses exactly that cell, which is the most
	// dangerous of the four.
	records, err := csv.NewReader(strings.NewReader(rec.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records) != len(dangerous)+1 {
		t.Fatalf("records = %d, want %d", len(records), len(dangerous)+1)
	}

	for _, record := range records[1:] {
		cell := record[0]
		if cell == "" {
			t.Fatal("a cell was dropped instead of neutralized")
		}
		switch cell[0] {
		case '=', '+', '-', '@', '\t', '\r':
			t.Errorf("cell %q still opens with a formula trigger", cell)
		}
	}

	// The value itself must survive — neutralized, not dropped.
	if !strings.Contains(rec.Body.String(), "calc") {
		t.Error("the cell contents were lost rather than neutralized")
	}
}

// TestWriteCSV_LeavesOrdinaryValuesAlone guards against over-escaping.
func TestWriteCSV_LeavesOrdinaryValuesAlone(t *testing.T) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest("GET", "/", nil), rec)
	if err := writeCSV(c, &QueryResult{
		Columns: []string{"name"},
		Rows:    []map[string]interface{}{{"name": "alice"}, {"name": "13812345678"}},
	}); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "\nalice\n") {
		t.Errorf("an ordinary value was altered; body = %q", body)
	}
	if strings.Contains(body, "'alice") {
		t.Error("an ordinary value was prefixed")
	}
}

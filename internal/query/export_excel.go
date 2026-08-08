package query

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/xuri/excelize/v2"
)

// ---------------------------------------------------------------------------
// Excel Export — Audit Logs
// ---------------------------------------------------------------------------

// StreamExportAuditLogsExcel streams audit logs as Excel (.xlsx) to the given writer.
// Uses excelize StreamWriter for low-memory streaming.
// columns selects specific columns; nil means all columns.
// Returns total rows written.
func (s *ExportService) StreamExportAuditLogsExcel(ctx context.Context, w io.Writer, actor ExportActor, filters AuditExportFilters, columns map[string]int) (int64, error) {
	f := excelize.NewFile()
	defer func() {
		_ = f.Close()
	}()

	sheetName := "Sheet1"

	// Determine which columns to export
	colNames := auditColumnNames
	colIndices := auditColumnIndices(columns, len(auditColumnNames))

	// Freeze first row
	_ = f.SetPanes(sheetName, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
	})

	// StreamWriter for row-by-row writing
	sw, err := f.NewStreamWriter(sheetName)
	if err != nil {
		return 0, fmt.Errorf("创建 Excel StreamWriter 失败: %w", err)
	}

	// Write header row with bold style
	headerStyleID, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	headerRow := make([]interface{}, len(colIndices))
	for i, colIdx := range colIndices {
		headerRow[i] = colNames[colIdx]
	}
	// Apply style to header cells
	for i := range colIndices {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellStyle(sheetName, cell, cell, headerStyleID)
	}
	if err := sw.SetRow("A1", headerRow); err != nil {
		return 0, fmt.Errorf("写入 Excel 表头失败: %w", err)
	}

	rows, err := s.fetchAuditExportRows(ctx, actor, filters)
	if err != nil {
		return 0, err
	}

	var written int64
	for _, a := range rows {
		select {
		case <-ctx.Done():
			_ = sw.Flush()
			return written, ctx.Err()
		default:
		}

		row := buildAuditExcelRow(&a, colIndices)
		rowNum := written + 2 // +1 for header, +1 for 1-based
		cell, _ := excelize.CoordinatesToCellName(1, int(rowNum))
		if err := sw.SetRow(cell, row); err != nil {
			continue
		}
		written++
	}

	if err := sw.Flush(); err != nil {
		return written, fmt.Errorf("flush Excel StreamWriter 失败: %w", err)
	}

	// Set column widths
	for i := range colIndices {
		colLetter, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheetName, colLetter, colLetter, 15)
	}

	// Write to output writer
	if _, err := f.WriteTo(w); err != nil {
		return written, fmt.Errorf("写入 Excel 文件失败: %w", err)
	}

	return written, nil
}

// ---------------------------------------------------------------------------
// Excel Export — Tickets
// ---------------------------------------------------------------------------

// StreamExportTicketsExcel streams tickets as Excel (.xlsx) to the given writer.
// columns selects specific columns; nil means all columns.
// Returns total rows written.
func (s *ExportService) StreamExportTicketsExcel(ctx context.Context, w io.Writer, actor ExportActor, filters TicketExportFilters, columns map[string]int) (int64, error) {
	f := excelize.NewFile()
	defer func() {
		_ = f.Close()
	}()

	sheetName := "Sheet1"

	colNames := ticketColumnNames
	colIndices := ticketColumnIndices(columns, len(ticketColumnNames))

	// Freeze first row
	_ = f.SetPanes(sheetName, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
	})

	sw, err := f.NewStreamWriter(sheetName)
	if err != nil {
		return 0, fmt.Errorf("创建 Excel StreamWriter 失败: %w", err)
	}

	// Write header row with bold style
	headerStyleID, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	headerRow := make([]interface{}, len(colIndices))
	for i, colIdx := range colIndices {
		headerRow[i] = colNames[colIdx]
	}
	for i := range colIndices {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellStyle(sheetName, cell, cell, headerStyleID)
	}
	if err := sw.SetRow("A1", headerRow); err != nil {
		return 0, fmt.Errorf("写入 Excel 表头失败: %w", err)
	}

	// Query data
	rows, err := s.fetchTicketExportRows(ctx, actor, filters)
	if err != nil {
		return 0, err
	}

	var written int64
	for _, t := range rows {
		select {
		case <-ctx.Done():
			_ = sw.Flush()
			return written, ctx.Err()
		default:
		}

		row := buildTicketExcelRow(&t, colIndices)
		rowNum := written + 2
		cell, _ := excelize.CoordinatesToCellName(1, int(rowNum))
		if err := sw.SetRow(cell, row); err != nil {
			continue
		}
		written++
	}

	if err := sw.Flush(); err != nil {
		return written, fmt.Errorf("flush Excel StreamWriter 失败: %w", err)
	}

	// Set column widths
	for i := range colIndices {
		colLetter, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheetName, colLetter, colLetter, 15)
	}

	if _, err := f.WriteTo(w); err != nil {
		return written, fmt.Errorf("写入 Excel 文件失败: %w", err)
	}

	return written, nil
}

// ---------------------------------------------------------------------------
// Row builders — convert scanned struct to selective cell values
// ---------------------------------------------------------------------------

// buildAuditExcelRow builds a row of cell values for audit log export.
// All string values are escaped via escapeExcelFormula to prevent formula injection.
func buildAuditExcelRow(a *auditCSVRow, colIndices []int) []interface{} {
	allValues := []interface{}{
		fmt.Sprintf("%d", a.ID),
		formatTimeValue(a.CreatedAt),
		a.Username,
		a.Action,
		fmt.Sprintf("%d", a.DatasourceID),
		a.Database,
		escapeExcelFormula(a.SQLContent),
		escapeExcelFormula(a.SQLSummary),
		fmt.Sprintf("%d", a.ResultRows),
		fmt.Sprintf("%d", a.AffectedRows),
		fmt.Sprintf("%d", a.ExecutionTimeMs),
		a.ErrorMessage,
		a.DesensitizedFields,
		a.IPAddress,
		a.AIReviewResult,
		fmt.Sprintf("%d", a.TicketID),
	}

	if colIndices == nil {
		return allValues
	}

	row := make([]interface{}, len(colIndices))
	for i, idx := range colIndices {
		row[i] = allValues[idx]
	}
	return row
}

// buildTicketExcelRow builds a row of cell values for ticket export.
func buildTicketExcelRow(t *ticketCSVRow, colIndices []int) []interface{} {
	allValues := []interface{}{
		fmt.Sprintf("%d", t.ID),
		t.SubmitterName,
		fmt.Sprintf("%d", t.SubmitterID),
		fmt.Sprintf("%d", t.DatasourceID),
		t.Database,
		escapeExcelFormula(t.SQLContent),
		escapeExcelFormula(t.SQLSummary),
		t.DBType,
		t.ChangeReason,
		t.Status,
		t.RiskLevel,
		t.ReviewerName,
		t.ReviewComment,
		formatOptionalTime(t.ScheduledAt),
		formatOptionalTime(t.ExecutedAt),
		formatTimeValue(t.CreatedAt),
		formatTimeValue(t.UpdatedAt),
	}

	if colIndices == nil {
		return allValues
	}

	row := make([]interface{}, len(colIndices))
	for i, idx := range colIndices {
		row[i] = allValues[idx]
	}
	return row
}

// ---------------------------------------------------------------------------
// Column index helpers
// ---------------------------------------------------------------------------

// auditColumnIndices returns the column indices to export.
// If columns is nil (all columns), returns 0..len-1.
func auditColumnIndices(columns map[string]int, total int) []int {
	if columns == nil {
		indices := make([]int, total)
		for i := range indices {
			indices[i] = i
		}
		return indices
	}
	return sortColumnIndices(columns, total)
}

// ticketColumnIndices returns the column indices to export.
func ticketColumnIndices(columns map[string]int, total int) []int {
	if columns == nil {
		indices := make([]int, total)
		for i := range indices {
			indices[i] = i
		}
		return indices
	}
	return sortColumnIndices(columns, total)
}

// sortColumnIndices extracts and sorts the indices from the column map,
// maintaining the original column order.
func sortColumnIndices(columns map[string]int, total int) []int {
	present := make([]bool, total)
	for _, idx := range columns {
		if idx >= 0 && idx < total {
			present[idx] = true
		}
	}
	var result []int
	for i, p := range present {
		if p {
			result = append(result, i)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Excel formula injection protection
// ---------------------------------------------------------------------------

// escapeExcelFormula escapes values that could be interpreted as Excel formulas.
// Cells starting with =, +, -, @, \t, \r are prefixed with an apostrophe.
func escapeExcelFormula(s string) string {
	if len(s) == 0 {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// ---------------------------------------------------------------------------
// Time formatting helpers
// ---------------------------------------------------------------------------

func formatTimeValue(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

package service

import (
	"context"
	"database/sql"
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
func (s *ExportService) StreamExportAuditLogsExcel(ctx context.Context, w io.Writer, username string, filters AuditExportFilters, columns map[string]int) (int64, error) {
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

	// Query data
	filterClauses := buildAuditFilterClauses(filters)
	whereClause, args := BuildWhereClause(filterClauses)

	querySQL := fmt.Sprintf(
		`SELECT a.id, a.user_id, a.action, a.datasource_id, a.database, a.sql_content, a.sql_summary,
		        a.result_rows, a.affected_rows, a.execution_time_ms, a.error_message,
		        a.desensitized_fields, a.ip_address, a.ai_review_result, a.ticket_id, a.created_at,
		        COALESCE(u.username, '') AS username
		 FROM audit_logs a
		 LEFT JOIN users u ON a.user_id = u.id
		 %s ORDER BY a.created_at DESC LIMIT ?`,
		whereClause,
	)
	queryArgs := append(args, ExportMaxRows)

	rows, err := s.database.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return 0, fmt.Errorf("查询审计日志失败: %w", err)
	}
	defer rows.Close()

	var written int64
	for rows.Next() {
		select {
		case <-ctx.Done():
			_ = sw.Flush()
			return written, ctx.Err()
		default:
		}

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

		row := buildAuditExcelRow(&a, colIndices)
		rowNum := written + 2 // +1 for header, +1 for 1-based
		cell, _ := excelize.CoordinatesToCellName(1, int(rowNum))
		if err := sw.SetRow(cell, row); err != nil {
			continue
		}
		written++
	}

	if err := rows.Err(); err != nil {
		_ = sw.Flush()
		return written, fmt.Errorf("iterate audit rows: %w", err)
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
func (s *ExportService) StreamExportTicketsExcel(ctx context.Context, w io.Writer, username string, filters TicketExportFilters, columns map[string]int) (int64, error) {
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
	filterClauses := buildTicketFilterClauses(filters)
	whereClause, args := BuildWhereClause(filterClauses)

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
		 %s ORDER BY t.created_at DESC LIMIT ?`,
		whereClause,
	)
	queryArgs := append(args, ExportMaxRows)

	rows, err := s.database.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return 0, fmt.Errorf("查询工单失败: %w", err)
	}
	defer rows.Close()

	var written int64
	for rows.Next() {
		select {
		case <-ctx.Done():
			_ = sw.Flush()
			return written, ctx.Err()
		default:
		}

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

		row := buildTicketExcelRow(&t, colIndices)
		rowNum := written + 2
		cell, _ := excelize.CoordinatesToCellName(1, int(rowNum))
		if err := sw.SetRow(cell, row); err != nil {
			continue
		}
		written++
	}

	if err := rows.Err(); err != nil {
		_ = sw.Flush()
		return written, fmt.Errorf("iterate ticket rows: %w", err)
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
		formatNullTime(t.ScheduledAt),
		formatNullTime(t.ExecutedAt),
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

func formatNullTime(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format("2006-01-02 15:04:05")
}

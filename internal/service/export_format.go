package service

import "fmt"

// ExportFormat defines the output format for an export.
type ExportFormat string

const (
	// ExportFormatCSV exports data as CSV with BOM header.
	ExportFormatCSV ExportFormat = "csv"
	// ExportFormatExcel exports data as Excel (.xlsx).
	ExportFormatExcel ExportFormat = "xlsx"
)

// ValidExportFormats is the set of accepted format values.
var ValidExportFormats = map[ExportFormat]bool{
	ExportFormatCSV:   true,
	ExportFormatExcel: true,
}

// ParseExportFormat parses a format string, returning CSV as default.
func ParseExportFormat(s string) ExportFormat {
	f := ExportFormat(s)
	if ValidExportFormats[f] {
		return f
	}
	return ExportFormatCSV
}

// auditColumnNames defines the allowed column names for audit log export.
// The index maps to the column's position in the SELECT query result.
var auditColumnNames = []string{
	"ID", "时间", "用户", "操作", "数据源ID", "数据库",
	"SQL内容", "SQL摘要", "返回行数", "影响行数", "耗时(ms)",
	"错误信息", "脱敏字段", "IP地址", "AI评审", "工单ID",
}

// auditColumnSet is the whitelist set for quick lookup.
var auditColumnSet map[string]int

func init() {
	auditColumnSet = make(map[string]int, len(auditColumnNames))
	for i, name := range auditColumnNames {
		auditColumnSet[name] = i
	}
}

// ticketColumnNames defines the allowed column names for ticket export.
var ticketColumnNames = []string{
	"ID", "提交人", "提交人ID", "数据源ID", "数据库",
	"SQL内容", "SQL摘要", "数据库类型", "变更原因",
	"状态", "风险等级", "审批人", "审批意见",
	"定时执行时间", "实际执行时间", "创建时间", "更新时间",
}

// ticketColumnSet is the whitelist set for quick lookup.
var ticketColumnSet map[string]int

func init() {
	ticketColumnSet = make(map[string]int, len(ticketColumnNames))
	for i, name := range ticketColumnNames {
		ticketColumnSet[name] = i
	}
}

// ValidateExportColumns validates requested columns against the whitelist.
// Returns a map of column_name -> column_index (for selective export),
// or an error if any column is not in the whitelist.
// If columns is nil/empty, returns nil (meaning "all columns").
func ValidateExportColumns(columns []string, exportType ExportType) (map[string]int, error) {
	if len(columns) == 0 {
		return nil, nil // all columns
	}

	whitelist := auditColumnSet
	switch exportType {
	case ExportTypeTicket:
		whitelist = ticketColumnSet
	}

	result := make(map[string]int, len(columns))
	for _, col := range columns {
		idx, ok := whitelist[col]
		if !ok {
			return nil, fmt.Errorf("不允许的导出列名: %s", col)
		}
		result[col] = idx
	}
	return result, nil
}

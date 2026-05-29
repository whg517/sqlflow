package service

import (
	"regexp"
	"strings"
)

// SQLAnalysis holds the result of parsing a SQL statement.
type SQLAnalysis struct {
	SQLType        string   `json:"sql_type"`         // SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, DROP, TRUNCATE, OTHER
	Operations     []string `json:"operations"`       // e.g. ["SELECT"], ["INSERT", "UPDATE"]
	AffectedTables []string `json:"affected_tables"`  // table names extracted
	IsDDL          bool     `json:"is_ddl"`           // DDL statement
	IsDML          bool     `json:"is_dml"`           // DML statement
	IsRead         bool     `json:"is_read"`          // read-only query
}

// SQLAnalyzer parses SQL statements and extracts metadata.
type SQLAnalyzer struct{}

// NewSQLAnalyzer creates a new SQLAnalyzer.
func NewSQLAnalyzer() *SQLAnalyzer {
	return &SQLAnalyzer{}
}

// sqlTypeRegex matches the leading SQL keyword.
var (
	reSingleLineComment = regexp.MustCompile(`--[^
]*`)
	reMultiLineComment  = regexp.MustCompile(`/\*.*?\*/`)
	reWhitespace        = regexp.MustCompile(`\s+`)
)

var sqlTypeRegex = regexp.MustCompile(`(?i)^\s*(SELECT|INSERT\s+INTO|UPDATE|DELETE\s+FROM|DELETE|CREATE\s+(?:TEMPORARY\s+)?(?:UNIQUE\s+)?(?:OR\s+REPLACE\s+)?(?:EXTERNAL\s+)?TABLE|CREATE\s+(?:OR\s+REPLACE\s+)?(?:ALGORITHM\s*=\s*\w+\s*)?(?:DEFINER\s*=\s*\S+\s*)?(?:SQL\s+SECURITY\s+\w+\s*)?VIEW|CREATE\s+(?:UNIQUE\s+)?(?:FULLTEXT\s+)?(?:SPATIAL\s+)?INDEX|CREATE\s+(?:OR\s+REPLACE\s+)?(?:AGGREGATE\s+)?FUNCTION|CREATE\s+DATABASE|CREATE\s+SCHEMA|ALTER\s+TABLE|DROP\s+TABLE|DROP\s+DATABASE|DROP\s+INDEX|DROP\s+VIEW|TRUNCATE\s+TABLE?\s*|GRANT|REVOKE|WITH\s+\w+\s+AS\s*\(|MERGE\s+INTO|REPLACE\s+INTO)`)
var firstKeywordRegex = regexp.MustCompile(`(?i)^\s*(SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|DROP|TRUNCATE|GRANT|REVOKE|WITH|MERGE|REPLACE)`)

// Analyze parses a SQL statement and returns metadata.
func (a *SQLAnalyzer) Analyze(sql string) *SQLAnalysis {
	result := &SQLAnalysis{
		SQLType:        "OTHER",
		Operations:     []string{},
		AffectedTables: []string{},
	}

	if strings.TrimSpace(sql) == "" {
		return result
	}

	// Normalize whitespace
	normalized := normalizeSQL(sql)

	// Extract first keyword
	keyword := extractFirstKeyword(normalized)
	if keyword == "" {
		return result
	}

	upperKeyword := strings.ToUpper(keyword)
	result.Operations = []string{upperKeyword}

	switch upperKeyword {
	case "SELECT":
		result.SQLType = "SELECT"
		result.IsDML = true
		result.IsRead = true
		result.AffectedTables = extractFromTables(normalized)

	case "INSERT":
		result.SQLType = "INSERT"
		result.IsDML = true
		result.AffectedTables = extractInsertTables(normalized)

	case "UPDATE":
		result.SQLType = "UPDATE"
		result.IsDML = true
		result.AffectedTables = extractUpdateTables(normalized)

	case "DELETE":
		result.SQLType = "DELETE"
		result.IsDML = true
		result.AffectedTables = extractDeleteTables(normalized)

	case "CREATE":
		result.SQLType = "CREATE"
		result.IsDDL = true
		result.AffectedTables = extractCreateTables(normalized)

	case "ALTER":
		result.SQLType = "ALTER"
		result.IsDDL = true
		result.AffectedTables = extractAlterTables(normalized)

	case "DROP":
		result.SQLType = "DROP"
		result.IsDDL = true
		result.AffectedTables = extractDropTables(normalized)

	case "TRUNCATE":
		result.SQLType = "TRUNCATE"
		result.IsDDL = true
		result.AffectedTables = extractTruncateTables(normalized)

	case "GRANT", "REVOKE":
		result.SQLType = upperKeyword
		result.IsDDL = true

	case "WITH":
		// CTE — could be SELECT or DML inside
		result.SQLType = detectCTEType(normalized)
		result.AffectedTables = extractCTETables(normalized)
		if result.SQLType == "SELECT" {
			result.IsRead = true
			result.IsDML = true
		} else {
			result.IsDML = true
		}

	case "MERGE":
		result.SQLType = "MERGE"
		result.IsDML = true
		result.AffectedTables = extractMergeTables(normalized)

	case "REPLACE":
		result.SQLType = "REPLACE"
		result.IsDML = true
		result.AffectedTables = extractReplaceTables(normalized)

	default:
		result.SQLType = "OTHER"
	}

	// Deduplicate tables
	result.AffectedTables = dedup(result.AffectedTables)

	return result
}

// normalizeSQL collapses whitespace and removes comments.
func normalizeSQL(sql string) string {
	// Remove single-line comments
	s := regexp.MustCompile(`--[^\n]*`).ReplaceAllString(sql, " ")
	// Remove multi-line comments
	s = regexp.MustCompile(`/\*.*?\*/`).ReplaceAllString(s, " ")
	// Collapse whitespace
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// extractFirstKeyword returns the first SQL keyword.
func extractFirstKeyword(sql string) string {
	m := firstKeywordRegex.FindStringSubmatch(sql)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

// Table extraction helpers using regex (not a full parser, but covers common cases).

var (
	// tableChar matches word chars, dots, backticks, quotes, and hyphens for table names.
	tableChar       = `[\w\.\` + "`" + `"'\-]+`
	fromRegex       = regexp.MustCompile(`(?i)\bFROM\s+(` + tableChar + `)`)
	joinRegex       = regexp.MustCompile(`(?i)\bJOIN\s+(` + tableChar + `)`)
	insertIntoRegex = regexp.MustCompile(`(?i)\bINTO\s+(` + tableChar + `)`)
	updateTableRegex = regexp.MustCompile(`(?i)\bUPDATE\s+(` + tableChar + `)`)
	deleteFromRegex = regexp.MustCompile(`(?i)\bFROM\s+(` + tableChar + `)`)
	createTableRegex = regexp.MustCompile(`(?i)\bTABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(` + tableChar + `)`)
	alterTableRegex = regexp.MustCompile(`(?i)\bTABLE\s+(` + tableChar + `)`)
	dropTableRegex  = regexp.MustCompile(`(?i)\bTABLE\s+(?:IF\s+EXISTS\s+)?(` + tableChar + `)`)
	truncateRegex   = regexp.MustCompile(`(?i)\bTABLE\s*\??\s*(` + tableChar + `)`)
	mergeIntoRegex  = regexp.MustCompile(`(?i)\bMERGE\s+INTO\s+(` + tableChar + `)`)
	mergeUsingRegex = regexp.MustCompile(`(?i)\bUSING\s+(` + tableChar + `)`)
)

func cleanTableName(name string) string {
	// Remove backticks, quotes, and schema prefix (keep only table name)
	name = strings.Trim(name, "`\"'")
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

func extractFromTables(sql string) []string {
	var tables []string

	// FROM clause
	froms := fromRegex.FindAllStringSubmatch(sql, -1)
	for _, m := range froms {
		tables = append(tables, cleanTableName(m[1]))
	}

	// JOIN clauses
	joins := joinRegex.FindAllStringSubmatch(sql, -1)
	for _, m := range joins {
		tables = append(tables, cleanTableName(m[1]))
	}

	return tables
}

func extractInsertTables(sql string) []string {
	m := insertIntoRegex.FindStringSubmatch(sql)
	if len(m) > 1 {
		return []string{cleanTableName(m[1])}
	}
	return nil
}

func extractUpdateTables(sql string) []string {
	m := updateTableRegex.FindStringSubmatch(sql)
	if len(m) > 1 {
		return []string{cleanTableName(m[1])}
	}
	return nil
}

func extractDeleteTables(sql string) []string {
	m := deleteFromRegex.FindStringSubmatch(sql)
	if len(m) > 1 {
		return []string{cleanTableName(m[1])}
	}
	return nil
}

func extractCreateTables(sql string) []string {
	m := createTableRegex.FindStringSubmatch(sql)
	if len(m) > 1 {
		return []string{cleanTableName(m[1])}
	}
	return nil
}

func extractAlterTables(sql string) []string {
	m := alterTableRegex.FindStringSubmatch(sql)
	if len(m) > 1 {
		return []string{cleanTableName(m[1])}
	}
	return nil
}

func extractDropTables(sql string) []string {
	m := dropTableRegex.FindStringSubmatch(sql)
	if len(m) > 1 {
		return []string{cleanTableName(m[1])}
	}
	return nil
}

func extractTruncateTables(sql string) []string {
	m := truncateRegex.FindStringSubmatch(sql)
	if len(m) > 1 {
		return []string{cleanTableName(m[1])}
	}
	// TRUNCATE without TABLE keyword: "TRUNCATE users"
	rest := regexp.MustCompile(`(?i)TRUNCATE\s+TABLE?\s*` + "(" + tableChar + ")").FindStringSubmatch(sql)
	if len(rest) > 1 {
		return []string{cleanTableName(rest[1])}
	}
	return nil
}

func extractMergeTables(sql string) []string {
	var tables []string
	if m := mergeIntoRegex.FindStringSubmatch(sql); len(m) > 1 {
		tables = append(tables, cleanTableName(m[1]))
	}
	if m := mergeUsingRegex.FindStringSubmatch(sql); len(m) > 1 {
		tables = append(tables, cleanTableName(m[1]))
	}
	return tables
}

func extractReplaceTables(sql string) []string {
	// REPLACE INTO table ...
	m := insertIntoRegex.FindStringSubmatch(sql)
	if len(m) > 1 {
		return []string{cleanTableName(m[1])}
	}
	return nil
}

func extractCTETables(sql string) []string {
	// Extract tables from the main query after CTE definition
	// Simple approach: extract all FROM/JOIN references
	var tables []string
	froms := fromRegex.FindAllStringSubmatch(sql, -1)
	for _, m := range froms {
		tables = append(tables, cleanTableName(m[1]))
	}
	joins := joinRegex.FindAllStringSubmatch(sql, -1)
	for _, m := range joins {
		tables = append(tables, cleanTableName(m[1]))
	}
	return tables
}

// detectCTEType determines the effective statement type inside a CTE.
func detectCTEType(sql string) string {
	upper := strings.ToUpper(sql)
	// Check if there's INSERT/UPDATE/DELETE after the CTE
	if strings.Contains(upper, " INSERT ") || strings.Contains(upper, " INSERT\n") {
		return "INSERT"
	}
	if strings.Contains(upper, " UPDATE ") || strings.Contains(upper, " UPDATE\n") {
		return "UPDATE"
	}
	if strings.Contains(upper, " DELETE ") || strings.Contains(upper, " DELETE\n") {
		return "DELETE"
	}
	return "SELECT"
}

// dedup removes duplicate strings while preserving order.
func dedup(ss []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != "" && !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

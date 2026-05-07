package sqlparser

import (
	"fmt"
	"strings"

	"github.com/pingcap/parser"
	"github.com/pingcap/parser/ast"
	"github.com/pingcap/parser/model"
)

// MySQLParseResult holds the parsed result of a MySQL SQL statement.
type MySQLParseResult struct {
	Operation     OperationType
	Tables        []string
	HasWhere      bool
	IsDropDatabase bool
	IsDropTable   bool
	IsTruncate    bool
	HasLimit      bool
	LimitCount    int64 // 0 means no limit or unknown
}

// mysqlParser is a lazily-initialized shared parser instance.
var mysqlParser *parser.Parser

func getMySQLParser() *parser.Parser {
	if mysqlParser == nil {
		mysqlParser = parser.New()
	}
	return mysqlParser
}

// ParseMySQL parses a MySQL SQL statement using AST analysis.
func ParseMySQL(sql string) (*MySQLParseResult, error) {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return nil, fmt.Errorf("empty SQL statement")
	}

	// Handle multiple statements: only parse the first one
	if idx := strings.IndexByte(sql, ';'); idx >= 0 {
		sql = sql[:idx]
		sql = strings.TrimSpace(sql)
		if sql == "" {
			return nil, fmt.Errorf("empty SQL statement")
		}
	}

	// Remove trailing semicolons
	sql = strings.TrimRight(sql, ";")

	stmtNodes, _, err := getMySQLParser().Parse(sql, "", "")
	if err != nil {
		return nil, fmt.Errorf("SQL syntax error: %w", err)
	}
	if len(stmtNodes) == 0 {
		return nil, fmt.Errorf("no valid SQL statement found")
	}

	result := &MySQLParseResult{}
	// Process first statement only
	result.extractFromNode(stmtNodes[0])
	return result, nil
}

// extractFromNode dispatches based on statement node type.
func (r *MySQLParseResult) extractFromNode(node ast.StmtNode) {
	switch stmt := node.(type) {
	case *ast.SelectStmt:
		r.Operation = OpSelect
		if stmt.From != nil {
			r.extractTablesFromResultSet(stmt.From.TableRefs)
		}
		r.HasWhere = stmt.Where != nil
		if stmt.Where != nil {
			r.extractTablesFromExpr(stmt.Where)
		}
		if stmt.Limit != nil {
			r.HasLimit = true
			r.LimitCount = extractLimitCount(stmt.Limit)
		}
	case *ast.InsertStmt:
		r.Operation = OpDML
		if stmt.Table != nil {
			r.extractTablesFromResultSet(stmt.Table.TableRefs)
		}
	case *ast.UpdateStmt:
		r.Operation = OpUpdate
		if stmt.TableRefs != nil {
			r.extractTablesFromResultSet(stmt.TableRefs.TableRefs)
		}
		r.HasWhere = stmt.Where != nil
		if stmt.Where != nil {
			r.extractTablesFromExpr(stmt.Where)
		}
		if stmt.Limit != nil {
			r.HasLimit = true
		}
	case *ast.DeleteStmt:
		r.Operation = OpDelete
		if stmt.TableRefs != nil {
			r.extractTablesFromResultSet(stmt.TableRefs.TableRefs)
		}
		r.HasWhere = stmt.Where != nil
		if stmt.Where != nil {
			r.extractTablesFromExpr(stmt.Where)
		}
		if stmt.Limit != nil {
			r.HasLimit = true
		}
	case *ast.DropTableStmt:
		r.Operation = OpDDL
		r.IsDropTable = true
		for _, t := range stmt.Tables {
			r.Tables = append(r.Tables, t.Name.String())
		}
	case *ast.DropDatabaseStmt:
		r.Operation = OpDDL
		r.IsDropDatabase = true
		r.Tables = append(r.Tables, stmt.Name)
	case *ast.TruncateTableStmt:
		r.Operation = OpDDL
		r.IsTruncate = true
		if stmt.Table != nil {
			r.Tables = append(r.Tables, stmt.Table.Name.String())
		}
	case *ast.AlterTableStmt:
		r.Operation = OpDDL
		if stmt.Table != nil {
			r.Tables = append(r.Tables, stmt.Table.Name.String())
		}
	case *ast.CreateTableStmt:
		r.Operation = OpDDL
		if stmt.Table != nil {
			r.Tables = append(r.Tables, stmt.Table.Name.String())
		}
	default:
		// Fallback for unrecognized statements
		r.Operation = OpDDL
	}
}

// extractTablesFromExpr walks an expression tree looking for subqueries
// and extracts tables from them.
func (r *MySQLParseResult) extractTablesFromExpr(expr ast.ExprNode) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.SubqueryExpr:
		if e.Query != nil {
			r.extractTablesFromResultSet(e.Query)
		}
	case *ast.BinaryOperationExpr:
		r.extractTablesFromExpr(e.L)
		r.extractTablesFromExpr(e.R)
	case *ast.UnaryOperationExpr:
		r.extractTablesFromExpr(e.V)
	case *ast.IsNullExpr:
		r.extractTablesFromExpr(e.Expr)
	case *ast.IsTruthExpr:
		r.extractTablesFromExpr(e.Expr)
	case *ast.PatternInExpr:
		r.extractTablesFromExpr(e.Expr)
		for _, item := range e.List {
			r.extractTablesFromExpr(item)
		}
		if e.Sel != nil {
			r.extractTablesFromResultSet(e.Sel)
		}
	case *ast.CompareSubqueryExpr:
		r.extractTablesFromExpr(e.L)
		r.extractTablesFromResultSet(e.R)
	case *ast.ExistsSubqueryExpr:
		r.extractTablesFromResultSet(e.Sel)
	case *ast.ParenthesesExpr:
		r.extractTablesFromExpr(e.Expr)
	case *ast.ColumnNameExpr:
		// Column reference — no tables to extract here
	}
}

// extractTablesFromResultSet walks a Join tree and collects table names.
func (r *MySQLParseResult) extractTablesFromResultSet(node ast.ResultSetNode) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *ast.TableSource:
		if tn, ok := n.Source.(*ast.TableName); ok {
			name := tn.Name.String()
			if name != "" && !isSQLKeyword(strings.ToLower(name)) {
				r.Tables = appendIfAbsent(r.Tables, name)
			}
		} else {
			// Subquery or other source — recurse
			r.extractTablesFromResultSet(n.Source)
		}
	case *ast.Join:
		r.extractTablesFromResultSet(n.Left)
		r.extractTablesFromResultSet(n.Right)
	case *ast.TableName:
		name := n.Name.String()
		if name != "" && !isSQLKeyword(strings.ToLower(name)) {
			r.Tables = appendIfAbsent(r.Tables, name)
		}
	case *ast.SelectStmt:
		// Subquery in FROM clause
		if n.From != nil {
			r.extractTablesFromResultSet(n.From.TableRefs)
		}
	case *ast.UnionStmt:
		for _, sel := range n.SelectList.Selects {
			r.extractTablesFromResultSet(sel)
		}
	}
}

// extractLimitCount extracts the LIMIT count value from a Limit node.
func extractLimitCount(limit *ast.Limit) int64 {
	if limit == nil || limit.Count == nil {
		return 0
	}
	if ve, ok := limit.Count.(*simpleValueExpr); ok {
		if val, ok := ve.GetValue().(int64); ok {
			return val
		}
		if val, ok := ve.GetValue().(float64); ok {
			return int64(val)
		}
		if val, ok := ve.GetValue().(string); ok {
			var n int64
			fmt.Sscanf(val, "%d", &n)
			return n
		}
	}
	// For ValueExpr from driver init, try via type assertion to ValueExpr interface
	if ve, ok := limit.Count.(ast.ValueExpr); ok {
		switch v := ve.GetValue().(type) {
		case int64:
			return v
		case float64:
			return int64(v)
		case string:
			var n int64
			fmt.Sscanf(v, "%d", &n)
			return n
		}
	}
	return 0
}

// appendIfAbsent adds a string to a slice only if not already present.
func appendIfAbsent(slice []string, s string) []string {
	for _, v := range slice {
		if strings.EqualFold(v, s) {
			return slice
		}
	}
	return append(slice, s)
}

// Ensure model.CIStr is available (used by AST types).
var _ model.CIStr

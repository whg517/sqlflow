package sqlparser

import (
	"fmt"
	"strings"

	pgquery "github.com/pganalyze/pg_query_go/v6"
)

// PostgreSQLParseResult holds the parsed result of a PostgreSQL SQL statement.
type PostgreSQLParseResult struct {
	Operation      OperationType
	Tables         []string
	HasWhere       bool
	IsDropDatabase bool
	IsDropTable    bool
	IsTruncate     bool
	HasLimit       bool
	HasReturning   bool // PG-specific: RETURNING clause
}

// ParsePostgreSQL parses a PostgreSQL SQL statement using the libpg_query AST.
// Returns a structured result with operation type, tables, and PG-specific flags.
func ParsePostgreSQL(sql string) (*PostgreSQLParseResult, error) {
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

	result := &PostgreSQLParseResult{}

	parseResult, err := pgquery.Parse(sql)
	if err != nil {
		return nil, fmt.Errorf("PostgreSQL parse error: %w", err)
	}

	if len(parseResult.Stmts) == 0 {
		return nil, fmt.Errorf("no valid SQL statement found")
	}

	stmt := parseResult.Stmts[0]
	if stmt.Stmt == nil {
		return nil, fmt.Errorf("empty statement node")
	}

	result.extractFromPGNode(stmt.Stmt)

	return result, nil
}

// extractFromPGNode dispatches based on the PG AST node type.
func (r *PostgreSQLParseResult) extractFromPGNode(node *pgquery.Node) {
	switch n := node.Node.(type) {
	case *pgquery.Node_SelectStmt:
		r.extractFromSelect(n.SelectStmt)
	case *pgquery.Node_InsertStmt:
		r.extractFromInsert(n.InsertStmt)
	case *pgquery.Node_UpdateStmt:
		r.extractFromUpdate(n.UpdateStmt)
	case *pgquery.Node_DeleteStmt:
		r.extractFromDelete(n.DeleteStmt)
	case *pgquery.Node_DropStmt:
		r.extractFromDrop(n.DropStmt)
	case *pgquery.Node_TruncateStmt:
		r.extractFromTruncate(n.TruncateStmt)
	case *pgquery.Node_AlterTableStmt:
		r.extractFromAlterTable(n.AlterTableStmt)
	case *pgquery.Node_CreateStmt:
		r.extractFromCreate(n.CreateStmt)
	case *pgquery.Node_DropdbStmt:
		r.extractFromDropDB(n.DropdbStmt)
	case *pgquery.Node_IndexStmt:
		r.extractFromIndex(n.IndexStmt)
	case *pgquery.Node_ViewStmt:
		r.extractFromView(n.ViewStmt)
	case *pgquery.Node_CreateTableAsStmt:
		r.extractFromCreateTableAs(n.CreateTableAsStmt)
	case *pgquery.Node_GrantStmt:
		r.Operation = OpDDL
	case *pgquery.Node_TransactionStmt:
		r.Operation = OpDDL
	default:
		// Unknown statement type — classify as DDL for safety
		r.Operation = OpDDL
	}
}

func (r *PostgreSQLParseResult) extractFromSelect(stmt *pgquery.SelectStmt) {
	r.Operation = OpSelect

	// Extract target relation
	if stmt.FromClause != nil {
		for _, item := range stmt.FromClause {
			if item == nil {
				continue
			}
			r.extractTablesFromRangeVar(item)
		}
	}

	// Handle CTEs (WITH ... SELECT ...)
	if stmt.WithClause != nil && stmt.WithClause.Ctes != nil {
		for _, cte := range stmt.WithClause.Ctes {
			if cte == nil {
				continue
			}
			if cteNode, ok := cte.Node.(*pgquery.Node_CommonTableExpr); ok {
				if cteNode.CommonTableExpr.Ctequery != nil {
					r.extractFromPGNode(cteNode.CommonTableExpr.Ctequery)
				}
			}
		}
	}

	// Check for WHERE clause
	r.HasWhere = stmt.WhereClause != nil

	// Check for LIMIT
	r.HasLimit = stmt.LimitCount != nil || stmt.LimitOption == pgquery.LimitOption_LIMIT_OPTION_COUNT

	// Handle subqueries in FROM
	if stmt.FromClause != nil {
		for _, item := range stmt.FromClause {
			if item == nil {
				continue
			}
			if sub, ok := item.Node.(*pgquery.Node_SubLink); ok {
				_ = sub // subquery tables already handled
			}
			if join, ok := item.Node.(*pgquery.Node_JoinExpr); ok {
				r.extractTablesFromJoin(join.JoinExpr)
			}
		}
	}
}

func (r *PostgreSQLParseResult) extractFromInsert(stmt *pgquery.InsertStmt) {
	r.Operation = OpDML
	if stmt.Relation != nil {
		r.addTable(stmt.Relation.Relname)
	}
	r.HasReturning = len(stmt.ReturningList) > 0
}

func (r *PostgreSQLParseResult) extractFromUpdate(stmt *pgquery.UpdateStmt) {
	r.Operation = OpUpdate
	if stmt.Relation != nil {
		r.addTable(stmt.Relation.Relname)
	}
	r.HasWhere = stmt.WhereClause != nil
	r.HasReturning = len(stmt.ReturningList) > 0
}

func (r *PostgreSQLParseResult) extractFromDelete(stmt *pgquery.DeleteStmt) {
	r.Operation = OpDelete
	if stmt.Relation != nil {
		r.addTable(stmt.Relation.Relname)
	}
	r.HasWhere = stmt.WhereClause != nil
	r.HasReturning = len(stmt.ReturningList) > 0
}

func (r *PostgreSQLParseResult) extractFromDrop(stmt *pgquery.DropStmt) {
	r.Operation = OpDDL
	switch stmt.RemoveType {
	case pgquery.ObjectType_OBJECT_TABLE:
		r.IsDropTable = true
	case pgquery.ObjectType_OBJECT_INDEX:
		// Index drop — DDL but not table drop
	default:
		// Other drops (schema, etc.)
	}
	for _, obj := range stmt.Objects {
		if obj == nil {
			continue
		}
		if list, ok := obj.Node.(*pgquery.Node_List); ok {
			for _, item := range list.List.Items {
				if str, ok := item.Node.(*pgquery.Node_String_); ok {
					r.addTable(str.String_.Sval)
				}
			}
		} else if str, ok := obj.Node.(*pgquery.Node_String_); ok {
			r.addTable(str.String_.Sval)
		}
	}
}

func (r *PostgreSQLParseResult) extractFromTruncate(stmt *pgquery.TruncateStmt) {
	r.Operation = OpDDL
	r.IsTruncate = true
	for _, node := range stmt.Relations {
		if node == nil {
			continue
		}
		if rv, ok := node.Node.(*pgquery.Node_RangeVar); ok {
			r.addTable(rv.RangeVar.Relname)
		}
	}
}

func (r *PostgreSQLParseResult) extractFromAlterTable(stmt *pgquery.AlterTableStmt) {
	r.Operation = OpDDL
	if stmt.Relation != nil {
		r.addTable(stmt.Relation.Relname)
	}
}

func (r *PostgreSQLParseResult) extractFromCreate(stmt *pgquery.CreateStmt) {
	r.Operation = OpDDL
	if stmt.Relation != nil {
		r.addTable(stmt.Relation.Relname)
	}
}

func (r *PostgreSQLParseResult) extractFromDropDB(stmt *pgquery.DropdbStmt) {
	r.Operation = OpDDL
	r.IsDropDatabase = true
	if stmt.Dbname != "" {
		r.Tables = append(r.Tables, stmt.Dbname)
	}
}

func (r *PostgreSQLParseResult) extractFromIndex(stmt *pgquery.IndexStmt) {
	r.Operation = OpDDL
	if stmt.Relation != nil {
		r.addTable(stmt.Relation.Relname)
	}
}

func (r *PostgreSQLParseResult) extractFromView(stmt *pgquery.ViewStmt) {
	r.Operation = OpDDL
	if stmt.View != nil {
		r.addTable(stmt.View.Relname)
	}
}

func (r *PostgreSQLParseResult) extractFromCreateTableAs(stmt *pgquery.CreateTableAsStmt) {
	r.Operation = OpDDL
	if stmt.Into != nil && stmt.Into.Rel != nil {
		r.addTable(stmt.Into.Rel.Relname)
	}
}

// extractTablesFromRangeVar extracts table names from range variables (FROM clause items).
func (r *PostgreSQLParseResult) extractTablesFromRangeVar(node *pgquery.Node) {
	if node == nil {
		return
	}
	switch n := node.Node.(type) {
	case *pgquery.Node_RangeVar:
		rv := n.RangeVar
		if rv.Relname != "" {
			r.addTable(rv.Relname)
		}
	case *pgquery.Node_JoinExpr:
		r.extractTablesFromJoin(n.JoinExpr)
	case *pgquery.Node_RangeSubselect:
		// Subquery in FROM — extract tables from the subquery
		if n.RangeSubselect.Subquery != nil {
			subResult := &PostgreSQLParseResult{}
			subResult.extractFromPGNode(n.RangeSubselect.Subquery)
			for _, t := range subResult.Tables {
				r.addTable(t)
			}
		}
	case *pgquery.Node_RangeFunction:
		// Function call in FROM (e.g. generate_series) — no table to extract
	case *pgquery.Node_List:
		for _, item := range n.List.Items {
			r.extractTablesFromRangeVar(item)
		}
	}
}

// extractTablesFromJoin extracts tables from JOIN expressions.
func (r *PostgreSQLParseResult) extractTablesFromJoin(join *pgquery.JoinExpr) {
	if join == nil {
		return
	}
	if join.Larg != nil {
		r.extractTablesFromRangeVar(join.Larg)
	}
	if join.Rarg != nil {
		r.extractTablesFromRangeVar(join.Rarg)
	}
}

// addTable adds a table name if not already present (case-insensitive).
func (r *PostgreSQLParseResult) addTable(name string) {
	if name == "" {
		return
	}
	name = strings.ToLower(name)
	for _, t := range r.Tables {
		if strings.EqualFold(t, name) {
			return
		}
	}
	r.Tables = append(r.Tables, name)
}

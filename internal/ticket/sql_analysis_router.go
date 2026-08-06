package ticket

import (
	"log"

	"github.com/whg517/sqlflow/internal/driver"
)

// analyzeTicketSQL derives the operation and affected objects a ticket's risk
// score is computed from.
//
// The routing is by declared query form, not by type name. SQL-form datasources
// keep the keyword analyzer because the score depends on granularity the
// driver's OperationType does not carry: it collapses CREATE, ALTER, DROP and
// TRUNCATE into one "ddl" value, while the evaluator separates them by 50
// points.
//
// Everything else goes through the driver. SQLAnalyzer matches a leading SQL
// keyword, so a MongoDB command body — which starts with "{" — matched nothing,
// came out as SQLType "OTHER" and scored 15. That is the low band, and low is
// what an approval policy may auto-approve, so a ticket that emptied a
// collection was filed as the least dangerous kind of change there is.
func analyzeTicketSQL(dbType, sqlContent string) *SQLAnalysis {
	if isSQLForm(dbType) {
		return NewSQLAnalyzer().Analyze(sqlContent)
	}

	parsed, err := driver.ParseFor(dbType, sqlContent)
	if err != nil {
		// A body the driver refuses has not been shown to be safe. Returning
		// the unknown shape keeps it out of the low band; see the default arm
		// of RiskEvaluator.Evaluate.
		log.Printf("ticket: parse %s body for risk evaluation: %v", dbType, err)
		return &SQLAnalysis{SQLType: "OTHER", Operations: []string{}, AffectedTables: []string{}}
	}
	return analysisFromParseResult(parsed)
}

// isSQLForm reports whether the datasource's queries are written as SQL.
//
// An unregistered type answers false, which routes it through ParseFor and
// fails there — better than silently handing an unknown dialect to a parser
// built for MySQL.
func isSQLForm(dbType string) bool {
	desc, err := driver.DescribeType(dbType)
	return err == nil && desc.QueryForm == driver.QueryFormSQL
}

// analysisFromParseResult converts a driver's reading of a query into the shape
// the risk evaluator scores.
func analysisFromParseResult(parsed *driver.ParseResult) *SQLAnalysis {
	analysis := &SQLAnalysis{
		SQLType:        "OTHER",
		Operations:     []string{},
		AffectedTables: parsed.Targets,
	}
	if analysis.AffectedTables == nil {
		analysis.AffectedTables = []string{}
	}

	switch parsed.Operation {
	case driver.OpSelect:
		analysis.SQLType = "SELECT"
		analysis.IsDML = true
		analysis.IsRead = true
	case driver.OpDML:
		analysis.SQLType = "INSERT"
		analysis.IsDML = true
	case driver.OpUpdate:
		analysis.SQLType = "UPDATE"
		analysis.IsDML = true
	case driver.OpDelete:
		analysis.SQLType = "DELETE"
		analysis.IsDML = true
	case driver.OpDDL:
		// The driver reports "ddl" without saying which. ALTER is the middle of
		// the DDL band, so it neither understates a schema change nor claims the
		// certainty that a DROP score would.
		analysis.SQLType = "ALTER"
		analysis.IsDDL = true
	}
	if analysis.SQLType != "OTHER" {
		analysis.Operations = []string{analysis.SQLType}
	}
	return analysis
}

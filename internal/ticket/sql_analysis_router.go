package ticket

import (
	"fmt"
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

// maxTicketStatements bounds one ticket.
//
// Each statement costs a parse, and the PostgreSQL parser is cgo. Nothing stops
// a pasted schema dump today; a bound turns that into a refusal with a reason
// instead of a request that takes minutes and grades everything at once.
const maxTicketStatements = 100

// ticketPlan is the decomposition of a ticket body: the statements that will
// run, and the analysis those exact statements were folded into.
//
// They travel together because the defect this replaces was that they did not.
// The executor split with strings.Split(body, ";") while the analyzer read only
// the first keyword, so "SELECT 1; DROP TABLE users" was graded low and ran a
// DROP. Two derivations of the same body can disagree; one cannot.
type ticketPlan struct {
	Statements []string
	Analysis   *SQLAnalysis
}

// planTicketSQL splits a body and folds the per-statement analyzes into one.
//
// It returns an error rather than a safe-looking default when the body cannot
// be decomposed. "We could not read this" is not "this is harmless", and the
// caller has somewhere to put that — a refused ticket the submitter can fix,
// rather than an approval chain chosen from a guess.
func planTicketSQL(dbType, sqlContent string) (*ticketPlan, error) {
	statements, err := driver.SplitStatementsFor(dbType, sqlContent)
	if err != nil {
		return nil, err
	}
	if len(statements) > maxTicketStatements {
		return nil, fmt.Errorf("%w: %d 条，上限 %d", ErrTooManyStatements, len(statements), maxTicketStatements)
	}

	analyzes := make([]*SQLAnalysis, 0, len(statements))
	for _, stmt := range statements {
		analyzes = append(analyzes, analyzeTicketSQL(dbType, stmt))
	}

	return &ticketPlan{Statements: statements, Analysis: foldAnalyses(analyzes)}, nil
}

// foldAnalyses merges per-statement analyzes into the one a ticket is graded on.
//
// The merge is monotone by construction — it can only add tables and turn flags
// on — so the folded analysis never scores below the worst single statement.
// The type is taken from the worst-scoring statement rather than the first,
// which is the whole point: a DROP behind a SELECT has to grade as a DROP.
func foldAnalyses(analyzes []*SQLAnalysis) *SQLAnalysis {
	folded := &SQLAnalysis{SQLType: "OTHER", Operations: []string{}, AffectedTables: []string{}, IsRead: true}

	seen := map[string]bool{}
	worstScore := -1

	for _, a := range analyzes {
		if score := NewRiskEvaluator().Evaluate(a).Score; score > worstScore {
			worstScore = score
			folded.SQLType = a.SQLType
		}
		folded.IsDDL = folded.IsDDL || a.IsDDL
		folded.IsDML = folded.IsDML || a.IsDML
		folded.IsRead = folded.IsRead && a.IsRead
		folded.Operations = append(folded.Operations, a.Operations...)
		for _, tbl := range a.AffectedTables {
			if !seen[tbl] {
				seen[tbl] = true
				folded.AffectedTables = append(folded.AffectedTables, tbl)
			}
		}
	}

	if len(analyzes) == 0 {
		folded.IsRead = false
	}
	return folded
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

package driver

import (
	"errors"
	"strings"
)

// ErrNoStatement is returned for a body with nothing executable in it.
//
// Declared here rather than aliased from the parser package so the port keeps
// no dependency on a parser implementation; the SQL drivers translate their
// own sentinel to this one.
var ErrNoStatement = errors.New("未找到可执行的语句")

// SplitStatementsFor returns the statements a datasource type will execute for
// a body.
//
// It resolves the driver from the registry, so it needs no connection — the
// ticket path has to grade a body long before anything connects. That mirrors
// ParseFor and DescribeType, which exist for the same reason.
//
// There is deliberately no second entry point taking a live Driver. The
// executor must split with the type the ticket was graded under, not with the
// type the datasource happens to have now: a datasource that was retyped
// between approval and execution would otherwise run a body cut a different way
// from the one an approver read.
func SplitStatementsFor(typeName, body string) ([]string, error) {
	d, err := NewDriver(typeName)
	if err != nil {
		return nil, err
	}

	splitter, ok := d.(StatementSplitter)
	if !ok {
		// One body, one statement. This is the correct answer for a document or
		// DSL driver, and it also fixes a live defect: the ticket executor used
		// to run MongoDB command bodies through strings.Split(body, ";"), so a
		// semicolon inside a JSON string value shredded the document.
		trimmed := strings.TrimSpace(body)
		if trimmed == "" {
			return nil, ErrNoStatement
		}
		return []string{trimmed}, nil
	}

	return splitter.SplitStatements(body)
}

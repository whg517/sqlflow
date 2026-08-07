package sqlparser

import (
	"errors"
	"strings"

	pgquery "github.com/pganalyze/pg_query_go/v6"
)

var (
	// ErrMultipleStatements is returned where exactly one statement is expected.
	//
	// It replaces silent truncation. ParseMySQL and ParsePostgreSQL each used to
	// begin by cutting the input at the first ';' byte, so a caller handing over
	// a multi-statement body received a confident description of the first
	// statement and no indication that the rest existed — while the executor ran
	// all of them. An error is the only answer that cannot be mistaken for a
	// complete one.
	ErrMultipleStatements = errors.New("此处只允许单条语句")

	// ErrNoStatement is returned for input that contains nothing executable.
	//
	// Distinct from a zero-length split result, which a caller could read as
	// "nothing to do" and proceed. Comment-only bodies and inputs the splitter
	// silently discards land here.
	ErrNoStatement = errors.New("未找到可执行的语句")

	// ErrUndelimitable is returned when statement boundaries cannot be
	// established from the text alone.
	ErrUndelimitable = errors.New("无法确定语句边界")
)

// SplitPostgreSQLDialect splits a PostgreSQL body into statements.
//
// It uses libpg_query's parser-backed splitter rather than the scanner-backed
// one. The scanner shreds a body whose function is written with BEGIN ATOMIC,
// because the semicolons inside that body are ordinary statement separators to
// a lexer; the parser knows they belong to the routine. Producing three
// fragments there is the partial-execution shape this whole change exists to
// remove.
func SplitPostgreSQLDialect(body string) ([]string, error) {
	if strings.TrimSpace(body) == "" {
		return nil, ErrNoStatement
	}

	parts, err := pgquery.SplitWithParser(body, true)
	if err != nil {
		return nil, err
	}

	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		if stmt := strings.TrimSpace(part); stmt != "" {
			statements = append(statements, stmt)
		}
	}
	if len(statements) == 0 {
		// libpg_query returns zero statements for input it does not recognize
		// as SQL at all — a MySQL version comment, for instance. Zero is not
		// "nothing to do"; the text is still there and something will run it.
		return nil, ErrNoStatement
	}
	return statements, nil
}

// SplitMySQLDialect splits a MySQL or SQLite body into statements.
//
// It scans rather than parses, and that is forced rather than preferred. The
// vendored pingcap/parser rejects statements this platform must accept —
// ALTER TABLE ... RENAME COLUMN, common table expressions, window functions,
// CREATE TRIGGER — so using the grammar as the splitter would make ordinary
// change tickets impossible to file. Its Scanner is not usable directly either:
// Lex takes an unexported type.
//
// Scanning is also what the mysql client does, which is why DELIMITER exists at
// all: the boundary between statements is a lexical question, and the server
// never sees more than one statement anyway.
//
// The one thing it cannot know is sql_mode. Under NO_BACKSLASH_ESCAPES a
// trailing backslash does not escape the closing quote, so this scanner stays
// inside a string that the server considers finished. That direction is the
// safe one: the fragments merge rather than separate, the merged text fails to
// analyze cleanly, and it scores as unknown rather than as its first keyword.
func SplitMySQLDialect(body string) ([]string, error) {
	statements := scanMySQLStatements(body)
	// A body that is only comments carries no statement. The scanner keeps
	// comment text because it belongs to the statement it annotates, so
	// "comments only" has to be recognized here rather than by emptiness.
	if len(statements) == 0 || !hasExecutableText(statements) {
		return nil, ErrNoStatement
	}
	if err := refuseUndelimitedRoutine(statements); err != nil {
		return nil, err
	}
	return statements, nil
}

// hasExecutableText reports whether any statement holds something other than
// comments and whitespace.
func hasExecutableText(statements []string) bool {
	for _, stmt := range statements {
		if strings.TrimSpace(stripComments(stmt)) != "" {
			return true
		}
	}
	return false
}

// stripComments removes comment runs so the caller can ask whether anything is
// left. An executable /*! ... */ comment is deliberately not removed: its
// contents run.
func stripComments(stmt string) string {
	var out strings.Builder
	runes := []rune(stmt)
	for i := 0; i < len(runes); {
		if n, ok := skipComment(runes, i, &strings.Builder{}); ok {
			i = n
			continue
		}
		if n, ok := skipQuoted(runes, i, &out); ok {
			i = n
			continue
		}
		out.WriteRune(runes[i])
		i++
	}
	return out.String()
}

// routinePrefixes are the statements whose bodies carry their own semicolons.
var routinePrefixes = []string{"procedure", "function", "trigger", "event"}

// refuseUndelimitedRoutine rejects a routine definition that was cut apart.
//
// CREATE PROCEDURE p() BEGIN INSERT ...; DELETE FROM b; END is the worst input
// in the whole enumeration: a lexical scanner cuts it into three, the first
// fragment fails, and the second is a live unconditional DELETE that no
// approver ever read as such. The server grammar has no DELIMITER statement, so
// the only correct handling of a routine in a multi-statement body is to refuse
// it and ask for the delimiter the mysql client would have required.
//
// A routine wrapped in DELIMITER arrives as one statement and passes untouched.
func refuseUndelimitedRoutine(statements []string) error {
	if len(statements) < 2 {
		return nil
	}
	for _, stmt := range statements {
		if isRoutineDefinition(stmt) {
			return ErrUndelimitable
		}
	}
	return nil
}

// isRoutineDefinition reports whether a statement opens a routine body.
func isRoutineDefinition(stmt string) bool {
	lower := strings.ToLower(strings.TrimSpace(stmt))
	if !strings.HasPrefix(lower, "create") {
		return false
	}
	// DEFINER and OR REPLACE sit between CREATE and the object kind, so match
	// the kind anywhere in the opening clause rather than at a fixed offset.
	head := lower
	if len(head) > 120 {
		head = head[:120]
	}
	for _, kind := range routinePrefixes {
		if strings.Contains(head, " "+kind+" ") || strings.HasSuffix(head, " "+kind) {
			return true
		}
	}
	return false
}

// scanMySQLStatements walks the body one rune at a time, tracking the contexts
// in which a semicolon does not terminate a statement.
func scanMySQLStatements(body string) []string {
	var (
		statements []string
		current    strings.Builder
		delimiter  = ";"
	)

	flush := func() {
		if stmt := strings.TrimSpace(current.String()); stmt != "" {
			statements = append(statements, stmt)
		}
		current.Reset()
	}

	runes := []rune(body)
	for i := 0; i < len(runes); {
		// DELIMITER is a client directive, not SQL. It only counts at the start
		// of a statement, which is the only place the mysql client honors it.
		if strings.TrimSpace(current.String()) == "" {
			if n, next, ok := readDelimiterDirective(runes, i); ok {
				delimiter = next
				current.Reset()
				i = n
				continue
			}
		}

		if n, ok := skipQuoted(runes, i, &current); ok {
			i = n
			continue
		}
		if n, ok := skipComment(runes, i, &current); ok {
			i = n
			continue
		}

		if hasDelimiterAt(runes, i, delimiter) {
			flush()
			i += len([]rune(delimiter))
			continue
		}

		current.WriteRune(runes[i])
		i++
	}
	flush()

	return statements
}

// skipQuoted copies a quoted run verbatim and reports where it ends.
//
// Single and double quotes accept both the doubled-quote escape and the
// backslash escape; backticks and brackets accept only doubling. Brackets are
// SQLite's identifier quoting, which this scanner serves too because SQLite
// reuses the MySQL analysis path.
func skipQuoted(runes []rune, i int, out *strings.Builder) (int, bool) {
	var closing rune
	backslashEscapes := false

	switch runes[i] {
	case '\'', '"':
		closing = runes[i]
		backslashEscapes = true
	case '`':
		closing = '`'
	case '[':
		closing = ']'
	default:
		return i, false
	}

	out.WriteRune(runes[i])
	i++
	for i < len(runes) {
		c := runes[i]
		if backslashEscapes && c == '\\' && i+1 < len(runes) {
			out.WriteRune(c)
			out.WriteRune(runes[i+1])
			i += 2
			continue
		}
		if c == closing {
			// A doubled closing quote is a literal one, not the end.
			if i+1 < len(runes) && runes[i+1] == closing {
				out.WriteRune(c)
				out.WriteRune(c)
				i += 2
				continue
			}
			out.WriteRune(c)
			return i + 1, true
		}
		out.WriteRune(c)
		i++
	}
	// Unterminated: the rest of the input belongs to the literal. Merging is the
	// safe direction — the statement will fail to analyze rather than split.
	return len(runes), true
}

// skipComment copies a comment verbatim and reports where it ends.
//
// A MySQL executable comment — /*!nnnnn ... */ — is not skipped. The server
// runs its contents: the vendored parser reads /*!50000 DROP TABLE users */ as
// a DropTableStmt. Treating it as a comment would hide a live DROP from the
// analyzer, which is the exact failure this change exists to close, so it falls
// through and is scanned as code.
func skipComment(runes []rune, i int, out *strings.Builder) (int, bool) {
	// -- requires whitespace after it in MySQL; # does not.
	if runes[i] == '#' ||
		(runes[i] == '-' && i+2 < len(runes) && runes[i+1] == '-' && isSpace(runes[i+2])) ||
		(runes[i] == '-' && i+2 == len(runes) && runes[i+1] == '-') {
		for i < len(runes) && runes[i] != '\n' {
			out.WriteRune(runes[i])
			i++
		}
		return i, true
	}

	if runes[i] == '/' && i+1 < len(runes) && runes[i+1] == '*' {
		if i+2 < len(runes) && runes[i+2] == '!' {
			return i, false // executable, scan as code
		}
		out.WriteRune(runes[i])
		out.WriteRune(runes[i+1])
		i += 2
		for i < len(runes) {
			if runes[i] == '*' && i+1 < len(runes) && runes[i+1] == '/' {
				out.WriteRune(runes[i])
				out.WriteRune(runes[i+1])
				return i + 2, true
			}
			out.WriteRune(runes[i])
			i++
		}
		return len(runes), true
	}

	return i, false
}

// readDelimiterDirective recognizes a DELIMITER line and returns the new
// terminator.
//
// The directive is consumed and emits no statement: it is an instruction to the
// client about how to cut the text, and the server would reject it.
func readDelimiterDirective(runes []rune, i int) (int, string, bool) {
	const keyword = "delimiter"
	rest := string(runes[i:])
	if len(rest) < len(keyword)+1 {
		return i, "", false
	}
	if !strings.EqualFold(rest[:len(keyword)], keyword) {
		return i, "", false
	}
	if !isSpace(rune(rest[len(keyword)])) {
		return i, "", false
	}

	line := rest
	if idx := strings.IndexByte(rest, '\n'); idx >= 0 {
		line = rest[:idx]
	}
	token := strings.TrimSpace(line[len(keyword):])
	if token == "" {
		return i, "", false
	}
	return i + len([]rune(line)), token, true
}

// hasDelimiterAt reports whether the delimiter starts at this position.
func hasDelimiterAt(runes []rune, i int, delimiter string) bool {
	d := []rune(delimiter)
	if i+len(d) > len(runes) {
		return false
	}
	for j, c := range d {
		if runes[i+j] != c {
			return false
		}
	}
	return true
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

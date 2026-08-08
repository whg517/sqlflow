package query

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/whg517/sqlflow/internal/audit"
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/driver"
	pgdriver "github.com/whg517/sqlflow/internal/driver/postgresql"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/security"
	"github.com/whg517/sqlflow/internal/testutil"
)

// explainFixture points a postgresql datasource at the running test database,
// so a plan is produced by a real engine rather than a fake that would agree
// with whatever the code did.
type explainFixture struct {
	svc    *Service
	testDB *sql.DB
	dsID   int64
	uid    int64
}

func newExplainFixture(t *testing.T) *explainFixture {
	t.Helper()

	testDB := testutil.NewDB(t).DB
	seedCasbinRules(t, testDB)
	wrap := testutil.WrapSQL(t, testDB)

	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(wrap, testutil.EncryptionKey, connpool.NewManager(), poolMgr, auditlog.Discard)

	ds := &model.DataSource{
		Name: "explain-ds", Type: "postgresql", Host: "10.0.0.1", Port: 5432,
		Username: "postgres", PasswordEncrypted: "secret", Database: "testdb",
	}
	if err := dsSvc.CreateDataSource(context.Background(), ds); err != nil {
		t.Fatalf("seed datasource: %v", err)
	}
	poolMgr.InjectForTest(ds.ID, pgdriver.NewWithDB(testDB))

	permSvc, err := security.NewService(wrap)
	if err != nil {
		t.Fatalf("permission service: %v", err)
	}

	return &explainFixture{
		svc: NewService(wrap, dsSvc, NewHistoryService(wrap), permSvc,
			audit.NewService(wrap, 0, 0), testutil.EncryptionKey, poolMgr, Limits{}),
		testDB: testDB,
		dsID:   ds.ID,
		uid:    testutil.SeedUser(t, testDB, "explain_actor", "admin"),
	}
}

func (f *explainFixture) explain(sqlContent string) (*ExplainResult, error) {
	return f.svc.ExplainQuery(context.Background(), f.uid, "admin", f.dsID, "testdb", sqlContent)
}

// TestExplainQuery_RefusesStatementsThatWriteThroughEXPLAIN is this entrance's
// reason for having gates at all.
//
// EXPLAIN ANALYZE is not a description of a statement, it is the statement:
// PostgreSQL runs the plan for real in order to report actual timings. So an
// entrance that accepts anything beginning with EXPLAIN accepts everything. The
// prefix test did exactly that — it matched the word EXPLAIN, stripped that
// word, and the driver prepended it back, so "EXPLAIN ANALYZE DELETE FROM t"
// reached the database intact. I reproduced it before writing this: three rows
// in, zero rows out, and the caller got a plan and a 200.
//
// The fix is not a longer prefix list. This entrance may only accept statements
// it would have been willing to execute, which is what driver.ParseFor already
// answers for every other entrance.
func TestExplainQuery_RefusesStatementsThatWriteThroughEXPLAIN(t *testing.T) {
	f := newExplainFixture(t)

	if _, err := f.testDB.Exec(`CREATE TABLE explain_victim (id INT)`); err != nil {
		t.Fatalf("create victim: %v", err)
	}
	if _, err := f.testDB.Exec(`INSERT INTO explain_victim VALUES (1),(2),(3)`); err != nil {
		t.Fatalf("seed victim: %v", err)
	}

	// wantErr names WHICH refusal is expected, not merely that one happened.
	//
	// Asserting only "an error came back" would let any one gate cover for all
	// the others: these statements are also high-risk, so the risk verdict alone
	// would keep the test green while the operation verdict was deleted. I tried
	// exactly that as a sabotage and the test passed, which is how this column
	// got here.
	writes := []struct {
		name    string
		sql     string
		wantErr error
	}{
		{"analyze_delete", "EXPLAIN ANALYZE DELETE FROM explain_victim", ErrExplainNonSelect},
		{"parenthesised_analyze", "EXPLAIN (ANALYZE) DELETE FROM explain_victim", ErrExplainNonSelect},
		{"analyze_with_options", "EXPLAIN (ANALYZE, COSTS FALSE) DELETE FROM explain_victim", ErrExplainNonSelect},
		{"analyze_update", "EXPLAIN ANALYZE UPDATE explain_victim SET id = 99", ErrExplainNonSelect},
		// Caught one gate earlier, and that is worth recording: a bare DELETE
		// parses cleanly and the block rules recognize it, while
		// "ANALYZE DELETE ..." degrades to the keyword fallback and does not.
		// The operation verdict is what catches the EXPLAIN ANALYZE forms.
		{"bare_delete", "DELETE FROM explain_victim", ErrSQLBlocked},
		{"analyze_insert", "EXPLAIN ANALYZE INSERT INTO explain_victim VALUES (4)", ErrExplainNonSelect},
	}

	for _, w := range writes {
		t.Run(w.name, func(t *testing.T) {
			before := countRows(t, f.testDB, "explain_victim")

			_, err := f.explain(w.sql)
			if err == nil {
				t.Error("a write statement was accepted by the plan entrance")
			} else if !errors.Is(err, w.wantErr) {
				t.Errorf("refused with %v, want %v — a different gate caught this, so the one under test is unverified", err, w.wantErr)
			}

			if after := countRows(t, f.testDB, "explain_victim"); after != before {
				t.Errorf("rows went %d -> %d: the statement executed through EXPLAIN", before, after)
			}
		})
	}
}

// TestExplainQuery_StillPlansReads is the positive control.
//
// A gate that refused everything would pass the test above and be useless, so
// the reads this entrance exists for have to keep working — including the form
// where the user typed EXPLAIN themselves.
func TestExplainQuery_StillPlansReads(t *testing.T) {
	f := newExplainFixture(t)
	if _, err := f.testDB.Exec(`CREATE TABLE explain_reads (id INT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	for _, q := range []string{
		"SELECT * FROM explain_reads",
		"EXPLAIN SELECT * FROM explain_reads",
		"explain select * from explain_reads",
		"WITH x AS (SELECT 1 AS n) SELECT n FROM x",
	} {
		res, err := f.explain(q)
		if err != nil {
			t.Errorf("ExplainQuery(%q): %v", q, err)
			continue
		}
		if res == nil {
			t.Errorf("ExplainQuery(%q) returned no result", q)
		}
	}
}

// TestExplainQuery_ReportsAPlanForPostgreSQL pins that the plan reaches the
// caller in a form something can render.
//
// The response shape was decided by sniffing for a MySQL column name, so a
// PostgreSQL plan — a single QUERY PLAN text column — produced an empty Plan
// slice and a formatted MySQL header with no rows under it. The UI showed
// "无执行计划数据" for a query the server had planned successfully.
func TestExplainQuery_ReportsAPlanForPostgreSQL(t *testing.T) {
	f := newExplainFixture(t)
	if _, err := f.testDB.Exec(`CREATE TABLE explain_shape (id INT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	res, err := f.explain("SELECT * FROM explain_shape")
	if err != nil {
		t.Fatalf("ExplainQuery: %v", err)
	}
	if len(res.Columns) == 0 || len(res.Rows) == 0 {
		t.Fatalf("no plan rows came back: columns=%v rows=%d", res.Columns, len(res.Rows))
	}
	// Formatted has to be built from the columns this driver returned. Asserting
	// only that it is non-empty proves nothing: the old code always produced a
	// MySQL header, which is non-empty and useless.
	if !strings.Contains(res.Formatted, res.Columns[0]) {
		t.Errorf("Formatted does not mention the driver's own column %q:\n%s", res.Columns[0], res.Formatted)
	}
	if strings.Contains(res.Formatted, "select_type") {
		t.Errorf("a PostgreSQL plan was rendered against MySQL's header:\n%s", res.Formatted)
	}
	for _, c := range res.Columns {
		if c == "select_type" {
			t.Error("a PostgreSQL plan was reported with MySQL's columns")
		}
	}
}

func countRows(t *testing.T, conn *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

package query

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/whg517/sqlflow/internal/audit"
	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/driver"
	mysqldriver "github.com/whg517/sqlflow/internal/driver/mysql"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/security"
	"github.com/whg517/sqlflow/internal/testutil"
)

// rawPhone is the value the mask rule in these fixtures protects. Every
// assertion here is ultimately "the caller did not receive this string".
const rawPhone = "13812345678"

// scopeFixture is a query Service whose one datasource is configured for
// database "testdb", whose driver is backed by a real table holding rawPhone,
// and whose mask rule is scoped to that same "testdb".
//
// The point of the fixture is that the rule and the datasource agree: any
// caller-supplied database that disagrees with both is a lie, and the only
// question is whether the platform lets that lie decide which rules load.
type scopeFixture struct {
	svc    *Service
	dsID   int64
	metaDB *sql.DB
}

func newScopeFixture(t *testing.T) *scopeFixture {
	t.Helper()

	metaDB := setupQueryTestDB(t)
	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(testutil.WrapSQL(t, metaDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)

	ctx, cancel := queryCtx(t)
	defer cancel()
	dsID := seedQueryDatasource(t, dsSvc, ctx) // Database: "testdb"

	// The target the driver actually reads. It is a separate schema from the
	// platform's own, so a query that reached the wrong one would be visible.
	targetDB := testutil.NewDB(t).DB
	if _, err := targetDB.Exec(`CREATE TABLE customers (id int, phone text)`); err != nil {
		t.Fatalf("create target table: %v", err)
	}
	if _, err := targetDB.Exec(`INSERT INTO customers (id, phone) VALUES (1, $1)`, rawPhone); err != nil {
		t.Fatalf("seed target row: %v", err)
	}
	poolMgr.InjectForTest(dsID, mysqldriver.NewWithDB(targetDB))

	permSvc, err := security.NewService(testutil.WrapSQL(t, metaDB))
	if err != nil {
		t.Fatalf("create permission service: %v", err)
	}
	seedPolicy(t, metaDB, permSvc, "developer", fmt.Sprintf("ds_%d", dsID), "*", "select")
	seedMaskRule(t, metaDB, dsID, "testdb", "customers", "phone", "phone")

	historySvc := NewHistoryService(testutil.WrapSQL(t, metaDB))
	auditSvc := audit.NewService(testutil.WrapSQL(t, metaDB), 0, 0)
	svc := NewService(testutil.WrapSQL(t, metaDB), dsSvc, historySvc, permSvc, auditSvc, testutil.EncryptionKey, poolMgr, Limits{})

	return &scopeFixture{svc: svc, dsID: dsID, metaDB: metaDB}
}

// assertNoRawPhone fails when any cell of the result still carries the
// protected value.
func assertNoRawPhone(t *testing.T, rows []map[string]any) {
	t.Helper()
	if len(rows) == 0 {
		t.Fatal("no rows: the fixture must return the protected row for the assertion to mean anything")
	}
	for _, row := range rows {
		for col, v := range row {
			if s, ok := v.(string); ok && strings.Contains(s, rawPhone) {
				t.Errorf("column %q returned the unmasked value %q", col, s)
			}
		}
	}
}

// TestExecuteQuery_ForeignDatabaseCannotDropMaskRules is the regression for the
// masking bypass reachable from the query workbench's database box.
//
// The box is free text and the value went two places at once: to the driver,
// which discarded it — the pool keys on datasource ID and the DSN pins the
// database — and to loadMaskRules, which kept only the rules matching that
// database or no database at all. Naming any database no rule was scoped to
// therefore dropped every rule protecting the database the rows actually came
// from, and the query returned plaintext.
//
// The assertion is the invariant itself (脱敏不能被绕过), not the mechanism:
// the caller must not receive the protected value, whether the platform
// refuses the request or masks the rows.
func TestExecuteQuery_ForeignDatabaseCannotDropMaskRules(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		// mustSucceed marks the scopes that name the datasource's real database.
		// They pin the fixture: if these fail, a later refusal proves nothing.
		mustSucceed bool
	}{
		{"unspecified scope", "", true},
		{"the datasource's own database", "testdb", true},
		{"a database no rule is scoped to", "someone_elses_db", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newScopeFixture(t)
			ctx, cancel := queryCtx(t)
			defer cancel()

			result, err := f.svc.ExecuteQuery(ctx, testActorID, "dev", "developer", f.dsID,
				tt.requested, "SELECT id, phone FROM customers")
			if err != nil {
				if tt.mustSucceed {
					t.Fatalf("ExecuteQuery(%q) error = %v, want rows", tt.requested, err)
				}
				// Refusing a scope the platform cannot honor leaks nothing.
				return
			}
			assertNoRawPhone(t, result.Rows)
			if !result.Desensitized {
				t.Error("Desensitized = false, want true")
			}
		})
	}
}

// TestExportQuery_ForeignDatabaseCannotDropMaskRules covers the second
// entrance to the same rows. Export shares loadMaskRules with the query path,
// so it shared the bypass.
func TestExportQuery_ForeignDatabaseCannotDropMaskRules(t *testing.T) {
	f := newScopeFixture(t)
	ctx, cancel := queryCtx(t)
	defer cancel()

	seedPolicy(t, f.metaDB, f.svc.permSvc, "developer", fmt.Sprintf("ds_%d", f.dsID), "*", "export")

	result, err := f.svc.ExportQuery(ctx, testActorID, "dev", "developer", f.dsID,
		"someone_elses_db", "SELECT id, phone FROM customers")
	if err != nil {
		return // refused, nothing leaked
	}
	assertNoRawPhone(t, result.Rows)
}

// TestQueryEntrancesRefuseAForeignDatabase pins how the refusal reads.
//
// Substituting the datasource's own scope would also close the bypass, but it
// would answer "run this against prod" with staging rows and an audit row
// saying staging — a caller acting on data from a database they did not ask for
// is a different failure, not a fix. Every entrance answers the same way.
func TestQueryEntrancesRefuseAForeignDatabase(t *testing.T) {
	f := newScopeFixture(t)
	ctx, cancel := queryCtx(t)
	defer cancel()

	seedPolicy(t, f.metaDB, f.svc.permSvc, "developer", fmt.Sprintf("ds_%d", f.dsID), "*", "export")

	const sql = "SELECT id, phone FROM customers"
	entrances := map[string]func() error{
		"query": func() error {
			_, err := f.svc.ExecuteQuery(ctx, testActorID, "dev", "developer", f.dsID, "someone_elses_db", sql)
			return err
		},
		"export": func() error {
			_, err := f.svc.ExportQuery(ctx, testActorID, "dev", "developer", f.dsID, "someone_elses_db", sql)
			return err
		},
		"explain": func() error {
			_, err := f.svc.ExplainQuery(ctx, testActorID, "developer", f.dsID, "someone_elses_db", sql)
			return err
		},
	}
	for name, call := range entrances {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, datasource.ErrDatabaseScopeMismatch) {
				t.Errorf("error = %v, want ErrDatabaseScopeMismatch", err)
			}
		})
	}
}

// TestExecuteQuery_TrailRecordsTheDatabaseActuallyUsed closes the other half of
// the defect: the caller sends nothing, and both the history row and the audit
// row must still name the database the rows came from. They used to copy the
// request, so an unspecified scope was recorded as "" — a trail that cannot say
// where a query ran is no trail.
func TestExecuteQuery_TrailRecordsTheDatabaseActuallyUsed(t *testing.T) {
	f := newScopeFixture(t)
	ctx, cancel := queryCtx(t)
	defer cancel()

	if _, err := f.svc.ExecuteQuery(ctx, testActorID, "dev", "developer", f.dsID,
		"", "SELECT id, phone FROM customers"); err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}

	for _, tt := range []struct{ what, query string }{
		{"query history", `SELECT database FROM query_history WHERE datasource_id = $1`},
		{"audit log", `SELECT database FROM audit_logs WHERE datasource_id = $1 AND action = 'query'`},
	} {
		var recorded string
		if err := f.metaDB.QueryRow(tt.query, f.dsID).Scan(&recorded); err != nil {
			t.Fatalf("read %s: %v", tt.what, err)
		}
		if recorded != "testdb" {
			t.Errorf("%s recorded database %q, want the datasource's %q", tt.what, recorded, "testdb")
		}
	}
}

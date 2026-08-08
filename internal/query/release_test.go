package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/audit"
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/driver"
	mysqldriver "github.com/whg517/sqlflow/internal/driver/mysql"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/security"
	"github.com/whg517/sqlflow/internal/testutil"
)

// releaseFixture is a query Service wired to a target database it can actually
// read, which is what makes an end-to-end masking assertion possible.
//
// The existing suite never injects a driver on the ExecuteQuery path — all
// thirteen calls in query_test.go assert an error — so nothing proved that a
// real driver's rows come back masked. That is the gap these tests close.
type releaseFixture struct {
	svc    *Service
	wrap   *db.DB
	testDB *sql.DB
	dsID   int64
}

func newReleaseFixture(t *testing.T) *releaseFixture {
	t.Helper()

	testDB := testutil.NewDB(t).DB
	seedCasbinRules(t, testDB)
	wrap := testutil.WrapSQL(t, testDB)

	connMgr := connpool.NewManager()
	poolMgr := driver.NewPoolManager()
	dsSvc := datasource.NewService(wrap, testutil.EncryptionKey, connMgr, poolMgr, auditlog.Discard)

	ds := &model.DataSource{
		Name: "release-ds", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret", Database: "testdb",
	}
	if err := dsSvc.CreateDataSource(context.Background(), ds); err != nil {
		t.Fatalf("seed datasource: %v", err)
	}

	// The pool is fed the test PostgreSQL itself, so a SELECT reaches real rows.
	poolMgr.InjectForTest(ds.ID, mysqldriver.NewWithDB(testDB))

	permSvc, err := security.NewService(wrap)
	if err != nil {
		t.Fatalf("permission service: %v", err)
	}
	svc := NewService(wrap, dsSvc, NewHistoryService(wrap), permSvc,
		audit.NewService(wrap, 0, 0), testutil.EncryptionKey, poolMgr, Limits{})
	if _, err := testDB.Exec(
		`CREATE TABLE people (id INT, phone TEXT)`); err != nil {
		t.Fatalf("create target table: %v", err)
	}
	if _, err := testDB.Exec(
		`INSERT INTO people (id, phone) VALUES (1, '13812345678')`); err != nil {
		t.Fatalf("seed target row: %v", err)
	}

	return &releaseFixture{svc: svc, wrap: wrap, testDB: testDB, dsID: ds.ID}
}

// maskPhoneOn adds a rule that masks people.phone for this datasource.
func (f *releaseFixture) maskPhoneOn(t *testing.T) {
	t.Helper()
	now := time.Now()
	if _, err := f.testDB.Exec(
		`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type,
		                         custom_regex, custom_template, created_at, updated_at)
		 VALUES ($1, 'testdb', 'people', 'phone', 'phone', '', '', $2, $3)`,
		f.dsID, now, now,
	); err != nil {
		t.Fatalf("seed mask rule: %v", err)
	}
}

// TestExecuteQuery_MasksRealDriverRows is the assertion the suite never made.
//
// Masking was only ever exercised by calling the unexported helpers directly,
// which proves the helpers work but not that anything calls them. This runs the
// public entrance against a live driver and reads the value the caller receives.
func TestExecuteQuery_MasksRealDriverRows(t *testing.T) {
	f := newReleaseFixture(t)
	f.maskPhoneOn(t)

	result, err := f.svc.ExecuteQuery(context.Background(), 1, "user1", "developer",
		f.dsID, "testdb", "SELECT id, phone FROM people", "mysql")
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(result.Rows))
	}

	phone := fmt.Sprint(result.Rows[0]["phone"])
	if phone == "13812345678" {
		t.Error("phone came back in the clear despite a matching mask rule")
	}
	if !strings.Contains(phone, "*") {
		t.Errorf("phone = %q, want a masked value", phone)
	}
	if !result.Desensitized {
		t.Error("Desensitized = false, want true")
	}
}

// TestExecuteQuery_RefusesWhenMaskRulesCannotBeLoaded pins the fail-closed rule.
//
// loadMaskRules returned []mask.Rule with no error and answered nil when the
// read failed, and its own comment conceded the consequence: callers treat an
// empty rule set as "nothing to protect", so one failed read served the rows in
// the clear. A result that cannot be shown to be safe must be refused, not
// returned — invariant 4 does not have an availability exception.
func TestExecuteQuery_RefusesWhenMaskRulesCannotBeLoaded(t *testing.T) {
	f := newReleaseFixture(t)
	f.maskPhoneOn(t)

	// Break the rule store the way an outage would, after the datasource read
	// that happens earlier in the same request has already succeeded.
	if _, err := f.testDB.Exec(`DROP TABLE mask_rules`); err != nil {
		t.Fatalf("drop mask_rules: %v", err)
	}

	result, err := f.svc.ExecuteQuery(context.Background(), 1, "user1", "developer",
		f.dsID, "testdb", "SELECT id, phone FROM people", "mysql")
	if err == nil {
		got := ""
		if len(result.Rows) > 0 {
			got = fmt.Sprint(result.Rows[0]["phone"])
		}
		t.Fatalf("ExecuteQuery succeeded with the rule store unreadable, returning phone=%q", got)
	}
	if !errors.Is(err, ErrMaskRulesUnavailable) {
		t.Errorf("err = %v, want ErrMaskRulesUnavailable", err)
	}
}

// TestExplainQuery_RefusesInternalDatasourceForNonAdmin closes the one gate
// ExplainQuery was missing.
//
// ExecuteQuery and ExportQuery both refuse the platform's own metadata store to
// anyone but an admin; the plan entrance did not, and the shipped seed policy
// grants `developer` select on every object, so nothing else stood there. The
// rows a plan returns are query-plan metadata rather than table data, so this
// is an exposure of the platform's own schema rather than of user data — but it
// is an entrance that skipped a check its siblings apply, which is exactly the
// shape that makes a fifth entrance dangerous.
func TestExplainQuery_RefusesInternalDatasourceForNonAdmin(t *testing.T) {
	f := newReleaseFixture(t)

	if _, err := f.testDB.Exec(
		`UPDATE datasources SET extra_config = '{"system":true}' WHERE id = $1`, f.dsID,
	); err != nil {
		t.Fatalf("mark datasource internal: %v", err)
	}

	_, err := f.svc.ExplainQuery(context.Background(), 1, "developer",
		f.dsID, "testdb", "SELECT id FROM people")
	if !errors.Is(err, ErrInternalDatasourceOnly) {
		t.Errorf("developer: err = %v, want ErrInternalDatasourceOnly", err)
	}

	// The gate is about role, not about the entrance being unusable.
	if _, err := f.svc.ExplainQuery(context.Background(), 1, "admin",
		f.dsID, "testdb", "SELECT id FROM people"); errors.Is(err, ErrInternalDatasourceOnly) {
		t.Error("admin was refused the internal datasource")
	}
}

// TestShare_MasksOnRead is the point of giving a share a scope.
//
// A published result is read at /s/:token by someone who is not authenticated
// and holds no grant, so every rule matching its targets applies. Before the
// record carried a datasource, a database and a target list, none could be
// looked up: masking was not omitted here, it was unrepresentable, and
// CLAUDE.md's claim that query, export and share share one rule set was false
// for the third of them.
//
// The rows stored here are deliberately in the clear, which is the case the old
// design could not address at all — a publisher holding an unmask grant stored
// raw values and they went to the public URL exactly as they were.
func TestShare_MasksOnRead(t *testing.T) {
	f := newReleaseFixture(t)
	f.maskPhoneOn(t)

	shareSvc := NewShareService(f.wrap, "share-secret-at-least-32-bytes-long", f.svc, auditlog.Discard, Limits{})

	created, err := shareSvc.CreateShare(context.Background(), &CreateShareRequest{
		UserID: 1, Username: "user1", Role: "developer",
		Columns:      []string{"id", "phone"},
		Rows:         []map[string]interface{}{{"id": 1, "phone": "13812345678"}},
		ExpiresAt:    time.Now().Add(time.Hour),
		DatasourceID: f.dsID,
		SQLContent:   "SELECT id, phone FROM people",
	})
	if err != nil {
		t.Fatalf("CreateShare: %v", err)
	}

	pub, err := shareSvc.GetShare(context.Background(), created.Token, "")
	if err != nil {
		t.Fatalf("GetShare: %v", err)
	}
	if len(pub.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(pub.Rows))
	}

	phone := fmt.Sprint(pub.Rows[0]["phone"])
	if phone == "13812345678" {
		t.Error("the public link served the phone number in the clear")
	}
	if !strings.Contains(phone, "*") {
		t.Errorf("phone = %q, want a masked value", phone)
	}
}

// TestShare_SummaryIsServerDerived pins that the statement shown to an
// anonymous reader is not whatever the client chose to send.
//
// sql_summary was never a summary: the browser sent
// `currentSql.trim().substring(0, 200)` — the raw statement — and GetShare
// handed it to anyone with the link, while auditlog.Summarize existed the whole
// time and this path simply never called it.
func TestShare_SummaryIsServerDerived(t *testing.T) {
	f := newReleaseFixture(t)

	shareSvc := NewShareService(f.wrap, "share-secret-at-least-32-bytes-long", f.svc, auditlog.Discard, Limits{})
	raw := "SELECT id, phone FROM people WHERE phone = '13812345678' /* " +
		strings.Repeat("x", 300) + " */"

	created, err := shareSvc.CreateShare(context.Background(), &CreateShareRequest{
		UserID: 1, Username: "user1", Role: "developer",
		Columns:      []string{"id"},
		Rows:         []map[string]interface{}{{"id": 1}},
		ExpiresAt:    time.Now().Add(time.Hour),
		DatasourceID: f.dsID,
		SQLContent:   raw,
	})
	if err != nil {
		t.Fatalf("CreateShare: %v", err)
	}

	pub, err := shareSvc.GetShare(context.Background(), created.Token, "")
	if err != nil {
		t.Fatalf("GetShare: %v", err)
	}
	if want := auditlog.Summarize(raw); pub.SQLSummary != want {
		t.Errorf("SQLSummary = %q, want the server-derived %q", pub.SQLSummary, want)
	}
	if pub.SQLSummary == raw {
		t.Error("the raw statement was published verbatim")
	}
}

// TestShare_RefusesATableTheActorCannotRead pins that publishing authorizes on
// its own account.
//
// Sharing is a second entrance to the same rows. Invariant 1 gives it no
// exemption, and the grant that produced the result can have been withdrawn
// since it ran.
func TestShare_RefusesATableTheActorCannotRead(t *testing.T) {
	f := newReleaseFixture(t)

	// A role the seed grants nothing to.
	//
	// Deleting the developer policy instead would prove nothing: the enforcer
	// loads its rules into memory when the service is constructed, so a DELETE
	// afterwards leaves the decision unchanged — the first version of this test
	// did exactly that and passed against a caller who was still permitted.
	shareSvc := NewShareService(f.wrap, "share-secret-at-least-32-bytes-long", f.svc, auditlog.Discard, Limits{})
	_, err := shareSvc.CreateShare(context.Background(), &CreateShareRequest{
		UserID: 1, Username: "user1", Role: "guest",
		Columns:      []string{"id", "phone"},
		Rows:         []map[string]interface{}{{"id": 1, "phone": "13812345678"}},
		ExpiresAt:    time.Now().Add(time.Hour),
		DatasourceID: f.dsID,
		SQLContent:   "SELECT id, phone FROM people",
	})
	if err == nil {
		t.Error("a caller with no select grant published the table anyway")
	}
}

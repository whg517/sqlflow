package ticket

import (
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/audit"
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/driver"
	mysqldriver "github.com/whg517/sqlflow/internal/driver/mysql"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/crypto"
	"github.com/whg517/sqlflow/internal/testutil"
)

// Shared fixtures for tests that exercise real ticket execution: the scheduler
// path and the post-approval SQL integrity check.

// ticketExecTestKey is a 16-byte AES-128 key used to encrypt the test
// datasource password so that executeSQL can decrypt it.
const ticketExecTestKey = "0123456789abcdef"

// ticketExecDatabase is the logical database name shared by the test tickets
// and the injected target connection. connpool keys its pool by
// (datasource id, host, port, database), so the two must agree.
const ticketExecDatabase = "app"

// setupTicketExecTest wires a Service that can actually execute a ticket.
// The datasource password is decryptable and the target connection is
// pre-injected into the pool, so execution touches no network: another SQLite
// database stands in for the governed target.
//
// The platform DB comes from testutil, which uses the same db.Open as
// production — including SetMaxOpenConns(1). That single-connection pool is
// what makes read/write interleaving bugs observable.
func setupTicketExecTest(t *testing.T) (platform *db.DB, ticketSvc *Service, target *db.DB) {
	t.Helper()

	platform = testutil.NewDB(t)
	auditSvc := audit.NewService(platform, 0, 0)

	encPassword, err := crypto.Encrypt("pw", ticketExecTestKey)
	if err != nil {
		t.Fatalf("encrypt datasource password: %v", err)
	}
	var dsID int64
	if err := platform.QueryRow(
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		"test-ds", "mysql", "127.0.0.1", 3306, "root", encPassword, "active",
	).Scan(&dsID); err != nil {
		t.Fatalf("insert datasource: %v", err)
	}

	target = testutil.NewDB(t)
	if _, err := target.Exec(`CREATE TABLE demo (n INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create demo table: %v", err)
	}
	if _, err := target.Exec(`INSERT INTO demo (n) VALUES (0)`); err != nil {
		t.Fatalf("seed demo table: %v", err)
	}

	connMgr := connpool.NewManager()
	poolMgr := driver.NewPoolManager()
	// Stand in for the governed target: execution runs through the driver, so
	// the driver is what gets injected.
	poolMgr.InjectForTest(dsID, mysqldriver.NewWithDB(target.DB), &driver.Config{ID: dsID, Database: ticketExecDatabase})

	dsSvc := datasource.NewService(platform, ticketExecTestKey, connMgr, poolMgr)
	ticketSvc = New(Deps{
		DB:            platform,
		Audit:         auditSvc,
		Datasource:    dsSvc,
		PoolManager:   poolMgr,
		EncryptionKey: ticketExecTestKey,
	})

	return platform, ticketSvc, target
}

// insertTicket inserts a ticket in the given status and returns its ID.
// A zero scheduledAt is stored as NULL.
func insertTicket(t *testing.T, platform *db.DB, status model.TicketStatus, sqlContent string, scheduledAt time.Time) int64 {
	t.Helper()

	var scheduled any
	if !scheduledAt.IsZero() {
		scheduled = scheduledAt
	}

	now := time.Now()
	var id int64
	if err := platform.QueryRow(
		`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary, db_type,
		                      change_reason, status, risk_level, scheduled_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
		1, 1, ticketExecDatabase, sqlContent, sqlContent, "mysql",
		"ticket execution test", status, "low", scheduled, now, now,
	).Scan(&id); err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	return id
}

func ticketStatus(t *testing.T, platform *db.DB, id int64) string {
	t.Helper()
	var status string
	if err := platform.QueryRow(`SELECT status FROM tickets WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("read ticket #%d status: %v", id, err)
	}
	return status
}

func demoValue(t *testing.T, target *db.DB) int {
	t.Helper()
	var n int
	if err := target.QueryRow(`SELECT n FROM demo`).Scan(&n); err != nil {
		t.Fatalf("read demo.n: %v", err)
	}
	return n
}

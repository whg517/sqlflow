package datasource

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/testutil"
)

// recordingAudit collects what a service wrote, so a test can assert on the
// evidence rather than on the operation's return value.
type recordingAudit struct {
	mu      sync.Mutex
	records []auditlog.Record
}

func (r *recordingAudit) Write(_ context.Context, rec auditlog.Record) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec)
}

func (r *recordingAudit) actions() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.records))
	for i, rec := range r.records {
		out[i] = rec.Action
	}
	return out
}

func (r *recordingAudit) find(action string) (auditlog.Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.records {
		if rec.Action == action {
			return rec, true
		}
	}
	return auditlog.Record{}, false
}

func newAuditedDatasourceService(t *testing.T) (*Service, *recordingAudit) {
	t.Helper()
	testDB := setupDatasourceTestDB(t)
	audit := &recordingAudit{}
	svc := NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, connpool.NewManager(), nil, audit)
	return svc, audit
}

func mysqlDatasource(name string) *model.DataSource {
	return &model.DataSource{
		Name: name, Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", PasswordEncrypted: "secret", Database: "app",
	}
}

// TestDatasourceLifecycleIsAudited pins invariant 3 for this domain.
//
// internal/datasource wrote no audit records at all: not for creating,
// updating, disabling or deleting a datasource, and not for a failed connection
// test. Those are the operations that decide which databases the platform can
// reach and with whose credentials, so "who pointed this at that host" had no
// answer beyond process logs.
func TestDatasourceLifecycleIsAudited(t *testing.T) {
	svc, audit := newAuditedDatasourceService(t)
	ctx := t.Context()
	const operator = int64(7)
	ctx = auditlog.WithActor(ctx, operator)

	ds := mysqlDatasource("app-mysql")
	if err := svc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("create: %v", err)
	}

	updated := *ds
	updated.Host = "10.0.0.2"
	if err := svc.UpdateDataSource(ctx, ds.ID, &updated); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := svc.DisableDataSource(ctx, ds.ID); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := svc.DeleteDataSource(ctx, ds.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	for _, want := range []string{
		"datasource_create", "datasource_update", "datasource_disable", "datasource_delete",
	} {
		rec, ok := audit.find(want)
		if !ok {
			t.Errorf("no %s record; got %v", want, audit.actions())
			continue
		}
		if rec.DatasourceID != ds.ID {
			t.Errorf("%s recorded datasource %d, want %d", want, rec.DatasourceID, ds.ID)
		}
		if rec.UserID != operator {
			t.Errorf("%s recorded user %d, want %d", want, rec.UserID, operator)
		}
	}
}

// TestFailedConnectionTestIsAudited is the case the invariant names directly.
//
// A connection that cannot be established is exactly the event an operator
// needs later, and it was the one leaving no trace.
func TestFailedConnectionTestIsAudited(t *testing.T) {
	svc, audit := newAuditedDatasourceService(t)
	ctx := auditlog.WithActor(t.Context(), 7)

	// Port 1 refuses immediately, so this fails without waiting on a network
	// timeout.
	ds := mysqlDatasource("unreachable")
	ds.Host = "127.0.0.1"
	ds.Port = 1

	if err := svc.TestConnection(ctx, ds); err == nil {
		t.Fatal("connecting to a closed port succeeded")
	}

	rec, ok := audit.find("datasource_test_connection_failed")
	if !ok {
		t.Fatalf("a failed connection test left no record; got %v", audit.actions())
	}
	if rec.ErrorMessage == "" {
		t.Error("the record carries no error message, which is the part an operator reads")
	}
	if strings.Contains(rec.ErrorMessage, "secret") {
		t.Errorf("the record leaks the password: %s", rec.ErrorMessage)
	}
}

// TestSuccessfulConnectionTestIsNotAudited keeps the volume meaningful: an
// audit log that records every keystroke of a form is one nobody reads.
func TestSuccessfulConnectionTestIsNotAudited(t *testing.T) {
	svc, audit := newAuditedDatasourceService(t)
	ctx := auditlog.WithActor(t.Context(), 7)

	ds := mysqlDatasource("unreachable")
	ds.Host = "127.0.0.1"
	ds.Port = 1
	_ = svc.TestConnection(ctx, ds)

	for _, action := range audit.actions() {
		if action == "datasource_test_connection" {
			t.Error("a successful-connection action was recorded for a failed test")
		}
	}
}

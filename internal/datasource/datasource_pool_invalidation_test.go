package datasource

import (
	"testing"

	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/testutil"
)

// unconnectedDriver is a real driver that was never connected, so Close on
// eviction is a no-op rather than a nil dereference.
func unconnectedDriver(t *testing.T) driver.Driver {
	t.Helper()
	d, err := driver.NewDriver("sqlite")
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	return d
}

// newInvalidationTestService wires both caches, since the point is that a write
// has to reach all of them.
func newInvalidationTestService(t *testing.T) (*Service, *connpool.Manager, *driver.PoolManager) {
	t.Helper()
	testDB := setupDatasourceTestDB(t)
	connMgr := connpool.NewManager()
	poolMgr := driver.NewPoolManager()
	svc := NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, connMgr, poolMgr, auditlog.Discard)
	return svc, connMgr, poolMgr
}

// seedInvalidationESDatasource stores an ES datasource and primes both caches
// for it, as a live process would have after one query and one index browse.
func seedInvalidationESDatasource(t *testing.T, svc *Service, connMgr *connpool.Manager, poolMgr *driver.PoolManager) *model.DataSource {
	t.Helper()
	ds := &model.DataSource{
		Name: "logs-es", Type: "elasticsearch",
		Host: "elasticsearch", Port: 9200,
		ESUrls: "https://es.example.com:9200", ESAuthType: "basic",
		Username: "elastic", PasswordEncrypted: "old-password",
		ESVerifyCerts: true,
	}
	if err := svc.CreateDataSource(t.Context(), ds); err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	connMgr.InjectESForTest(ds.ID, []string{"https://es.example.com:9200"}, nil)
	poolMgr.InjectForTest(ds.ID, unconnectedDriver(t))
	return ds
}

// TestUpdateDropsBothCachedConnections is the invariant a write has to hold:
// after it, nothing is still talking to the datasource on the old terms.
//
// removeDatasourcePool only cleared driver.PoolManager. The Elasticsearch
// client that index and field browsing use lives in connpool, and
// RemoveElasticsearch had no caller at all outside its own test — so rotating
// an ES password left that client authenticating with the old one until the
// process restarted.
//
// The cache key is datasource id plus URLs and deliberately carries no
// credentials, so changing a password does not change the key. Invalidating on
// write is the fix; putting secrets in map keys would not be.
func TestUpdateDropsBothCachedConnections(t *testing.T) {
	svc, connMgr, poolMgr := newInvalidationTestService(t)
	ds := seedInvalidationESDatasource(t, svc, connMgr, poolMgr)

	updated := *ds
	updated.PasswordEncrypted = "rotated-password"
	if err := svc.UpdateDataSource(t.Context(), ds.ID, &updated); err != nil {
		t.Fatalf("update datasource: %v", err)
	}

	if ids := connMgr.CachedESIDs(); len(ids) != 0 {
		t.Errorf("elasticsearch clients still cached for %v — index browsing keeps the old credentials", ids)
	}
	if ids := poolMgr.ManagedIDs(); len(ids) != 0 {
		t.Errorf("driver connections still cached for %v", ids)
	}
}

// TestDeleteDropsBothCachedConnections covers the other write.
//
// A client left behind after a delete is a dangling reference: the datasource
// it belongs to no longer exists, so nothing will ever evict it.
func TestDeleteDropsBothCachedConnections(t *testing.T) {
	svc, connMgr, poolMgr := newInvalidationTestService(t)
	ds := seedInvalidationESDatasource(t, svc, connMgr, poolMgr)

	// Deleting requires disabling first, and disabling evicts too — so prime
	// the caches again afterwards to isolate what the delete itself does.
	if err := svc.DisableDataSource(t.Context(), ds.ID); err != nil {
		t.Fatalf("disable datasource: %v", err)
	}
	connMgr.InjectESForTest(ds.ID, []string{"https://es.example.com:9200"}, nil)
	poolMgr.InjectForTest(ds.ID, unconnectedDriver(t))

	if err := svc.DeleteDataSource(t.Context(), ds.ID); err != nil {
		t.Fatalf("delete datasource: %v", err)
	}

	if ids := connMgr.CachedESIDs(); len(ids) != 0 {
		t.Errorf("elasticsearch clients survived the delete: %v", ids)
	}
	if ids := poolMgr.ManagedIDs(); len(ids) != 0 {
		t.Errorf("driver connections survived the delete: %v", ids)
	}
}

// TestInvalidationLeavesOtherDatasourcesAlone guards against the eviction being
// applied so broadly that every write dropped everyone's connections.
func TestInvalidationLeavesOtherDatasourcesAlone(t *testing.T) {
	svc, connMgr, poolMgr := newInvalidationTestService(t)
	target := seedInvalidationESDatasource(t, svc, connMgr, poolMgr)

	if err := svc.DisableDataSource(t.Context(), target.ID); err != nil {
		t.Fatalf("disable datasource: %v", err)
	}

	const bystander = int64(9999)
	connMgr.InjectESForTest(bystander, []string{"https://other.example.com:9200"}, nil)
	poolMgr.InjectForTest(bystander, unconnectedDriver(t))

	if err := svc.DeleteDataSource(t.Context(), target.ID); err != nil {
		t.Fatalf("delete datasource: %v", err)
	}

	if ids := connMgr.CachedESIDs(); len(ids) != 1 || ids[0] != bystander {
		t.Errorf("cached elasticsearch ids = %v, want just the bystander %d", ids, bystander)
	}
	if ids := poolMgr.ManagedIDs(); len(ids) != 1 || ids[0] != bystander {
		t.Errorf("managed driver ids = %v, want just the bystander %d", ids, bystander)
	}
}

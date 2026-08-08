package datasource

import (
	"testing"

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

// newInvalidationTestService wires the pool a write has to reach.
//
// There used to be two caches, and the point of these tests was that a write
// reached both. The second — an Elasticsearch client that index browsing
// reached through connpool — is gone, so the invariant is the same and there is
// one place to check it.
func newInvalidationTestService(t *testing.T) (*Service, *driver.PoolManager) {
	t.Helper()
	testDB := setupDatasourceTestDB(t)
	poolMgr := driver.NewPoolManager()
	svc := NewService(testutil.WrapSQL(t, testDB), testutil.EncryptionKey, poolMgr, auditlog.Discard)
	return svc, poolMgr
}

// seedInvalidationESDatasource stores an ES datasource and primes the pool for
// it, as a live process would have after one query.
func seedInvalidationESDatasource(t *testing.T, svc *Service, poolMgr *driver.PoolManager) *model.DataSource {
	t.Helper()
	ds := &model.DataSource{
		Name: "logs-es", Type: "elasticsearch",
		Host: "elasticsearch", Port: 9200,
		ExtraConfig: `{"urls":["https://es.example.com:9200"],"auth_type":"basic","verify_certs":true}`,
		Username:    "elastic", PasswordEncrypted: "old-password",
	}
	if err := svc.CreateDataSource(t.Context(), ds); err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	poolMgr.InjectForTest(ds.ID, unconnectedDriver(t))
	return ds
}

// TestUpdateDropsTheCachedConnection is the invariant a write has to hold:
// after it, nothing is still talking to the datasource on the old terms.
//
// removeDatasourcePool once cleared only part of what was cached: a second
// Elasticsearch client, reached through connpool by index browsing, kept
// authenticating with a rotated password until the process restarted. That
// client no longer exists, so this asserts the property on the cache that does.
//
// The cache key is datasource id plus URLs and deliberately carries no
// credentials, so changing a password does not change the key. Invalidating on
// write is the fix; putting secrets in map keys would not be.
func TestUpdateDropsTheCachedConnection(t *testing.T) {
	svc, poolMgr := newInvalidationTestService(t)
	ds := seedInvalidationESDatasource(t, svc, poolMgr)

	updated := *ds
	updated.PasswordEncrypted = "rotated-password"
	if err := svc.UpdateDataSource(t.Context(), ds.ID, &updated); err != nil {
		t.Fatalf("update datasource: %v", err)
	}

	if ids := poolMgr.ManagedIDs(); len(ids) != 0 {
		t.Errorf("driver connections still cached for %v", ids)
	}
}

// TestDeleteDropsTheCachedConnection covers the other write.
//
// A client left behind after a delete is a dangling reference: the datasource
// it belongs to no longer exists, so nothing will ever evict it.
func TestDeleteDropsTheCachedConnection(t *testing.T) {
	svc, poolMgr := newInvalidationTestService(t)
	ds := seedInvalidationESDatasource(t, svc, poolMgr)

	// Deleting requires disabling first, and disabling evicts too — so prime
	// the caches again afterwards to isolate what the delete itself does.
	if err := svc.DisableDataSource(t.Context(), ds.ID); err != nil {
		t.Fatalf("disable datasource: %v", err)
	}
	poolMgr.InjectForTest(ds.ID, unconnectedDriver(t))

	if err := svc.DeleteDataSource(t.Context(), ds.ID); err != nil {
		t.Fatalf("delete datasource: %v", err)
	}

	if ids := poolMgr.ManagedIDs(); len(ids) != 0 {
		t.Errorf("driver connections survived the delete: %v", ids)
	}
}

// TestInvalidationLeavesOtherDatasourcesAlone guards against the eviction being
// applied so broadly that every write dropped everyone's connections.
func TestInvalidationLeavesOtherDatasourcesAlone(t *testing.T) {
	svc, poolMgr := newInvalidationTestService(t)
	target := seedInvalidationESDatasource(t, svc, poolMgr)

	if err := svc.DisableDataSource(t.Context(), target.ID); err != nil {
		t.Fatalf("disable datasource: %v", err)
	}

	const bystander = int64(9999)
	poolMgr.InjectForTest(bystander, unconnectedDriver(t))

	if err := svc.DeleteDataSource(t.Context(), target.ID); err != nil {
		t.Fatalf("delete datasource: %v", err)
	}

	if ids := poolMgr.ManagedIDs(); len(ids) != 1 || ids[0] != bystander {
		t.Errorf("managed driver ids = %v, want just the bystander %d", ids, bystander)
	}
}

package driver_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/driver"
)

// countingDriver records how many instances were connected and closed.
//
// Counting is the only way to see the defect from outside: a leaked pool is
// still perfectly usable, it is just unreachable, so nothing in the manager's
// public surface reports it.
type countingDriver struct {
	mockDriver
	connects *atomic.Int64
	closes   *atomic.Int64
}

func (c *countingDriver) Connect(ctx context.Context, cfg *driver.Config) error {
	// Connecting takes time against a real database, and that time is the
	// window the old check-then-act left open: callers that all missed the
	// cache were all inside Connect at once, and the last write won. A
	// rendezvous barrier would be exact but would deadlock once the fix means
	// fewer callers reach this point.
	time.Sleep(10 * time.Millisecond)
	c.connects.Add(1)
	return nil
}

func (c *countingDriver) Close() error {
	c.closes.Add(1)
	return nil
}

var (
	poolTestConnects atomic.Int64
	poolTestCloses   atomic.Int64
)

func init() {
	driver.Register("counting", func() driver.Driver {
		return &countingDriver{
			mockDriver: mockDriver{typ: "counting"},
			connects:   &poolTestConnects,
			closes:     &poolTestCloses,
		}
	})
}

func countingConfig(id int64) *driver.Config {
	return &driver.Config{ID: id, Extra: map[string]interface{}{"_type": "counting"}}
}

// TestPoolManagerConcurrentGetLeaksNothing pins the invariant a connection pool
// exists to provide: every connection it opens is either reachable or closed.
//
// Get read the map under a read lock, released it, connected, then wrote the
// entry back unconditionally. With N goroutines missing the cache together, N
// drivers were connected and N-1 were overwritten — each holding up to MaxOpen
// TCP connections that Remove and Close could no longer reach, because neither
// can walk to an entry that is not in the map. Saving a datasource, which
// evicts the entry, is exactly what produces the simultaneous miss.
func TestPoolManagerConcurrentGetLeaksNothing(t *testing.T) {
	poolTestConnects.Store(0)
	poolTestCloses.Store(0)

	pm := driver.NewPoolManager()
	cfg := countingConfig(1)

	const callers = 16
	var ready, done sync.WaitGroup
	start := make(chan struct{})
	ready.Add(callers)
	done.Add(callers)

	errs := make([]error, callers)
	for i := range callers {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			_, errs[i] = pm.Get(t.Context(), cfg)
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
	}

	pm.Close()

	connects := poolTestConnects.Load()
	closes := poolTestCloses.Load()
	if connects != closes {
		t.Errorf("connected %d drivers but closed %d — %d were overwritten and can no longer be reached",
			connects, closes, connects-closes)
	}
}

// TestPoolManagerConcurrentGetReturnsOneDriver checks callers agree on which
// connection they got.
//
// Two callers holding different drivers for one datasource is not just wasteful:
// transactions, session state and prepared statements stop being shared the way
// a pool implies they are.
func TestPoolManagerConcurrentGetReturnsOneDriver(t *testing.T) {
	poolTestConnects.Store(0)
	poolTestCloses.Store(0)

	pm := driver.NewPoolManager()
	defer pm.Close()
	cfg := countingConfig(2)

	const callers = 16
	got := make([]driver.Driver, callers)
	var ready, done sync.WaitGroup
	start := make(chan struct{})
	ready.Add(callers)
	done.Add(callers)

	for i := range callers {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			d, err := pm.Get(t.Context(), cfg)
			if err != nil {
				t.Errorf("caller %d: %v", i, err)
				return
			}
			got[i] = d
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()

	for i, d := range got {
		if d != got[0] {
			t.Fatalf("caller %d holds a different driver than caller 0 — the pool handed out %d connections for one datasource",
				i, poolTestConnects.Load())
		}
	}
}

// TestPoolManagerConcurrentAccessIsRaceFree exercises the read paths together.
//
// Run under -race this fails on any unsynchronised write to a shared entry.
// The manager used to touch a bookkeeping timestamp from Get after releasing
// the read lock, and from GetCached while only holding it.
func TestPoolManagerConcurrentAccessIsRaceFree(t *testing.T) {
	pm := driver.NewPoolManager()
	defer pm.Close()
	pm.InjectForTest(3, &mockDriver{typ: "mock"})

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = pm.GetCached(3)
			_ = pm.ManagedIDs()
		}()
	}
	wg.Wait()
}

// TestPoolManagerGetRejectsConfigWithoutType covers the entry point's only
// unchecked assumption.
//
// Get asserted cfg.Extra["_type"].(string) without the comma-ok form. Every
// production caller goes through BuildConfigFromDataSource, which always sets
// it, but a public method that panics on a hand-built Config is a trap rather
// than an API.
func TestPoolManagerGetRejectsConfigWithoutType(t *testing.T) {
	pm := driver.NewPoolManager()
	defer pm.Close()

	if _, err := pm.Get(t.Context(), &driver.Config{ID: 4}); err == nil {
		t.Error("a config with no _type was accepted")
	}
	if _, err := pm.Get(t.Context(), &driver.Config{ID: 5, Extra: map[string]interface{}{"_type": 42}}); err == nil {
		t.Error("a config whose _type is not a string was accepted")
	}
}

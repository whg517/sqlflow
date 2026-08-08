package query

import (
	"context"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/testutil"
)

// blockingDriver never returns from a query until its context is done.
//
// It stands in for the real thing this test is about: a datasource that
// accepts the connection and then takes longer than anyone is willing to wait.
// A driver that returned promptly could not tell whether the platform imposed
// a deadline or merely happened not to need one.
type blockingDriver struct{}

func (b *blockingDriver) Type() string                { return "blocking" }
func (b *blockingDriver) QueryForm() driver.QueryForm { return driver.QueryFormSQL }
func (b *blockingDriver) ConfigSchema() []driver.ConfigField {
	return []driver.ConfigField{{Name: "host", Label: "Host", Kind: "text", Storage: driver.StorageColumn}}
}
func (b *blockingDriver) Connect(context.Context, *driver.Config) error { return nil }
func (b *blockingDriver) Close() error                                  { return nil }
func (b *blockingDriver) Ping(context.Context) error                    { return nil }

func (b *blockingDriver) ExecuteQuery(ctx context.Context, _ string, _ int) (*driver.QueryResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingDriver) Parse(string) (*driver.ParseResult, error) {
	return &driver.ParseResult{Operation: driver.OpSelect, RiskLevel: driver.RiskLow}, nil
}

func init() {
	driver.Register("blocking", func() driver.Driver { return &blockingDriver{} })
}

// TestExecuteQueryHonorsTheConfiguredTimeout pins that the platform, not the
// driver, bounds how long a query may run.
//
// The 30-second bound used to cover only poolMgr.Get; the execution itself got
// the caller's unbounded context, on the theory — stated in a comment — that
// "the driver bounds the execution itself". Three of the five drivers do not.
// PostgreSQL sets no query deadline at all and MySQL's Timeout is the dial
// timeout, so a slow query on either ran until the client gave up, holding a
// pooled connection the whole time. The DeadlineExceeded branch in the caller
// could never fire either, because the context it tested had no deadline.
func TestExecuteQueryHonorsTheConfiguredTimeout(t *testing.T) {
	testDB := testutil.NewDB(t)
	svc := newQueryServiceWithLimits(t, testDB, Limits{Timeout: 200 * time.Millisecond})

	uid := testutil.SeedUser(t, testDB.DB, "timeout_user", "admin")
	dsID := testutil.SeedDatasourceOfType(t, testDB.DB, "blocking-ds", "blocking")

	// Generous next to the 200ms limit: if the service imposes its own bound the
	// call returns long before this, and if it does not the elapsed time makes
	// that unmistakable rather than hanging the suite.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	_, err := svc.ExecuteQuery(ctx, uid, "timeout_user", "admin", dsID,
		testutil.DatasourceDatabase, "SELECT 1", "blocking")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a query that never returns was reported as successful")
	}
	if elapsed > 3*time.Second {
		t.Errorf("query took %v; the configured 200ms timeout was not applied to execution", elapsed)
	}
}

// TestLimitsDefaultsAreAppliedToTheZeroValue keeps an omitted config section
// from meaning "no timeout and no row cap".
//
// Zero is the value viper produces for a key nobody wrote, so treating it
// literally would turn every unconfigured deployment into an unbounded one —
// the opposite of what leaving a limit unset asks for.
func TestLimitsDefaultsAreAppliedToTheZeroValue(t *testing.T) {
	got := Limits{}.withDefaults()

	if got.Timeout <= 0 {
		t.Errorf("Timeout = %v, want a positive default", got.Timeout)
	}
	if got.MaxRows <= 0 {
		t.Errorf("MaxRows = %d, want a positive default", got.MaxRows)
	}
	if got.ExportMaxRows <= 0 {
		t.Errorf("ExportMaxRows = %d, want a positive default", got.ExportMaxRows)
	}
	if got.ShareMaxRows <= 0 {
		t.Errorf("ShareMaxRows = %d, want a positive default", got.ShareMaxRows)
	}
}

// TestLimitsKeepConfiguredValues is the other half: a deployment that does set
// a limit must get the one it set, not the default.
func TestLimitsKeepConfiguredValues(t *testing.T) {
	got := Limits{
		Timeout:       5 * time.Second,
		MaxRows:       10,
		ExportMaxRows: 20,
		ShareMaxRows:  30,
	}.withDefaults()

	if got.Timeout != 5*time.Second || got.MaxRows != 10 ||
		got.ExportMaxRows != 20 || got.ShareMaxRows != 30 {
		t.Errorf("withDefaults overwrote configured values: %+v", got)
	}
}

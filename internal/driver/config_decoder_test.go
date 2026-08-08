package driver_test

import (
	"context"
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
)

// clickhouseish is a driver that needs configuration nobody else does.
//
// It stands in for the next data source type. The question this file asks is
// whether such a type can carry its own settings without editing anything
// outside its own package — that was the promise, and BuildConfigFromDataSource
// used to break it with a `switch dsType` naming elasticsearch and mongodb.
type clickhouseish struct{ driver.Driver }

func (clickhouseish) Type() string                { return "clickhouseish" }
func (clickhouseish) QueryForm() driver.QueryForm { return driver.QueryFormSQL }

// DecodeConfig reads the keys this driver understands, and nothing else knows.
func (clickhouseish) DecodeConfig(cfg *driver.Config, extra map[string]any) error {
	if v, ok := extra["cluster"].(string); ok {
		cfg.Extra["cluster"] = v
	}
	// A default the driver owns: no other layer knows what it should be.
	if _, ok := cfg.Extra["secure"]; !ok {
		secure := true
		if v, ok := extra["secure"].(bool); ok {
			secure = v
		}
		cfg.Extra["secure"] = secure
	}
	return nil
}

func (clickhouseish) Connect(context.Context, *driver.Config) error { return nil }
func (clickhouseish) Close() error                                  { return nil }
func (clickhouseish) Ping(context.Context) error                    { return nil }
func (clickhouseish) Parse(string) (*driver.ParseResult, error) {
	return &driver.ParseResult{Operation: driver.OpSelect}, nil
}

func (clickhouseish) ExecuteQuery(context.Context, string, int) (*driver.QueryResult, error) {
	return nil, nil
}

// fakeDataSource is the minimum a caller has to supply.
type fakeDataSource struct {
	typ   string
	extra string
}

func (f fakeDataSource) GetID() int64           { return 7 }
func (f fakeDataSource) GetType() string        { return f.typ }
func (f fakeDataSource) GetHost() string        { return "h" }
func (f fakeDataSource) GetPort() int           { return 1 }
func (f fakeDataSource) GetUsername() string    { return "u" }
func (f fakeDataSource) GetDatabase() string    { return "d" }
func (f fakeDataSource) GetSSLMode() string     { return "" }
func (f fakeDataSource) GetSchemaName() string  { return "" }
func (f fakeDataSource) GetMaxOpen() int        { return 0 }
func (f fakeDataSource) GetMaxIdle() int        { return 0 }
func (f fakeDataSource) GetMaxLifetime() int    { return 0 }
func (f fakeDataSource) GetMaxIdleTime() int    { return 0 }
func (f fakeDataSource) GetExtraConfig() string { return f.extra }

// TestANewTypeCarriesItsOwnConfig is the change-surface promise, made testable.
//
// Adding a data source type is supposed to touch two places: implement Driver,
// register it. Configuration broke that — the shared builder held a
// `switch dsType` with a branch per driver, so a new type's settings had to be
// added to a file in this package, and Elasticsearch's went further still, into
// six dedicated database columns, the model, the adapter, the request structs
// and the form.
//
// Nothing in this test names elasticsearch or mongodb, and nothing outside the
// fake driver knows what "cluster" means.
func TestANewTypeCarriesItsOwnConfig(t *testing.T) {
	driver.Register("clickhouseish", func() driver.Driver { return clickhouseish{} })

	cfg, err := driver.BuildConfigFromDataSource(
		fakeDataSource{typ: "clickhouseish", extra: `{"cluster":"analytics","secure":false}`},
		driver.Secrets{Password: "p"},
	)
	if err != nil {
		t.Fatalf("BuildConfigFromDataSource: %v", err)
	}

	if got := cfg.Extra["cluster"]; got != "analytics" {
		t.Errorf("cluster = %v, want analytics — the driver's own key did not arrive", got)
	}
	if got := cfg.Extra["secure"]; got != false {
		t.Errorf("secure = %v, want false", got)
	}
}

// TestADriverOwnsItsDefaults checks the other direction: a key the caller left
// out gets whatever the driver decides, not a zero value chosen by a layer that
// does not know the field.
func TestADriverOwnsItsDefaults(t *testing.T) {
	driver.Register("clickhouseish2", func() driver.Driver { return clickhouseish{} })

	cfg, err := driver.BuildConfigFromDataSource(
		fakeDataSource{typ: "clickhouseish2", extra: `{}`},
		driver.Secrets{},
	)
	if err != nil {
		t.Fatalf("BuildConfigFromDataSource: %v", err)
	}
	if got := cfg.Extra["secure"]; got != true {
		t.Errorf("secure = %v, want true — the driver's default did not apply", got)
	}
}

// TestADriverWithoutExtraConfigStillBuilds keeps the interface optional: most
// drivers need nothing beyond host, port and credentials.
func TestADriverWithoutExtraConfigStillBuilds(t *testing.T) {
	cfg, err := driver.BuildConfigFromDataSource(
		fakeDataSource{typ: "mysql"}, driver.Secrets{Password: "p"},
	)
	if err != nil {
		t.Fatalf("BuildConfigFromDataSource: %v", err)
	}
	if cfg.Password != "p" {
		t.Errorf("Password = %q, want p", cfg.Password)
	}
}

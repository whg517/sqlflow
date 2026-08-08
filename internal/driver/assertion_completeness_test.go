package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/all"
)

// optionalInterfaces is every contract a driver may satisfy structurally.
//
// The satisfies func is the authority — it asks the type system — and the name
// is what the assertion block must spell out.
var optionalInterfaces = []struct {
	name      string
	satisfies func(driver.Driver) bool
}{
	{"MetadataBrowser", func(d driver.Driver) bool { _, ok := d.(driver.MetadataBrowser); return ok }},
	{"StatementExecutor", func(d driver.Driver) bool { _, ok := d.(driver.StatementExecutor); return ok }},
	{"ParameterizedQueryExecutor", func(d driver.Driver) bool { _, ok := d.(driver.ParameterizedQueryExecutor); return ok }},
	{"ParameterBinder", func(d driver.Driver) bool { _, ok := d.(driver.ParameterBinder); return ok }},
	{"QueryExplainer", func(d driver.Driver) bool { _, ok := d.(driver.QueryExplainer); return ok }},
	{"ConfigValidator", func(d driver.Driver) bool { _, ok := d.(driver.ConfigValidator); return ok }},
	{"ConfigDecoder", func(d driver.Driver) bool { _, ok := d.(driver.ConfigDecoder); return ok }},
	{"StatementSplitter", func(d driver.Driver) bool { _, ok := d.(driver.StatementSplitter); return ok }},
}

// TestEveryDriverAssertsWhatItSatisfies makes the assertion convention binding.
//
// CLAUDE.md requires each driver to state its contracts as compile-time
// assertions, because optional interfaces are satisfied structurally: a method
// that is renamed or never lands does not break the build, it just makes the
// capability silently report false. A refactor once dropped two ExplainQuery
// methods that way and the only symptom was every datasource reporting
// explain=false.
//
// The convention was itself unenforced, and had already drifted — Elasticsearch
// implemented all of MetadataBrowser and asserted none of it, the one incomplete
// block of the five. A convention that only a human checks is a convention that
// eventually is not checked, so this test does it: what the type system says a
// driver satisfies, its source must say too.
func TestEveryDriverAssertsWhatItSatisfies(t *testing.T) {
	// Package directory per registered type. Adding a driver means adding a line
	// here, which is the point: an unlisted driver would be silently unchecked.
	packages := map[string]string{
		"mysql":         "mysql",
		"postgresql":    "postgresql",
		"sqlite":        "sqlite",
		"mongodb":       "mongodb",
		"elasticsearch": "elasticsearch",
	}

	root := driverRoot(t)

	for _, typ := range productionDriverTypes {
		t.Run(typ, func(t *testing.T) {
			dir, ok := packages[typ]
			if !ok {
				t.Fatalf("driver %q is registered but has no package mapping here", typ)
			}

			d, err := driver.NewDriver(typ)
			if err != nil {
				t.Fatalf("NewDriver(%s): %v", typ, err)
			}
			source := packageSource(t, filepath.Join(root, dir))

			for _, iface := range optionalInterfaces {
				if !iface.satisfies(d) {
					continue
				}
				// The assertion may be written with any amount of alignment
				// padding, so match on the two ends rather than the whole line.
				needle := "_ driver." + iface.name
				if !strings.Contains(source, needle) {
					t.Errorf("satisfies driver.%s but never asserts it;\n"+
						"  add `%s = (*%sDriver)(nil)` to the compile-time assertion block",
						iface.name, needle, strings.ToUpper(typ[:1])+typ[1:])
				}
			}
		})
	}
}

// packageSource concatenates the non-test Go source of a package.
func packageSource(t *testing.T, dir string) string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	var b strings.Builder
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		b.Write(content)
	}
	return b.String()
}

// driverRoot locates internal/driver from the test's working directory.
func driverRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if filepath.Base(dir) != "driver" {
		t.Fatalf("expected to run inside internal/driver, got %s", dir)
	}
	return dir
}

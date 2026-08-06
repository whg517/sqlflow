package driver_test

import (
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/all"
)

// TestDescribeType_QueryForms pins the axis that decides which editor the UI
// shows. Both Elasticsearch and MySQL are queryable, which is why "queryable"
// was never the useful distinction — the two are composed differently and must
// be distinguishable without the UI hardcoding type names.
func TestDescribeType_QueryForms(t *testing.T) {
	tests := []struct {
		typeName string
		want     driver.QueryForm
	}{
		{"mysql", driver.QueryFormSQL},
		{"postgresql", driver.QueryFormSQL},
		{"sqlite", driver.QueryFormSQL},
		{"mongodb", driver.QueryFormDocument},
		{"elasticsearch", driver.QueryFormDSL},
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			desc, err := driver.DescribeType(tt.typeName)
			if err != nil {
				t.Fatalf("DescribeType(%q): %v", tt.typeName, err)
			}
			if desc.QueryForm != tt.want {
				t.Errorf("QueryForm = %q, want %q", desc.QueryForm, tt.want)
			}
			if desc.Type != tt.typeName {
				t.Errorf("Type = %q, want %q", desc.Type, tt.typeName)
			}
		})
	}
}

// TestDescribeType_Capabilities checks the descriptor against what the drivers
// implement.
//
// It used to assert seven hand-declared bits. Five were declared identically by
// every driver or contradicted by the driver's own behavior, so asserting them
// only restated the source file being tested; the remaining two are now derived
// from MetadataBrowser and StatementExecutor, so this asserts a fact the
// compiler already maintains rather than a declaration someone could forget.
func TestDescribeType_Capabilities(t *testing.T) {
	es, err := driver.DescribeType("elasticsearch")
	if err != nil {
		t.Fatalf("DescribeType(elasticsearch): %v", err)
	}
	if es.TicketExec {
		t.Error("elasticsearch must not declare ticket_exec — it has no DML/DDL path")
	}
	if !es.Metadata {
		t.Error("elasticsearch should declare metadata — it lists indices and fields")
	}

	mysql, err := driver.DescribeType("mysql")
	if err != nil {
		t.Fatalf("DescribeType(mysql): %v", err)
	}
	if !mysql.TicketExec {
		t.Error("mysql should declare ticket_exec")
	}
	if !mysql.Metadata {
		t.Error("mysql should declare metadata")
	}
}

// TestDescribeType_Parameterized reflects the optional-interface capability.
// MySQL and PostgreSQL bind parameters; Elasticsearch does not.
func TestDescribeType_Parameterized(t *testing.T) {
	for _, tt := range []struct {
		typeName string
		want     bool
	}{
		{"mysql", true},
		{"postgresql", true},
		{"elasticsearch", false},
	} {
		desc, err := driver.DescribeType(tt.typeName)
		if err != nil {
			t.Fatalf("DescribeType(%q): %v", tt.typeName, err)
		}
		if desc.Parameterized != tt.want {
			t.Errorf("%s: Parameterized = %v, want %v", tt.typeName, desc.Parameterized, tt.want)
		}
	}
}

func TestDescribeType_UnknownType(t *testing.T) {
	if _, err := driver.DescribeType("oracle"); err == nil {
		t.Fatal("DescribeType(oracle) should fail for an unregistered type")
	}
}

// TestDescribeType_Explain guards the capability the query workbench reads to
// decide whether to offer a query plan.
//
// It exists because a refactor once dropped both ExplainQuery methods without
// breaking the build: the interface is optional, so the only symptom was every
// data source quietly reporting explain=false. Asserting the descriptor catches
// what a compile cannot.
func TestDescribeType_Explain(t *testing.T) {
	for typeName, want := range map[string]bool{
		"mysql":         true,
		"postgresql":    true,
		"sqlite":        false,
		"mongodb":       false,
		"elasticsearch": false,
	} {
		desc, err := driver.DescribeType(typeName)
		if err != nil {
			t.Fatalf("DescribeType(%q): %v", typeName, err)
		}
		if desc.Explain != want {
			t.Errorf("%s: Explain = %v, want %v", typeName, desc.Explain, want)
		}
	}
}

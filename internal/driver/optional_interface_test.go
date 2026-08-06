package driver_test

import (
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/all"
)

// TestStatementExecutionIsAnInterfaceNotAFlag is the Liskov fix, stated as a
// test.
//
// ExecuteStatement and ExecuteStatements were mandatory on Driver, so SQLite
// and Elasticsearch had to supply bodies that only ever return "not supported".
// A value that cannot honor part of the contract it satisfies is not a
// substitute for it, and nothing but a hand-written CapTicketExec check stood
// between a caller and those stubs. Whether a driver can run statements is
// structural — the method is there or it is not — so the type system can decide
// it, exactly as it already does for QueryExplainer.
func TestStatementExecutionIsAnInterfaceNotAFlag(t *testing.T) {
	tests := []struct {
		typ     string
		execute bool
	}{
		{"mysql", true},
		{"postgresql", true},
		{"mongodb", true},
		{"sqlite", false},
		{"elasticsearch", false},
	}

	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			d, err := driver.NewDriver(tt.typ)
			if err != nil {
				t.Fatalf("NewDriver: %v", err)
			}

			_, ok := d.(driver.StatementExecutor)
			if ok != tt.execute {
				t.Errorf("StatementExecutor = %v, want %v", ok, tt.execute)
			}

			// Whatever the type system says has to match what the endpoint
			// reports, or the UI and the executor disagree about the same driver.
			desc := driver.Describe(d)
			if desc.TicketExec != tt.execute {
				t.Errorf("Descriptor.TicketExec = %v, want %v", desc.TicketExec, tt.execute)
			}
		})
	}
}

// TestMetadataBrowsingIsAnInterfaceNotAFlag covers the other half of the split.
//
// CapMetadata was declared by all five drivers, so the check that read it could
// never fire — the guard existed but gated nothing. Moving it to an interface
// keeps the answer and makes it something the compiler maintains.
func TestMetadataBrowsingIsAnInterfaceNotAFlag(t *testing.T) {
	for _, typ := range productionDriverTypes {
		t.Run(typ, func(t *testing.T) {
			d, err := driver.NewDriver(typ)
			if err != nil {
				t.Fatalf("NewDriver: %v", err)
			}
			if _, ok := d.(driver.MetadataBrowser); !ok {
				t.Error("does not satisfy MetadataBrowser, but metadata browsing is routed to it")
			}
			if !driver.Describe(d).Metadata {
				t.Error("Descriptor.Metadata is false for a driver that browses metadata")
			}
		})
	}
}

// TestDriverInterfaceHoldsOnlyUniversalMethods keeps the base contract honest.
//
// A method belongs on Driver only if every driver can mean it. The moment one
// has to answer "not supported", the method is describing a subset and belongs
// in an optional interface — otherwise the compiler is satisfied and the caller
// is not.
func TestDriverInterfaceHoldsOnlyUniversalMethods(t *testing.T) {
	for _, typ := range productionDriverTypes {
		d, err := driver.NewDriver(typ)
		if err != nil {
			t.Fatalf("NewDriver(%s): %v", typ, err)
		}

		// Type, QueryForm and Parse need no connection and must answer for real
		// on every driver.
		if d.Type() != typ {
			t.Errorf("%s: Type() = %q", typ, d.Type())
		}
		if d.QueryForm() == "" {
			t.Errorf("%s: QueryForm() is empty", typ)
		}
	}
}

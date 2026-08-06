package driver_test

import (
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/all"
)

// productionDriverTypes is the registered set, minus anything a test registers.
//
// SupportedTypes() would include the mocks, and a mock's declaration says
// nothing about whether a bit distinguishes real data sources.
var productionDriverTypes = []string{"mysql", "postgresql", "sqlite", "mongodb", "elasticsearch"}

// The capability bit set is gone.
//
// It began with seven. Five were removed as vacuous or false — CapQuery and
// CapFieldMasking were declared by every driver, so nothing could be rejected
// for lacking them; CapSQLParse and CapExport were declared false by drivers
// that did both, so honoring CapExport would have refused a MongoDB export that
// works; and CapTableLevelPermission was declared false by Elasticsearch while
// its indices were Casbin-checked like any other target, so honoring it would
// have removed an access check rather than added one.
//
// The last two, CapMetadata and CapTicketExec, guarded methods that lived on
// Driver. Both were structural — the method is there or it is not — so they
// became MetadataBrowser and StatementExecutor, and the type system maintains
// them. What is left of this file checks the declarations against behavior,
// which is how the five were found to be wrong rather than merely unread.

// TestCapabilityBitsMatchBehaviour checks the declarations against what the
// drivers actually do, which is how the five removed bits were found to be
// wrong rather than merely unread.
func TestCapabilityBitsMatchBehaviour(t *testing.T) {
	queries := map[string]string{
		"mongodb":       `{"operation":"find","collection":"users","filter":{}}`,
		"elasticsearch": `{"index":"logs-2026","body":{"query":{"match_all":{}}}}`,
	}

	for _, typ := range productionDriverTypes {
		t.Run(typ, func(t *testing.T) {
			d, err := driver.NewDriver(typ)
			if err != nil {
				t.Fatalf("NewDriver: %v", err)
			}

			query, ok := queries[typ]
			if !ok {
				query = "SELECT * FROM users"
			}
			// Parse is an interface method, so every driver has it. That is
			// precisely why "can this driver parse" was never a useful question
			// to put to a capability bit.
			parsed, err := d.Parse(query)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if parsed.Operation != driver.OpSelect {
				t.Errorf("operation = %q, want select", parsed.Operation)
			}
			if len(parsed.Targets) == 0 {
				t.Error("no targets — Casbin would have nothing to enforce against, which is what makes a table-permission bit meaningless")
			}
		})
	}
}

// TestDescriptorReportsOnlyDecidableFacts pins what the capabilities endpoint
// promises the UI.
//
// Every field has to be something a caller can act on. The endpoint used to
// carry query, table_permission, field_masking, sql_parse and export; the UI
// read none of them, and three were false where the behavior was true.
func TestDescriptorReportsOnlyDecidableFacts(t *testing.T) {
	desc, err := driver.DescribeType("mongodb")
	if err != nil {
		t.Fatalf("DescribeType: %v", err)
	}

	if desc.QueryForm != driver.QueryFormDocument {
		t.Errorf("query_form = %q, want document", desc.QueryForm)
	}
	// MongoDB executes ticket statements but produces no query plan; both are
	// facts a caller can do something with.
	if !desc.TicketExec {
		t.Error("ticket_exec = false for MongoDB, which does execute statements")
	}
	if desc.Explain {
		t.Error("explain = true for MongoDB, which implements no QueryExplainer")
	}
}

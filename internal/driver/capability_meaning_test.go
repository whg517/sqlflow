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

// TestEveryCapabilityBitIsFalsifiable is the rule that keeps the set honest: a
// bit no driver ever turns off is a constant wearing a flag's clothes.
//
// Five bits failed it and are gone. CapQuery and CapFieldMasking were declared
// by every driver, so nothing could ever be rejected for lacking them.
// CapSQLParse and CapExport were declared false by drivers that demonstrably
// did both — Parse returns an operation and targets for MongoDB and
// Elasticsearch alike, and the export path is driver-agnostic — so honoring
// them would have refused a MongoDB export that works.
//
// CapTableLevelPermission was worse than vacuous. Elasticsearch declared false
// while its indices were being Casbin-checked like any other target, so
// honoring it would have removed an access check rather than added one. It
// also asked the wrong party: table permissions are enforced by Casbin over
// Parse's targets, and masking by platform/mask over the result — neither needs
// anything from the driver, and whether a result can be masked at all is what
// ResultShape already answers.
//
// CapMetadata is the known exception. It is equally vacuous — every driver
// declares it — but unlike the five it has an enforcement point
// (datasource.metadataDriver), so deleting it here would drop that answer with
// nothing in its place. ListTables and GetColumns are interface methods, which
// by the rule stated on QueryExplainer makes this structural: it becomes a
// MetadataBrowser optional interface, and that is the same change that splits
// ExecuteStatement out of Driver. Until then the bit stays and this test says
// why rather than pretending it passes.
func TestEveryCapabilityBitIsFalsifiable(t *testing.T) {
	bits := []struct {
		name string
		cap  driver.Capability
	}{
		{"CapTicketExec", driver.CapTicketExec},
	}

	for _, bit := range bits {
		var declared int
		for _, typ := range productionDriverTypes {
			d, err := driver.NewDriver(typ)
			if err != nil {
				t.Fatalf("NewDriver(%s): %v", typ, err)
			}
			if d.Capabilities().Has(bit.cap) {
				declared++
			}
		}
		switch declared {
		case len(productionDriverTypes):
			t.Errorf("%s is declared by all %d drivers — it can never reject anything, so it carries no information",
				bit.name, len(productionDriverTypes))
		case 0:
			t.Errorf("%s is declared by no driver — it gates a feature nothing provides", bit.name)
		}
	}
}

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

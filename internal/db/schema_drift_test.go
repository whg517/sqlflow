package db_test

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"slices"

	"github.com/whg517/sqlflow/internal/testutil"
)

// TestEntSchemaMatchesMigrations verifies that every column ent declares exists
// in the migrated database.
//
// ent auto-migration is deliberately not enabled — the migration files are the
// single source of truth — so the two definitions are kept in step by hand, and
// that is precisely the arrangement that drifts. Before this check existed, the
// only signal was a query failing at runtime against a column that was never
// created, or a type mismatch that surfaced on the first write.
//
// The table and column names come from ent's own generated metadata rather than
// a list maintained here, so adding an entity cannot leave the check behind.
func TestEntSchemaMatchesMigrations(t *testing.T) {
	entTables := readEntMetadata(t)
	if len(entTables) == 0 {
		t.Fatal("no ent metadata found — the generated packages moved?")
	}

	d := testutil.NewDB(t)
	actual := map[string]string{}
	rows, err := d.QueryContext(context.Background(), `
		SELECT table_name, column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = current_schema()`)
	if err != nil {
		t.Fatalf("read columns: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var table, column, dataType string
		if err := rows.Scan(&table, &column, &dataType); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		actual[table+"."+column] = dataType
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate columns: %v", err)
	}
	if len(actual) == 0 {
		t.Fatal("the test schema has no columns — migration did not run")
	}

	var missing []string
	for table, columns := range entTables {
		for _, c := range columns {
			if _, ok := actual[table+"."+c]; !ok {
				missing = append(missing, table+"."+c)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("ent declares %d column(s) the migration does not create:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}

	// The other direction matters just as much, and was the gap that let
	// sla_config.auto_reject_enabled sit as BIGINT for as long as it did: the
	// column existed and Go treated it as a bool, but no ent field described it,
	// so nothing compared the two.
	var unmodelled []string
	for key := range actual {
		table := strings.SplitN(key, ".", 2)[0]
		columns, known := entTables[table]
		if !known {
			continue // a table with no entity at all is a separate decision
		}
		if !slices.Contains(columns, strings.SplitN(key, ".", 2)[1]) {
			unmodelled = append(unmodelled, key)
		}
	}
	sort.Strings(unmodelled)
	if len(unmodelled) > 0 {
		t.Errorf("the migration creates %d column(s) no ent field describes:\n  %s",
			len(unmodelled), strings.Join(unmodelled, "\n  "))
	}
}

// TestBooleanColumnsAreBoolean checks the drift that costs the most to find
// later.
//
// SQLite stored booleans as INTEGER, so code and schema could disagree for
// years without complaint. PostgreSQL rejects an integer written to a boolean
// column — but only when that write happens, which for a rarely-exercised flag
// can be long after the change that broke it.
func TestBooleanColumnsAreBoolean(t *testing.T) {
	d := testutil.NewDB(t)

	// Declared in the ent schemas as field.Bool.
	boolColumns := []string{
		"api_tokens.is_active",
		"approval_policies.auto_approve_enabled",
		"approval_policies.enabled",
		"approval_policies.is_default",
		"approval_records.auto_approved",
		"datasources.es_verify_certs",
		"feishu_webhooks.enabled",
		"oidc_providers.enabled",
		"refresh_tokens.revoked",
		"roles.is_builtin",
		"shared_results.revoked",
		"sla_config.auto_reject_enabled",
		"sla_config.enabled",
		"sql_templates.is_public",
		"tickets.auto_approved",
		"webhook_subscriptions.enabled",
	}

	for _, col := range boolColumns {
		parts := strings.SplitN(col, ".", 2)
		var dataType string
		err := d.QueryRow(`
			SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`,
			parts[0], parts[1],
		).Scan(&dataType)
		if err != nil {
			t.Errorf("%s: %v", col, err)
			continue
		}
		if dataType != "boolean" {
			t.Errorf("%s is %q, want boolean", col, dataType)
		}
	}
}

// readEntMetadata pulls each entity's Table and Columns out of the generated
// packages by parsing them.
//
// Parsing rather than importing: importing every generated package here would
// make this test depend on all of them, and the values are plain constants and
// string slices that the AST exposes directly.
func readEntMetadata(t *testing.T) map[string][]string {
	t.Helper()
	root := entRoot(t)

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read ent dir: %v", err)
	}

	result := map[string][]string{}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "schema" || e.Name() == "enttest" ||
			e.Name() == "migrate" || e.Name() == "runtime" || e.Name() == "predicate" ||
			e.Name() == "hook" || e.Name() == "internal" {
			continue
		}
		metaFile := filepath.Join(root, e.Name(), e.Name()+".go")
		if _, err := os.Stat(metaFile); err != nil {
			continue
		}
		table, columns := parseEntMeta(t, metaFile)
		if table != "" && len(columns) > 0 {
			result[table] = columns
		}
	}
	return result
}

// parseEntMeta extracts the Table constant and the Columns slice, resolving the
// Field* constants the slice refers to.
func parseEntMeta(t *testing.T, path string) (string, []string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	consts := map[string]string{}
	var table string
	var columnIdents []string

	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if i >= len(spec.Values) {
				continue
			}
			switch v := spec.Values[i].(type) {
			case *ast.BasicLit:
				if v.Kind != token.STRING {
					continue
				}
				s, err := strconv.Unquote(v.Value)
				if err != nil {
					continue
				}
				consts[name.Name] = s
				if name.Name == "Table" {
					table = s
				}
			case *ast.CompositeLit:
				if name.Name != "Columns" {
					continue
				}
				for _, el := range v.Elts {
					if id, ok := el.(*ast.Ident); ok {
						columnIdents = append(columnIdents, id.Name)
					}
				}
			}
		}
		return true
	})

	columns := make([]string, 0, len(columnIdents))
	for _, id := range columnIdents {
		if v, ok := consts[id]; ok {
			columns = append(columns, v)
		}
	}
	return table, columns
}

// entRoot locates internal/db/ent relative to this test's working directory.
func entRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Join(dir, "ent")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("ent package not found at %s: %v", root, err)
	}
	return root
}

var _ = fmt.Sprint // keep fmt available for future assertions

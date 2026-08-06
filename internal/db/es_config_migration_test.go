package db_test

import (
	"testing"

	"github.com/whg517/sqlflow/internal/testutil"
)

// TestESConfigBackfill checks that a datasource written before the migration
// keeps working after it.
//
// The five Elasticsearch settings moved from dedicated columns into the
// extra_config JSON object. If the backfill drops or mistypes any of them the
// symptom is not a failed migration — it is a datasource that starts refusing
// connections, on the error path, after an upgrade.
//
// The migration runs against a schema that no longer has the columns, so the
// row is written the way the old schema would have and the assertions read what
// the driver would decode.
func TestESConfigBackfill(t *testing.T) {
	database := testutil.NewDB(t)

	// Re-create the shape the old schema had, so the backfill has something to
	// find. The columns are dropped by the migration under test, which has
	// already run by the time NewDB returns.
	if _, err := database.DB.Exec(`
		ALTER TABLE datasources
			ADD COLUMN IF NOT EXISTS es_urls TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS es_version TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS es_auth_type TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS es_index_pattern TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS es_verify_certs BOOLEAN NOT NULL DEFAULT TRUE`,
	); err != nil {
		t.Fatalf("restore legacy columns: %v", err)
	}

	seed := func(name, urls, authType, indexPattern string, verify bool, extra string) {
		t.Helper()
		if _, err := database.DB.Exec(
			`INSERT INTO datasources
			 (name, type, host, port, username, password_encrypted, database, status,
			  es_urls, es_version, es_auth_type, es_index_pattern, es_verify_certs, extra_config,
			  created_at, updated_at)
			 VALUES ($1, 'elasticsearch', 'elasticsearch', 9200, 'u', '', '', 'active',
			         $2, '8.x', $3, $4, $5, $6, now(), now())`,
			name, urls, authType, indexPattern, verify, extra,
		); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	seed("two-urls", "https://es1:9200, https://es2:9200", "basic", "logs-*", true, "")
	seed("verify-off", "https://es.lab:9200", "none", "", false, "")
	seed("already-has-extra", "https://es3:9200", "api_key", "", true, `{"note":"prod","auth_type":"basic"}`)

	// Run the migration, which is what an upgrade executes.
	if _, err := database.DB.Exec(esConfigMigrationSQL); err != nil {
		t.Fatalf("migration: %v", err)
	}

	// Running it again must be a no-op rather than an error: the test fixtures
	// build schemas with ApplySchema, which does not record versions, so the
	// versioned migrator re-runs every file afterwards.
	if _, err := database.DB.Exec(esConfigMigrationSQL); err != nil {
		t.Fatalf("migration is not re-runnable: %v", err)
	}

	read := func(name string) map[string]interface{} {
		t.Helper()
		var raw string
		if err := database.DB.QueryRow(
			`SELECT extra_config FROM datasources WHERE name = $1`, name,
		).Scan(&raw); err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return decodeJSON(t, raw)
	}

	t.Run("urls become an array", func(t *testing.T) {
		got := read("two-urls")
		urls, ok := got["urls"].([]interface{})
		if !ok {
			t.Fatalf("urls = %#v, want an array", got["urls"])
		}
		if len(urls) != 2 || urls[0] != "https://es1:9200" || urls[1] != "https://es2:9200" {
			t.Errorf("urls = %v, want the two entries trimmed", urls)
		}
		if got["auth_type"] != "basic" {
			t.Errorf("auth_type = %v, want basic", got["auth_type"])
		}
		if got["index_pattern"] != "logs-*" {
			t.Errorf("index_pattern = %v, want logs-*", got["index_pattern"])
		}
		if got["verify_certs"] != true {
			t.Errorf("verify_certs = %v, want true", got["verify_certs"])
		}
	})

	t.Run("verify_certs false survives", func(t *testing.T) {
		got := read("verify-off")
		if got["verify_certs"] != false {
			t.Errorf("verify_certs = %v, want false — a deliberate opt-out was lost",
				got["verify_certs"])
		}
	})

	t.Run("empty settings are omitted rather than stored blank", func(t *testing.T) {
		got := read("verify-off")
		if _, present := got["index_pattern"]; present {
			t.Error("index_pattern was stored empty; absent and blank must stay distinguishable")
		}
	})

	t.Run("existing extra_config wins", func(t *testing.T) {
		got := read("already-has-extra")
		if got["note"] != "prod" {
			t.Errorf("note = %v, want prod — an unrelated key was dropped", got["note"])
		}
		if got["auth_type"] != "basic" {
			t.Errorf("auth_type = %v, want basic — the column overwrote what the user wrote",
				got["auth_type"])
		}
	})
}

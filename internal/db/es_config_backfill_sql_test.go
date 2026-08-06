package db_test

import (
	"encoding/json"
	"testing"
)

// esConfigMigrationSQL is migration 000003 verbatim, minus comments.
//
// The test seeds rows in the pre-migration shape and runs it, so the assertions
// cover the same statements an upgrade executes rather than a paraphrase of
// them.
const esConfigMigrationSQL = `DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'datasources'
          AND column_name = 'es_urls'
    ) THEN
        RETURN;
    END IF;

    UPDATE datasources
    SET extra_config = (
        jsonb_build_object(
            'urls', CASE
                WHEN coalesce(es_urls, '') = '' THEN NULL
                ELSE to_jsonb(
                    array_remove(
                        array(SELECT btrim(unnest(string_to_array(es_urls, ',')))),
                        ''
                    )
                )
            END,
            'auth_type', nullif(es_auth_type, ''),
            'index_pattern', nullif(es_index_pattern, ''),
            'version', nullif(es_version, ''),
            'verify_certs', to_jsonb(es_verify_certs)
        )
        || coalesce(nullif(extra_config, '')::jsonb, '{}'::jsonb)
    )::text
    WHERE type = 'elasticsearch';

    UPDATE datasources
    SET extra_config = (jsonb_strip_nulls(extra_config::jsonb))::text
    WHERE type = 'elasticsearch' AND coalesce(extra_config, '') <> '';

    ALTER TABLE datasources
        DROP COLUMN es_urls,
        DROP COLUMN es_version,
        DROP COLUMN es_auth_type,
        DROP COLUMN es_index_pattern,
        DROP COLUMN es_verify_certs;
END
$$;`

func decodeJSON(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode extra_config %q: %v", raw, err)
	}
	return out
}

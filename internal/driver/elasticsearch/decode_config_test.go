package elasticsearch

import (
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
)

// TestDecodeURLsAcceptsBothShapes covers the migration boundary.
//
// urls used to be a comma-separated string in a dedicated es_urls column; it is
// a JSON array in extra_config now. A datasource written before the migration
// and one written after have to resolve to the same list, or connections break
// on upgrade for anyone whose row the backfill did not reach.
func TestDecodeURLsAcceptsBothShapes(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  int
	}{
		{"array", []interface{}{"https://es1:9200", "https://es2:9200"}, 2},
		{"legacy comma string", "https://es1:9200,https://es2:9200", 2},
		{"legacy string with spaces", "https://es1:9200 , https://es2:9200 ", 2},
		{"single", []interface{}{"https://es:9200"}, 1},
		{"empty string", "", 0},
		{"only commas", ",,", 0},
		{"absent", nil, 0},
		{"array with blanks", []interface{}{"https://es:9200", "", "  "}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decodeURLs(tt.value); len(got) != tt.want {
				t.Errorf("decodeURLs(%v) = %v, want %d entries", tt.value, got, tt.want)
			}
		})
	}
}

// TestDecodeConfigDefaultsVerifyCertsOn pins the default at the layer that owns
// it.
//
// It used to be a column default that the insert overrode, and an adapter that
// ignored the default it was handed — so an omitted field disabled certificate
// verification. The driver decides now, and the answer is on.
func TestDecodeConfigDefaultsVerifyCertsOn(t *testing.T) {
	d := &ESDriver{}

	cfg := &driver.Config{Extra: map[string]interface{}{}}
	if err := d.DecodeConfig(cfg, map[string]interface{}{}); err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if cfg.Extra["verify_certs"] != true {
		t.Errorf("verify_certs = %v, want true", cfg.Extra["verify_certs"])
	}

	cfg = &driver.Config{Extra: map[string]interface{}{}}
	if err := d.DecodeConfig(cfg, map[string]interface{}{"verify_certs": false}); err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if cfg.Extra["verify_certs"] != false {
		t.Error("verify_certs = true although extra_config said false")
	}
}

// TestDecodeConfigDefaultsAuthTypeToNone keeps an unconfigured datasource from
// being treated as basic auth with empty credentials, which fails at the
// cluster with a message about authentication rather than about configuration.
func TestDecodeConfigDefaultsAuthTypeToNone(t *testing.T) {
	d := &ESDriver{}
	cfg := &driver.Config{Extra: map[string]interface{}{}}
	if err := d.DecodeConfig(cfg, map[string]interface{}{}); err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if cfg.Extra["auth_type"] != "none" {
		t.Errorf("auth_type = %v, want none", cfg.Extra["auth_type"])
	}
}

package driver_test

import (
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
)

// esDataSource is an Elasticsearch datasource whose settings live where they
// live now: in the extra_config object, decoded by the driver.
func esDataSource(extraConfig string) *mockDataSource {
	return &mockDataSource{
		dsType:      "elasticsearch",
		host:        "-", // the placeholder the handler writes for a non-null column
		port:        9200,
		extraConfig: extraConfig,
	}
}

// TestExtraConfigIsTheOnlySourceOfDriverSettings pins what replaced the
// columns.
//
// Five Elasticsearch settings used to live in dedicated columns, read through a
// key switch in the datasource adapter and merged with extra_config field by
// field in a `switch dsType` here. The merge existed because the two sources
// could disagree; with one source there is nothing to reconcile, and the
// defaults for absent keys belong to the driver.
//
// The case that mattered is the last one: an unrelated key must not disturb the
// rest. It used to, because the JSON and column branches were exclusive and any
// valid object — including {} — selected the JSON branch, dropping urls,
// auth_type and verify_certs together.
func TestExtraConfigIsTheOnlySourceOfDriverSettings(t *testing.T) {
	tests := []struct {
		name        string
		extraConfig string
		wantURLs    []string
		wantAuth    string
		wantVerify  bool
	}{
		{
			name:        "settings arrive as written",
			extraConfig: `{"urls":["https://es1:9200","https://es2:9200"],"auth_type":"basic","verify_certs":true}`,
			wantURLs:    []string{"https://es1:9200", "https://es2:9200"},
			wantAuth:    "basic",
			wantVerify:  true,
		},
		{
			name:        "absent settings take the driver's defaults",
			extraConfig: "",
			wantURLs:    nil,
			wantAuth:    "none",
			wantVerify:  true,
		},
		{
			name:        "an empty object is the same as none",
			extraConfig: `{}`,
			wantURLs:    nil,
			wantAuth:    "none",
			wantVerify:  true,
		},
		{
			name:        "an unrelated key disturbs nothing",
			extraConfig: `{"note":"prod","urls":["https://es1:9200"],"auth_type":"basic"}`,
			wantURLs:    []string{"https://es1:9200"},
			wantAuth:    "basic",
			wantVerify:  true,
		},
		{
			name:        "verify_certs false is honored",
			extraConfig: `{"urls":["https://es.lab:9200"],"auth_type":"none","verify_certs":false}`,
			wantURLs:    []string{"https://es.lab:9200"},
			wantAuth:    "none",
			wantVerify:  false,
		},
		{
			name:        "a legacy comma string still resolves",
			extraConfig: `{"urls":"https://es1:9200,https://es2:9200","auth_type":"basic"}`,
			wantURLs:    []string{"https://es1:9200", "https://es2:9200"},
			wantAuth:    "basic",
			wantVerify:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := driver.BuildConfigFromDataSource(esDataSource(tt.extraConfig), driver.Secrets{})
			if err != nil {
				t.Fatalf("BuildConfigFromDataSource: %v", err)
			}

			urls, _ := cfg.Extra["urls"].([]string)
			if len(urls) != len(tt.wantURLs) {
				t.Fatalf("urls = %v, want %v", urls, tt.wantURLs)
			}
			for i := range urls {
				if urls[i] != tt.wantURLs[i] {
					t.Errorf("urls[%d] = %q, want %q", i, urls[i], tt.wantURLs[i])
				}
			}
			if got := cfg.Extra["auth_type"]; got != tt.wantAuth {
				t.Errorf("auth_type = %v, want %v", got, tt.wantAuth)
			}
			if got := cfg.Extra["verify_certs"]; got != tt.wantVerify {
				t.Errorf("verify_certs = %v, want %v", got, tt.wantVerify)
			}
		})
	}
}

// TestAPIKeyNeverComesFromExtraConfig keeps the credential axis separate.
//
// extra_config holds what was written verbatim, and the stored copy of a secret
// is encrypted — so a key read from there would reach the cluster as ciphertext,
// which is exactly the failure that made connection tests pass while queries
// returned 401.
func TestAPIKeyNeverComesFromExtraConfig(t *testing.T) {
	ds := esDataSource(`{"urls":["https://es:9200"],"auth_type":"api_key","api_key":"ciphertext"}`)

	cfg, err := driver.BuildConfigFromDataSource(ds, driver.Secrets{APIKey: "plaintext"})
	if err != nil {
		t.Fatalf("BuildConfigFromDataSource: %v", err)
	}
	if got := cfg.Extra["api_key"]; got != "plaintext" {
		t.Errorf("api_key = %v, want the decrypted value from Secrets", got)
	}
}

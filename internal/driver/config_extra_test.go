package driver_test

import (
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
)

// esDataSource is an Elasticsearch datasource with the legacy columns filled,
// which is how every stored one looks.
func esDataSource(extraConfig string) *mockDataSource {
	return &mockDataSource{
		dsType: "elasticsearch",
		host:   "elasticsearch", // the placeholder the handler writes for ES
		port:   9200,
		extras: map[string]string{
			"es_urls":          "https://es1:9200,https://es2:9200",
			"es_auth_type":     "basic",
			"es_index_pattern": "logs-*",
		},
		extraBools:  map[string]bool{"es_verify_certs": true},
		extraConfig: extraConfig,
	}
}

// TestExtraConfigOverridesFieldByField pins the fallback.
//
// The JSON branch and the column branch used to be exclusive, and extraMap is
// non-nil for any valid JSON object — including {}. So writing an unrelated key
// into extra_config silently dropped urls, auth_type and verify_certs
// together. The driver then fell back to host and port, and host is the
// literal "elasticsearch" the handler stores to satisfy a NOT NULL column, so
// it went looking for http://elasticsearch:0 with no credentials.
//
// The comment said extra_config "takes priority"; the code made it take over.
func TestExtraConfigOverridesFieldByField(t *testing.T) {
	tests := []struct {
		name        string
		extraConfig string
		wantURLs    []string
		wantAuth    string
		wantVerify  bool
	}{
		{
			name:        "absent leaves the columns in charge",
			extraConfig: "",
			wantURLs:    []string{"https://es1:9200", "https://es2:9200"},
			wantAuth:    "basic",
			wantVerify:  true,
		},
		{
			name:        "empty object changes nothing",
			extraConfig: `{}`,
			wantURLs:    []string{"https://es1:9200", "https://es2:9200"},
			wantAuth:    "basic",
			wantVerify:  true,
		},
		{
			name:        "unrelated key changes nothing",
			extraConfig: `{"note":"prod"}`,
			wantURLs:    []string{"https://es1:9200", "https://es2:9200"},
			wantAuth:    "basic",
			wantVerify:  true,
		},
		{
			name:        "one key overrides only itself",
			extraConfig: `{"auth_type":"api_key"}`,
			wantURLs:    []string{"https://es1:9200", "https://es2:9200"},
			wantAuth:    "api_key",
			wantVerify:  true,
		},
		{
			name:        "every key overrides",
			extraConfig: `{"urls":"https://es9:9200","auth_type":"none","verify_certs":false}`,
			wantURLs:    []string{"https://es9:9200"},
			wantAuth:    "none",
			wantVerify:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := driver.BuildConfigFromDataSource(esDataSource(tt.extraConfig), driver.Secrets{Password: "p"})
			if err != nil {
				t.Fatalf("BuildConfigFromDataSource: %v", err)
			}

			urls, _ := cfg.Extra["urls"].([]string)
			if len(urls) != len(tt.wantURLs) {
				t.Fatalf("urls = %v, want %v", urls, tt.wantURLs)
			}
			for i, want := range tt.wantURLs {
				if urls[i] != want {
					t.Errorf("urls[%d] = %q, want %q", i, urls[i], want)
				}
			}
			if got := cfg.Extra["auth_type"]; got != tt.wantAuth {
				t.Errorf("auth_type = %v, want %q", got, tt.wantAuth)
			}
			if got := cfg.Extra["verify_certs"]; got != tt.wantVerify {
				t.Errorf("verify_certs = %v, want %v", got, tt.wantVerify)
			}
		})
	}
}

// TestExtraConfigRejectsMalformedJSON keeps a typo from being read as "no
// extra config at all", which would silently connect on the column values.
func TestExtractConfigRejectsMalformedJSON(t *testing.T) {
	if _, err := driver.BuildConfigFromDataSource(esDataSource(`{"urls":`), driver.Secrets{}); err == nil {
		t.Error("malformed extra_config was accepted")
	}
}

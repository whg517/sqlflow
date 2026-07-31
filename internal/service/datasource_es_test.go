package service

import (
	"testing"
)

// TestValidateESURLs_HTTPSEnforcement verifies HTTPS enforcement for public
// endpoints while allowing HTTP on private development networks.
func TestValidateESURLs_HTTPSEnforcement(t *testing.T) {
	tests := []struct {
		name    string
		urls    string
		wantErr bool
	}{
		{name: "https allowed", urls: "https://es.example.com:9200", wantErr: false},
		{name: "multiple https", urls: "https://es1:9200,https://es2:9200", wantErr: false},
		{name: "https with path", urls: "https://es.example.com:9200/elastic", wantErr: false},
		{name: "public http blocked", urls: "http://es.example.com:9200", wantErr: true},
		{name: "private ipv4 http allowed", urls: "http://10.168.106.114:9200", wantErr: false},
		{name: "loopback http allowed", urls: "http://127.0.0.1:9200", wantErr: false},
		{name: "localhost http allowed", urls: "http://localhost:9200", wantErr: false},
		{name: "mixed public http blocked", urls: "https://es1:9200,http://es.example.com:9200", wantErr: true},
		{name: "unsupported scheme", urls: "ftp://10.0.0.1:9200", wantErr: true},
		{name: "missing hostname", urls: "http://", wantErr: true},
		{name: "empty ok", urls: "", wantErr: false},
		{name: "whitespace only", urls: "  ", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateESURLs(tt.urls)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateESURLs(%q) error = %v, wantErr %v", tt.urls, err, tt.wantErr)
			}
		})
	}
}

// TestParseESUrls verifies URL parsing.
func TestParseESUrls(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{"single url", "https://es:9200", 1},
		{"two urls", "https://es1:9200,https://es2:9200", 2},
		{"with spaces", "https://es1:9200 , https://es2:9200 ", 2},
		{"empty", "", 0},
		{"only commas", ",,", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseESUrls(tt.raw)
			if len(got) != tt.want {
				t.Errorf("parseESUrls(%q) = %d urls, want %d", tt.raw, len(got), tt.want)
			}
		})
	}
}

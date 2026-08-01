package datasource

import (
	"testing"
)

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

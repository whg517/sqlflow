package datasource

import (
	"errors"
	"testing"
)

// TestResolveQueryScope pins the rule the query, export, explain and ticket
// paths all share: the scope is the datasource's, and it is the only value a
// caller can get back.
func TestResolveQueryScope(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		requested  string
		want       string
		wantErr    bool
	}{
		{"no request means the datasource's own", "app", "", "app", false},
		{"the datasource's own database", "app", "app", "app", false},
		{"case is not the caller's problem", "app", "APP", "app", false},
		{"surrounding space is not either", "app", "  app  ", "app", false},
		{"a foreign database is refused", "app", "prod", "", true},
		{"an unconfigured datasource still refuses one", "", "prod", "", true},
		{"an unconfigured datasource with no request is fine", "", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveQueryScope(tt.configured, tt.requested)
			if tt.wantErr {
				if !errors.Is(err, ErrDatabaseScopeMismatch) {
					t.Fatalf("error = %v, want ErrDatabaseScopeMismatch", err)
				}
				if got != "" {
					t.Errorf("scope = %q, want empty on refusal", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("scope = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveQueryScope_NeverReturnsTheRequest is the property behind the
// masking fix: whatever the caller sends, the value that comes back — and so
// the value that filters mask rules and lands in the audit row — is the
// datasource's.
func TestResolveQueryScope_NeverReturnsTheRequest(t *testing.T) {
	for _, requested := range []string{"", "app", "APP", "prod", "'; DROP TABLE users --"} {
		got, err := ResolveQueryScope("app", requested)
		if err != nil {
			continue // refused, so nothing was returned to trust
		}
		if got != "app" {
			t.Errorf("ResolveQueryScope(%q) = %q, want the configured %q", requested, got, "app")
		}
	}
}

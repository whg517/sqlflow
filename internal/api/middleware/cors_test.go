package middleware

import (
	"os"
	"testing"
)

func TestSplitByComma(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"single", "a", []string{"a"}},
		{"two", "a,b", []string{"a", "b"}},
		{"three", "a,b,c", []string{"a", "b", "c"}},
		{"trailing comma", "a,", []string{"a", ""}},
		{"leading comma", ",a", []string{"", "a"}},
		{"empty", "", []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitByComma(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("len = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestTrimSpace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"  hello  ", "hello"},
		{"\thello\t", "hello"},
		{"  ", ""},
		{"", ""},
		{"a  b", "a  b"},
	}

	for _, tt := range tests {
		got := trimSpace(tt.input)
		if got != tt.want {
			t.Errorf("trimSpace(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSplitAndTrim(t *testing.T) {
	got := splitAndTrim(" a , b , c ")
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("got %v, want [a b c]", got)
	}
}

func TestSplitAndTrim_Empty(t *testing.T) {
	got := splitAndTrim("")
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestCORS_Default(t *testing.T) {
	// Ensure SQLFLOW_CORS_ORIGINS is not set
	os.Unsetenv("SQLFLOW_CORS_ORIGINS")

	mw := CORS()
	if mw == nil {
		t.Fatal("CORS middleware should not be nil")
	}
}

func TestCORS_WithOrigins(t *testing.T) {
	os.Setenv("SQLFLOW_CORS_ORIGINS", "https://example.com, https://app.example.com")
	defer os.Unsetenv("SQLFLOW_CORS_ORIGINS")

	mw := CORS()
	if mw == nil {
		t.Fatal("CORS middleware should not be nil")
	}
}

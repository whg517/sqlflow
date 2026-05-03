package handler

import (
	"testing"
)

func TestCSVEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "hello", "hello"},
		{"comma", "hello,world", `"hello,world"`},
		{"quote", `say "hi"`, `"say ""hi"""`},
		{"newline", "line1\nline2", "\"line1\nline2\""},
		{"carriage_return", "line1\rline2", "\"line1\rline2\""},
		{"empty", "", ""},
		{"number", "123", "123"},
		{"chinese", "你好世界", "你好世界"},
		{"mixed", `a,b"c\nd`, `"a,b""c\nd"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := csvEscape(tt.input)
			if got != tt.want {
				t.Errorf("csvEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

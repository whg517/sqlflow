package auditlog

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSummarize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"short", "SELECT 1", "SELECT 1"},
		{"exactly_at_limit", strings.Repeat("a", SummaryMaxRunes), strings.Repeat("a", SummaryMaxRunes)},
		{"one_over", strings.Repeat("a", SummaryMaxRunes+1), strings.Repeat("a", SummaryMaxRunes)},
		{"empty", "", ""},
		// The byte length is 3x the rune count here, so a byte-based cut would
		// both keep too little and land inside a character.
		{"multibyte_under_limit", strings.Repeat("查", 40), strings.Repeat("查", 40)},
		{"multibyte_over_limit", strings.Repeat("查", 200), strings.Repeat("查", SummaryMaxRunes)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Summarize(tt.in)
			if got != tt.want {
				t.Errorf("Summarize() = %q (%d runes), want %q (%d runes)",
					got, utf8.RuneCountInString(got), tt.want, utf8.RuneCountInString(tt.want))
			}
		})
	}
}

// TestSummarize_AlwaysValidUTF8 is the property that matters for the audit
// trail: whatever comes in, what gets stored must still be readable text.
func TestSummarize_AlwaysValidUTF8(t *testing.T) {
	inputs := []string{
		"SELECT * FROM users WHERE note = '" + strings.Repeat("中文注释", 50) + "'",
		strings.Repeat("é", 150),
		strings.Repeat("🔥", 150),
		"-- " + strings.Repeat("说明", 80) + "\nDELETE FROM t",
	}
	for _, in := range inputs {
		got := Summarize(in)
		if !utf8.ValidString(got) {
			t.Errorf("Summarize(%.20q...) produced invalid UTF-8: %q", in, got)
		}
		if n := utf8.RuneCountInString(got); n > SummaryMaxRunes {
			t.Errorf("Summarize(%.20q...) returned %d runes, want <= %d", in, n, SummaryMaxRunes)
		}
	}
}

func TestOrDiscard(t *testing.T) {
	if got := OrDiscard(nil); got != Discard {
		t.Errorf("OrDiscard(nil) = %v, want Discard", got)
	}
	w := &recordingWriter{}
	if got := OrDiscard(w); got != w {
		t.Errorf("OrDiscard(w) = %v, want the writer unchanged", got)
	}
}

// TestDiscard_Writes verifies Discard swallows records without panicking — the
// whole reason it exists in place of a nil Writer.
func TestDiscard_Writes(t *testing.T) {
	Discard.Write(context.Background(), Record{Action: "anything"})
}

type recordingWriter struct{ records []Record }

func (w *recordingWriter) Write(_ context.Context, rec Record) {
	w.records = append(w.records, rec)
}

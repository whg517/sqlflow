package audit

import (
	"html"
	"strings"
)

// highlightWindowRunes bounds a snippet on either side of the first match.
//
// Audit records hold whole statements, which can run to kilobytes; returning
// all of it as a "highlight" would make the search response the largest payload
// in the product for no benefit.
const highlightWindowRunes = 32

// highlight wraps every case-insensitive occurrence of keyword in <mark> and
// trims the result to a window around the first one.
//
// The matching is a plain substring scan rather than anything the database
// offers, because the two search paths disagree about what a match is:
// ts_headline highlights word tokens, so on 订单状态 it would return the text
// with nothing marked even though the record was found by trigram. A substring
// scan marks what the user typed, whichever path found the row.
//
// Both the text and the keyword are HTML-escaped before the tags go in. The
// content is user-authored SQL, so emitting it raw inside markup would hand any
// consumer that renders the field an injection point — and the field exists to
// be rendered.
func highlight(text, keyword string) string {
	if text == "" || keyword == "" {
		return ""
	}

	lowerText := strings.ToLower(text)
	lowerKeyword := strings.ToLower(keyword)

	// Case folding is not always length-preserving — 'İ' lowercases to two runes
	// — and the scan below carries byte offsets from the folded string back to
	// the original. Where the lengths disagree those offsets would land mid-rune
	// and corrupt the snippet, so that input gets the unmarked form instead.
	if len(lowerText) != len(text) || len(lowerKeyword) != len(keyword) {
		return html.EscapeString(truncateRunes(text, highlightWindowRunes*2))
	}

	first := strings.Index(lowerText, lowerKeyword)
	if first < 0 {
		// Found through the other field, or through a token boundary this scan
		// does not see. A truncated plain snippet is still more useful than
		// nothing, and stays consistent with the marked case.
		return html.EscapeString(truncateRunes(text, highlightWindowRunes*2))
	}

	// The window is measured in runes but the index is in bytes, so the text is
	// cut on rune boundaries around the match rather than by slicing offsets.
	runes := []rune(text)
	matchStart := len([]rune(text[:first]))
	matchEnd := matchStart + len([]rune(keyword))

	start := max(matchStart-highlightWindowRunes, 0)
	end := min(matchEnd+highlightWindowRunes, len(runes))

	var b strings.Builder
	if start > 0 {
		b.WriteString("...")
	}
	b.WriteString(markAll(string(runes[start:end]), lowerKeyword))
	if end < len(runes) {
		b.WriteString("...")
	}
	return b.String()
}

// markAll escapes segment and wraps each occurrence of the keyword in <mark>.
func markAll(segment, lowerKeyword string) string {
	var b strings.Builder
	lower := strings.ToLower(segment)
	for {
		i := strings.Index(lower, lowerKeyword)
		if i < 0 {
			b.WriteString(html.EscapeString(segment))
			return b.String()
		}
		b.WriteString(html.EscapeString(segment[:i]))
		b.WriteString("<mark>")
		// The original casing, not the keyword's: the snippet shows the record.
		b.WriteString(html.EscapeString(segment[i : i+len(lowerKeyword)]))
		b.WriteString("</mark>")
		segment = segment[i+len(lowerKeyword):]
		lower = lower[i+len(lowerKeyword):]
	}
}

// truncateRunes cuts s to at most n runes, appending an ellipsis if it cut.
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

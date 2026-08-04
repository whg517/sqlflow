package sqlutil

import "testing"

func TestNumberPlaceholders(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"none", "SELECT 1", "SELECT 1"},
		{"single", "WHERE id = ?", "WHERE id = $1"},
		{
			"dynamic set then where — the collision this exists to prevent",
			"UPDATE t SET a = ?, b = ? WHERE id = ?",
			"UPDATE t SET a = $1, b = $2 WHERE id = $3",
		},
		{
			"a question mark inside a literal is not a placeholder",
			"WHERE msg = 'really?' AND id = ?",
			"WHERE msg = 'really?' AND id = $1",
		},
		{
			"literals do not disturb the count that follows",
			"WHERE a = ? AND b = 'x?y' AND c = ?",
			"WHERE a = $1 AND b = 'x?y' AND c = $2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NumberPlaceholders(tt.in); got != tt.want {
				t.Errorf("NumberPlaceholders(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

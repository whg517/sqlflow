package mask

import (
	"testing"
)

func TestMaskPhone(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard", "13812341234", "138****1234"},
		{"short", "1234567", "1234567"},
		{"very_short", "123", "***"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskPhone(tt.input)
			if got != tt.want {
				t.Errorf("maskPhone(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskIDCard(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard_18", "310101199001011234", "310***********1234"},
		{"short", "123456", "******"},
		{"very_short", "123", "***"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskIDCard(tt.input)
			if got != tt.want {
				t.Errorf("maskIDCard(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"two_chars", "张三", "张*"},
		{"three_chars", "张三丰", "张**"},
		{"single_char", "张", "张"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskName(tt.input)
			if got != tt.want {
				t.Errorf("maskName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard", "zhangg@example.com", "z*****@example.com"},
		{"short_local", "a@b.com", "a@b.com"},
		{"no_at", "notanemail", "**********"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskEmail(tt.input)
			if got != tt.want {
				t.Errorf("maskEmail(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskBankCard(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard_16", "6222021234561234", "************1234"},
		{"short", "1234", "1234"},
		{"very_short", "123", "***"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskBankCard(tt.input)
			if got != tt.want {
				t.Errorf("maskBankCard(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskAddress(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard", "上海市浦东新区张江路123号", "上海市浦东新********"},
		{"short", "上海市", "***"},
		{"very_short", "上海", "**"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskAddress(tt.input)
			if got != tt.want {
				t.Errorf("maskAddress(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskFull(t *testing.T) {
	got := maskFull("hello")
	if got != "*****" {
		t.Errorf("maskFull(%q) = %q, want %q", "hello", got, "*****")
	}
}

func TestApplyField(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		rule  Rule
		want  interface{}
	}{
		{"nil_value", nil, Rule{MaskType: MaskPhone}, nil},
		{"non_string", 12345, Rule{MaskType: MaskPhone}, 12345},
		{"phone", "13812341234", Rule{MaskType: MaskPhone}, "138****1234"},
		{"full", "secret", Rule{MaskType: MaskFull}, "******"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyField(tt.value, tt.rule)
			if got != tt.want {
				t.Errorf("ApplyField() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyToRow(t *testing.T) {
	rules := []Rule{
		{Field: "phone", MaskType: MaskPhone},
		{Field: "name", MaskType: MaskName},
	}
	row := map[string]interface{}{
		"phone": "13812341234",
		"name":  "张三",
		"email": "test@example.com",
	}
	masked := ApplyToRow(row, rules)

	found := false
	for _, f := range masked {
		if f == "phone" || f == "name" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected phone and name to be in masked fields, got %v", masked)
	}
	if row["phone"] != "138****1234" {
		t.Errorf("phone not masked correctly: %v", row["phone"])
	}
	if row["name"] != "张*" {
		t.Errorf("name not masked correctly: %v", row["name"])
	}
	if row["email"] != "test@example.com" {
		t.Errorf("email should not be masked: %v", row["email"])
	}
}

func TestApplyToRows(t *testing.T) {
	rules := []Rule{
		{Field: "phone", MaskType: MaskPhone, TableName: "users"},
	}
	rows := []map[string]interface{}{
		{"phone": "13812341234", "name": "张三"},
		{"phone": "13987654321", "name": "李四"},
	}
	masked := ApplyToRows(rows, rules)
	if len(masked) != 1 || masked[0] != "phone" {
		t.Errorf("expected [phone] in masked fields, got %v", masked)
	}
	if rows[0]["phone"] != "138****1234" {
		t.Errorf("first row phone not masked: %v", rows[0]["phone"])
	}
	if rows[1]["phone"] != "139****4321" {
		t.Errorf("second row phone not masked: %v", rows[1]["phone"])
	}
}

func TestMatchRules(t *testing.T) {
	rules := []Rule{
		{TableName: "users", Field: "phone"},
		{TableName: "orders", Field: "amount"},
		{TableName: "*", Field: "email"},
	}
	matched := MatchRules(rules, "users")
	if len(matched) != 2 {
		t.Errorf("expected 2 matched rules for 'users', got %d", len(matched))
	}
	matched = MatchRules(rules, "unknown")
	if len(matched) != 1 {
		t.Errorf("expected 1 matched rule (wildcard) for 'unknown', got %d", len(matched))
	}
}

func TestApplyToRowEmptyRules(t *testing.T) {
	row := map[string]interface{}{"phone": "13812341234"}
	masked := ApplyToRow(row, nil)
	if len(masked) != 0 {
		t.Errorf("expected no masked fields with nil rules, got %v", masked)
	}
}

func TestApplyToRowsEmptyRows(t *testing.T) {
	rules := []Rule{{Field: "phone", MaskType: MaskPhone, TableName: "users"}}
	masked := ApplyToRows(nil, rules)
	if len(masked) != 0 {
		t.Errorf("expected no masked fields with nil rows, got %v", masked)
	}
}

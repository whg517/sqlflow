package mask

import (
	"testing"
)

// --- maskPhone ---

func TestMaskPhone(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard_11", "13812341234", "138****1234"},
		{"boundary_7_no_mask", "1234567", "1234567"},
		{"8_chars", "12345678", "123*5678"},
		{"9_chars", "123456789", "123**6789"},
		{"short_6", "123456", "******"},
		{"very_short_3", "123", "***"},
		{"very_short_1", "1", "*"},
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

// --- maskIDCard ---

func TestMaskIDCard(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard_18", "310101199001011234", "310***********1234"},
		{"standard_15", "310101900101123", "310********1123"},
		{"boundary_7", "1234567", "1234567"},
		{"short_6", "123456", "******"},
		{"very_short_3", "123", "***"},
		{"very_short_1", "1", "*"},
		{"empty", "", ""},
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

// --- maskName ---

func TestMaskName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"two_chars", "张三", "张*"},
		{"three_chars", "张三丰", "张**"},
		{"four_chars", "欧阳修也", "欧***"},
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

// --- maskEmail ---

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard", "zhangg@example.com", "z*****@example.com"},
		{"short_local_2", "ab@test.com", "a*@test.com"},
		{"single_char_local", "a@b.com", "a@b.com"},
		{"no_at_sign", "notanemail", "**********"},
		{"at_sign_first", "@nodomain.com", "@nodomain.com"},
		{"long_local", "verylongname@company.org", "v***********@company.org"},
		{"empty", "", ""},
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

// --- maskBankCard ---

func TestMaskBankCard(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard_16", "6222021234561234", "************1234"},
		{"standard_19", "6222021234561234567", "***************4567"},
		{"boundary_4", "1234", "1234"},
		{"short_3", "123", "***"},
		{"short_1", "1", "*"},
		{"empty", "", ""},
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

// --- maskAddress ---

func TestMaskAddress(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard", "上海市浦东新区张江路123号", "上海市浦东新********"},
		{"exact_7", "上海市浦东新区", "上海市浦东新*"},
		{"8_chars", "上海市浦东新区1", "上海市浦东新**"},
		{"short_3", "上海市", "***"},
		{"very_short_2", "上海", "**"},
		{"very_short_1", "上", "*"},
		{"empty", "", ""},
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

// --- maskFull ---

func TestMaskFull(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ascii", "hello", "*****"},
		{"chinese", "你好世界", "****"},
		{"mixed", "a你b", "***"},
		{"single", "x", "*"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskFull(tt.input)
			if got != tt.want {
				t.Errorf("maskFull(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- maskCustomRegex ---

func TestMaskCustomRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		pattern  string
		template string
		want     string
	}{
		{"empty_pattern_falls_back_to_full", "secret", "", "", "******"},
		{"invalid_regex_falls_back_to_full", "secret", "[", "", "******"},
		{"no_match_falls_back_to_full", "hello", `\d+`, "", "*****"},
		{"match_with_template", "abc123def", `(\d+)`, "[$1]", "abc[123]def"},
		{"match_no_template_uses_full_mask", "abc123def", `\d+`, "", "abc*********def"},
		{"replace_all_digits", "a1b2c3", `\d`, "*", "a*b*c*"},
		{"chinese_pattern", "张三丰", `^\p{Han}+$`, "***", "***"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskCustomRegex(tt.input, tt.pattern, tt.template)
			if got != tt.want {
				t.Errorf("maskCustomRegex(%q, %q, %q) = %q, want %q",
					tt.input, tt.pattern, tt.template, got, tt.want)
			}
		})
	}
}

// --- ApplyField ---

func TestApplyField(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		rule  Rule
		want  interface{}
	}{
		{"nil_value", nil, Rule{MaskType: MaskPhone}, nil},
		{"non_string_int", 12345, Rule{MaskType: MaskPhone}, 12345},
		{"non_string_float", 3.14, Rule{MaskType: MaskFull}, 3.14},
		{"phone", "13812341234", Rule{MaskType: MaskPhone}, "138****1234"},
		{"id_card", "310101199001011234", Rule{MaskType: MaskIDCard}, "310***********1234"},
		{"name", "张三", Rule{MaskType: MaskName}, "张*"},
		{"email", "zhangg@example.com", Rule{MaskType: MaskEmail}, "z*****@example.com"},
		{"bank_card", "6222021234561234", Rule{MaskType: MaskBankCard}, "************1234"},
		{"address", "上海市浦东新区张江路123号", Rule{MaskType: MaskAddress}, "上海市浦东新********"},
		{"full", "secret", Rule{MaskType: MaskFull}, "******"},
		{"custom_with_regex", "abc123def", Rule{MaskType: MaskCustom, CustomRegex: `(\d+)`, CustomTemplate: "[$1]"}, "abc[123]def"},
		{"custom_no_regex_falls_back", "secret", Rule{MaskType: MaskCustom}, "******"},
		{"unknown_type_passthrough", "hello", Rule{MaskType: "unknown"}, "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyField(tt.value, tt.rule)
			if got != tt.want {
				t.Errorf("ApplyField() = %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

// --- ApplyToRow ---

func TestApplyToRow(t *testing.T) {
	t.Run("multiple_fields", func(t *testing.T) {
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

		maskedMap := make(map[string]bool)
		for _, f := range masked {
			maskedMap[f] = true
		}
		if !maskedMap["phone"] || !maskedMap["name"] {
			t.Errorf("expected phone and name in masked fields, got %v", masked)
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
	})

	t.Run("empty_rules", func(t *testing.T) {
		row := map[string]interface{}{"phone": "13812341234"}
		masked := ApplyToRow(row, nil)
		if len(masked) != 0 {
			t.Errorf("expected no masked fields with nil rules, got %v", masked)
		}
	})

	t.Run("no_matching_fields", func(t *testing.T) {
		rules := []Rule{{Field: "nonexistent", MaskType: MaskFull}}
		row := map[string]interface{}{"phone": "13812341234"}
		masked := ApplyToRow(row, rules)
		if len(masked) != 0 {
			t.Errorf("expected no masked fields, got %v", masked)
		}
	})

	t.Run("non_string_value_in_row", func(t *testing.T) {
		rules := []Rule{{Field: "age", MaskType: MaskFull}}
		row := map[string]interface{}{"age": 25}
		masked := ApplyToRow(row, rules)
		if len(masked) != 0 {
			t.Errorf("non-string values should not be masked, got %v", masked)
		}
		if row["age"] != 25 {
			t.Errorf("non-string value should remain unchanged: %v", row["age"])
		}
	})
}

// --- ApplyToRows ---

func TestApplyToRows(t *testing.T) {
	t.Run("multiple_rows", func(t *testing.T) {
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
	})

	t.Run("nil_rows", func(t *testing.T) {
		rules := []Rule{{Field: "phone", MaskType: MaskPhone, TableName: "users"}}
		masked := ApplyToRows(nil, rules)
		if len(masked) != 0 {
			t.Errorf("expected no masked fields with nil rows, got %v", masked)
		}
	})

	t.Run("empty_rows", func(t *testing.T) {
		rules := []Rule{{Field: "phone", MaskType: MaskPhone}}
		masked := ApplyToRows([]map[string]interface{}{}, rules)
		if len(masked) != 0 {
			t.Errorf("expected no masked fields with empty rows, got %v", masked)
		}
	})

	t.Run("empty_rules", func(t *testing.T) {
		rows := []map[string]interface{}{
			{"phone": "13812341234"},
		}
		masked := ApplyToRows(rows, nil)
		if len(masked) != 0 {
			t.Errorf("expected no masked fields with nil rules, got %v", masked)
		}
	})
}

// --- MatchRules ---

func TestMatchRules(t *testing.T) {
	rules := []Rule{
		{TableName: "users", Field: "phone"},
		{TableName: "orders", Field: "amount"},
		{TableName: "*", Field: "email"},
	}

	t.Run("exact_match", func(t *testing.T) {
		matched := MatchRules(rules, "users")
		if len(matched) != 2 {
			t.Errorf("expected 2 matched rules for 'users', got %d", len(matched))
		}
	})

	t.Run("wildcard_only", func(t *testing.T) {
		matched := MatchRules(rules, "unknown")
		if len(matched) != 1 {
			t.Errorf("expected 1 matched rule (wildcard) for 'unknown', got %d", len(matched))
		}
	})

	t.Run("no_match_no_wildcard", func(t *testing.T) {
		noWildcards := []Rule{
			{TableName: "users", Field: "phone"},
			{TableName: "orders", Field: "amount"},
		}
		matched := MatchRules(noWildcards, "products")
		if len(matched) != 0 {
			t.Errorf("expected 0 matched rules, got %d", len(matched))
		}
	})

	t.Run("nil_rules", func(t *testing.T) {
		matched := MatchRules(nil, "users")
		if len(matched) != 0 {
			t.Errorf("expected 0 matched rules with nil, got %d", len(matched))
		}
	})
}

// --- IsValidMaskType ---

func TestIsValidMaskType(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"phone", true},
		{"id_card", true},
		{"name", true},
		{"email", true},
		{"bank_card", true},
		{"address", true},
		{"full", true},
		{"custom", true},
		{"unknown", false},
		{"", false},
		{"PHONE", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsValidMaskType(tt.input)
			if got != tt.want {
				t.Errorf("IsValidMaskType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// --- ValidMaskTypes ---

func TestValidMaskTypes(t *testing.T) {
	types := ValidMaskTypes()
	if len(types) != 8 {
		t.Errorf("expected 8 mask types, got %d", len(types))
	}
	// Verify all constants are included
	expected := map[string]bool{
		string(MaskPhone): false, string(MaskIDCard): false, string(MaskName): false,
		string(MaskEmail): false, string(MaskBankCard): false, string(MaskAddress): false,
		string(MaskFull): false, string(MaskCustom): false,
	}
	for _, mt := range types {
		if _, ok := expected[mt]; !ok {
			t.Errorf("unexpected mask type: %q", mt)
		}
		expected[mt] = true
	}
	for k, v := range expected {
		if !v {
			t.Errorf("missing mask type: %q", k)
		}
	}
}

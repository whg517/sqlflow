package mask

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaskType represents the type of masking to apply.
type MaskType string

const (
	MaskPhone     MaskType = "phone"
	MaskIDCard    MaskType = "id_card"
	MaskName      MaskType = "name"
	MaskEmail     MaskType = "email"
	MaskBankCard  MaskType = "bank_card"
	MaskAddress   MaskType = "address"
	MaskFull      MaskType = "full"
	MaskCustom    MaskType = "custom"
)

// Rule defines a masking rule for a specific field.
type Rule struct {
	DatasourceID   int64
	Database       string
	TableName      string
	Field          string
	MaskType       MaskType
	CustomRegex    string
	CustomTemplate string
}

// ApplyField applies a mask rule to a single string value.
func ApplyField(value interface{}, rule Rule) interface{} {
	if value == nil {
		return nil
	}
	s, ok := value.(string)
	if !ok {
		return value
	}

	switch rule.MaskType {
	case MaskPhone:
		return maskPhone(s)
	case MaskIDCard:
		return maskIDCard(s)
	case MaskName:
		return maskName(s)
	case MaskEmail:
		return maskEmail(s)
	case MaskBankCard:
		return maskBankCard(s)
	case MaskAddress:
		return maskAddress(s)
	case MaskFull:
		return maskFull(s)
	default:
		return value
	}
}

// ApplyToRow applies matching mask rules to a single data row.
// Returns the list of fields that were masked.
func ApplyToRow(row map[string]interface{}, rules []Rule) []string {
	var masked []string
	for _, rule := range rules {
		if val, exists := row[rule.Field]; exists {
			maskedVal := ApplyField(val, rule)
			if maskedVal != val {
				row[rule.Field] = maskedVal
				masked = append(masked, rule.Field)
			}
		}
	}
	return masked
}

// ApplyToRows applies matching mask rules to all rows in a result set.
// Returns the list of fields that were masked.
func ApplyToRows(rows []map[string]interface{}, rules []Rule) []string {
	maskedFields := make(map[string]bool)
	for i := range rows {
		masked := ApplyToRow(rows[i], rules)
		for _, f := range masked {
			maskedFields[f] = true
		}
	}
	result := make([]string, 0, len(maskedFields))
	for f := range maskedFields {
		result = append(result, f)
	}
	return result
}

// MatchRules returns rules that apply to the given table and optional field.
func MatchRules(rules []Rule, tableName string) []Rule {
	var matched []Rule
	for _, r := range rules {
		if r.TableName == tableName || r.TableName == "*" {
			matched = append(matched, r)
		}
	}
	return matched
}

func maskPhone(s string) string {
	if utf8.RuneCountInString(s) < 7 {
		return strings.Repeat("*", utf8.RuneCountInString(s))
	}
	runes := []rune(s)
	for i := 3; i < len(runes)-4; i++ {
		runes[i] = '*'
	}
	return string(runes)
}

func maskIDCard(s string) string {
	runes := []rune(s)
	n := len(runes)
	if n < 7 {
		return strings.Repeat("*", n)
	}
	for i := 3; i < n-4; i++ {
		runes[i] = '*'
	}
	return string(runes)
}

func maskName(s string) string {
	runes := []rune(s)
	n := len(runes)
	if n <= 1 {
		return s
	}
	for i := 1; i < n; i++ {
		runes[i] = '*'
	}
	return string(runes)
}

func maskEmail(s string) string {
	at := strings.Index(s, "@")
	if at < 0 {
		return maskFull(s)
	}
	if at <= 1 {
		return s
	}
	return fmt.Sprintf("%s%s%s", string([]rune(s)[:1]), strings.Repeat("*", at-1), s[at:])
}

func maskBankCard(s string) string {
	runes := []rune(s)
	n := len(runes)
	if n < 4 {
		return strings.Repeat("*", n)
	}
	for i := 0; i < n-4; i++ {
		runes[i] = '*'
	}
	return string(runes)
}

func maskAddress(s string) string {
	runes := []rune(s)
	n := len(runes)
	if n < 6 {
		return strings.Repeat("*", n)
	}
	for i := 6; i < n; i++ {
		runes[i] = '*'
	}
	return string(runes)
}

func maskFull(s string) string {
	return strings.Repeat("*", utf8.RuneCountInString(s))
}

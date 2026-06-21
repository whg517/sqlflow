package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ConditionFieldWhitelist defines the allowed field names in policy conditions JSON.
var ConditionFieldWhitelist = map[string]bool{
	"risk_level":      true,
	"sql_type":        true,
	"environment":     true,
	"database":        true,
	"affected_tables": true,
	"affected_rows":   true,
	"submitter":       true,
}

// ConditionOperatorWhitelist defines the allowed operator strings.
var ConditionOperatorWhitelist = map[string]bool{
	"=":        true,
	"!=":       true,
	"IN":       true,
	"NOT IN":   true,
	"LIKE":     true,
	"CONTAINS": true,
}

const (
	maxConditionValueLen = 500 // max characters per single value
	maxConditionDepth    = 2   // max nesting depth for AND/OR groups
)

// conditionNode represents a node in the condition tree.
// It can be either a leaf (field+op+value) or a group (AND/OR with children).
type conditionNode struct {
	Logic    string          `json:"logic,omitempty"`    // "AND" or "OR" for groups
	Field    string          `json:"field,omitempty"`    // leaf: field name
	Op       string          `json:"op,omitempty"`       // leaf: operator
	Value    json.RawMessage `json:"value,omitempty"`    // leaf: value (string or []string)
	Children []conditionNode `json:"children,omitempty"` // group: child nodes
}

// ValidateConditions validates the conditions JSON string against the whitelist rules.
// Returns an error if the conditions are invalid.
func ValidateConditions(conditions string) error {
	conditions = strings.TrimSpace(conditions)
	if conditions == "" || conditions == "{}" || conditions == "[]" {
		return nil // empty conditions = match all
	}

	// Try parsing as a group node first (top-level AND/OR)
	var node conditionNode
	if err := json.Unmarshal([]byte(conditions), &node); err != nil {
		// Could be a raw object without logic — treat as single leaf
		var rawMap map[string]json.RawMessage
		if err2 := json.Unmarshal([]byte(conditions), &rawMap); err2 == nil {
			return validateRawConditionMap(rawMap, 0)
		}
		return fmt.Errorf("无效的条件 JSON: %v", err)
	}

	// If it has logic field, validate as group; otherwise validate as leaf
	if node.Logic != "" {
		return validateConditionNode(node, 0)
	}

	// If it has field/op, validate as single leaf
	if node.Field != "" {
		return validateConditionNode(node, 0)
	}

	// Fallback: try as raw map (legacy format like {"risk_levels":[...]})
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(conditions), &rawMap); err == nil {
		return validateRawConditionMap(rawMap, 0)
	}

	return nil
}

// validateConditionNode validates a single condition node (leaf or group).
func validateConditionNode(node conditionNode, depth int) error {
	if depth > maxConditionDepth {
		return fmt.Errorf("条件嵌套层数超过最大限制 %d", maxConditionDepth)
	}

	// Group node (AND/OR)
	if node.Logic != "" {
		logic := strings.ToUpper(node.Logic)
		if logic != "AND" && logic != "OR" {
			return fmt.Errorf("无效的逻辑运算符: %s（仅支持 AND/OR）", node.Logic)
		}
		if len(node.Children) == 0 {
			return fmt.Errorf("逻辑组 %s 必须包含至少一个子条件", logic)
		}
		for _, child := range node.Children {
			if err := validateConditionNode(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}

	// Leaf node
	if node.Field == "" {
		return fmt.Errorf("条件缺少 field 字段")
	}
	if !ConditionFieldWhitelist[node.Field] {
		return fmt.Errorf("不支持的条件字段: %s", node.Field)
	}

	if node.Op == "" {
		return fmt.Errorf("条件缺少 op 字段")
	}
	if !ConditionOperatorWhitelist[strings.ToUpper(node.Op)] {
		return fmt.Errorf("不支持的运算符: %s（支持: =, !=, IN, NOT IN, LIKE, CONTAINS）", node.Op)
	}

	if len(node.Value) == 0 {
		return fmt.Errorf("条件缺少 value 字段")
	}
	return validateConditionValue(node.Value)
}

// validateConditionValue validates the value field of a leaf condition.
func validateConditionValue(raw json.RawMessage) error {
	// Try as string
	var strVal string
	if err := json.Unmarshal(raw, &strVal); err == nil {
		if len(strVal) > maxConditionValueLen {
			return fmt.Errorf("条件值长度超过限制 %d 字符", maxConditionValueLen)
		}
		return nil
	}

	// Try as []string
	var sliceVal []string
	if err := json.Unmarshal(raw, &sliceVal); err == nil {
		if len(sliceVal) == 0 {
			return fmt.Errorf("数组条件值不能为空")
		}
		for _, v := range sliceVal {
			if len(v) > maxConditionValueLen {
				return fmt.Errorf("条件值长度超过限制 %d 字符", maxConditionValueLen)
			}
		}
		return nil
	}

	// Try as number
	var numVal float64
	if err := json.Unmarshal(raw, &numVal); err == nil {
		return nil
	}

	return fmt.Errorf("条件值必须是字符串、数字或字符串数组")
}

// validateRawConditionMap validates a legacy-style conditions map (e.g., {"risk_levels":["high"]}).
// This is for backward compatibility with the existing PolicyCondition struct.
func validateRawConditionMap(rawMap map[string]json.RawMessage, depth int) error {
	if depth > maxConditionDepth {
		return fmt.Errorf("条件嵌套层数超过最大限制 %d", maxConditionDepth)
	}

	// Allow known legacy keys that map to PolicyCondition struct fields
	legacyKeys := map[string]bool{
		"risk_levels":  true,
		"sql_types":    true,
		"environments": true,
		"databases":    true,
	}

	for key, val := range rawMap {
		// Legacy keys are allowed
		if legacyKeys[key] {
			var vals []string
			if err := json.Unmarshal(val, &vals); err == nil {
				for _, v := range vals {
					if len(v) > maxConditionValueLen {
						return fmt.Errorf("条件值长度超过限制 %d 字符", maxConditionValueLen)
					}
				}
				continue
			}
			// Single string value
			var sv string
			if err := json.Unmarshal(val, &sv); err == nil {
				if len(sv) > maxConditionValueLen {
					return fmt.Errorf("条件值长度超过限制 %d 字符", maxConditionValueLen)
				}
				continue
			}
			return fmt.Errorf("条件字段 %s 的值格式无效", key)
		}

		// Check whitelist for structured condition fields
		if ConditionFieldWhitelist[key] {
			var sv string
			if err := json.Unmarshal(val, &sv); err == nil {
				if len(sv) > maxConditionValueLen {
					return fmt.Errorf("条件值长度超过限制 %d 字符", maxConditionValueLen)
				}
				continue
			}
			var sliceVal []string
			if err := json.Unmarshal(val, &sliceVal); err == nil {
				for _, v := range sliceVal {
					if len(v) > maxConditionValueLen {
						return fmt.Errorf("条件值长度超过限制 %d 字符", maxConditionValueLen)
					}
				}
				continue
			}
			return fmt.Errorf("条件字段 %s 的值格式无效", key)
		}

		return fmt.Errorf("不支持的条件字段: %s", key)
	}

	return nil
}

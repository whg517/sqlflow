package ticket

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/whg517/sqlflow/internal/model"
)

// The approval-condition language.
//
// One module owns it end to end: parsing produces the value the matcher reads,
// so a condition the matcher cannot honor cannot become a policy row at all.
//
// It used to be three languages that never met. The form wrote
// {"logic":"AND","conditions":[{"field","operator","value"}]}; the validator
// expected {"logic","children":[{"field","op","value"}]} and rejected every
// non-empty set the form could build, so the only saveable condition was "{}";
// and the matcher read a fourth shape entirely — flat arrays named risk_levels,
// sql_types, databases, environments — which meant a hand-crafted set that did
// pass validation unmarshaled into an all-empty value and matched everything.
// Combined with auto_approve_enabled that is a catch-all auto-approver, which
// is a client deciding its own approval path (invariant 5).
//
// Two rules follow from that history:
//
//   - An unrecognized field or operator is an error, never something skipped.
//     Skipping widened the match; the whole point of a condition is to narrow it.
//   - "No conditions" means match-all only when the policy says so explicitly
//     (the empty document). It is never the residue of something that failed to
//     parse, because nothing that fails to parse is stored.

// ConditionField names something about a ticket a policy may test.
type ConditionField string

const (
	FieldRiskLevel ConditionField = "risk_level"
	FieldSQLType   ConditionField = "sql_type"
	FieldDatabase  ConditionField = "database"
)

// ConditionOperator is how a field is compared.
type ConditionOperator string

const (
	OpIn    ConditionOperator = "in"     // the ticket's value is one of these
	OpNotIn ConditionOperator = "not_in" // the ticket's value is none of these
)

// ConditionLogic joins the rules in a condition.
type ConditionLogic string

const (
	LogicAnd ConditionLogic = "AND"
	LogicOr  ConditionLogic = "OR"
)

// maxConditionRules bounds a stored condition. A policy with hundreds of rules
// is a policy nobody can reason about, and the matcher runs on every ticket
// creation.
const maxConditionRules = 32

// ConditionRule is one comparison.
type ConditionRule struct {
	Field    ConditionField    `json:"field"`
	Operator ConditionOperator `json:"operator"`
	Values   []string          `json:"values"`
}

// Condition is a parsed, matchable policy condition.
//
// It has no exported constructor other than ParseCondition, so a Condition in
// hand has already been checked. That is what makes "unsupported conditions
// cannot become a policy row" true by construction rather than by convention.
type Condition struct {
	Logic ConditionLogic  `json:"logic"`
	Rules []ConditionRule `json:"conditions"`

	// matchAll records that this condition came from an explicitly empty
	// document, as opposed to having no rules for some other reason. The
	// distinction is the difference between "this policy applies to everything"
	// and "nothing survived", and conflating them is how a broken condition set
	// became a catch-all.
	matchAll bool
}

// conditionFieldValues restricts the values a field accepts.
//
// A field with a closed set is validated against it, so a policy scoped to
// "hgih" is refused at save time rather than silently matching nothing forever.
// The values come from the two places that produce them — RiskLevel* in
// risk_evaluator.go and the SQLType assignments in sql_analyzer.go — rather
// than from a guess. A closed set that does not match what the analyzer emits
// would make correct policies unwritable, which is the same failure as
// accepting ones that can never match, pointed the other way.
var conditionFieldValues = map[ConditionField][]string{
	FieldRiskLevel: {RiskLevelLow, RiskLevelMedium, RiskLevelHigh, RiskLevelCritical},
	FieldSQLType: {
		"SELECT", "INSERT", "UPDATE", "DELETE", "MERGE", "REPLACE",
		"CREATE", "ALTER", "DROP", "TRUNCATE", "OTHER",
	},
	// FieldDatabase is open: database names are the deployment's business.
}

// ParseCondition is the only way to obtain a Condition.
//
// An empty document means match-all, which is what the default policy stores.
// Anything else must name a field and an operator this matcher implements —
// an unknown one is refused rather than ignored.
func ParseCondition(raw string) (Condition, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" || trimmed == "[]" || trimmed == "null" {
		return Condition{Logic: LogicAnd, matchAll: true}, nil
	}

	var c Condition
	if err := json.Unmarshal([]byte(trimmed), &c); err != nil {
		return Condition{}, fmt.Errorf("审批条件不是合法的 JSON: %w", err)
	}

	switch ConditionLogic(strings.ToUpper(string(c.Logic))) {
	case LogicAnd, "":
		c.Logic = LogicAnd
	case LogicOr:
		c.Logic = LogicOr
	default:
		return Condition{}, fmt.Errorf("无效的逻辑运算符 %q（仅支持 AND/OR）", c.Logic)
	}

	if len(c.Rules) == 0 {
		// A document that carries a logic operator and no rules is a form the
		// user did not finish, not a request to match everything.
		return Condition{}, fmt.Errorf("审批条件为空；若要匹配全部工单请使用空条件 {}")
	}
	if len(c.Rules) > maxConditionRules {
		return Condition{}, fmt.Errorf("审批条件过多（%d 条，上限 %d 条）", len(c.Rules), maxConditionRules)
	}

	for i := range c.Rules {
		r := &c.Rules[i]
		if _, ok := conditionFieldValues[r.Field]; !ok && r.Field != FieldDatabase {
			return Condition{}, fmt.Errorf("不支持的条件字段 %q（支持：%s）",
				r.Field, strings.Join(supportedConditionFieldNames(), "、"))
		}
		switch r.Operator {
		case OpIn, OpNotIn:
		case "":
			r.Operator = OpIn
		default:
			return Condition{}, fmt.Errorf("不支持的条件运算符 %q（支持：in、not_in）", r.Operator)
		}
		if len(r.Values) == 0 {
			return Condition{}, fmt.Errorf("条件字段 %q 未提供取值", r.Field)
		}
		if allowed, closed := conditionFieldValues[r.Field]; closed {
			for _, v := range r.Values {
				if !containsAny(allowed, v) {
					return Condition{}, fmt.Errorf("条件字段 %q 的取值 %q 无效（支持：%s）",
						r.Field, v, strings.Join(allowed, "、"))
				}
			}
		}
	}
	return c, nil
}

// Matches reports whether this condition selects the ticket.
func (c Condition) Matches(t *model.Ticket) bool {
	if c.matchAll || len(c.Rules) == 0 {
		return true
	}

	for _, r := range c.Rules {
		hit := r.matches(t)
		if c.Logic == LogicOr && hit {
			return true
		}
		if c.Logic == LogicAnd && !hit {
			return false
		}
	}
	return c.Logic == LogicAnd
}

func (r ConditionRule) matches(t *model.Ticket) bool {
	var actual string
	switch r.Field {
	case FieldRiskLevel:
		actual = t.RiskLevel
	case FieldSQLType:
		// sql_analyzer leaves this empty for statements it could not classify,
		// and a policy scoped to OTHER should still catch those.
		actual = t.SQLType
		if actual == "" {
			actual = "OTHER"
		}
	case FieldDatabase:
		actual = t.Database
	default:
		// Unreachable: ParseCondition refuses unknown fields. Refusing to match
		// is the safe answer if it ever becomes reachable, because a condition
		// that cannot be evaluated must not widen the policy.
		return false
	}

	in := containsAny(r.Values, actual)
	if r.Operator == OpNotIn {
		return !in
	}
	return in
}

// ConditionFieldDescriptor tells a client what it may build.
//
// The server declares the language and the form renders from the declaration —
// the same move ConfigSchema() []ConfigField already made for datasource forms.
// The alternative is what was there: the form hard-coding a third vocabulary,
// including risk values (HIGH/MODERATE/LOW) the server has never used.
type ConditionFieldDescriptor struct {
	Field     ConditionField      `json:"field"`
	Label     string              `json:"label"`
	Operators []ConditionOperator `json:"operators"`
	// Values is the closed set of accepted values, or nil when the field is open.
	Values []string `json:"values,omitempty"`
}

// ConditionSchema declares the whole language in one place.
func ConditionSchema() []ConditionFieldDescriptor {
	ops := []ConditionOperator{OpIn, OpNotIn}
	return []ConditionFieldDescriptor{
		{Field: FieldRiskLevel, Label: "风险等级", Operators: ops, Values: conditionFieldValues[FieldRiskLevel]},
		{Field: FieldSQLType, Label: "SQL 类型", Operators: ops, Values: conditionFieldValues[FieldSQLType]},
		{Field: FieldDatabase, Label: "数据库", Operators: ops},
	}
}

func supportedConditionFieldNames() []string {
	out := make([]string, 0, len(ConditionSchema()))
	for _, d := range ConditionSchema() {
		out = append(out, string(d.Field))
	}
	return out
}

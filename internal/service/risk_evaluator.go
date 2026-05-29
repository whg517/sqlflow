package service

import "strings"

// RiskLevel constants.
const (
	RiskLevelLow      = "low"
	RiskLevelMedium   = "medium"
	RiskLevelHigh     = "high"
	RiskLevelCritical = "critical"
)

// RiskEvaluation holds the result of risk evaluation.
type RiskEvaluation struct {
	Level  string `json:"level"`
	Reason string `json:"reason"`
	Score  int    `json:"score"` // 0-100
}

// RiskEvaluator evaluates SQL risk based on analysis.
type RiskEvaluator struct{}

// NewRiskEvaluator creates a new RiskEvaluator.
func NewRiskEvaluator() *RiskEvaluator {
	return &RiskEvaluator{}
}

// Evaluate computes risk level based on SQLAnalysis and optional context.
func (e *RiskEvaluator) Evaluate(analysis *SQLAnalysis) *RiskEvaluation {
	score := 0
	reasons := []string{}

	// Rule 1: Statement type
	switch analysis.SQLType {
	case "SELECT":
		// SELECT is low risk by default
		if !analysis.IsRead {
			score += 5
		}
	case "INSERT":
		score += 20
		reasons = append(reasons, "INSERT 语句")
	case "UPDATE":
		score += 35
		reasons = append(reasons, "UPDATE 语句")
	case "DELETE":
		score += 45
		reasons = append(reasons, "DELETE 语句")
	case "REPLACE", "MERGE":
		score += 30
		reasons = append(reasons, analysis.SQLType+" 语句")
	case "CREATE":
		score += 30
		reasons = append(reasons, "CREATE (DDL) 语句")
	case "ALTER":
		score += 60
		reasons = append(reasons, "ALTER (DDL) 语句，结构变更")
	case "DROP":
		score += 80
		reasons = append(reasons, "DROP (DDL) 语句，删除对象")
	case "TRUNCATE":
		score += 70
		reasons = append(reasons, "TRUNCATE 语句，清空表数据")
	case "GRANT", "REVOKE":
		score += 50
		reasons = append(reasons, "权限变更语句")
	default:
		score += 15
		reasons = append(reasons, "未知语句类型")
	}

	// Rule 2: DDL gets higher base risk
	if analysis.IsDDL {
		score += 15
	}

	// Rule 3: Multiple affected tables increase risk
	tableCount := len(analysis.AffectedTables)
	switch {
	case tableCount == 0:
		// No tables detected — neutral
	case tableCount == 1:
		// Single table — neutral
	case tableCount <= 3:
		score += 10
		reasons = append(reasons, "影响多张表")
	default:
		score += 25
		reasons = append(reasons, "影响大量表(>3)")
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	level := scoreToLevel(score)

	return &RiskEvaluation{
		Level:  level,
		Reason: strings.Join(reasons, "; "),
		Score:  score,
	}
}

// EvaluateWithSQL evaluates risk directly from SQL string.
func (e *RiskEvaluator) EvaluateWithSQL(sql string) *RiskEvaluation {
	analyzer := NewSQLAnalyzer()
	analysis := analyzer.Analyze(sql)
	return e.Evaluate(analysis)
}

func scoreToLevel(score int) string {
	switch {
	case score <= 15:
		return RiskLevelLow
	case score <= 40:
		return RiskLevelMedium
	case score <= 65:
		return RiskLevelHigh
	default:
		return RiskLevelCritical
	}
}

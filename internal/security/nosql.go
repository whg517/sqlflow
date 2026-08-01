package security

import "github.com/whg517/sqlflow/internal/platform/sqlparser"

// MongoOpToCasbinAct maps a MongoDB operation to a Casbin action string, so the
// RBAC model that governs SQL also governs NoSQL.
//
// Unrecognized operations map to "select" — the least privileged action — so a
// parser that learns a new operation before this switch does cannot widen
// access by accident.
func MongoOpToCasbinAct(op sqlparser.MongoOperation) string {
	switch op {
	case sqlparser.MongoOpFind, sqlparser.MongoOpAggregate:
		return "select"
	case sqlparser.MongoOpInsert:
		return "insert"
	case sqlparser.MongoOpUpdate:
		return "update"
	case sqlparser.MongoOpDelete:
		return "delete"
	default:
		return "select"
	}
}

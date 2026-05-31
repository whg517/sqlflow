package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AuditLog holds the schema definition for the AuditLog entity.
// Maps to: audit_logs table
type AuditLog struct {
	ent.Schema
}

func (AuditLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "audit_logs"},
	}
}

func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("action").
			Default(""),
		field.Int64("datasource_id").
			Default(0),
		field.String("database").
			Default(""),
		field.String("sql_content").
			Default(""),
		field.String("sql_summary").
			Default(""),
		field.Int64("result_rows").
			Default(0),
		field.Int64("affected_rows").
			Default(0),
		field.Int64("execution_time_ms").
			Default(0),
		field.String("error_message").
			Default(""),
		field.String("desensitized_fields").
			Default(""),
		field.String("ip_address").
			Default(""),
		field.String("ai_review_result").
			Default(""),
		field.Int64("ticket_id").
			Default(0),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (AuditLog) Edges() []ent.Edge {
	return nil
}

func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("action"),
		index.Fields("datasource_id"),
		index.Fields("created_at"),
		index.Fields("ticket_id"),
	}
}

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Ticket holds the schema definition for the Ticket entity.
// Maps to: tickets table
type Ticket struct {
	ent.Schema
}

func (Ticket) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tickets"},
	}
}

func (Ticket) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("submitter_id"),
		field.Int64("datasource_id"),
		field.String("database").
			Default(""),
		field.String("sql_content").
			NotEmpty(),
		field.String("sql_summary").
			Default(""),
		field.String("sql_type").
			Default(""),
		field.String("db_type").
			Default("mysql"),
		field.String("change_reason").
			Default(""),
		field.String("status").
			Default("SUBMITTED"),
		field.String("affected_tables").
			Default("[]"),
		field.String("risk_level").
			Default(""),
		field.String("ai_review_result").
			Default(""),
		field.Int64("reviewer_id").
			Default(0),
		field.String("review_comment").
			Default(""),
		field.String("sql_hash").
			Default(""),
		field.Int("revision").
			Default(1),
		field.Int("current_stage").
			Default(0),
		field.Int("total_stages").
			Default(0),
		field.Bool("auto_approved").
			Default(false),
		field.String("auto_approve_reason").
			Default("").
			Optional(),
		field.Int64("policy_id").
			Optional().
			Nillable(),
		field.Time("scheduled_at").
			Optional().
			Nillable(),
		field.Time("executed_at").
			Optional().
			Nillable(),
		field.Time("sla_deadline").
			Optional().
			Nillable(),
		field.String("sla_status").
			Default("normal"),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
		field.Time("updated_at").
			Default(timeNow).
			UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (Ticket) Edges() []ent.Edge {
	return nil
}

func (Ticket) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("submitter_id"),
		index.Fields("status"),
		index.Fields("datasource_id"),
		index.Fields("scheduled_at"),
		index.Fields("sla_deadline"),
	}
}

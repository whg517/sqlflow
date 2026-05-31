package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ApprovalRecord holds the schema definition for the ApprovalRecord entity.
// Maps to: approval_records table
type ApprovalRecord struct {
	ent.Schema
}

func (ApprovalRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "approval_records"},
	}
}

func (ApprovalRecord) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("ticket_id"),
		field.Int64("policy_id").
			Optional().
			Nillable(),
		field.Int("stage").
			Default(0),
		field.Int("total_stages").
			Default(0),
		field.String("approver_role").
			Default(""),
		field.Int64("approver_id").
			Optional().
			Nillable(),
		field.String("approver_name").
			Optional().
			Nillable(),
		field.String("action").
			Default(""),
		field.String("comment").
			Optional().
			Nillable(),
		field.Bool("auto_approved").
			Default(false),
		field.String("auto_reason").
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (ApprovalRecord) Edges() []ent.Edge {
	return nil
}

func (ApprovalRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ticket_id"),
		index.Fields("approver_id"),
	}
}

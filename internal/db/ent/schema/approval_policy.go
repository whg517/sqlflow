package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ApprovalPolicy holds the schema definition for the ApprovalPolicy entity.
// Maps to: approval_policies table
type ApprovalPolicy struct {
	ent.Schema
}

func (ApprovalPolicy) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "approval_policies"},
	}
}

func (ApprovalPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			Unique(),
		field.String("description").
			Optional().
			Nillable(),
		field.Bool("enabled").
			Default(true),
		field.Int("priority").
			Default(0),
		field.String("conditions").
			Default("{}"),
		field.String("approval_chain").
			Default("[]"),
		field.Bool("auto_approve_enabled").
			Default(false),
		field.String("auto_approve_reason").
			Optional().
			Nillable(),
		field.Bool("is_default").
			Default(false),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
		field.Time("updated_at").
			Default(timeNow).
			UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (ApprovalPolicy) Edges() []ent.Edge {
	return nil
}

func (ApprovalPolicy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled", "priority"),
	}
}

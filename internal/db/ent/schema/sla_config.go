package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SLAConfig holds the schema definition for the SLAConfig entity.
// Maps to: sla_config table
type SLAConfig struct {
	ent.Schema
}

func (SLAConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sla_config"},
	}
}

func (SLAConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("priority").
			NotEmpty(),
		field.Int("timeout_minutes"),
		field.Int("reminder_percent").
			Default(80),
		field.String("escalate_to_role").
			Default("admin"),
		field.String("escalate_to_user").
			Default(""),
		field.Bool("enabled").
			Default(true),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
		field.Time("updated_at").
			Default(timeNow).
			UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (SLAConfig) Edges() []ent.Edge {
	return nil
}

func (SLAConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("priority").Unique(),
	}
}

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SLAActionLog holds the schema definition for the SLAActionLog entity.
// Maps to: sla_action_log table
type SLAActionLog struct {
	ent.Schema
}

func (SLAActionLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sla_action_log"},
	}
}

func (SLAActionLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("ticket_id"),
		field.String("action_type").
			NotEmpty(),
		field.String("dedup_key").
			NotEmpty().
			Unique(),
		field.String("notified_user").
			Default(""),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
		field.Int64("sla_config_id").
			Optional().
			Nillable(),
	}
}

func (SLAActionLog) Edges() []ent.Edge {
	return nil
}

func (SLAActionLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ticket_id"),
		index.Fields("created_at"),
	}
}

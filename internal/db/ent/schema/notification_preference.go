package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// NotificationPreference holds one user's channel choice for one event type.
// Maps to: notification_preferences table
type NotificationPreference struct {
	ent.Schema
}

func (NotificationPreference) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "notification_preferences"}}
}

func (NotificationPreference) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("event_type"),
		// A JSON array of channel names, stored as text to match the existing
		// column rather than migrating to jsonb as part of the database move.
		field.String("channels").Default("[]"),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("now()")),
	}
}

func (NotificationPreference) Edges() []ent.Edge { return nil }

func (NotificationPreference) Indexes() []ent.Index {
	return []ent.Index{index.Fields("user_id", "event_type").Unique()}
}

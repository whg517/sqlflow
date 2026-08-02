package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WebhookSubscription holds the schema definition for a generic webhook target.
// Maps to: webhook_subscriptions table
type WebhookSubscription struct {
	ent.Schema
}

func (WebhookSubscription) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "webhook_subscriptions"}}
}

func (WebhookSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.String("url"),
		field.String("encrypted_secret"),
		field.String("events").Default("[]"),
		field.Bool("enabled").Default(true),
		field.Int64("failure_count").Default(0),
		field.Time("last_triggered_at").Optional().Nillable(),
		field.String("created_by").Default(""),
		field.Time("created_at").Default(timeNow).Annotations(entsql.DefaultExpr("now()")),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("now()")),
	}
}

func (WebhookSubscription) Edges() []ent.Edge { return nil }

func (WebhookSubscription) Indexes() []ent.Index {
	return []ent.Index{index.Fields("enabled")}
}

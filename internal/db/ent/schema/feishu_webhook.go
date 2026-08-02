package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// FeishuWebhook holds the schema definition for a Feishu webhook target.
// Maps to: feishu_webhooks table
type FeishuWebhook struct {
	ent.Schema
}

func (FeishuWebhook) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "feishu_webhooks"}}
}

func (FeishuWebhook) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		// The URL is a credential — it grants posting rights to whoever holds it
		// — so it is stored encrypted, with a hash alongside for deduplication.
		field.String("encrypted_url"),
		field.String("url_hash"),
		field.String("scene").Default("general"),
		field.Bool("enabled").Default(true),
		field.Float("rate_limit_rps").Default(1.0),
		field.String("created_by").Default(""),
		field.Time("created_at").Default(timeNow).Annotations(entsql.DefaultExpr("now()")),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("now()")),
	}
}

func (FeishuWebhook) Edges() []ent.Edge { return nil }

func (FeishuWebhook) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("url_hash").Unique(),
		index.Fields("enabled"),
	}
}

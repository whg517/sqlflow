package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WebVital holds the schema definition for the WebVital entity.
// Maps to: web_vitals table
type WebVital struct {
	ent.Schema
}

func (WebVital) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "web_vitals"},
	}
}

func (WebVital) Fields() []ent.Field {
	return []ent.Field{
		field.String("metric_name").
			NotEmpty(),
		field.Float("value"),
		field.String("rating").
			Default(""),
		field.String("path").
			Default(""),
		field.String("navigation_type").
			Default(""),
		field.String("user_agent").
			Default(""),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (WebVital) Edges() []ent.Edge {
	return nil
}

func (WebVital) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("metric_name"),
		index.Fields("created_at"),
	}
}

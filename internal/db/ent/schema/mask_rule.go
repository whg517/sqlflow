package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// MaskRule holds the schema definition for the MaskRule entity.
// Maps to: mask_rules table
type MaskRule struct {
	ent.Schema
}

func (MaskRule) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "mask_rules"},
	}
}

func (MaskRule) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("datasource_id").
			Default(0),
		field.String("database").
			Default(""),
		field.String("table_name").
			Default(""),
		field.String("field").
			Default(""),
		field.String("mask_type").
			Default(""),
		field.String("custom_regex").
			Default(""),
		field.String("custom_template").
			Default(""),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
		field.Time("updated_at").
			Default(timeNow).
			UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (MaskRule) Edges() []ent.Edge {
	return nil
}

func (MaskRule) Indexes() []ent.Index {
	return nil
}

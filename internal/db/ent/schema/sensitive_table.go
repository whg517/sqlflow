package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SensitiveTable holds the schema definition for the SensitiveTable entity.
// Maps to: sensitive_tables table
type SensitiveTable struct {
	ent.Schema
}

func (SensitiveTable) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sensitive_tables"},
	}
}

func (SensitiveTable) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("datasource_id").
			Default(0),
		field.String("database").
			Default(""),
		field.String("table_name").
			Default(""),
		field.String("sensitivity_level").
			Default("medium"),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
		field.Time("updated_at").
			Default(timeNow).
			UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (SensitiveTable) Edges() []ent.Edge {
	return nil
}

func (SensitiveTable) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("datasource_id", "database", "table_name").Unique(),
	}
}

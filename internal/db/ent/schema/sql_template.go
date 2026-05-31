package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SQLTemplate holds the schema definition for the SQLTemplate entity.
// Maps to: sql_templates table
type SQLTemplate struct {
	ent.Schema
}

func (SQLTemplate) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sql_templates"},
	}
}

func (SQLTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("name").
			NotEmpty(),
		field.String("description").
			Default(""),
		field.String("sql_content").
			NotEmpty(),
		field.String("db_type").
			Default("mysql"),
		field.String("category").
			Default("general"),
		field.String("params_json").
			Default("[]"),
		field.Bool("is_public").
			Default(false),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
		field.Time("updated_at").
			Default(timeNow).
			UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (SQLTemplate) Edges() []ent.Edge {
	return nil
}

func (SQLTemplate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("category"),
		index.Fields("user_id", "name").Unique(),
	}
}

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SharedResult holds the schema definition for the SharedResult entity.
// Maps to: shared_results table
type SharedResult struct {
	ent.Schema
}

func (SharedResult) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "shared_results"},
	}
}

func (SharedResult) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("username").
			Default(""),
		field.String("token").
			NotEmpty().
			Unique(),
		field.String("columns_json").
			Default("[]"),
		field.String("rows_json").
			Default("[]"),
		field.Int64("row_count").
			Default(0),
		field.Time("expires_at"),
		field.String("password_hash").
			Default("").
			StructTag(`json:"-"`),
		field.String("sql_summary").
			Default(""),
		field.String("datasource_name").
			Default(""),
		field.Bool("revoked").
			Default(false),
		field.Time("revoked_at").
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (SharedResult) Edges() []ent.Edge {
	return nil
}

func (SharedResult) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("token"),
		index.Fields("user_id"),
		index.Fields("expires_at"),
	}
}

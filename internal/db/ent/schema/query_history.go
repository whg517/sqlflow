package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// QueryHistory holds the schema definition for the QueryHistory entity.
// Maps to: query_history table
type QueryHistory struct {
	ent.Schema
}

func (QueryHistory) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "query_history"},
	}
}

func (QueryHistory) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("datasource_id"),
		field.String("database").
			Default(""),
		field.String("sql_content").
			NotEmpty(),
		field.String("sql_summary").
			Default(""),
		field.String("db_type").
			Default("mysql"),
		field.Int64("execution_time").
			Default(0),
		field.Int64("result_rows").
			Default(0),
		field.Int64("affected_rows").
			Default(0),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (QueryHistory) Edges() []ent.Edge {
	return nil
}

func (QueryHistory) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("execution_time"),
		index.Fields("created_at"),
	}
}

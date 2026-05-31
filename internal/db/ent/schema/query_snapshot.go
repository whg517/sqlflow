package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// QuerySnapshot holds the schema definition for the QuerySnapshot entity.
// Maps to: query_snapshots table
type QuerySnapshot struct {
	ent.Schema
}

func (QuerySnapshot) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "query_snapshots"},
	}
}

func (QuerySnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("label").
			Default(""),
		field.String("columns_json").
			NotEmpty().
			StorageKey("columns"),
		field.String("rows_json").
			NotEmpty().
			StorageKey("rows"),
		field.Int64("row_count").
			Default(0),
		field.Int64("query_history_id").
			Default(0),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (QuerySnapshot) Edges() []ent.Edge {
	return nil
}

func (QuerySnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
	}
}

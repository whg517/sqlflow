package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Comment holds the schema definition for the Comment entity.
// Maps to: comments table
type Comment struct {
	ent.Schema
}

func (Comment) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "comments"},
	}
}

func (Comment) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("order_id"),
		field.Int64("user_id"),
		field.String("content").
			NotEmpty(),
		field.Int64("parent_id").
			Default(0),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (Comment) Edges() []ent.Edge {
	return nil
}

func (Comment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("order_id"),
		index.Fields("parent_id"),
	}
}

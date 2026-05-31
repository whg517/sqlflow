package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// RefreshToken holds the schema definition for the RefreshToken entity.
// Maps to: refresh_tokens table
type RefreshToken struct {
	ent.Schema
}

func (RefreshToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "refresh_tokens"},
	}
}

func (RefreshToken) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("token").
			NotEmpty().
			Unique(),
		field.Time("expires_at"),
		field.Bool("revoked").
			Default(false),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (RefreshToken) Edges() []ent.Edge {
	return nil
}

func (RefreshToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("token"),
	}
}

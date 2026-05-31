package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// APIToken holds the schema definition for the APIToken entity.
// Maps to: api_tokens table
type APIToken struct {
	ent.Schema
}

func (APIToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "api_tokens"},
	}
}

func (APIToken) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("name").
			NotEmpty(),
		field.String("token_hash").
			NotEmpty().
			StructTag(`json:"-"`),
		field.String("token_prefix").
			Default(""),
		field.String("scopes").
			Default(""),
		field.Time("expires_at"),
		field.Time("last_used_at").
			Optional().
			Nillable(),
		field.Int64("use_count").
			Default(0),
		field.Bool("is_active").
			Default(true),
		field.String("description").
			Default(""),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
		field.Time("updated_at").
			Default(timeNow).
			UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (APIToken) Edges() []ent.Edge {
	return nil
}

func (APIToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("token_hash"),
	}
}

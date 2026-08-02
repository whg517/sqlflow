package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Role holds the schema definition for a persisted role.
// Maps to: roles table
type Role struct {
	ent.Schema
}

func (Role) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "roles"}}
}

func (Role) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Unique(),
		field.String("display_name"),
		field.String("description").Default(""),
		// Built-in roles cannot be deleted; see ADR-0008.
		field.Bool("is_builtin").Default(false),
		field.String("status").Default("active"),
		field.Time("created_at").Default(timeNow).Annotations(entsql.DefaultExpr("now()")),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("now()")),
	}
}

func (Role) Edges() []ent.Edge { return nil }

func (Role) Indexes() []ent.Index {
	return []ent.Index{index.Fields("name").Unique()}
}

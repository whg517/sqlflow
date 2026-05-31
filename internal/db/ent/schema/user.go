package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// User holds the schema definition for the User entity.
// Maps to: users table
type User struct {
	ent.Schema
}

// Annotations configures the SQL table name.
func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "users"},
	}
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("username").
			NotEmpty().
			Unique(),
		field.String("password_hash").
			NotEmpty().
			SchemaType(map[string]string{
				"sqlite3": "text",
			}).
			StructTag(`json:"-"`),
		field.String("role").
			Default("developer"),
		field.String("dingtalk_user_id").
			Default("").
			Optional(),
		field.String("dingtalk_union_id").
			Default("").
			Optional(),
		field.String("oidc_subject").
			Default(""),
		field.String("oidc_provider").
			Default(""),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
		field.Time("updated_at").
			Default(timeNow).
			UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return nil
}

// Indexes of the User.
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("oidc_subject", "oidc_provider"),
	}
}

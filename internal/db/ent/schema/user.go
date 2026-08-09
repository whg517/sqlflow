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
			Default(timeNow).Annotations(entsql.DefaultExpr("now()")),
		field.Time("updated_at").
			Default(timeNow).
			UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("now()")),
		// password_changed_at is the timestamp of the most recent password
		// reset. The JWT middleware compares a token's iat against it: a
		// token issued before this moment is rejected, so resetting a
		// user's password immediately invalidates any access tokens they
		// already hold — which is the entire point of an admin-initiated
		// reset, since it is almost always done because the old credential
		// may have leaked.
		field.Time("password_changed_at").
			Default(timeNow).
			UpdateDefault(timeNow).
			Annotations(entsql.DefaultExpr("now()")),
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

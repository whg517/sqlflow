package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// OIDCProvider holds the schema definition for the OIDCProvider entity.
// Maps to: oidc_providers table
type OIDCProvider struct {
	ent.Schema
}

func (OIDCProvider) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oidc_providers"},
	}
}

func (OIDCProvider) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			Unique(),
		field.String("issuer").
			NotEmpty(),
		field.String("client_id").
			NotEmpty(),
		field.String("client_secret").
			Default("").
			StructTag(`json:"-"`),
		field.String("scopes").
			Default("openid profile email"),
		field.Bool("enabled").
			Default(true),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
		field.Time("updated_at").
			Default(timeNow).
			UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (OIDCProvider) Edges() []ent.Edge {
	return nil
}

func (OIDCProvider) Indexes() []ent.Index {
	return nil
}

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CasbinRule holds the schema definition for a Casbin policy row.
// Maps to: casbin_rule table
//
// The column names are Casbin's, not ours: ptype selects the policy type and
// v0..v5 are its positional arguments, whose meaning depends on the model. See
// internal/authz for the tuples this project actually stores.
type CasbinRule struct {
	ent.Schema
}

func (CasbinRule) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "casbin_rule"}}
}

func (CasbinRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("ptype").Default(""),
		field.String("v0").Default(""),
		field.String("v1").Default(""),
		field.String("v2").Default(""),
		field.String("v3").Default(""),
		field.String("v4").Default(""),
		field.String("v5").Default(""),
	}
}

func (CasbinRule) Edges() []ent.Edge { return nil }

func (CasbinRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ptype", "v0"),
	}
}

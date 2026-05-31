package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TempPolicy holds the schema definition for the TempPolicy entity.
// Maps to: temp_policies table
type TempPolicy struct {
	ent.Schema
}

func (TempPolicy) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "temp_policies"},
	}
}

func (TempPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("sub").
			NotEmpty(),
		field.String("dom").
			NotEmpty(),
		field.String("obj").
			NotEmpty(),
		field.String("act").
			NotEmpty(),
		field.Time("expires_at"),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (TempPolicy) Edges() []ent.Edge {
	return nil
}

func (TempPolicy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("expires_at"),
		index.Fields("sub", "dom", "obj", "act").Unique(),
	}
}

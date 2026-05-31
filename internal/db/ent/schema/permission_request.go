package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PermissionRequest holds the schema definition for the PermissionRequest entity.
// Maps to: permission_requests table
type PermissionRequest struct {
	ent.Schema
}

func (PermissionRequest) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "permission_requests"},
	}
}

func (PermissionRequest) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("applicant_id"),
		field.Int64("datasource_id"),
		field.String("database").
			NotEmpty(),
		field.String("table_name").
			Default(""),
		field.String("actions").
			Default(""),
		field.String("reason").
			Default(""),
		field.String("status").
			Default("PENDING"),
		field.Int64("approver_id").
			Optional().
			Nillable(),
		field.String("approve_comment").
			Default(""),
		field.Time("approved_at").
			Optional().
			Nillable(),
		field.Time("expires_at"),
		field.Time("revoked_at").
			Optional().
			Nillable(),
		field.Int64("revoked_by").
			Optional().
			Nillable(),
		field.String("revoke_reason").
			Default(""),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
		field.Time("updated_at").
			Default(timeNow).
			UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (PermissionRequest) Edges() []ent.Edge {
	return nil
}

func (PermissionRequest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("applicant_id"),
		index.Fields("status"),
		index.Fields("datasource_id", "database"),
	}
}

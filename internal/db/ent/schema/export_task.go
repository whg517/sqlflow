package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ExportTask holds the schema definition for the ExportTask entity.
// Maps to: export_tasks table
type ExportTask struct {
	ent.Schema
}

func (ExportTask) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "export_tasks"},
	}
}

func (ExportTask) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("username").
			Default(""),
		field.String("export_type").
			Default(""),
		field.String("status").
			Default("pending"),
		field.String("filename").
			Default(""),
		field.String("file_path").
			Default(""),
		field.Int64("total_rows").
			Default(0),
		field.Int64("file_bytes").
			Default(0),
		field.String("filters_json").
			Default("{}"),
		field.String("error_msg").
			Default(""),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
		field.Time("completed_at").
			Optional().
			Nillable(),
	}
}

func (ExportTask) Edges() []ent.Edge {
	return nil
}

func (ExportTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("status"),
	}
}

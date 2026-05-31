package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ExecutionResult holds the schema definition for the ExecutionResult entity.
// Maps to: execution_results table
type ExecutionResult struct {
	ent.Schema
}

func (ExecutionResult) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "execution_results"},
	}
}

func (ExecutionResult) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("ticket_id"),
		field.Int("statement_index").
			Default(0),
		field.String("sql").
			NotEmpty(),
		field.String("status").
			Default(""),
		field.Int64("rows_affected").
			Default(0),
		field.String("error").
			Default(""),
		field.Int64("duration_ms").
			Default(0),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (ExecutionResult) Edges() []ent.Edge {
	return nil
}

func (ExecutionResult) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ticket_id"),
	}
}

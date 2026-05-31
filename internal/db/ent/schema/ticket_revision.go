package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TicketRevision holds the schema definition for the TicketRevision entity.
// Maps to: ticket_revisions table
type TicketRevision struct {
	ent.Schema
}

func (TicketRevision) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ticket_revisions"},
	}
}

func (TicketRevision) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("ticket_id"),
		field.Int("revision"),
		field.String("sql_content").
			NotEmpty(),
		field.String("sql_summary").
			Default(""),
		field.String("change_reason").
			Default(""),
		field.String("risk_level").
			Default(""),
		field.String("ai_review_result").
			Default(""),
		field.Int64("reviewer_id").
			Default(0),
		field.String("review_comment").
			Default(""),
		field.String("status").
			Default(""),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (TicketRevision) Edges() []ent.Edge {
	return nil
}

func (TicketRevision) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ticket_id"),
		index.Fields("ticket_id", "revision"),
	}
}

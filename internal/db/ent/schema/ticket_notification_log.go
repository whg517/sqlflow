package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TicketNotificationLog records that a ticket event was notified.
// Maps to: ticket_notification_logs table
//
// It exists to deduplicate: the scheduler can revisit a ticket, and an operator
// should not be paged twice for the same transition.
type TicketNotificationLog struct {
	ent.Schema
}

func (TicketNotificationLog) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "ticket_notification_logs"}}
}

func (TicketNotificationLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("ticket_id"),
		field.String("event_type"),
		field.Time("sent_at").Default(timeNow).Annotations(entsql.DefaultExpr("now()")),
		field.String("status").Default("sent"),
	}
}

func (TicketNotificationLog) Edges() []ent.Edge { return nil }

func (TicketNotificationLog) Indexes() []ent.Index {
	return []ent.Index{index.Fields("ticket_id", "event_type")}
}

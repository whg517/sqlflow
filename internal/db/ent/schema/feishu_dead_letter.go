package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// FeishuDeadLetter holds a Feishu notification that exhausted its retries.
// Maps to: feishu_dead_letters table
//
// Failed notifications are kept rather than dropped: a notification that never
// arrived is exactly the thing an operator needs to see afterwards.
type FeishuDeadLetter struct {
	ent.Schema
}

func (FeishuDeadLetter) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "feishu_dead_letters"}}
}

func (FeishuDeadLetter) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("webhook_id"),
		field.String("payload"),
		// The unique index is on this rather than on payload: PostgreSQL caps a
		// btree entry at roughly 2704 bytes and a card carrying a long SQL
		// statement exceeds that, which would reject the notification on the
		// error path where it is least likely to be noticed.
		field.String("payload_hash").Default(""),
		field.String("error_message").Default(""),
		field.Int64("attempt_count").Default(0),
		field.Time("last_attempt_at").Default(timeNow).Annotations(entsql.DefaultExpr("now()")),
		field.Time("created_at").Default(timeNow).Annotations(entsql.DefaultExpr("now()")),
	}
}

// Indexes declares the constraint the retry counter depends on.
//
// One row per failed notification, with a count of how many times sending it
// was tried. Without the constraint two workers failing the same message at
// once each create a row, and the operator sees one failure twice.
func (FeishuDeadLetter) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("webhook_id", "payload_hash").Unique(),
	}
}

func (FeishuDeadLetter) Edges() []ent.Edge { return nil }

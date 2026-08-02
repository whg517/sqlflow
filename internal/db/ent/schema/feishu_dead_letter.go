package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
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
		field.String("error_message").Default(""),
		field.Int64("attempt_count").Default(0),
		field.Time("last_attempt_at").Default(timeNow).Annotations(entsql.DefaultExpr("now()")),
		field.Time("created_at").Default(timeNow).Annotations(entsql.DefaultExpr("now()")),
	}
}

func (FeishuDeadLetter) Edges() []ent.Edge { return nil }

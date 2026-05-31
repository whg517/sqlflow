package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// GitLink holds the schema definition for the GitLink entity.
// Maps to: git_links table
type GitLink struct {
	ent.Schema
}

func (GitLink) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "git_links"},
	}
}

func (GitLink) Fields() []ent.Field {
	return []ent.Field{
		field.String("entity_type").
			Default("ticket"),
		field.Int64("entity_id").
			Default(0),
		field.String("link_type").
			Default("commit"),
		field.String("commit_hash").
			Default(""),
		field.String("commit_msg").
			Default(""),
		field.String("author_name").
			Default(""),
		field.String("author_email").
			Default(""),
		field.Int("pr_number").
			Default(0),
		field.String("pr_title").
			Default(""),
		field.String("pr_url").
			Default(""),
		field.String("repo_url").
			Default(""),
		field.String("branch").
			Default(""),
		field.Int64("created_by").
			Default(0),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (GitLink) Edges() []ent.Edge {
	return nil
}

func (GitLink) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("entity_type", "entity_id"),
	}
}

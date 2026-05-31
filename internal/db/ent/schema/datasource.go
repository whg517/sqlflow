package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// DataSource holds the schema definition for the DataSource entity.
// Maps to: datasources table
type DataSource struct {
	ent.Schema
}

func (DataSource) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "datasources"},
	}
}

func (DataSource) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			Unique(),
		field.String("type").
			NotEmpty(),
		field.String("host").
			NotEmpty(),
		field.Int("port"),
		field.String("username").
			Default(""),
		field.String("password_encrypted").
			Default("").
			StructTag(`json:"-"`),
		field.String("database").
			Default(""),
		field.Int("max_open").
			Default(10),
		field.Int("max_idle").
			Default(5),
		field.Int("max_lifetime").
			Default(3600),
		field.Int("max_idle_time").
			Default(600),
		field.String("status").
			Default("active"),
		// PostgreSQL specific
		field.String("sslmode").
			Default(""),
		field.String("schema_name").
			Default(""),
		// Elasticsearch specific
		field.String("es_urls").
			Default(""),
		field.String("es_version").
			Default(""),
		field.String("es_auth_type").
			Default(""),
		field.String("es_api_key").
			Default("").
			StructTag(`json:"-"`),
		field.String("es_index_pattern").
			Default(""),
		field.Bool("es_verify_certs").
			Default(true),
		field.Time("created_at").
			Default(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
		field.Time("updated_at").
			Default(timeNow).
			UpdateDefault(timeNow).Annotations(entsql.DefaultExpr("datetime('now')")),
	}
}

func (DataSource) Edges() []ent.Edge {
	return nil
}

func (DataSource) Indexes() []ent.Index {
	return nil
}

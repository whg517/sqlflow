-- Elasticsearch datasource support
ALTER TABLE datasources ADD COLUMN es_urls TEXT DEFAULT '';
ALTER TABLE datasources ADD COLUMN es_version TEXT DEFAULT '';
ALTER TABLE datasources ADD COLUMN es_auth_type TEXT DEFAULT '';
ALTER TABLE datasources ADD COLUMN es_api_key TEXT DEFAULT '';
ALTER TABLE datasources ADD COLUMN es_index_pattern TEXT DEFAULT '';
ALTER TABLE datasources ADD COLUMN es_verify_certs INTEGER NOT NULL DEFAULT 1;

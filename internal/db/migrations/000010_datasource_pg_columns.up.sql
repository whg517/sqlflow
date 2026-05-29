-- PostgreSQL datasource support: sslmode and schema_name columns
ALTER TABLE datasources ADD COLUMN sslmode TEXT DEFAULT '';
ALTER TABLE datasources ADD COLUMN schema_name TEXT DEFAULT '';

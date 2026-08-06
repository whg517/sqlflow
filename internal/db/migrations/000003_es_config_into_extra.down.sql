-- 回滚重建这五列并从 extra_config 抄回。extra_config 保持不变：它现在是这些设置的
-- 事实来源，清空它会让回滚变成数据丢失。
ALTER TABLE datasources
    ADD COLUMN IF NOT EXISTS es_urls TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS es_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS es_auth_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS es_index_pattern TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS es_verify_certs BOOLEAN NOT NULL DEFAULT TRUE;

UPDATE datasources
SET es_urls = coalesce(
        (SELECT string_agg(value #>> '{}', ',')
         FROM jsonb_array_elements(extra_config::jsonb -> 'urls')),
        extra_config::jsonb ->> 'urls',
        ''),
    es_auth_type = coalesce(extra_config::jsonb ->> 'auth_type', ''),
    es_index_pattern = coalesce(extra_config::jsonb ->> 'index_pattern', ''),
    es_version = coalesce(extra_config::jsonb ->> 'version', ''),
    es_verify_certs = coalesce((extra_config::jsonb ->> 'verify_certs')::boolean, TRUE)
WHERE type = 'elasticsearch' AND coalesce(extra_config, '') <> '';

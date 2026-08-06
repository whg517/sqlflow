-- 把 Elasticsearch 的专属配置列收进 extra_config。
--
-- 这五列是「新增数据源类型只需改两处」这个承诺的最大一处欠账。它们从 DDL 一路穿透到
-- ent schema、model、adapter 的键名 switch、三个请求结构体和前端表单——六层，每层都
-- 不知道这些值是什么意思。而同样需要专属配置的 MongoDB 一列都没有，adapter 里只能返回
-- 空串加一句 TODO。同一类需求两种处理方式，说明这个轴没有归属者。
--
-- 现在归属者是驱动自己：driver.ConfigDecoder（对称于既有的 ConfigValidator）。
--
-- es_api_key 不在此列。它是凭据不是配置，与 password_encrypted 同轴——密文存储、
-- 读取时解密、绝不进 extra_config（extra_config 存的是原样写入的内容，而凭据的存储
-- 副本是加密的）。凭据轴的泛化是另一件事。
--
-- 整段包在 DO 块里并先检查列是否存在：迁移必须可重跑。测试夹具用 ApplySchema 建库，
-- 它不写 schema_migrations，所以随后的 Migrate() 会把每个文件再跑一遍——上一个迁移
-- 恰好因为通篇 IF NOT EXISTS 而幸免，这个不会。

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'datasources'
          AND column_name = 'es_urls'
    ) THEN
        RETURN;
    END IF;

    -- 回填。ES 数据源可能已有 extra_config，所以是合并而非覆盖：已有的键优先，
    -- 因为它是使用者显式写下的，而列里的值可能是表单的陈旧默认。
    UPDATE datasources
    SET extra_config = (
        jsonb_build_object(
            'urls', CASE
                WHEN coalesce(es_urls, '') = '' THEN NULL
                ELSE to_jsonb(
                    array_remove(
                        array(SELECT btrim(unnest(string_to_array(es_urls, ',')))),
                        ''
                    )
                )
            END,
            'auth_type', nullif(es_auth_type, ''),
            'index_pattern', nullif(es_index_pattern, ''),
            'version', nullif(es_version, ''),
            'verify_certs', to_jsonb(es_verify_certs)
        )
        || coalesce(nullif(extra_config, '')::jsonb, '{}'::jsonb)
    )::text
    WHERE type = 'elasticsearch';

    -- jsonb_strip_nulls 去掉未配置的键，让「没写」和「写了空值」保持可区分。
    UPDATE datasources
    SET extra_config = (jsonb_strip_nulls(extra_config::jsonb))::text
    WHERE type = 'elasticsearch' AND coalesce(extra_config, '') <> '';

    ALTER TABLE datasources
        DROP COLUMN es_urls,
        DROP COLUMN es_version,
        DROP COLUMN es_auth_type,
        DROP COLUMN es_index_pattern,
        DROP COLUMN es_verify_certs;
END
$$;

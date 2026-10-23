-- Run in a reviewed maintenance window with a measured lock_timeout.
-- Deliberately transactional non-concurrent builds: rollback leaves no invalid
-- concurrent index. Install before reader opt-in, never during startup.
BEGIN;
DO $$
DECLARE extension_schema TEXT;
BEGIN
    SELECT n.nspname INTO extension_schema FROM pg_extension e
    JOIN pg_namespace n ON n.oid=e.extnamespace WHERE e.extname='pg_trgm';
    IF extension_schema IS NULL THEN RAISE EXCEPTION 'pg_trgm must be provisioned by the operator'; END IF;
    CREATE INDEX role_search_keyword_gin ON role_search USING GIN(fts_keyword);
    EXECUTE format('CREATE INDEX %I ON %I.%I USING GIN(fts_lexeme_text %I.gin_trgm_ops)',
                   'role_search_text_gin', current_schema(), 'role_search', extension_schema);
    CREATE INDEX permission_search_keyword_gin ON permission_search USING GIN(fts_keyword);
    EXECUTE format('CREATE INDEX %I ON %I.%I USING GIN(fts_lexeme_text %I.gin_trgm_ops)',
                   'permission_search_text_gin', current_schema(), 'permission_search', extension_schema);
    CREATE INDEX application_search_keyword_gin ON application_search USING GIN(fts_keyword);
    EXECUTE format('CREATE INDEX %I ON %I.%I USING GIN(fts_lexeme_text %I.gin_trgm_ops)',
                   'application_search_text_gin', current_schema(), 'application_search', extension_schema);
    CREATE INDEX user_role_search_keyword_gin ON user_role_search USING GIN(fts_keyword);
    EXECUTE format('CREATE INDEX %I ON %I.%I USING GIN(fts_lexeme_text %I.gin_trgm_ops)',
                   'user_role_search_text_gin', current_schema(), 'user_role_search', extension_schema);
END;
$$;
COMMIT;

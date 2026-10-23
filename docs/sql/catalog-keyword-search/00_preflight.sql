-- Read-only metadata. Run with psql -X -v ON_ERROR_STOP=1 on the explicitly
-- selected source database/schema. Contains no source rows or credentials.
BEGIN READ ONLY;

SELECT current_schema() AS source_schema, current_setting('server_version') AS version,
       has_schema_privilege(current_schema(), 'CREATE') AS can_create_schema_objects;

SELECT e.extname, n.nspname AS extension_schema, e.extversion
FROM pg_extension e JOIN pg_namespace n ON n.oid = e.extnamespace
WHERE e.extname = 'pg_trgm';

SELECT c.relname AS table_name,
       has_table_privilege(c.oid, 'SELECT') AS can_select,
       has_table_privilege(c.oid, 'INSERT') AS can_insert,
       has_table_privilege(c.oid, 'UPDATE') AS can_update,
       has_table_privilege(c.oid, 'TRIGGER') AS can_create_trigger
FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname=current_schema()
  AND c.relname IN ('auth','profile','role','permissions','applications','user_role',
    'role_search','permission_search','application_search','user_role_search')
ORDER BY c.relname;

SELECT table_name, column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND table_name IN ('auth', 'profile', 'role', 'permissions', 'applications',
                     'user_role', 'user_profile_search', 'role_search',
                     'permission_search', 'application_search', 'user_role_search')
ORDER BY table_name, ordinal_position;

SELECT c.relname AS table_name, con.conname, pg_get_constraintdef(con.oid) AS definition,
       con.convalidated
FROM pg_constraint con JOIN pg_class c ON c.oid = con.conrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = current_schema()
  AND c.relname IN ('auth', 'profile', 'role', 'permissions', 'applications',
                   'user_role', 'user_profile_search', 'role_search',
                   'permission_search', 'application_search', 'user_role_search')
ORDER BY c.relname, con.conname;

SELECT tablename, indexname, indexdef
FROM pg_indexes WHERE schemaname = current_schema()
  AND tablename IN ('user_profile_search', 'role_search', 'permission_search',
                    'application_search', 'user_role_search')
ORDER BY tablename, indexname;

SELECT c.relname AS table_name, t.tgname, pg_get_triggerdef(t.oid) AS definition
FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = current_schema() AND NOT t.tgisinternal
  AND c.relname IN ('auth', 'profile', 'role', 'permissions', 'applications', 'user_role')
ORDER BY c.relname, t.tgname;

SELECT p.oid::regprocedure::text AS signature, pg_get_functiondef(p.oid) AS definition
FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = current_schema() AND p.prokind = 'f'
  AND p.proname IN ('refresh_user_profile_search', 'trg_auth_refresh_user_profile_search',
                    'trg_profile_refresh_user_profile_search')
ORDER BY signature;

COMMIT;

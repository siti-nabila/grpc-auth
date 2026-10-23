-- Read-only retention check after consumers return to compatibility modes.
-- Do not drop projections/triggers during caller rollback.
BEGIN READ ONLY;
SELECT c.relname AS retained_projection, count(i.indexrelid) AS indexes
FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
LEFT JOIN pg_index i ON i.indrelid=c.oid AND i.indisvalid
WHERE n.nspname=current_schema()
  AND c.relname IN ('role_search','permission_search','application_search','user_role_search')
GROUP BY c.relname ORDER BY c.relname;
SELECT count(*) AS retained_triggers
FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid
JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname=current_schema() AND NOT t.tgisinternal
  AND t.tgname IN ('role_search_insert','role_search_update',
    'permission_search_insert','permission_search_update',
    'application_search_insert','application_search_update',
    'user_role_search_lock_insert','user_role_search_lock_update',
    'user_role_search_insert','user_role_search_update');
COMMIT;

-- Read-only completeness/content check; every discrepancy must be zero.
-- Does not log raw document values or personal data. Initial formulas only;
-- update expected formula here whenever document composition is extended.
BEGIN READ ONLY;
SELECT 'role_search' AS projection,
       count(*) FILTER (WHERE p.role_id IS NULL) AS missing,
       count(*) FILTER (WHERE p.fts_keyword IS DISTINCT FROM to_tsvector('simple', catalog_search_normalize(concat_ws(' ', s.role_name, s.code)))
                        OR p.fts_lexeme_text IS DISTINCT FROM array_to_string(tsvector_to_array(p.fts_keyword), ' ')) AS stale
FROM role s LEFT JOIN role_search p ON p.role_id=s.id;
SELECT 'role_search' AS projection, count(*) AS orphan
FROM role_search p LEFT JOIN role s ON s.id=p.role_id WHERE s.id IS NULL;
SELECT 'role_search' AS projection, count(*) AS duplicate_keys
FROM (SELECT role_id FROM role_search GROUP BY role_id HAVING count(*) > 1) repeated;
SELECT 'permission_search' AS projection,
       count(*) FILTER (WHERE p.permission_id IS NULL) AS missing,
       count(*) FILTER (WHERE p.fts_keyword IS DISTINCT FROM to_tsvector('simple', catalog_search_normalize(s.code))
                        OR p.fts_lexeme_text IS DISTINCT FROM array_to_string(tsvector_to_array(p.fts_keyword), ' ')) AS stale
FROM permissions s LEFT JOIN permission_search p ON p.permission_id=s.id;
SELECT 'permission_search' AS projection, count(*) AS orphan
FROM permission_search p LEFT JOIN permissions s ON s.id=p.permission_id WHERE s.id IS NULL;
SELECT 'permission_search' AS projection, count(*) AS duplicate_keys
FROM (SELECT permission_id FROM permission_search GROUP BY permission_id HAVING count(*) > 1) repeated;
SELECT 'application_search' AS projection,
       count(*) FILTER (WHERE p.application_id IS NULL) AS missing,
       count(*) FILTER (WHERE p.fts_keyword IS DISTINCT FROM to_tsvector('simple', catalog_search_normalize(concat_ws(' ', s.code, s.name)))
                        OR p.fts_lexeme_text IS DISTINCT FROM array_to_string(tsvector_to_array(p.fts_keyword), ' ')) AS stale
FROM applications s LEFT JOIN application_search p ON p.application_id=s.id;
SELECT 'application_search' AS projection, count(*) AS orphan
FROM application_search p LEFT JOIN applications s ON s.id=p.application_id WHERE s.id IS NULL;
SELECT 'application_search' AS projection, count(*) AS duplicate_keys
FROM (SELECT application_id FROM application_search GROUP BY application_id HAVING count(*) > 1) repeated;
SELECT 'user_role_search' AS projection,
       count(*) FILTER (WHERE p.user_role_id IS NULL) AS missing,
       count(*) FILTER (WHERE p.fts_keyword IS DISTINCT FROM to_tsvector('simple', catalog_search_normalize(r.role_name))
                        OR p.fts_lexeme_text IS DISTINCT FROM array_to_string(tsvector_to_array(p.fts_keyword), ' ')) AS stale
FROM user_role s JOIN role r ON r.id=s.role_id LEFT JOIN user_role_search p ON p.user_role_id=s.id;
SELECT 'user_role_search' AS projection, count(*) AS orphan
FROM user_role_search p LEFT JOIN user_role s ON s.id=p.user_role_id WHERE s.id IS NULL;
SELECT 'user_role_search' AS projection, count(*) AS duplicate_keys
FROM (SELECT user_role_id FROM user_role_search GROUP BY user_role_id HAVING count(*) > 1) repeated;
COMMIT;

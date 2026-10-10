-- Read-only. Jalankan sebelum 01_schema_backfill.sql.
-- Tidak menampilkan email, password/hash, token, atau data profil.
BEGIN TRANSACTION READ ONLY;

SELECT table_name, column_name, data_type, is_nullable, column_default
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name IN ('auth', 'profile', 'role', 'user_role', 'user_profile_search')
ORDER BY table_name, ordinal_position;

SELECT t.relname AS table_name, c.conname AS constraint_name,
       pg_get_constraintdef(c.oid) AS definition
FROM pg_constraint c
JOIN pg_class t ON t.oid = c.conrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE n.nspname = 'public'
  AND t.relname IN ('auth', 'profile', 'role', 'user_role', 'user_profile_search')
ORDER BY t.relname, c.conname;

SELECT tablename, indexname, indexdef
FROM pg_indexes
WHERE schemaname = 'public'
  AND tablename IN ('auth', 'profile', 'role', 'user_role', 'user_profile_search')
ORDER BY tablename, indexname;

SELECT 'auth_invalid_email' AS check_name, count(*) AS violations
FROM public.auth
WHERE email IS NULL OR btrim(email) = '' OR email <> lower(btrim(email))
UNION ALL
SELECT 'auth_missing_password', count(*) FROM public.auth
WHERE password IS NULL OR btrim(password) = ''
UNION ALL
SELECT 'auth_duplicate_email_groups', count(*)
FROM (SELECT email FROM public.auth GROUP BY email HAVING count(*) > 1) d
UNION ALL
SELECT 'profile_duplicate_user_groups', count(*)
FROM (SELECT user_id FROM public.profile GROUP BY user_id HAVING count(*) > 1) d
UNION ALL
SELECT 'profile_orphan_user', count(*) FROM public.profile p
WHERE NOT EXISTS (SELECT 1 FROM public.auth a WHERE a.id = p.user_id)
UNION ALL
SELECT 'user_role_duplicate_pairs', count(*)
FROM (SELECT user_id, role_id FROM public.user_role
      GROUP BY user_id, role_id HAVING count(*) > 1) d
UNION ALL
SELECT 'user_role_orphan_user', count(*) FROM public.user_role ur
WHERE NOT EXISTS (SELECT 1 FROM public.auth a WHERE a.id = ur.user_id)
UNION ALL
SELECT 'user_role_orphan_role', count(*) FROM public.user_role ur
WHERE NOT EXISTS (SELECT 1 FROM public.role r WHERE r.id = ur.role_id)
UNION ALL
SELECT 'search_orphan_profile', count(*) FROM public.user_profile_search s
WHERE NOT EXISTS (SELECT 1 FROM public.profile p WHERE p.id = s.profile_id)
UNION ALL
SELECT 'role_code_sequence_exhausted', count(*) FROM public.role
WHERE role_code = 2147483647;

-- Zero rows berarti platform admin aktif belum tersedia setelah backfill.
-- Ini tidak membuat user atau assignment admin baru.
SELECT count(DISTINCT a.id) AS existing_active_admin_candidates
FROM public.auth a
JOIN public.user_role ur ON ur.user_id = a.id
JOIN public.role r ON r.id = ur.role_id
WHERE a.deleted_at IS NULL AND r.role_code = 2;

-- Counter kumulatif sejak reset statistik. Gunakan EXPLAIN pada query nyata
-- sebelum menghapus idx_id_email; idx_scan=0 sendiri bukan bukti index redundant.
SELECT schemaname, relname, indexrelname, idx_scan
FROM pg_stat_user_indexes
WHERE schemaname = 'public'
  AND relname IN ('auth', 'profile', 'role', 'user_role', 'user_profile_search')
ORDER BY relname, indexrelname;

SELECT e.extname, n.nspname AS extension_schema
FROM pg_extension e JOIN pg_namespace n ON n.oid = e.extnamespace
WHERE e.extname = 'pg_trgm';

COMMIT;

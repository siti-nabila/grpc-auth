-- Jalankan setelah 01_schema_backfill.sql. Seed dapat diulang (no-op jika sama).
-- role_code=2 -> iam.platform_admin telah disetujui user.
-- Tidak membuat user, password, session, refresh token, assignment, atau app access.
BEGIN;
SET LOCAL TIME ZONE 'Asia/Jakarta';
SET LOCAL lock_timeout = '10s';
SET LOCAL statement_timeout = '5min';
SELECT pg_advisory_xact_lock(7301, 1);
SELECT id FROM public.admin_guard WHERE id = 1 FOR UPDATE;
LOCK TABLE public.role, public.permissions, public.role_permissions
    IN SHARE ROW EXCLUSIVE MODE;

INSERT INTO public.applications (code, name, status, is_system)
VALUES
    ('iam', 'Identity and Access Management', 'ACTIVE', true),
    ('clm', 'Contract Lifecycle Management', 'ACTIVE', false)
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name, status = EXCLUDED.status, is_system = EXCLUDED.is_system
WHERE (applications.name, applications.status, applications.is_system)
      IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.status, EXCLUDED.is_system);

INSERT INTO public.permissions (application_id, code, description)
SELECT a.id, p.code, p.description
FROM (VALUES
    ('iam', 'iam.user.read', 'List dan detail user administrasi'),
    ('iam', 'iam.user.create', 'Membuat user melalui admin'),
    ('iam', 'iam.user.update', 'Mengubah field administratif user'),
    ('iam', 'iam.user.status.update', 'Activate, suspend, reactivate, deactivate user'),
    ('iam', 'iam.role.read', 'List dan detail role'),
    ('iam', 'iam.role.create', 'Membuat custom role aplikasi bisnis'),
    ('iam', 'iam.role.update', 'Mengubah metadata/status custom role'),
    ('iam', 'iam.role.permission.update', 'Replace permission custom role'),
    ('iam', 'iam.user.role.grant', 'Grant role ke user'),
    ('iam', 'iam.user.role.revoke', 'Revoke assignment role'),
    ('iam', 'iam.user.application.grant', 'Grant akses aplikasi dan role awal'),
    ('iam', 'iam.user.application.revoke', 'Revoke akses aplikasi dan role terkait'),
    ('iam', 'iam.permission.read', 'Membaca catalog permission/aplikasi'),
    ('iam', 'iam.session.read', 'Membaca metadata sesi user lain'),
    ('iam', 'iam.session.revoke', 'Mencabut sesi user lain'),
    ('iam', 'iam.audit.read', 'Membaca audit administratif'),
    ('clm', 'clm.contract.read', 'Membaca kontrak sesuai resource authorization'),
    ('clm', 'clm.contract.create', 'Membuat draft kontrak'),
    ('clm', 'clm.contract.update', 'Mengedit draft kontrak'),
    ('clm', 'clm.contract.submit', 'Mengajukan review kontrak'),
    ('clm', 'clm.contract.review', 'Memberikan hasil review kontrak'),
    ('clm', 'clm.contract.approve', 'Memberikan keputusan approval kontrak'),
    ('clm', 'clm.contract.archive', 'Mengarsipkan kontrak'),
    ('clm', 'clm.audit.read', 'Membaca audit kontrak sesuai resource authorization')
) AS p(application_code, code, description)
JOIN public.applications a ON a.code = p.application_code
ON CONFLICT (code) DO UPDATE
SET description = EXCLUDED.description
WHERE permissions.application_id = EXCLUDED.application_id
  AND permissions.description IS DISTINCT FROM EXCLUDED.description;

-- Seed role dan sinkronisasi mapping sistem.
-- Setiap role/mapping yang benar-benar berubah mencatat audit+outbox atomik.
-- Member/role legacy tidak memperoleh permission administratif.
DO $$
DECLARE
    s record;
    r public.role%ROWTYPE;
    app_id integer;
    old_codes text[];
    new_codes text[];
    before_data jsonb;
    after_data jsonb;
    event_kind text;
    evt uuid;
    changed boolean;
BEGIN
    SELECT id INTO STRICT app_id FROM public.applications WHERE code = 'iam';

    FOR s IN
        SELECT * FROM (VALUES
            ('iam.member', 1, 'Member', 'Legacy default role tanpa permission admin'),
            ('iam.platform_admin', 2, 'Platform Administrator', 'Seluruh permission administrasi IAM'),
            ('iam.auditor', NULL::integer, 'IAM Auditor', 'Read-only user, role, permission, audit')
        ) AS v(code, numeric_code, name, description)
    LOOP
        SELECT * INTO r FROM public.role WHERE code = s.code FOR UPDATE;
        changed := false;
        before_data := NULL;

        IF NOT FOUND THEN
            -- Table lock membuat alokasi numeric code untuk seed serial.
            INSERT INTO public.role
                (role_name, role_code, role_description, application_id, code, status, is_system)
            VALUES
                (s.name,
                 coalesce(s.numeric_code,
                     (SELECT greatest(coalesce(max(role_code), 0), 0) + 1 FROM public.role)),
                 s.description, app_id, s.code, 'ACTIVE', true)
            RETURNING * INTO r;
            event_kind := 'iam.role.created.v1';
            changed := true;
        ELSE
            IF r.application_id <> app_id
               OR (s.numeric_code IS NOT NULL AND r.role_code <> s.numeric_code) THEN
                RAISE EXCEPTION 'System role code/application mapping conflict: %', s.code;
            END IF;
            IF r.role_name IS DISTINCT FROM s.name
               OR r.role_description IS DISTINCT FROM s.description
               OR r.status <> 'ACTIVE'
               OR r.is_system IS DISTINCT FROM true THEN
                before_data := jsonb_build_object('code', r.code, 'name', r.role_name,
                    'description', r.role_description, 'status', r.status, 'is_system', r.is_system);
                UPDATE public.role SET role_name = s.name, role_description = s.description,
                    status = 'ACTIVE', is_system = true, version = version + 1,
                    updated_at = CURRENT_TIMESTAMP
                WHERE id = r.id RETURNING * INTO r;
                event_kind := 'iam.role.updated.v1';
                changed := true;
            END IF;
        END IF;

        IF changed THEN
            after_data := jsonb_build_object('code', r.code, 'name', r.role_name,
                'description', r.role_description, 'status', r.status, 'is_system', r.is_system);
            INSERT INTO public.audit_logs
                (actor_service, action, target_type, target_id, safe_before, safe_after,
                 reason, request_id)
            VALUES ('database-migration', event_kind, 'role', r.id::text,
                    before_data, after_data, 'Synchronize IAM system role catalog',
                    'migration:iam-catalog-v1');
            evt := gen_random_uuid();
            INSERT INTO public.outbox_events
                (event_id, event_type, aggregate_type, aggregate_id, aggregate_version, payload)
            VALUES (evt, event_kind, 'role', r.id::text, r.version,
                jsonb_build_object('event_id', evt, 'event_type', event_kind, 'schema_version', 1,
                    'occurred_at', CURRENT_TIMESTAMP, 'producer', 'auth-service',
                    'aggregate_type', 'role', 'aggregate_id', r.id::text,
                    'aggregate_version', r.version, 'actor_service', 'database-migration',
                    'correlation_id', 'migration:iam-catalog-v1', 'data', after_data));
        END IF;

        SELECT coalesce(array_agg(p.code::text ORDER BY p.code), ARRAY[]::text[])
        INTO old_codes
        FROM public.role_permissions rp JOIN public.permissions p ON p.id = rp.permission_id
        WHERE rp.role_id = r.id;

        SELECT coalesce(array_agg(p.code::text ORDER BY p.code), ARRAY[]::text[])
        INTO new_codes FROM public.permissions p
        WHERE p.application_id = app_id AND (
            (s.code = 'iam.platform_admin'
            AND p.code IN (
                'iam.user.read', 'iam.user.create', 'iam.user.update', 'iam.user.status.update',
                'iam.role.read', 'iam.role.create', 'iam.role.update', 'iam.role.permission.update',
                'iam.user.role.grant', 'iam.user.role.revoke', 'iam.user.application.grant',
                'iam.user.application.revoke', 'iam.permission.read', 'iam.session.read',
                'iam.session.revoke', 'iam.audit.read'))
            OR (s.code = 'iam.auditor'
            AND p.code IN ('iam.user.read', 'iam.role.read', 'iam.permission.read', 'iam.audit.read'))
        );

        IF old_codes IS DISTINCT FROM new_codes THEN
            DELETE FROM public.role_permissions WHERE role_id = r.id;
            INSERT INTO public.role_permissions (role_id, permission_id)
            SELECT r.id, p.id FROM public.permissions p WHERE p.code = ANY(new_codes);
            UPDATE public.role SET version = version + 1, updated_at = CURRENT_TIMESTAMP
            WHERE id = r.id RETURNING * INTO r;
            before_data := jsonb_build_object('permission_codes', old_codes);
            after_data := jsonb_build_object('code', r.code, 'permission_codes', new_codes);
            event_kind := 'iam.role.permissions_changed.v1';
            INSERT INTO public.audit_logs
                (actor_service, action, target_type, target_id, safe_before, safe_after,
                 reason, request_id)
            VALUES ('database-migration', event_kind, 'role', r.id::text,
                    before_data, after_data, 'Synchronize IAM system role permission catalog',
                    'migration:iam-catalog-v1');
            evt := gen_random_uuid();
            INSERT INTO public.outbox_events
                (event_id, event_type, aggregate_type, aggregate_id, aggregate_version, payload)
            VALUES (evt, event_kind, 'role', r.id::text, r.version,
                jsonb_build_object('event_id', evt, 'event_type', event_kind, 'schema_version', 1,
                    'occurred_at', CURRENT_TIMESTAMP, 'producer', 'auth-service',
                    'aggregate_type', 'role', 'aggregate_id', r.id::text,
                    'aggregate_version', r.version, 'actor_service', 'database-migration',
                    'correlation_id', 'migration:iam-catalog-v1', 'data', after_data));
        END IF;
    END LOOP;

    IF EXISTS (SELECT 1 FROM public.permissions p JOIN public.applications a ON a.id = p.application_id
               WHERE split_part(p.code, '.', 1) <> a.code)
       OR EXISTS (SELECT 1 FROM public.role r JOIN public.applications a ON a.id = r.application_id
                  WHERE split_part(r.code, '.', 1) <> a.code)
       OR EXISTS (SELECT 1 FROM public.role_permissions rp
                  JOIN public.role r ON r.id = rp.role_id
                  JOIN public.permissions p ON p.id = rp.permission_id
                  WHERE r.application_id <> p.application_id) THEN
        RAISE EXCEPTION 'Catalog application/code/permission mismatch';
    END IF;

    PERFORM setval('public.role_numeric_code_seq',
        (SELECT greatest(coalesce(max(role_code), 0), 0)::bigint + 1 FROM public.role), false);
END $$;

-- Hasil aman untuk verifikasi, tanpa data pribadi.
SELECT a.code AS application_code, r.code AS role_code, r.role_code AS legacy_numeric_code,
       r.is_system, r.status, count(rp.permission_id) AS permission_count
FROM public.role r JOIN public.applications a ON a.id = r.application_id
LEFT JOIN public.role_permissions rp ON rp.role_id = r.id
GROUP BY a.code, r.id ORDER BY a.code, r.code;

-- Harus >= 1 sebelum enforcement IAM diaktifkan.
-- Jika 0, bootstrap admin eksplisit melalui auth service; seed tidak memilih user otomatis.
SELECT count(DISTINCT a.id) AS active_platform_admins
FROM public.auth a
JOIN public.user_role ur ON ur.user_id = a.id AND ur.status = 'ACTIVE'
JOIN public.role r ON r.id = ur.role_id
WHERE a.status = 'ACTIVE' AND r.status = 'ACTIVE' AND r.code = 'iam.platform_admin';

COMMIT;

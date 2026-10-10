-- OPSIONAL: jalankan ketika integrasi CLM dimulai, setelah seed catalog.
-- Membuat role awal tanpa grant user/app access.
-- Role yang sudah ada tidak direset agar perubahan admin tetap dipertahankan.
BEGIN;
SET LOCAL TIME ZONE 'Asia/Jakarta';
SET LOCAL lock_timeout = '10s';
SET LOCAL statement_timeout = '5min';
SELECT pg_advisory_xact_lock(7301, 1);
LOCK TABLE public.role, public.permissions, public.role_permissions
    IN SHARE ROW EXCLUSIVE MODE;

DO $$
DECLARE
    s record;
    r public.role%ROWTYPE;
    app_id integer;
    permission_count integer;
    evt uuid;
    data jsonb;
BEGIN
    SELECT id INTO STRICT app_id FROM public.applications WHERE code = 'clm';
    FOR s IN
        SELECT * FROM (VALUES
            ('clm.author', 'Contract Author', 'Membuat dan mengajukan draft kontrak',
                ARRAY['clm.contract.read', 'clm.contract.create', 'clm.contract.update', 'clm.contract.submit']),
            ('clm.legal_reviewer', 'Legal Reviewer', 'Melakukan review legal kontrak',
                ARRAY['clm.contract.read', 'clm.contract.review']),
            ('clm.approver', 'Contract Approver', 'Memberikan approval kontrak',
                ARRAY['clm.contract.read', 'clm.contract.approve']),
            ('clm.auditor', 'CLM Auditor', 'Membaca kontrak dan audit kontrak',
                ARRAY['clm.contract.read', 'clm.audit.read'])
        ) AS v(code, name, description, permissions)
    LOOP
        SELECT * INTO r FROM public.role WHERE code = s.code FOR UPDATE;
        IF FOUND THEN
            IF r.application_id <> app_id THEN
                RAISE EXCEPTION 'CLM role application mismatch: %', s.code;
            END IF;
            CONTINUE;
        END IF;

        SELECT count(*) INTO permission_count FROM public.permissions
        WHERE application_id = app_id AND code = ANY(s.permissions);
        IF permission_count <> cardinality(s.permissions) THEN
            RAISE EXCEPTION 'CLM permission catalog incomplete: %', s.code;
        END IF;

        INSERT INTO public.role
            (role_name, role_description, application_id, code, status, is_system)
        VALUES (s.name, s.description, app_id, s.code, 'ACTIVE', false)
        RETURNING * INTO r;

        INSERT INTO public.role_permissions (role_id, permission_id)
        SELECT r.id, p.id FROM public.permissions p
        WHERE p.application_id = app_id AND p.code = ANY(s.permissions);

        data := jsonb_build_object('code', r.code, 'application_code', 'clm',
            'name', r.role_name, 'status', r.status, 'permission_codes', s.permissions);
        INSERT INTO public.audit_logs
            (actor_service, action, target_type, target_id, safe_after, reason, request_id)
        VALUES ('database-migration', 'iam.role.created.v1', 'role', r.id::text,
            data, 'Seed initial CLM role catalog', 'migration:clm-role-catalog-v1');

        evt := gen_random_uuid();
        INSERT INTO public.outbox_events
            (event_id, event_type, aggregate_type, aggregate_id, aggregate_version, payload)
        VALUES (evt, 'iam.role.created.v1', 'role', r.id::text, r.version,
            jsonb_build_object('event_id', evt, 'event_type', 'iam.role.created.v1',
                'schema_version', 1, 'occurred_at', CURRENT_TIMESTAMP,
                'producer', 'auth-service', 'aggregate_type', 'role',
                'aggregate_id', r.id::text, 'aggregate_version', r.version,
                'actor_service', 'database-migration',
                'correlation_id', 'migration:clm-role-catalog-v1', 'data', data));
    END LOOP;
END $$;

COMMIT;

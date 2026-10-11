-- Prerequisite for auth-session-lifecycle. Review/run explicitly before cutover.
-- This file is prepared only; the agent does not execute it on the application DB.
-- Existing schema owner is public, as in user-management/01_schema_backfill.sql.
BEGIN;

-- Stop concurrent account invalidation and session issuance during the backfill.
LOCK TABLE public.auth, public.sessions IN SHARE ROW EXCLUSIVE MODE;
ALTER TABLE public.sessions ADD COLUMN auth_version bigint;
ALTER TABLE public.sessions ADD CONSTRAINT sessions_auth_version_ck
    CHECK (auth_version IS NULL OR auth_version >= 1);

-- ACTIVE sessions have not crossed global invalidation: that command revokes all
-- active sessions atomically. Never infer the historical generation of revoked
-- sessions from the current account. NULL legacy generations fail closed.
UPDATE public.sessions AS s
SET auth_version = a.auth_version
FROM public.auth AS a
WHERE a.id = s.user_id AND s.status = 'ACTIVE' AND s.revoked_at IS NULL;

-- No DEFAULT: every new Login must explicitly persist its authoritative generation.
COMMIT;

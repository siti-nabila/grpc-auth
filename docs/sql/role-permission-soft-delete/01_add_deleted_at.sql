-- Reviewed expansion only. Select the target schema explicitly in search_path.
-- Run before activating the new binary; never run automatically at startup.
DO $$
DECLARE
    target regclass := to_regclass('role_permissions');
    role_col smallint;
    permission_col smallint;
    col record;
BEGIN
    IF target IS NULL THEN
        RAISE EXCEPTION 'role_permissions is missing';
    END IF;
    LOCK TABLE role_permissions IN ACCESS EXCLUSIVE MODE;
    SELECT attnum INTO role_col FROM pg_attribute
      WHERE attrelid=target AND attname='role_id' AND NOT attisdropped;
    SELECT attnum INTO permission_col FROM pg_attribute
      WHERE attrelid=target AND attname='permission_id' AND NOT attisdropped;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid=target
        AND contype='p' AND convalidated
        AND conkey=ARRAY[role_col,permission_col]::smallint[]) THEN
        RAISE EXCEPTION 'role_permissions requires the existing composite primary key';
    END IF;
    SELECT atttypid, attnotnull, atthasdef, attidentity, attgenerated INTO col
      FROM pg_attribute WHERE attrelid=target AND attname='deleted_at' AND NOT attisdropped;
    IF FOUND THEN
        IF col.atttypid <> 'timestamptz'::regtype OR col.attnotnull OR col.atthasdef
            OR col.attidentity <> '' OR col.attgenerated <> '' THEN
            RAISE EXCEPTION 'role_permissions.deleted_at has an incompatible definition';
        END IF;
    ELSE
        ALTER TABLE role_permissions ADD COLUMN deleted_at timestamptz NULL;
    END IF;
END $$;

# Role permission soft delete

Review [01_add_deleted_at.sql](01_add_deleted_at.sql), explicitly select the
application schema through `search_path`, and apply it during a coordinated
maintenance window before activating the compatible service binary. The script
locks the table, checks the existing composite primary key, adds nullable
`deleted_at timestamptz` without a default, and rejects incompatible existing
columns. Existing rows remain active (`NULL`); no row is recreated or backfilled.
The script is repeatable for a compatible schema.

Applied to application database `user`, table `public.role_permissions`, on
2026-10-15 at 14:00:44 UTC under explicit user authorization. Commit and schema
verification succeeded; all 30 mappings and the composite primary key were
preserved. See the [application migration record](../../../openspec/changes/archive/2026-10-16-auth-repository-orm-only/evidence/user-database-migration.json).
Service activation and deployment remain separate.

Fresh installations use the matching definition in
[user-management schema](../user-management/01_schema_backfill.sql).
Do not rerun that baseline migration on an existing database.

Stop old readers/writers before enabling tombstone writes. After the first soft
delete, an old binary that ignores `deleted_at` can restore revoked authority.
Rollback must use a compatible binary that still honors tombstones. If none is
available, stop traffic and forward-fix. Keep the additive column; never clear
tombstones or drop the column to make an old binary usable. Before activation,
baseline binary rollback is possible while keeping the added nullable column.

Regrant restores the retained pair by clearing `deleted_at`. Existing audit
records remain the event history; mappings hard-deleted before this expansion
cannot be reconstructed. No application migration runs during startup or tests;
opt-in PostgreSQL scenarios apply the script only inside fixture-owned schemas.

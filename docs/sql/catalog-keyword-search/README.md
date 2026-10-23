# Catalog keyword search migration

This package is separate from existing user-management migration scripts.
`00_preflight.sql` reads metadata only in a READ ONLY transaction. It never
reads source row payloads, DSNs or credentials. Select the intended database
and source schema explicitly with the SQL client; do not rely on a default
connection. Review function definitions privately before sharing metadata.

Preflight must confirm PostgreSQL16+, source INTEGER PK/FKs, existing users
search keys/indexes, schema/extension/privilege and the complete users refresh
and wrapper definitions. A missing `pg_trgm`, unknown wrapper body or unverified
live schema is an unresolved gate, not evidence that deployment is ready.

No application database, migration or server is run automatically by this
change. Dedicated opt-in PostgreSQL fixtures and rollout evidence are tracked
in the OpenSpec change. Source functions/users formula remain untouched.

## Apply order

1. Read `00_preflight.sql` output and choose a measured installation window,
   client lock timeout and backfill batch size. Provision pg_trgm separately
   in the selected database; the harness/migration never installs it. Record
   extension schema and privilege; do not move an existing extension.
2. Set `search_path` explicitly to the source schema then pg_catalog. Run
   `01_schema_functions_triggers.sql` once with `ON_ERROR_STOP`. It locks
   source tables briefly while installing the additive projection/trigger
   boundary. Existing objects are not silently adopted. Every helper captures
   this reviewed search_path; never include an untrusted writable schema.
   Functions use SECURITY INVOKER. Before enabling writes, verify the existing
   runtime DB role has schema USAGE/helper EXECUTE and SELECT on projections;
   source writers also need SELECT/INSERT/UPDATE on their projections. If the
   migration owner differs from the application role, grant only to the
   explicitly verified existing role as part of the operator migration; do not
   grant projection writes to PUBLIC or use SECURITY DEFINER to bypass grants.
3. Install `02_backfill.sql`. Invoke `catalog_search_backfill` with four
   continuation IDs (role, permission, application, assignment) and a positive
   batch size chosen from measurement. Initial IDs are zero. Each call runs
   in a separate transaction; persist each returned last_id only after commit.
   Repeat until every source reports processed=0. Restarting a committed
   batch is safe; membership/status does not exclude source rows.
4. Run `03_indexes.sql` during the measured maintenance window. Builds are
   transactional and non-concurrent; failure rolls the index transaction
   back, so there is no invalid concurrent index to adopt on retry. The trigram
   operator class is qualified using the actual extension schema. Large-data
   concurrent builds need a separately reviewed operational script outside
   transaction blocks; do not silently substitute them here.
5. Run `04_verify.sql`; missing/stale/orphan/duplicate counts must all be zero.
   Confirm PK/FK/nonnull and both GIN indexes, trigger failure atomicity,
   concurrent rename/grant/reassign/backfill, and query plans before caller
   opt-in. Formula extension must also update the expected verification formula.
6. Deploy the additive backend, then perform separately owned consumer cutover
   after all readiness gates. Never opt in while projections are incomplete.

Refresh triggers cover role insert/name/code, permission insert/code,
application insert/code/name, assignment insert/role_id and role-name fan-out
to ACTIVE/REVOKED assignments. Assignment BEFORE triggers lock both old/new
role rows in ID order; refresh locks source before projection. Rename only
updates projection rows in assignment-key order, avoiding reverse locks on
source assignment rows. Status/version/reason alone do not refresh search.

## Rollback

Return callers to their old explicit fields and contains/prefix modes first;
discard keyword cursors. An older backend remains compatible with the additive
schema. Keep tables, helpers, indexes and triggers during compatibility; there
is no destructive rollback or runtime search fallback. Use
`05_rollback_check.sql` to verify the retained boundary still exists. Physical
cleanup needs a separate reviewed migration after no caller depends on it.

Default tests use mocks. Opt-in PostgreSQL tests read these reviewed scripts
and install them only in their owned disposable schema; users/application
schema and migrations are not executed. See the
[dedicated harness](../../../tests/integration/postgres/README.md).

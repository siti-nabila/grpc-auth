# Role management API

Backend implementation of OpenSpec `auth-role-management` is complete. These additive
contracts are published in api-contracts commit `fd1147049d52`. The production
assembly registers27 methods across7 services. All required mutation/read, full workspace/standalone test/vet/race/build and
dedicated PostgreSQL normal/race gates pass. Actual deployment remains Phase6.

## Catalog reads

| RPC | Exact current permission |
| --- | --- |
| `/role.v1.RoleService/ListRoles` | `iam.role.read` |
| `/role.v1.RoleService/GetRole` | `iam.role.read` |
| `/permission.v1.PermissionService/ListPermissions` | `iam.permission.read` |
| `/application.v1.ApplicationService/ListApplications` | `iam.permission.read` |

Each read requires a verified service caller allowed the exact method and IAM
namespace, plus an ACTIVE unrestricted user/session. The feature rechecks the
session and exact permission inside one primary read-only repeatable-read
transaction before reading the target, total or collection. Errors return no
partial data. An authorized missing role returns NOT_FOUND; a valid nonexistent
application filter returns an empty page.

Lists accept `paginator.v1.PageQuery`: page defaults1, limit defaults20 and must
not exceed100. `PageMeta.total` counts distinct matching roots before cursor,
offset or limit. Status alternatives ACTIVE/DISABLED combine OR; application,
status and search categories combine AND. Permission has no status filter.

Every public scalar supports ASC/DESC and ordered multi-sort. Duplicate or
unknown fields fail. Default order is id ASC; id ASC is appended when absent.
Text ordering and keyset comparison use PostgreSQL C collation. Nullable role
description, created_at and updated_at sort nulls last in both directions.

Role fields: id, code, name, description, application_id, application_code,
status, is_system, version, role_code, created_at, updated_at,
affected_user_count. Permission fields: id, application_id, application_code,
code, description. Application fields: id, code, name, status, is_system.

Projection absent returns all public fields; otherwise selected fields plus id.
Hidden sort values remain available to form the cursor. No credential fields or
assignment identities are returned. String role code differs from numeric
role_code. Nullable metadata remains absent instead of becoming an empty string
or zero timestamp. `affected_user_count` counts distinct existing users with an
ACTIVE assignment, including pending, suspended, deactivated and soft-deleted
accounts. REVOKED assignments do not count. Role/application/access status and
permission mapping cardinality do not change this count.

Search trims at most200 characters; empty means no search. UNSPECIFIED/CONTAINS
uses case-insensitive literal contains, PREFIX uses literal prefix. Percent,
underscore and backslash are escaped. Search fields combine OR. Role defaults
code/name and allows description/application_code; permission defaults
code/description and allows application_code; application allows code/name.
FULL_TEXT/TRIGRAM/combined modes and unsupported fields fail INVALID_ARGUMENT.

`last_id` accepts a versioned typed base64url cursor bound to method and normalized
sort/filter/search. Page, limit and projection may change with the cursor.
Positive numeric IDs are accepted only for ID-only order. Malformed, mistyped,
cross-method or query-mismatched cursors fail INVALID_ARGUMENT. Start a changed
query without a cursor. Without cursor, page>1 uses deterministic offset; with
cursor, page is metadata and no offset is added. Fetching limit+1 determines
has_next. has_prev means a cursor or page>1; terminal next_cursor is empty.
Cursors do not retain a snapshot between requests and do not grant authority.

## Role mutations

| RPC | Current permission | Key | Audit / event |
| --- | --- | --- | --- |
| CreateRole | iam.role.create | required | ROLE_CREATED / iam.role.created.v1 |
| UpdateRole | iam.role.update | optional | ROLE_UPDATED / iam.role.updated.v1 |
| ChangeRoleStatus | iam.role.update | required | ROLE_UPDATED / iam.role.updated.v1 |
| ReplaceRolePermissions | iam.role.permission.update | required | ROLE_PERMISSIONS_CHANGED / iam.role.permissions_changed.v1 |

The single `idempotency-key` metadata value must contain1–200 characters. Scope
is actor + full method + role/{id}, or role-code/{canonical_code} for creation.
Normalized presence, expected version and canonical permission set enter the
fingerprint. Exact retries within24 hours return the original safe response,
including historical nullable metadata/count, after current authority checks.
Conflicting reuse returns ABORTED; expired keys execute again. Replay creates no
additional audit, event, version increment or write. Commit errors are never
retried automatically.

Create accepts an ACTIVE business application, canonical lower-case two-segment
code≤100 characters, trimmed name1–50 and optional description≤100. It creates
an ACTIVE nonsystem role at version1 and allocates numeric role_code from the DB
sequence; gaps after rollback are allowed. Omitted/empty permissions create an
empty set. Codes are deduplicated and must already exist in that application.

Other mutations require role_id1–2147483647 and a present expected_role_version
1–9223372036854775807. Update requires a nonempty unique field mask containing
only name/description, with exactly matching optional-field presence. A selected
empty description clears SQL NULL; an omitted description stays unchanged.
Status requires explicit ACTIVE/DISABLED. Status and replacement require a
trimmed reason3–500. Replacement requires a PermissionSet wrapper: absent is
invalid, present empty clears all mappings. It replaces the complete same-app
registered set; it never creates permission definitions.

System roles, IAM roles and legacy IAM-protected roles reject all mutations,
including no-ops. Disabled business roles/applications remain manageable.
Version checking precedes no-op detection. An effective change increments once
and writes role/mappings, audit, outbox and keyed safe result atomically. A new
no-op writes one NO_OP audit, no event and no version increment, even for an
unkeyed UpdateRole. Failures roll back before a separate safe failure audit;
audit telemetry preserves the original error if the failure audit itself fails.

Create/Update/Replace return the safe role; Create/Replace also return canonical
permission items. Update/Replace include changed. Status returns role_id,
status, version and changed. Responses contain no credentials or user identities.
Role commands do not modify accounts, user versions/auth_version, sessions,
refresh tokens, assignments or application access. Existing JWTs resolve current
ACTIVE role/application/assignment/access contributions on each new snapshot;
revoked assignments stay revoked.

## Registered method inventory and ownership

The production assembly uses one reusable primary transaction/snapshot provider,
clock and event ID generator. TLS, verified exact-method service caller, IAM
namespace and unrestricted current session gates wrap all eight new methods.
Protected reads share120/minute, burst20 with user/session reads. Four role
mutations share30/minute, burst5 with user/session admin mutations. Invalid
input consumes admission. Rejection includes RetryInfo and performs no business
write; quota telemetry is bounded without per-rejection database audits.

| Service | Registered RPCs |
| --- | --- |
| user.v1.UserService | Register, Login, Me, TesRPC, ListUsers, GetUser, CreateUser, UpdateUser, ChangeUserStatus |
| profile.v1.ProfileService | UpdateProfile |
| auth.v1.AuthService | RefreshSession, Logout, ChangePassword |
| session.v1.SessionService | ListMySessions, RevokeMySession, RevokeAllMySessions, ListUserSessions, RevokeUserSession, RevokeAllUserSessions |
| role.v1.RoleService | ListRoles, GetRole, CreateRole, UpdateRole, ChangeRoleStatus, ReplaceRolePermissions |
| permission.v1.PermissionService | ListPermissions |
| application.v1.ApplicationService | ListApplications |

The prior19 methods retain their signatures. Increment B owns these eight
additions. Backend C owns user-role assignment, D owns application access,
and E owns effective-permission RPCs. The current resolver tests do not publish
those future endpoints. REST/frontend remains a separate repository scope.
Actual migration, admin/key/caller/consumer readiness, cutover and rollback remain
mandatory [Phase6 DEP-01](grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment).
No deployment or automatic OpenSpec sync/archive follows backend completion.

Implementation/test traceability and actual gates:
[evidence](../openspec/changes/archive/2026-10-12-auth-role-management/evidence.md),
[acceptance ledger](../openspec/changes/archive/2026-10-12-auth-role-management/design.md#acceptance-ledger),
[Phase2 plan](phase-2-management-plan.md#5-increment-b--role-dan-catalog).

## Retained permission mappings

ReplaceRolePermissions persists a feature-decided sorted diff on the supplied
ORM transaction: removed active pairs receive the command UTC timestamp in
`deleted_at`, retained deleted pairs are restored to NULL, and previously absent
pairs are inserted. Unchanged pairs and old tombstone timestamps are preserved.
The composite pair key covers deleted rows too. An identical active set remains
a no-op; keyed replay performs no mapping write. CreateRole inserts only the
initial nondeleted pairs. No hard DELETE is used.

Role detail, default Member selection, current authorization and effective
permission reads ignore deleted mappings. Role enable does not restore them.
Effective replacement still increments role version once and atomically commits
ROLE_PERMISSIONS_CHANGED, iam.role.permissions_changed.v1 and the safe keyed
result; user/auth/session state stays unchanged. Soft-delete/restore/insert or
later persistence failures roll back together. NO_OP emits no event or version
increment; replay emits no additional audit/event.

Before activation, review the [schema expansion and rollback guide](sql/role-permission-soft-delete/README.md).
Repository calls use the GitHub ORM transaction builders; the legacy pkg DBLogger
is unused. Begin/TryLock/Commit/Rollback remain owned by features.

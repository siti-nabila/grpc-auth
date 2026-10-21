# Application access — increment D

The default server registers 33 unary methods across the existing seven services.
Contract: `api-contracts v0.0.0-20261013075041-21082d7a243d`, commit
`21082d7a243da2b400579db7d1fd40ebdcbeb37f` on `feat/role-management`.
See [publication evidence](../openspec/changes/archive/2026-10-13-auth-application-access/evidence/publication.json).

| Full RPC | Current exact permissions | Shared quota |
| --- | --- | --- |
| `/user.v1.UserService/ListUserApplicationAccess` | `iam.user.read` | read120/min, burst20 |
| `/application.v1.ApplicationService/GrantApplicationAccess` | `iam.user.application.grant` AND `iam.user.role.grant` | mutation30/min, burst5 |
| `/application.v1.ApplicationService/RevokeApplicationAccess` | `iam.user.application.revoke` AND `iam.user.role.revoke` | mutation30/min, burst5 |

TLS, a valid service token, exact method allowlist, IAM namespace and a current
ACTIVE unrestricted user/session are required. Restricted sessions retain only
Me, ChangePassword and Logout. Permission contributions come from the primary
database; neither a previous interceptor check nor a retained result grants
authority. Existing limiter bounds, cleanup and RetryInfo apply unchanged.

## Stored history query

List returns stored ACTIVE and REVOKED access for a nondeleted target in any
account status, including disabled/system applications. It never creates entries
for ungranted applications. Missing/deleted targets return NOT_FOUND; an existing
target without access returns an empty page and its current user version.
Authority, target version, count and page use one primary READ ONLY REPEATABLE
READ snapshot.

| Public scalar | Type / presence | Sort |
| --- | --- | --- |
| application_id | positive INTEGER identity; always projected | ASC/DESC |
| application_code | text | ASC/DESC |
| name | text | ASC/DESC |
| application_status | ACTIVE/DISABLED | ASC/DESC |
| is_system | boolean | ASC/DESC |
| status | access ACTIVE/REVOKED | ASC/DESC |
| granted_by | optional user ID | ASC/DESC, NULLS LAST |
| granted_at | stored timestamp, nonnull DDL; omitted by projection | ASC/DESC |
| revoked_by | optional user ID | ASC/DESC, NULLS LAST |
| revoked_at | optional timestamp | ASC/DESC, NULLS LAST |
| revoke_reason | optional text | ASC/DESC, NULLS LAST |

`PageQuery` defaults page 1/limit 20, max 100. Filters accept normalized application
code and access statuses; statuses are OR, filter categories/search are AND.
Trimmed keyword is at most 200 characters, literal case-insensitive contains or
prefix on application_code/name (both by default). `%`, `_` and backslash remain
literals. Sort accepts multiple public scalars with application_id ASC as the
default/final tie breaker. Hidden sort values survive projection for continuation.
Opaque typed cursors bind method/target/filter/search/sort. Numeric last_id is
allowed only for application-ID sorting. Page>1 without cursor uses offset.
Totals count distinct stored application pairs before cursor; terminal cursor is
empty. Legacy B/C cursor encoding remains compatible.

## Compound commands

Both commands require positive user ID, normalized application code matching
`^[a-z][a-z0-9_]*$` (max50), reason trimmed 3–500, present positive int64-compatible
expected_user_version and exactly one nonblank idempotency-key (max200).
Grant validates 1–20 raw role entries before sorting/deduplicating; every selected
role must exist, be ACTIVE and belong to the same ACTIVE nonsystem application.
Target must be ACTIVE. No access/role write precedes entire-set validation.

Grant creates/reactivates access and selected assignments atomically. Active
provenance stays unchanged. Reactivation preserves pair identity and clears revoke
metadata. Unselected active roles stay active; unselected revoked roles stay revoked.
Revoke accepts nondeleted targets in any account status and disabled applications
or roles. It revokes ACTIVE access and **all** ACTIVE application assignments,
including more than 20. Absent access stays absent; old revoked provenance is not rewritten.
Other applications and credentials are unaffected.

| Result | Version / evidence |
| --- | --- |
| Effective compound change | user version+1 once; one APPLICATION_ACCESS_GRANTED/REVOKED audit, one corresponding pending application event, safe idempotency result |
| Full no-op | expected version still checked; unchanged version, one NO_OP audit, safe result, no event |
| Identical retained replay | original result after current AND authority; no repeated writes/evidence |

Response contains user_id, application_id, optional access_status, actual sorted
changed_role_ids, version and changed. Access-only change permits empty role IDs;
absent-access role cleanup permits absent status. A revoked event in that case
describes denied logical access without fabricating a stored access row. Event
payload has safe application identity/status/actual roles and committed user
version; free-form reason stays in audit. No separate per-role events are emitted.

Scope is actor/full method/user/{id}/application/{numeric_id}/exact key, retained
24h. Fingerprint contains normalized reason, app ID, expected version/presence and
canonical grant set. Preliminary authorized routing and locked rechecks share the
same transaction. Idem→policy→users→roles→applications→session locks use the shared
bounded discovery algorithm; discovered predecessors rollback before restart.
No mutation retries an uncertain commit. Current authority precedes replay; replay
precedes fresh target/version/eligibility. Different scopes are independent.

| Error condition | Contract |
| --- | --- |
| Missing/deleted user | auth NOT_FOUND |
| Unknown application / role | APPLICATION_NOT_FOUND / ROLE_NOT_FOUND |
| Inactive grant target / app / role | GRANT_TARGET_INACTIVE / APPLICATION_INACTIVE / ROLE_DISABLED |
| Wrong application, invalid shape/presence | INVALID_ARGUMENT |
| IAM or is_system application mutation | APPLICATION_ACCESS_SYSTEM_PROTECTED, AASP, FAILED_PRECONDITION, HTTP409 |
| Stale expected user version / changed retained payload / lock contention | user VERSION_CONFLICT / IDEMPOTENCY_CONFLICT / RESOURCE_BUSY, ABORTED |
| Dependency or unknown failure | safe UNAVAILABLE / INTERNAL; causes preserved internally |

The old SYSTEM_APPLICATION_PROTECTED/SAP role error and EN/ID messages are unchanged.
Every required write/evidence/result is atomic; precommit cancellation/panic/failure
rolls back before independent failure audit. Audit failure preserves original error.
auth_version/password/session/refresh behavior, last-admin and refresh committed
denial remain baseline regressions.

## Persistence and verification

Owned fixture access has INTEGER user/application/actor FKs, composite primary key,
ACTIVE/REVOKED check, nonnull granted_at, revocation invariant and
(application_id,status,user_id) index. It follows the agreed
[management DDL](sql/user-management/01_schema_backfill.sql). Archived B metadata
does not include access; these tests do not claim live application-schema parity.
Only dedicated schemas from private env-test.yaml are used; no application migration.

[Acceptance ledger](../openspec/changes/archive/2026-10-13-auth-application-access/evidence/progress.md)
maps AAP-01..12 and publication to source scenarios and actual command evidence.
Backend completion does not include E/effective-read RPCs, REST/UI, Phase4 or
[DEP-01 deployment readiness](grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment).

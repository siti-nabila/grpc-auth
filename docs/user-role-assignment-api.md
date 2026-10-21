# User role assignment API (increment C)

All methods require TLS, a method/namespace-allowed verified x-service-token,
separate Bearer JWT, ACTIVE nonrestricted current user/session and exact permission.
Actor is the verified principal; user_id is the separate target. Current authority
is checked before target disclosure and replay.

| Full method | Exact permission |
| --- | --- |
| /user.v1.UserService/ListUserRoles | iam.user.read |
| /user.v1.UserService/GrantUserRoles | iam.user.role.grant |
| /user.v1.UserService/RevokeUserRoles | iam.user.role.revoke |

## Historical list

Nondeleted targets in any account status can be listed. Default includes ACTIVE
and REVOKED assignments, disabled/system roles and disabled applications. A never
assigned role is absent. Metadata describes history, not effective permission.
Nullable legacy provenance is absent, never synthesized. Actor source IDs do not
expand profiles. user_id/version remain outside projection. Existing collections
on Me/GetUser/ListUsers keep their prior defaults.

PageQuery: page1, limit20 (max100), optional last_id cursor; PageMeta total counts
unique matching assignments before cursor/limit, zero total means zero total_pages.
application_code and assignment statuses combine AND, statuses combine OR.
Unknown application gives an empty matching page; unknown status/field/mode fails.
Search is case-insensitive literal CONTAINS (default) or PREFIX, keyword trimmed
at most200 Unicode characters; fields code/name/application_code, default code/name,
combined OR. Percent and underscore are literal.

All17 scalars below support ASC/DESC multi-sort in request order, NULLS LAST in
both directions. Duplicate/unknown sorts fail; id ASC is appended if absent.
Default sort id ASC. Projection defaults all, always includes id/role_id; hidden
sort values sustain cursors. No SQL expressions are accepted.

| Field | Wire type | Nullable |
| --- | --- | --- |
| id | uint64 | no |
| role_id | uint64 | no |
| code | string | no |
| role_code | int32 | no |
| name | string | no |
| status | string | no |
| role_status | string | no |
| application_id | uint64 | no |
| application_code | string | no |
| application_status | string | no |
| is_system | bool | no |
| application_access_status | string | yes |
| granted_by | uint64 | yes |
| granted_at | google.protobuf.Timestamp | yes |
| revoked_by | uint64 | yes |
| revoked_at | google.protobuf.Timestamp | yes |
| revoke_reason | string | yes |

Opaque versioned cursors bind full method, target user, normalized filters/search/
sort and typed/null keys. Projection, limit and page are not part of the digest.
Cross-user/method/query or malformed cursors fail. Positive numeric last_id is
accepted only for id-only sorting. Without cursor page>1 uses deterministic offset.
No snapshot continuity across separate requests is promised. A request uses one
primary snapshot for current authority, target, total, page and metadata.

## Commands

user_id and role_ids are1..2147483647; raw role_ids length1..20 before sorted
deduplication. reason is trimmed3..500 Unicode characters, valid UTF-8 without NUL.
expected_user_version must be present and1..9223372036854775807. Exactly one
nonblank idempotency-key metadata value, max200 characters, is mandatory.

Grant requires ACTIVE target, ACTIVE registered roles/applications, and ACTIVE
business application access. IAM needs no IAM access row; protected system role
metadata does not prevent an authorized assignment. New pair becomes ACTIVE;
REVOKED pair reuses its ID with fresh grant source and cleared revocation fields;
eligible ACTIVE pair is unchanged. No application access is created.

Revoke permits all nondeleted account states and disabled role/application/access.
All role IDs must be registered. ACTIVE pair becomes REVOKED preserving grant
provenance; REVOKED/registered absent pair is a no-op with no inserted row or
rewritten provenance. Self revoke of iam.platform_admin is forbidden even as a
nominal no-op. The shared last-admin guard with ChangeUserStatus retains at least
one ACTIVE nondeleted user with ACTIVE platform-admin assignment.

New commands compare user version before no-op. Effective batch increments it
once, returns sorted unique actual changed_role_ids and changed=true. No-op keeps
version, returns changed=false and empty changed_role_ids. auth_version, credentials,
sessions, refresh state, app access and unrequested assignments remain unchanged.

Idempotency scope: verified actor + exact full method + user/{id} + exact key.
Fingerprint: canonical role set, trimmed reason, expected version and presence.
Current authority precedes replay; replay precedes current target/version/eligibility.
Same retained payload returns the original result for24h with no new effects;
different payload conflicts; another target/method is independent; expiry is
processed under transaction exclusion as a new request.

Assignment/version/audit/outbox/result commit atomically. Effective batch records
USER_ROLES_GRANTED or USER_ROLES_REVOKED and exactly one pending user event
iam.user.roles_granted.v1 or iam.user.roles_revoked.v1 at committed version with
actual changed IDs. New full no-op records one NO_OP audit/result, no event/version.
Replay records neither. Failure rolls back precommit work; commit uncertainty is
a dependency failure, never fabricated success. Events omit freeform reason.

## Errors and admission

INVALID_ARGUMENT: fields/key/query/presence. NOT_FOUND: nondeleted target absent
(existing auth data-not-found catalog) or role.role_not_found. FAILED_PRECONDITION:
user.grant_target_inactive, role.role_disabled, application.application_inactive,
application.application_access_required, user.last_admin_required.
PERMISSION_DENIED: exact authority or user.self_admin_role_revoke_forbidden.
ABORTED: user.version_conflict, user.idempotency_conflict, user.resource_busy.
UNAVAILABLE: dependency failure; cancellation/deadline retain their mapping.
All catalogs have English/Indonesian messages with safe cause redaction.

List shares protected-read120/min burst20; grant/revoke share admin-mutation30/min
burst5 with existing methods, including applicable invalid inputs. Exhaustion is
structured RATE_LIMITED/RESOURCE_EXHAUSTED with RetryInfo and no business effects.
The additive contract preserves27 existing methods and registers30 total. Only
Login/Refresh issue credentials; Register remains PENDING/token-empty. Deployment
and consumer readiness remain the separate Phase6 DEP-01 gate.

## Storage and transaction ownership

| Stored pair before | Grant (eligible target/policy) | Revoke (registered role) |
| --- | --- | --- |
| absent | insert ACTIVE with current actor/time | no-op, no row inserted |
| ACTIVE | no-op; grant source unchanged | same ID REVOKED; preserve grant source; set revocation actor/time/reason |
| REVOKED | same ID ACTIVE; refresh grant source; clear all revocation fields | no-op; revocation source unchanged |

The feature owns Begin/Commit/Rollback and binds every reader/writer to one primary
handle. Ordered transaction locks cover scoped idempotency, all-revoke last-admin,
global policy, sorted actor/target users, requested and actor roles, applications,
then actor session. Discovery of a missing predecessor rolls back and restarts
with expanded keys, at most three transactions. An unavailable lock is ABORTED;
a lock error is UNAVAILABLE. No automatic retry follows an uncertain commit.
Historical listing uses one read-only REPEATABLE READ primary snapshot.

Schema assertions compare nine user_role columns and eight constraints with the
archived B metadata, including pair uniqueness, foreign keys and nullable grant
source. PostgreSQL renders the status check with per-element casts in the owned
fixture versus one array cast in the archive; the checked ACTIVE/REVOKED set is
equivalent. Other definitions match exactly. Tests verify real invalid status,
revocation, FK and unique writes fail. No application migration was executed.

## Registered inventory and delivery

| Service | Methods |
| --- | --- |
| user.v1.UserService | Register, Login, Me, TesRPC, ListUsers, GetUser, CreateUser, UpdateUser, ChangeUserStatus, ListUserRoles, GrantUserRoles, RevokeUserRoles |
| profile.v1.ProfileService | UpdateProfile |
| auth.v1.AuthService | RefreshSession, Logout, ChangePassword |
| session.v1.SessionService | ListMySessions, RevokeMySession, RevokeAllMySessions, ListUserSessions, RevokeUserSession, RevokeAllUserSessions |
| role.v1.RoleService | ListRoles, GetRole, CreateRole, UpdateRole, ChangeRoleStatus, ReplaceRolePermissions |
| permission.v1.PermissionService | ListPermissions |
| application.v1.ApplicationService | ListApplications |

Thirty full methods are registered through the production foundation. The same
transaction provider, clock and event ID generator are injected once. The27
prior methods and legacy user assignment tags1–5 remain compatible. Contract C
is published at api-contracts v0.0.0-20261012102712-bfb073390ea9; the backend uses
that explicit version with no local replace.

[Implementation evidence](../openspec/changes/archive/2026-10-12-auth-user-role-assignment/evidence.md),
[acceptance ledger](../openspec/changes/archive/2026-10-12-auth-user-role-assignment/design.md#acceptance-ledger),
[Phase2 C](phase-2-management-plan.md#6-increment-c--assignment-role).
C proves current assignment contribution with the original Login JWT. Application
access management D, permission-read/EvaluateAccess E and CLM resource actions
remain separate. The full original AC-09/10 are not globally marked complete.
[Phase6 DEP-01](grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment) remains OPEN.

# Effective permission reads — increment E

Two additive unary UserService methods extend the default server to35 methods
across the same seven services. Contract E is published and pinned as
`v0.0.0-20261014032725-545701c82e22`, commit
`545701c82e2234041df1bf4d2ce4fef7665dd75d`.

| Full method | Target / current authority |
| --- | --- |
| /user.v1.UserService/GetMyEffectivePermissions | Verified self, ACTIVE unrestricted actor/session; no administrative permission |
| /user.v1.UserService/GetUserEffectivePermissions | Explicit user_id, ACTIVE unrestricted actor/session and exact iam.user.read, including self-target; service IAM namespace required |

Every call requires TLS and an exact-method-allowed verified service token.
User identity comes from verified JWT/session, never browser metadata. The existing
EdDSA-only, issuer,15-minute access, absolute7-day session,30-second JWT skew,
ignored audience and restricted Me/ChangePassword/Logout rules remain unchanged.

## Inputs and namespace scope

Self accepts only `application_code`; admin adds `user_id` in1..2147483647.
The application filter trims whitespace; blank means all namespaces allowed for
the verified caller. Nonempty code uses lowercase `^[a-z][a-z0-9_]*$`, length<=50.
An explicit disallowed namespace returns PERMISSION_DENIED before existence lookup.
For an allowed namespace, unknown/DISABLED applications return an empty collection
for an otherwise authorized existing target. Admin authority is checked independently
of the response filter, so filtering to a business app does not remove the actor's
IAM permission check. Self business-only reads do not require IAM namespace authority.

Nil/malformed input returns INVALID_ARGUMENT. An authorized missing/soft-deleted
target returns auth.not_found/NOT_FOUND; a caller lacking iam.user.read receives
PERMISSION_DENIED without target disclosure. PENDING/SUSPENDED/DEACTIVATED targets
remain readable by an authorized active administrator.

## Safe result

Both methods return GetEffectivePermissionsResponse:

| Field | Meaning |
| --- | --- |
| user_id | Verified self or authorized target ID |
| user_status | Current target account status; always accompanies the view |
| version | Current target entity version; informational, not a permission cache key |
| permissions | Complete unique permission codes, lexicographically ascending |
| permissions[].roles | Unique source role_id/string code pairs, numeric role ID ascending |

Only ACTIVE assignments/roles/applications contribute; business permissions also
require ACTIVE application access. Role and permission application/namespace must
agree. IAM needs no application grant. Overlapping roles retain all contributing
sources; revoking one source removes only its contribution. No numeric-admin/name,
JWT-role, cache, wildcard, hierarchy or direct-permission bypass is introduced.

For a nonactive target, the same union describes retained ACTIVE-role configuration,
ignoring only target account status. It excludes REVOKED/DISABLED sources and does
not reconstruct historical mappings. This descriptive result grants no access,
restores no session and cannot replace a fresh protected-action decision, including
when user_status is ACTIVE. A role change can alter the result without changing
the target entity version.

There is no pagination, search, projection, caller sort or silent truncation.
Password/hash/token/refresh/session identifiers are not representable in the result.

## Snapshot, failure and admission

The feature owns one primary RepeatableRead ReadOnly snapshot for actor recheck,
admin permission, target state and permission contributions. Evaluations established
after a committed policy change see the new state. An in-flight consistent snapshot
may finish with its prior state; no cancellation-after-revoke guarantee is promised.

Dependency failures return common.dependency_unavailable/UNAVAILABLE rather than an
empty success or permission denial. Cancellation/deadline retain CANCELED and
DEADLINE_EXCEEDED. Panic returns a safe INTERNAL error. Failed finalization suppresses
the response; every path releases the leased connection. Existing locale/cause/error
redaction and independent failed-attempt audit remain. Audit persistence failure
preserves the original read error and increments bounded telemetry.

Both methods share the existing verified-user protected-read quota120/min burst20,
capacity10,000, idle TTL15m and cleanup1m. Invalid inputs consume admitted quota.
RESOURCE_EXHAUSTED/RATE_LIMITED includes RetryInfo; rejection does no subsequent
business work or per-rejection database audit. Quota never replaces authorization.

Successful reads write no domain state, version/auth_version, session/refresh,
success audit, outbox or idempotency result. No expected version or idempotency key
is required; incidental key metadata does not cache the view. Retrying after response
loss performs a fresh authorized read.

Implementation uses ReadMyEffectivePermissionsSnapshot and
ReadUserEffectivePermissionsSnapshot with explicit provider/now parameters, plus
SessionReadHandler and its existing startup dependencies. No new read service,
constructor, binder, connection, framework or schema is introduced.

Verification and current completion status: [E evidence](../openspec/changes/archive/2026-10-14-auth-effective-permission-reads/evidence/progress.md).
REST/UI integration, EvaluateAccess/ListAuditLogs and actual deployment readiness
Phase6 DEP-01 retain their separate scope and owners.

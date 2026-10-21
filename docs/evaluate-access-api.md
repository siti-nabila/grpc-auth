# EvaluateAccess API

`/authorization.v1.AuthorizationService/EvaluateAccess` evaluates one exact
permission for the user identified by the current access credential. The default
server registers 36 methods across eight services; all prior 35 remain available.
Contract module: `api-contracts v0.0.0-20261018052225-bfc60a40a858`.

## Authentication and input

Every call requires TLS, one verified `x-service-token` allowed for the exact
method, and one `authorization: Bearer <access token>`. Caller policy must allow
the requested application namespace before application/access/permission lookup.
Business-only callers need no IAM namespace or administrative user permission.
User/session must be ACTIVE, bound, unexpired, nondeleted and unrestricted, with
current auth_version. Restricted allowlist remains Me/ChangePassword/Logout.

| Request field | Required validation after trimming |
| --- | --- |
| application_code | Lowercase `^[a-z][a-z0-9_]*$`, length 1–50 |
| permission_code | Exact `^[a-z][a-z0-9_]*([.][a-z][a-z0-9_]*){2,}$`, length 1–150; first segment equals application_code |

Resource paths may have several segments, e.g. `iam.user.status.update`.
Nil/blank/malformed/uppercase/wildcard/overlength/mismatched codes return
INVALID_ARGUMENT; input is not silently lowercased. Empty application never
means all namespaces. Request has no target user/session, role, expected-version
or persistent-idempotency field; metadata user_id cannot redirect evaluation.

## Decisions and errors

After valid caller, input and user/session checks, evaluate in this order:

| Current state | allowed | reason |
| --- | --- | --- |
| Unknown application | false | PERMISSION_MISSING |
| Registered DISABLED application | false | APPLICATION_INACTIVE |
| ACTIVE business application, user access absent/REVOKED | false | APPLICATION_ACCESS_REQUIRED |
| Gates pass, exact effective permission absent/unknown | false | PERMISSION_MISSING |
| Gates pass, exact effective permission present | true | ALLOWED |

Unknown application behavior follows the user's 18 October 2026 decision. IAM skips
only its user application-access grant gate. ACTIVE role/assignment/application,
matching application IDs/namespace and nondeleted role-permission mappings are
required; business access must be ACTIVE. Overlapping sources preserve access
until none remains. JWT role snapshots, numeric admin codes, wildcard, events
and cached projections cannot grant access.

Invalid service/user/session is UNAUTHENTICATED. Forbidden method/namespace or
restricted operation is PERMISSION_DENIED. These are not boolean deny results.
Database/unavailable snapshot/lookup/finalization failures return UNAVAILABLE;
context cancellation/deadline and unexpected panic remain
CANCELED/DEADLINE_EXCEEDED/redacted INTERNAL. Failures suppress the decision.
Shared catalogs/localization/cause and request context are retained.

## Response and consistency

Safe fields: `allowed`, `reason`, verified self `user_id`, canonical session `sid`,
positive `user_version` and `evaluated_at` UTC. Reasons use the generated
ACCESS_DECISION_REASON_ enum prefix: zero UNSPECIFIED is never returned, ALLOWED=1,
PERMISSION_MISSING=2, APPLICATION_INACTIVE=3, APPLICATION_ACCESS_REQUIRED=4.
Sid is an identifier, not a credential. No password/hash/access/refresh token,
profile, full role list or permission union is returned.

Identity/version, application/access and contributions use one primary
RepeatableRead READ ONLY snapshot, including a feature-owned session recheck.
Feature/common owns begin/finalization/cleanup; readers use the same transaction
without lock, lifecycle, pool fallback or concurrent queries. Next snapshots
observe committed changes. In-flight snapshots may finish consistently with
their earlier state; there is no cross-database commit fence or cancellation of
already-authorized downstream work.

User version is an entity version, not a complete policy revision. Role mapping
changes may leave it unchanged. Responses are not cacheable/reusable access
tickets; consumers evaluate near sensitive actions and apply their resource rules.
Retries/response loss/incidental idempotency metadata always perform fresh reads.
An admin's descriptive effective-permission view of an inactive account does not
authorize that account or restore revoked sessions.

## Admission and effects

EvaluateAccess shares the existing verified-user ProtectedRead bucket with old
reads: 120/minute, burst 20, idle TTL 15 minutes, cleanup 1 minute, cap 10,000 entries.
Invalid input counts after verified identity. Rate rejection returns existing
RATE_LIMITED/RESOURCE_EXHAUSTED with bounded RetryInfo and locale, using aggregate
telemetry rather than one database audit per rejection.

Allowed read writes no business state, success audit, outbox or idempotency result.
False decision attempts exactly one independent SECURITY_ATTEMPT_FAILED audit
with verified service actor, RPC target and REQUEST_DENIED after snapshot cleanup.
No raw request, permission code, sid or credential is stored in that attempt.
Error paths retain one existing safe failure attempt. Audit failure/panic preserves
the original denial/error and updates bounded failure observation. No evaluator
path changes entity/auth_version, role/access, session or refresh state.

## Verification and delivery

Default tests under `tests/features/authorization`, `tests/handler`,
`tests/interceptors` and `tests/integration/auth_reads` use controlled SQL,
in-memory TLS and injected time. `TestPostgresAccessEvaluation` is separate,
opt-in and guarded to the dedicated test database with owned fixture schemas.
It covers original-JWT command visibility, soft-delete/restore, subset regrant,
other-app preservation, account/session denial, physical-backend snapshot
barriers, cleanup and audit failure. Evidence and actual required gate status:
[implementation evidence](../openspec/changes/archive/2026-10-18-auth-evaluate-access/evidence/progress.md).

This increment adds no schema migration, private configuration changes or
deployment. Actual consuming-client/TLS/caller/key/schema readiness remains
Phase 6 DEP-01. ListAuditLogs, complete Phase 4 mutation reconciliation and full
Phase 5 hardening remain separate.

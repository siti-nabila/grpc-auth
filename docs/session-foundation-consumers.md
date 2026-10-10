# Session foundation consumers

`PrepareFoundation` composes the real Login producer, PENDING Register, snapshot
Me/ListUsers and locked Profile into the default registration. All six methods
switch together. `NewGRPCServer` requires a complete pipeline, owns the limiter
cleanup worker and cancels/joins it on shutdown or listener failure. The startup
owner closes pools after draining RPCs. No runtime fallback or additional RPC is
introduced. External readiness remains the [deployment gate](foundation-activation-readiness.md).

## Exact method policy

Every method requires verified service authentication over TLS. User credentials
and service credentials remain separate metadata. Policy matches exact full
methods, including the generated method names used by current registration.

| Full method | User/session policy | Consumer behavior |
| --- | --- | --- |
| `/user.v1.UserService/Register` | Exempt user JWT | PENDING account, empty token, initial profile and nonadmin member; verified service audit actor |
| `/user.v1.UserService/Login` | Exempt user JWT | Real Login producer commits session/refresh/audit before credential response |
| `/user.v1.UserService/TesRPC` | Exempt user JWT | Existing welcome response remains available |
| `/user.v1.UserService/Me` | Current ACTIVE session, self owner | Same primary snapshot; restricted user receives minimum identity/profile/status/version/restriction without privilege collections |
| `/user.v1.UserService/ListUsers` | Current ACTIVE, nonrestricted, exact `iam.user.read` | IAM caller namespace and permission checked in the page snapshot, even for an empty page |
| `/profile.v1.ProfileService/UpdateProfile` | Current ACTIVE, nonrestricted, self owner | Recheck after ordered user/session locks; only name/address/phone |

The full restricted allowlist is `/user.v1.UserService/Me`,
`/auth.v1.AuthService/ChangePassword` and `/auth.v1.AuthService/Logout`.
ChangePassword and Logout are future contracts and are not registered here.
RefreshSession and management methods are not implicit exemptions. Role,
permission, application, admin/user management and full refresh/password/logout
lifecycle remain separate follow-ups.

## Read snapshot and compatibility

`SessionReadHandler` obtains immutable verified claims from the principal and
delegates to feature-owned primary **RepeatableRead READ ONLY** transactions.
Each feature rechecks authoritative user/session state within that transaction.
Me then reads safe identity, profile and active legacy numeric role codes/names;
restricted Me does not query privileged collections. ListUsers reads permission
contributions before count and data using the same handle. No interceptor grant
is reused as lasting authorization and forwarded `user_id` is ignored.

Existing list mapping remains `UserListRequestFromProto` and
`ListUsersResponseFromPage`: filters/date bounds, numeric role-code OR semantics,
DISTINCT rows, logical keyword search, auth_id ordering, last_id/NextCursor and
page metadata remain unchanged. Unknown selectable fields remain validation
errors; dependency causes retain safe localized UNAVAILABLE mapping. TesRPC is
unchanged. Legacy Me aggregation now uses sequential reads and has no shared
goroutine error or slices; the prepared consumer provides the actual snapshot
guarantee. See [snapshot design](authorization-snapshot.md).

## Command outcomes

Both consumers own Begin/Commit/Rollback. Every bound repository shares the
request transaction; no repository acquires another pool connection or starts
its own transaction. Mutation isolation remains ReadCommitted.

| Command/outcome | Mutation/version | Audit | Required pending event |
| --- | --- | --- | --- |
| Register | Explicit PENDING, user version/auth_version 1, empty initial profile, ACTIVE member assignment; no session/refresh/access | `USER_REGISTERED`, verified service actor, user actor null | `iam.user.created.v1`, initial version, user actor null |
| Effective UpdateProfile | Only selected self fields; user version +1 once; auth_version/session unchanged | `USER_PROFILE_UPDATED`, current verified user, safe before/after | `iam.user.profile_updated.v1`, same committed user version |
| Identical/empty-mask UpdateProfile | changed=false, no profile/version write | One `NO_OP`, safe after.no_op=true | None |
| Required write or precommit failure | Rollback tentative mutation, no success response | No committed success audit | No committed event |
| Failed auth/command after cleanup | No new success effects | Independent `SECURITY_ATTEMPT_FAILED` with verified service actor only; generic Login denial retains its existing `LOGIN_FAILED` recorder | None |
| Response lost after commit | Committed state may exist; retry is a new command | No persistent general replay | No credential response cache |

Register normalizes/validates email before account/policy locks and duplicate
lookup. It rejects supplied internal owner IDs. Its default member reader checks
numeric code 1, `iam.member`, ACTIVE system role and IAM application, and absence
of role permission grants. Missing/privileged default member is a configuration
dependency failure; the command never selects an admin or promotes the first
user. Register's published token field 1 is always empty; additive registration
ID/status carries the result. Authentication credentials never enter audit/events.

Profile acquires the user lock followed by its session lock, then rechecks
current status, expiry, auth_version, restriction and ownership. A principal
validated before revocation cannot authorize a later locked write. Absent
field_mask replaces all three scalar fields. Present mask updates only named
allowlisted fields; selected address/phone may clear, selected name must have
3–100 characters. Address/phone maxima are 250/20; invalid UTF-8 and forbidden
mask paths are rejected. Empty mask is a valid no-op. Persistence receives only
the fixed profile DTO, never an arbitrary update map or caller-supplied owner.

Events use the existing catalog/envelope, UTC timestamp and pending outbox row.
One bounded `x-request-id` supplies correlation/causation; otherwise event ID is
used. Optional traceparent is syntactically bounded. No broker is needed for
command commit. Commit errors never return success or retry automatically;
transport failure can have an ambiguous commit outcome.

## Independent failure audit

`NewSecurityAttemptInterceptor` is composed inside service authentication and
outside session/rate/consumer handling in the default server. It records only after command
cleanup. `RecordSecurityAttempt` owns a separate transaction. The synchronous
attempt has a two-second context bound independent of request cancellation;
it carries no raw request, rejected user identity, email, credential or error
text. Unknown service callers cannot supply an audit actor. Audit error/panic
preserves the original outcome and invokes the trusted bounded observer.

Login generic credential denial is already independently audited, so it is not
duplicated. Limiter quota/capacity rejections use its aggregate counters and
published RetryInfo, without database attempt writes per attack rejection. The default
composition owns the bounded observer/operational telemetry; there are no
identity-derived labels. See [producer handoff](login-session-producer.md).

No general idempotency is added to Register/Login/Profile. Management idempotency
AC-24..26 remains open until actual management consumers implement authorization
before replay, safe result storage, version/no-op rules and expiry semantics.

## Verification boundary

Root feature/handler/SQL-driver tests exercise mapping, ordered handle use,
failure injection, restricted/stale actor denial, no-op and localized redaction.
Dedicated PostgreSQL tests execute these production repositories/handlers with
real Login-produced credentials, distinct physical backends and channel
barriers. They prove primary snapshot visibility, post-commit privilege changes,
locked revocation recheck, required-write/deferred-commit rollback, cancellation,
panic and pool reuse. Search storage in the owned test fixture is minimal;
this does not certify production search triggers/indexes or deployment readiness.

Fresh commands/source hashes and database backend observations are recorded in
[consumer evidence](../openspec/changes/archive/2026-10-10-auth-session-authorization-foundation/evidence.md#consumer-components-resume--10-oktober-2026).
See [remaining activation gates](foundation-activation-readiness.md).

Complete registered-server verification additionally runs over in-memory TLS with
controlled SQL in the default suite, and the owned PostgreSQL fixture in the
opt-in suite. It covers real Login, normal/restricted consumers, PENDING Register,
profile mutation/no-op, fresh role/session denial, locale/dependency mapping,
legacy/sid-less rejection and RetryInfo. Fresh source evidence is recorded in
[server-verification.json](../openspec/changes/archive/2026-10-10-auth-session-authorization-foundation/server-verification.json).

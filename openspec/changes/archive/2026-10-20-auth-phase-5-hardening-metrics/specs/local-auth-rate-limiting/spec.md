# Spec Delta

## ADDED Requirements

### Requirement: Verified service admission precedes session and failed attempt work

After verified TLS/service credential and full-method allowlist authentication,
all 37 registered unary RPCs SHALL share one process-local token bucket per
verified service ID, refilling at 600 requests/minute with burst 100. This
additional stage SHALL run before session lookup, independent attempt auditing
and business handling; invalid input and malformed user credentials SHALL count
once service identity/method is verified. Logout, all self/admin revoke methods,
Register, Login, RefreshSession and TesRPC SHALL participate. Credential rotation,
user/IP changes or method changes for the same service SHALL NOT create a fresh
bucket. Unverified/method-disallowed callers SHALL NOT allocate state.

Service admission SHALL consume atomically under concurrency but SHALL be a
separate stage from existing identity quotas, without refund on later denial,
validation error, cancellation or downstream quota rejection. Rejected service
admission SHALL return existing RATE_LIMITED/RESOURCE_EXHAUSTED with quota or
capacity reason and positive rounded-up RetryInfo through locale encoding;
it SHALL NOT perform session/DB/audit work or consume downstream quota. State
SHALL share the existing 10,000-entry cap, fifteen-minute idle TTL and one-minute
cleanup, with no eviction of active exhausted keys. This explicit additional
aggregate quota is the user-approved local policy of 20 October 2026.

#### Scenario: One hundred requests exhaust a caller burst
- **WHEN** one method-allowed service concurrently invokes any mix of registered methods at fixed time more than 100 times
- **THEN** at most 100 calls enter Attempt/Session, later calls receive RATE_LIMITED and no downstream read/audit/mutation is executed for the rejected calls

#### Scenario: Controlled refill admits the next request
- **WHEN** a service bucket is exhausted and controlled time advances to one refill token at 100 milliseconds
- **THEN** exactly one new admission is available and an earlier retry remains rejected with positive RetryInfo rounded up to whole seconds

#### Scenario: Method identity and credential changes do not reset quota
- **WHEN** an exhausted service switches from Me to Logout, a self/admin revoke method or TesRPC, changes forwarded IP/user payload, or rotates a still-valid service credential
- **THEN** it shares the same exhausted bucket, preserving rate rejection without new audit or claimed business success

#### Scenario: Unknown or method-disallowed service reaches the boundary
- **WHEN** service authentication fails or its full method is disallowed
- **THEN** the existing authentication/authorization denial occurs before service quota allocation and no audit actor is fabricated

#### Scenario: An admitted malformed session requires an audit
- **WHEN** a service request is admitted but later session authentication denies it
- **THEN** one service token remains consumed and the existing independent safe attempt runs once after cleanup, without sampling or silently dropping required audit

#### Scenario: Capacity prevents a new service bucket
- **WHEN** an authenticated service needs a new key while the shared store contains 10,000 retained entries
- **THEN** it receives capacity RATE_LIMITED with one-minute retry guidance without evicting active keys, entering Session/Attempt or partially consuming downstream quotas

#### Scenario: Downstream quota rejection follows service admission
- **WHEN** service admission succeeds but the Login account or proven refresh-session bucket rejects
- **THEN** service consumption remains, only the rejecting downstream stage adds its rejection observation, and atomic downstream checks do not consume its other bucket

## MODIFIED Requirements

### Requirement: Numeric quota and atomic layered admission

After verified service authentication and the additional aggregate service
admission stage, `/user.v1.UserService/Login` SHALL require both IP quota
30/minute burst 10 and HMAC-normalized-account quota 5/minute burst 3.
`/user.v1.UserService/Register` SHALL require IP quota 5/minute burst 2.
`/user.v1.UserService/Me` and `/user.v1.UserService/ListUsers` SHALL share
protected-read quota keyed by verified user + RPC group, 120/minute burst 20.
All applicable identity-stage checks/consumption SHALL be atomic under
concurrency; rejecting one identity bucket SHALL NOT consume another identity
bucket. The aggregate service stage SHALL be independent and already-consumed
service tokens SHALL NOT be refunded on downstream rejection. Admission SHALL
precede expensive credential work; protected-read quota SHALL NOT replace
authoritative permission checks. Invalid requests SHALL count, without
account-existence disclosure. Future RPC groups SHALL not be falsely advertised
as implemented.

#### Scenario: Login account bucket exhausted
- **WHEN** an account bucket cannot admit Login while its IP bucket still has tokens and service admission has succeeded
- **THEN** Login is rate rejected without consuming the IP bucket or performing password/session business work, and its service-stage token remains consumed

#### Scenario: Burst and refill under concurrency
- **WHEN** concurrent requests consume a burst and controlled time advances
- **THEN** admitted requests never exceed configured available quota, refill follows each applicable numeric rate deterministically, and each stage retains its own atomicity

#### Scenario: Invalid input consumes admission quota
- **WHEN** a verified caller sends malformed or nil Login input
- **THEN** service and applicable IP/account admission are evaluated safely before handler validation, without panic or account lookup

#### Scenario: Protected read remains authorized
- **WHEN** a verified user has service/identity quota but lacks iam.user.read or is restricted and calls ListUsers
- **THEN** the request remains denied by authoritative authorization without directory data

### Requirement: Lifecycle RPCs use the agreed numeric admission groups

`/auth.v1.AuthService/RefreshSession` SHALL use trusted-IP 60/minute burst 10 and
verified-session 10/minute burst 2 quotas with atomic consumption of applicable
identity buckets after the independent service stage. A supplied sid or unknown
token SHALL NOT allocate a claimed session bucket.
`/auth.v1.AuthService/ChangePassword` SHALL use verified actor/group 5/minute
burst 2. `/session.v1.SessionService/ListMySessions` and
`/session.v1.SessionService/ListUserSessions` SHALL use protected-read 120/minute
burst 20, and RevokeUserSession/RevokeAllUserSessions SHALL use admin-mutation
30/minute burst 5 keyed by verified actor/group. Invalid input SHALL count
through applicable trusted admission without account disclosure. Existing
capacity 10,000, idle TTL 15 minutes, cleanup 1 minute, trusted-IP fallback and
bounded RetryInfo/RESOURCE_EXHAUSTED behavior SHALL remain. Logout and self revoke
operations SHALL participate in the explicitly agreed aggregate service quota
600/minute burst 100 but SHALL NOT gain an additional invented per-user quota.

#### Scenario: Refresh has valid identity but session quota is exhausted
- **WHEN** a proven refresh credential maps to an exhausted verified-session bucket while its trusted-IP bucket has quota after service admission
- **THEN** the request is rate rejected without consuming the IP bucket or performing rotation/reuse mutation, and already-consumed service quota is not refunded

#### Scenario: Unproven refresh identity
- **WHEN** a method-allowed service submits a malformed or unknown refresh token with a claimed session ID and has service quota
- **THEN** applicable trusted-IP admission counts the attempt but no claimed session bucket or business mutation is created

#### Scenario: Protected and admin session requests
- **WHEN** a verified actor exhausts the applicable read, admin mutation or password group
- **THEN** the request receives bounded RATE_LIMITED/RESOURCE_EXHAUSTED with RetryInfo and no business effects; available service or identity quota never bypasses current permission or restricted checks

#### Scenario: Logout and self revoke follow the aggregate exception
- **WHEN** a verified service exhausts its aggregate quota and calls `/auth.v1.AuthService/Logout`, `/session.v1.SessionService/RevokeMySession` or `/session.v1.SessionService/RevokeAllMySessions`
- **THEN** the call receives RATE_LIMITED before credential/session/audit work; with aggregate quota available, existing ownership/credential behavior runs without an added per-user numeric bucket

## Traceability

Owner: grpc-auth; implementation status NOT_STARTED. The
[P5 acceptance ledger](../../design.md#acceptance-ledger) maps P5-01/02/03/04/06/07
to M-10/M-11 and existing service/session/security regression gates. This delta
changes admission policy with explicit user approval, without changing RPC
descriptors, downstream quota values or Phase 6 DEP-01 ownership.

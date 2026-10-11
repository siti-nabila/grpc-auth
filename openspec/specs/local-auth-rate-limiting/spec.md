# Local Auth Rate Limiting

## Purpose

Membatasi admission Login, Register dan protected reads dengan quota lokal yang atomik, identitas terpercaya, state terbatas dan retry guidance yang konsisten dengan contract gRPC.

## Requirements

### Requirement: Numeric quota and atomic layered admission

After verified service authentication, `/user.v1.UserService/Login` SHALL require both IP quota 30/minute burst 10 and HMAC-normalized-account quota 5/minute burst 3. `/user.v1.UserService/Register` SHALL require IP quota 5/minute burst 2. `/user.v1.UserService/Me` and `/user.v1.UserService/ListUsers` SHALL share protected-read quota keyed by verified user + RPC group, 120/minute burst 20. All applicable checks/consumption SHALL be atomic under concurrency; rejecting one bucket SHALL NOT consume another bucket. Admission SHALL precede expensive credential work; protected-read quota SHALL NOT replace authoritative permission checks. Invalid requests SHALL count, without account-existence disclosure. Future RPC groups SHALL not be falsely advertised as implemented.

#### Scenario: Login account bucket exhausted
- **WHEN** an account bucket cannot admit Login while its IP bucket still has tokens
- **THEN** Login is rate rejected without consuming the IP bucket or performing password/session business work

#### Scenario: Burst and refill under concurrency
- **WHEN** concurrent requests consume a burst and controlled time advances
- **THEN** admitted requests never exceed configured available quota, and refill follows each applicable numeric rate deterministically

#### Scenario: Invalid input consumes admission quota
- **WHEN** a verified caller sends malformed or nil Login input
- **THEN** applicable IP/account admission is still evaluated safely before handler validation, without panic or account lookup

#### Scenario: Protected read remains authorized
- **WHEN** a verified user has quota but lacks iam.user.read or is restricted and calls ListUsers
- **THEN** the request remains denied by authoritative authorization without directory data

### Requirement: Trusted limiter identity and privacy

IP keys SHALL trust `x-client-ip` only from a verified rest-orc service whose forwarding sanitization is an external readiness prerequisite. Missing, invalid, multiple or untrusted forwarded values SHALL use verified service ID fallback rather than spoofed browser/peer identity. Account keys SHALL use HMAC of trim/lowercase-normalized account with a separately provisioned secret. Unauthenticated callers SHALL NOT allocate limiter state. Metrics/log labels SHALL be bounded group/method/reason, never account/user/IP/token/digest.

#### Scenario: Trusted forwarding
- **WHEN** verified rest-orc supplies one valid sanitized client IP
- **THEN** the IP bucket uses that trusted address while account admission still uses its independent HMAC key

#### Scenario: Spoofed or unavailable forwarding
- **WHEN** another caller sends x-client-ip or trusted forwarding is absent/invalid
- **THEN** admission uses the verified service ID fallback without accepting spoofed identity

#### Scenario: Caller authentication fails
- **WHEN** service authentication rejects a request
- **THEN** no account/IP limiter entry or expensive Login business work is created

### Requirement: Bounded process-local state and cleanup

Limiter state SHALL be process-local, concurrency-safe, capped at 10,000 entries, with idle TTL fifteen minutes and cleanup every one minute. Checks SHALL refresh existing-entry idle timestamps, including rejected checks. At capacity, existing keys SHALL remain usable and new keys SHALL receive a capacity rejection; active exhausted keys SHALL NOT be evicted to reset quota. Multi-bucket allocation SHALL remain atomic. Shutdown SHALL cleanly stop cleanup work. Restart/reset and multi-process quota limitations SHALL be documented.

#### Scenario: Capacity reached
- **WHEN** admission requires a new key while 10,000 nonexpired entries exist
- **THEN** the request is capacity rejected without evicting an active entry or partially allocating/consuming other buckets

#### Scenario: Existing key and idle cleanup
- **WHEN** capacity is full but an existing key is checked, or an entry becomes idle for fifteen minutes and cleanup runs
- **THEN** the existing key follows its current quota and only expired idle state is reclaimed, without resetting active exhausted keys

### Requirement: Structured rate rejection

Rate rejection SHALL return catalog `RATE_LIMITED` with RESOURCE_EXHAUSTED and structured standard RetryInfo through existing locale/error mapping. Guidance SHALL round up the maximum applicable refill delay; capacity rejection SHALL provide bounded cleanup retry guidance. Rejection SHALL perform no business mutation or expose account existence. Audit/log attack volume SHALL remain bounded.

#### Scenario: Layered retry duration
- **WHEN** multiple applicable buckets reject with different refill waits
- **THEN** RetryInfo reports the rounded-up maximum required wait with a bounded rejection reason, and the published client decoder can read it

#### Scenario: Capacity retry guidance
- **WHEN** entry capacity prevents allocation
- **THEN** the caller receives RESOURCE_EXHAUSTED/RATE_LIMITED with capacity reason and cleanup retry guidance rather than an internal error or zero-delay loop

Acceptance: [LSP-06/08/09 ledger](../../changes/archive/2026-10-10-auth-login-session-producer/design.md#acceptance-ledger). Local controlled-clock evidence covers existing groups only; external forwarding readiness and remaining lifecycle/management groups retain their owners.

### Requirement: Lifecycle RPCs use the agreed numeric admission groups

`/auth.v1.AuthService/RefreshSession` SHALL use trusted-IP 60/minute burst 10 and verified-session 10/minute burst 2 quotas with atomic consumption of applicable buckets. A supplied sid or unknown token SHALL NOT allocate a claimed session bucket. `/auth.v1.AuthService/ChangePassword` SHALL use verified actor/group 5/minute burst 2. `/session.v1.SessionService/ListMySessions` and `/session.v1.SessionService/ListUserSessions` SHALL use protected-read 120/minute burst 20, and RevokeUserSession/RevokeAllUserSessions SHALL use admin-mutation 30/minute burst 5 keyed by verified actor/group. Invalid input SHALL count through applicable trusted admission without account disclosure. Existing capacity 10,000, idle TTL 15 minutes, cleanup 1 minute, trusted-IP fallback and bounded RetryInfo/RESOURCE_EXHAUSTED behavior SHALL remain. Logout and self revoke operations SHALL not be assigned an invented numeric quota absent the agreed matrix.

#### Scenario: Refresh has valid identity but session quota is exhausted
- **WHEN** a proven refresh credential maps to an exhausted verified-session bucket while its trusted-IP bucket has quota
- **THEN** the request is rate rejected without consuming the IP bucket or performing rotation/reuse mutation

#### Scenario: Unproven refresh identity
- **WHEN** a method-allowed service submits a malformed or unknown refresh token with a claimed session ID
- **THEN** applicable trusted-IP admission counts the attempt but no claimed session bucket or business mutation is created

#### Scenario: Protected and admin session requests
- **WHEN** a verified actor exhausts the applicable read, admin mutation or password group
- **THEN** the request receives bounded RATE_LIMITED/RESOURCE_EXHAUSTED with RetryInfo and no business effects; available quota never bypasses current permission or restricted checks

Evidence/traceability: [SLC-08/09](../../changes/archive/2026-10-11-auth-session-lifecycle/design.md#acceptance-ledger), M-10/11. Backend subset PASS dan lifecycle message prerequisite PASS; actual evidence/source ada pada ledger/final verification. Actual consumer IP sanitization/RetryInfo handling remains mandatory Phase 6 DEP-01, outside backend completion.

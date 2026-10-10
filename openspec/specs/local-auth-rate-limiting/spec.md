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

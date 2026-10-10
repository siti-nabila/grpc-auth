# Login Session Issuance

## Purpose

Menyediakan Login yang menghasilkan persistent session dan initial refresh credential secara atomik, dengan authoritative eligibility serta response dan failure semantics yang aman.

## Requirements

### Requirement: Authoritative Login eligibility

`/user.v1.UserService/Login` SHALL require verified service authentication, valid existing email/password input, password proof and ACTIVE/nondeleted authoritative account state. It SHALL NOT require an existing user access JWT or accept payload user/role/service identity as authority. Account normalization SHALL preserve existing trim/lowercase behavior. State used for issuance SHALL be rechecked under the shared user lock; changed credentials or eligibility after password proof SHALL prevent issuance. Unknown account, wrong password and nonactive/deleted account SHALL return generic `LOGIN_FAILED`/UNAUTHENTICATED without account-existence detail. Dependency failure SHALL remain UNAVAILABLE, lock contention SHALL remain `RESOURCE_BUSY`/ABORTED, and request cancellation/deadline SHALL retain their transport semantics.

#### Scenario: Eligible existing account
- **WHEN** a verified allowed caller supplies valid credentials for an ACTIVE/nondeleted account and authoritative recheck succeeds
- **THEN** Login proceeds to issue a session for that account without requiring an existing JWT

#### Scenario: Indistinguishable credential denial
- **WHEN** the account is unknown, the password mismatches, or the account is PENDING, suspended, deactivated or deleted
- **THEN** Login returns the same generic catalog denial and no credential, without disclosing which condition occurred

#### Scenario: Credential or status changes during authentication
- **WHEN** password/account state changes before Login acquires the shared account serialization lock
- **THEN** Login rechecks current state, refuses stale password proof and commits no session/refresh issuance

#### Scenario: Dependency and contention are not credential denial
- **WHEN** the database is unavailable, the shared lock is busy, or the request is canceled
- **THEN** Login returns the corresponding dependency, busy or cancellation error without credential and without reporting password mismatch

### Requirement: Atomic initial session and refresh persistence

Successful Login SHALL commit one ACTIVE session, one initial unconsumed/unrevoked refresh token with a new family ID, and one safe `SESSION_CREATED` audit atomically. Session/refresh IDs SHALL be UUID, account IDs SHALL retain INTEGER semantics, and only a digest of an opaque refresh credential with at least 32 random bytes SHALL be persisted. Refresh and session expiry SHALL be the same absolute seven-day boundary from Login. Login SHALL NOT increment user entity/auth versions, create an uncatalogued outbox event, or persist credential replay results. Signing or required persistence failure SHALL prevent returning credentials; transaction cleanup SHALL preserve cancellation/panic/error behavior.

#### Scenario: Committed initial state
- **WHEN** Login succeeds
- **THEN** one session, one refresh record/family and one session-created audit are committed together, refresh consumption/revocation/replacement remain empty, and no raw credential or additional user version/event/result is stored

#### Scenario: Required issuance step fails
- **WHEN** session, refresh, audit or signing fails before commit
- **THEN** issuance is rolled back and no access or refresh credential is returned

#### Scenario: Commit outcome fails or is ambiguous
- **WHEN** commit reports a failure or its outcome cannot be established
- **THEN** Login returns an appropriate safe error without credentials and does not claim that a potentially committed session was certainly rolled back

### Requirement: Published Login response and response-loss semantics

Login SHALL retain `AuthRequest.email/password` fields 1/2 and `UserTokenResponse.token` field 1, and populate published `refresh_token` 4, `session_id` 5, `access_expires_at` 6, `session_expires_at` 7 and `must_change_password` 8 consistently with committed state. Credentials SHALL be returned only after successful commit and SHALL NOT appear in log/audit/event/metrics/idempotency storage. Login SHALL NOT deduplicate successful attempts or replay lost credential responses. Existing six RPC names, TesRPC and list pagination/filter contracts SHALL be preserved. This capability SHALL NOT expose an invented RefreshSession RPC.

#### Scenario: Successful credential response
- **WHEN** issuance commits successfully
- **THEN** the published response contains the session-bearing access token, opaque refresh token, matching session ID and expiries, and the locked account's restricted flag

#### Scenario: Response lost after commit
- **WHEN** the caller loses a committed Login response and submits another valid Login
- **THEN** the new attempt creates a distinct session and credentials, and the earlier response is not cached or replayed

### Requirement: Restricted account producer contract

An ACTIVE account with `must_change_password=true` SHALL be allowed to Login and SHALL receive that restriction in the response. The resulting credentials SHALL preserve foundation enforcement: only minimum Me, ChangePassword and Logout are permitted in the restricted allowlist; ListUsers and RefreshSession SHALL be denied. Missing lifecycle RPCs SHALL remain follow-up work, without claiming that issuance alone completes their acceptance.

#### Scenario: Restricted Login
- **WHEN** an ACTIVE account with valid credentials is marked must-change-password
- **THEN** Login issues a valid session and reports `must_change_password=true`, without granting unrestricted access

#### Scenario: Restricted credential consumed by foundation
- **WHEN** that credential is used against Me or ListUsers through the foundation consumer
- **THEN** Me permits only its minimum restricted response and ListUsers is denied without exposing directory data

### Requirement: Safe independent failed-attempt recording

Credential denial SHALL attempt independent safe failure auditing after any issuance rollback, without reusing an aborted transaction or asserting an unverified user actor. Audit failure SHALL preserve the original denial and emit only bounded redacted failure telemetry. Unknown-account recording SHALL use verified service authority and a safe target. Rate rejections SHALL use bounded aggregate telemetry rather than unbounded per-request database audit. Request context SHALL be preserved; a canceled request SHALL NOT create detached credential work.

#### Scenario: Denial audit succeeds
- **WHEN** credential authentication is denied and independent audit persistence is available
- **THEN** a safe failure record is persisted without password, password hash, raw token, email or forged user identity, and the caller receives the generic denial

#### Scenario: Denial audit fails
- **WHEN** failure recording is unavailable or canceled
- **THEN** the caller still receives the original denial and no credential, and only redacted bounded audit-failure telemetry is produced

Acceptance: [LSP-01/02/04/05/07/08 ledger](../../changes/archive/2026-10-10-auth-login-session-producer/design.md#acceptance-ledger). Real PostgreSQL atomicity is mandatory for component-ready; unrelated lifecycle ACs remain open.

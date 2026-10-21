# Spec Delta

## Purpose

Menyediakan logout dan administrasi sesi yang aman melalui ownership atau permission authoritative, dengan metadata terbatas, pencabutan atomik dan replay management tanpa credential.

## ADDED Requirements

### Requirement: Persisted session generation binds refresh and terminal Logout

New Login sessions SHALL persist the current account auth_version as an immutable positive sessions.auth_version. Refresh and Logout refresh proof SHALL compare this generation with the current account auth_version, including terminal repeat Logout; NULL or mismatched generation SHALL deny authentication. A separate additive migration file SHALL be prepared without execution against the application database. It SHALL backfill only ACTIVE nonrevoked sessions under transactional write exclusion; revoked legacy sessions SHALL remain unknown rather than infer issuance generation from current account, reason, timestamp or audit. Actual deployment migration remains Phase 6 DEP-01.

#### Scenario: Logout precedes global invalidation
- **WHEN** a session is already revoked by Logout and a subsequent revoke-all or password change increments the account auth_version
- **THEN** the old latest revoked refresh is rejected because its persisted session generation differs, even though its terminal session and token rows did not change

#### Scenario: Legacy revoked session has no issuance generation
- **WHEN** a revoked legacy session has NULL generation after the additive migration
- **THEN** its refresh cannot prove terminal Logout and the client must log in again; active known-generation sessions continue according to their bound generation


### Requirement: Logout requires exactly one independently verified credential mode

`/auth.v1.AuthService/Logout` SHALL require a verified TLS service caller and exactly one access JWT or refresh credential mode. Access proof SHALL retain signature/issuer/claims/expiry/ownership/auth_version checks; expired JWT alone is insufficient. Refresh proof SHALL match stored digest, family, owner/auth_version and expiry and SHALL be the latest unconsumed credential. Consumed refresh SHALL be rejected without triggering refresh-reuse mutation from Logout. Clients SHALL prefer a valid access token and use the latest refresh after access expiry; the server SHALL validate the selected mode without requiring an unseen access token. Absent optional refresh SHALL select access metadata mode; present nonempty refresh SHALL require absent authorization metadata; present empty SHALL be invalid. A proven owner SHALL revoke only that session/family; restricted users SHALL be allowed. A repeated logout whose still-valid access proof or latest unconsumed but revoked refresh establishes ownership of an already revoked session SHALL return changed=false without another terminal event; revoked refresh SHALL NOT authorize any other operation. This exception SHALL apply only to Logout, never other protected methods. Missing, ambiguous, unknown or expired proof SHALL be rejected without mutation or new credentials.

#### Scenario: Restricted owner logs out
- **WHEN** a restricted ACTIVE user supplies exactly one valid access or refresh proof for the current session
- **THEN** only that session/family is revoked and changed=true is returned after required persistence commits

#### Scenario: Ambiguous or expired credential
- **WHEN** both modes are supplied, neither is supplied, or an expired access JWT is the only proof
- **THEN** the request is rejected through validation/authentication mapping with no session revoked

#### Scenario: Repeat logout retains sufficient proof
- **WHEN** an otherwise valid unexpired credential still proves ownership of the already revoked target session
- **THEN** Logout returns changed=false without another event, while the same credential remains invalid for Me and other protected operations

#### Scenario: Consumed token is submitted to Logout
- **WHEN** a caller submits an old consumed refresh token after rotation, even if its expiry is still in the future
- **THEN** Logout rejects it without revoking the session/family or issuing credentials; RefreshSession reuse behavior is not invoked

#### Scenario: Optional refresh is explicitly empty
- **WHEN** refresh_token is present but empty, with or without an access header
- **THEN** validation fails rather than silently selecting access mode

### Requirement: Session lists enforce current authority and return only safe metadata

`/session.v1.SessionService/ListMySessions` SHALL use the verified unrestricted user's identity. `/session.v1.SessionService/ListUserSessions` SHALL require current exact iam.session.read permission before target disclosure. Both SHALL read authority and safe session data in one consistent primary snapshot, use published PageQuery/PageMeta with deterministic ID tie-breaker, default page 1/limit 20/max 100 and empty-list metadata. Default sort SHALL be created_at DESC, id DESC; the primary sort SHALL support created_at, last_seen_at or expires_at ASC/DESC with ID in the same direction as tie-breaker and nulls last. Unsupported fields or sort shapes SHALL be rejected. Composite cursor SHALL bind the primary value/null marker and ID to owner and normalized query; malformed/mismatched cursors SHALL be rejected. Both SHALL expose safe session identifiers/status/timestamps/bounded device metadata with correct current-session flag. Passwords, token values, hashes and credential headers MUST NOT appear. Restricted callers, payload owner substitution and numeric-role bypass SHALL be denied; dependency failure SHALL NOT become an empty successful page.

#### Scenario: Self list cannot select another owner
- **WHEN** an unrestricted user lists their sessions or attempts to substitute a payload owner
- **THEN** only sessions owned by the verified principal can be returned

#### Scenario: Administrator reads an empty target page
- **WHEN** a user with current iam.session.read permission requests a valid target page containing no sessions
- **THEN** safe empty-page metadata is returned; a caller without permission is denied even for that empty page

#### Scenario: Concurrent permission change or dependency failure
- **WHEN** permission is revoked before the next list snapshot or the primary read is unavailable
- **THEN** the next evaluation denies access or returns UNAVAILABLE respectively, without mixing policy and response snapshots

#### Scenario: Session timestamps tie or are null
- **WHEN** an authorized caller requests either direction of a supported timestamp sort and pages over equal timestamps or null last_seen_at
- **THEN** ID disambiguates ties, null values are last, and default requests follow created_at DESC, id DESC

#### Scenario: Session cursor belongs to another owner or query
- **WHEN** a caller submits a malformed cursor or one from a different target or sort
- **THEN** validation fails without returning another owner's sessions or silently reusing the cursor

### Requirement: Single-session revocation preserves ownership and terminal event uniqueness

`/session.v1.SessionService/RevokeMySession` SHALL require an unrestricted active verified user and target session ownership. `/session.v1.SessionService/RevokeUserSession` SHALL require current exact iam.session.revoke permission, a positive INTEGER-compatible user_id and a canonical UUID session_id bound to that target. Effective revocation SHALL atomically revoke the session/family, write SESSION_REVOKED and one pending iam.session.revoked.v1 with aggregate_version=2. User version/auth_version and other sessions SHALL remain unchanged. Already revoked targets SHALL produce changed=false without duplicate state events; admin no-op SHALL persist one NO_OP audit and safe result. Session status SHALL never return to ACTIVE.

#### Scenario: Cross-owner self revoke
- **WHEN** a valid user requests RevokeMySession for another user's session
- **THEN** ownership denial leaves every target row, success audit and outbox unchanged

#### Scenario: Admin target does not match its session
- **WHEN** an authorized administrator submits a session_id owned by a different user_id
- **THEN** the command rejects the mismatch without revoking either user's session

#### Scenario: Revoke is repeated or concurrent
- **WHEN** a session becomes revoked and another valid authorized command reaches it
- **THEN** at most one terminal event is committed, later state outcomes are changed=false, and no other session or user counter is changed

### Requirement: Global revocation invalidates user credentials atomically

`/session.v1.SessionService/RevokeAllMySessions` SHALL require an unrestricted active self principal. `/session.v1.SessionService/RevokeAllUserSessions` SHALL require current iam.session.revoke permission plus target user_id and expected_user_version. A new authorized global invalidation SHALL increment user version and auth_version once, revoke all target sessions/families including current where applicable, and atomically persist USER_SESSIONS_REVOKED plus one pending iam.user.sessions_revoked.v1 at the committed user version. It SHALL NOT emit duplicate per-session events or return credentials. Successful self revoke-all SHALL require login again; revoked credentials SHALL not authorize a subsequent self retry. Account status SHALL remain unchanged and new Login is permitted for an eligible user.

#### Scenario: Self revoke-all invalidates every device
- **WHEN** a verified owner submits a new self revoke-all command
- **THEN** all their existing credentials become invalid, counters advance once, one global event commits and the response reports login_required without a new token

#### Scenario: Stale admin target version
- **WHEN** an authorized new admin revoke-all command supplies an outdated expected_user_version
- **THEN** VERSION_CONFLICT/ABORTED is returned without mutation, success audit, event or committed result

#### Scenario: Login races global invalidation
- **WHEN** a new Login or refresh overlaps revoke-all
- **THEN** shared user/session serialization prevents old credentials surviving a later invalidation; a Login after committed revocation uses the new auth_version and a new session

### Requirement: Admin revocation replay requires current authority and safe scoped persistence

RevokeUserSession and RevokeAllUserSessions SHALL require metadata idempotency-key, nonempty and at most 200 characters. Scope SHALL be verified actor + full method + canonical user/{id}/session/{uuid} or user/{id}/sessions + key. Fingerprint SHALL include normalized target, payload presence and expected version when applicable. Safe result SHALL persist 24 hours atomically with mutation/audit/outbox and SHALL replay only after current caller/session/permission recheck, before target expected-version comparison. Identical replay SHALL add no mutation/audit/event/version; same scope/key with different payload SHALL return IDEMPOTENCY_CONFLICT/ABORTED. Different targets SHALL be distinct scopes. No password, token, authentication hash or credential replay SHALL be stored.

#### Scenario: Admin response is lost after commit
- **WHEN** the same valid actor repeats an identical scoped key within retention after losing the response
- **THEN** the saved safe result is returned without additional invalidation, audit or event even if the original expected target version is now old

#### Scenario: Replay actor loses permission
- **WHEN** a saved result exists but the actor's current session or iam.session.revoke permission is invalid
- **THEN** replay is denied without disclosing the stored result

#### Scenario: Conflicting request or different target
- **WHEN** the same scope/key has different fingerprint, or the same key is used for a different target
- **THEN** the first case conflicts without effects and the second uses its independent target scope

### Requirement: Lifecycle mutation outcomes retain atomic persistence and failure mapping

Every lifecycle mutation SHALL recheck current authority after shared ordered locks on the same primary transaction used for its writes. Busy SHALL return RESOURCE_BUSY/ABORTED without success effects; dependency failures SHALL return UNAVAILABLE, with request cancellation/deadline preserved. Every error before commit SHALL roll back the entire started business transaction, including business guard, read, conditional affected-row, write, cancellation or panic failures, not only required mutation/audit/outbox/idempotency write failures; commit uncertainty SHALL NOT become claimed success or certain rollback. After rollback/cleanup, an independent transaction SHALL record a safe failure reason using verified actor/service and bounded catalog/category data. Its bounded independent context SHALL permit recording after request cancellation without discarding correlation. Audit failure SHALL preserve the original error and emit bounded telemetry, without claiming persistence when the audit store is unavailable. Panic SHALL retain the existing recovery/error mapping. Proven refresh reuse SHALL instead commit required revocation/audit/outbox before denial and SHALL NOT roll back merely because an authentication error is returned. Credentials SHALL never enter logs, audit, outbox, noncredential responses or metric labels; only successful Login/Refresh credential responses SHALL contain credential values. Effective state events SHALL remain pending without RabbitMQ.

#### Scenario: Actor changes before mutation lock
- **WHEN** actor session/status/permission changes after initial authentication but before locked recheck
- **THEN** stale authority does not permit a target mutation or replay

#### Scenario: Required write fails or request is canceled
- **WHEN** a required write, transaction step or request context fails during revoke/logout
- **THEN** no successful response or partial known commit is claimed and cleanup releases transaction resources

#### Scenario: Business guard fails after a partial write
- **WHEN** a lifecycle transaction has written state and a later business guard, read or conditional update returns an error
- **THEN** every business write/audit/outbox/result is rolled back and a separate failure audit can record the safe reason after cleanup

#### Scenario: Independent failure audit also fails
- **WHEN** business rollback has completed but the separate audit transaction fails or times out
- **THEN** the original business error remains the RPC outcome and bounded telemetry records the audit failure without claiming audit persistence

Evidence/traceability: [SLC-04/06/07/08/09](../../design.md#acceptance-ledger), AC-16/18/24/25/26/28/31/34 and M-04..08/11. Backend subset PASS dan contract prerequisite PASS; actual evidence/source ada pada ledger/final verification. Register PENDING/token-empty, six existing RPCs, TesRPC and ListUsers default ID/numeric cursor compatibility remain; approved dynamic sorting/filter/count extension follows user-list-sorting; consuming-client compatibility stays mandatory Phase 6 DEP-01. Role/permission/app changes continue to preserve sessions/auth_version; suspend/deactivate global invalidation belongs to the user-status increment.

# Spec Delta

## Purpose

Memperbarui credential sesi melalui rotasi refresh single-use yang atomik, mendeteksi reuse tanpa membuka bypass, dan mempertahankan batas absolute session sejak Login.

## ADDED Requirements

### Requirement: Refresh proves the current session from an opaque credential

`/auth.v1.AuthService/RefreshSession` SHALL require TLS, a verified method-allowed service caller and one nonempty refresh_token, without requiring a valid access JWT. Identity SHALL come from matching the credential digest to persistent token/family/session/user state, never a payload user_id or sid. ACTIVE nondeleted user, ACTIVE unexpired session and unrevoked unexpired refresh state SHALL be rechecked under ordered locks. Unknown or invalid credential SHALL return generic UNAUTHENTICATED; a valid restricted user SHALL receive PERMISSION_DENIED without rotation. Dependency failure SHALL remain UNAVAILABLE and cancellation/deadline SHALL retain their mapping.

#### Scenario: Access expired but refresh remains valid
- **WHEN** an allowed service presents a valid unconsumed refresh token for an ACTIVE unrestricted account and unexpired session with an expired access JWT
- **THEN** refresh can proceed without using the access JWT as identity proof

#### Scenario: Unknown token or forged session identity
- **WHEN** a token digest is unknown or a payload claims another user or session
- **THEN** the server returns generic authentication denial and no target session is revoked

#### Scenario: Restricted or inactive account
- **WHEN** a proven credential belongs to a restricted ACTIVE user, or to an inactive/deleted user or expired/revoked session
- **THEN** restricted ACTIVE state returns PERMISSION_DENIED and invalid state returns UNAUTHENTICATED, with no rotation or credential response

### Requirement: Rotation commits consumption successor and audit before credential response

RefreshSession SHALL atomically consume the proven token, create one successor in the same family/session, set consumed_at/replaced_by and persist SESSION_REFRESH_ROTATED audit. Only a digest of an opaque successor with at least 32 random bytes SHALL be stored. Success SHALL return EdDSA access and fresh refresh credentials only after commit, with unchanged session ID and absolute session expiry. Access lifetime SHALL be at most 15 minutes bounded by session expiry; session/refresh SHALL remain within seven days from Login, with no extension from JWT 30-second skew. Exact ApplicationName issuer and ignored audience SHALL be preserved. Rotation SHALL NOT change user version/auth_version, create an uncatalogued rotation event or store credential replay results.

#### Scenario: Successful rotation near session expiry
- **WHEN** an unconsumed token is rotated shortly before the absolute session boundary
- **THEN** exactly one successor and rotation audit commit, access expiry is capped by the original session boundary, and no user counter or outbox event changes

#### Scenario: Signing or persistence fails
- **WHEN** successor generation/signing, consume/update, insert, required audit or commit fails
- **THEN** no credential response is returned, known precommit writes roll back, and an unknown commit outcome is not reported as definitely rolled back

#### Scenario: Response is lost after commit
- **WHEN** rotation commits but its credential response is lost or commit outcome cannot be established
- **THEN** the client must log in again; the server does not restore the consumed token, cache credentials, add grace or automatically retry rotation

### Requirement: Proven consumed-token reuse commits terminal revocation before denial

A proven consumed refresh token SHALL trigger terminal revocation of its session and entire family, REFRESH_REUSE_DETECTED audit and SESSION_REVOKED audit plus pending iam.session.revoked.v1 when revocation is effective. Security state and required evidence SHALL commit before UNAUTHENTICATED/REFRESH_TOKEN_REUSED is returned. Terminal session aggregate_version SHALL be 2 once; other sessions and user counters SHALL remain unchanged. Repeated detection SHALL NOT create duplicate terminal state events. Failure of a required security write SHALL return the dependency failure without claiming successful revocation.

#### Scenario: Consumed token is reused after rotation
- **WHEN** rotation has committed and the old token is subsequently proven consumed
- **THEN** the successor family and session become unusable, required audits and one terminal event commit before authentication denial, and another session remains active

#### Scenario: Revocation persistence fails
- **WHEN** required revoke/audit/outbox persistence fails during proven reuse handling
- **THEN** the security transaction rolls back and returns dependency failure without claiming revocation committed or returning credentials

#### Scenario: Concurrent refresh overlap
- **WHEN** two requests overlap on the same valid token
- **THEN** lock contention returns ABORTED/RESOURCE_BUSY without declaring reuse, or a postcommit contender observes consumption and commits reuse revocation; the server does not promise both callers retain an active session

Evidence/traceability: [SLC-02/03/08/09](../../design.md#acceptance-ledger), AC-17 and M-03/04/07/11. Backend status NOT_STARTED; published contract prerequisite BLOCKED. Actual client single-flight and deployment readiness remain [Phase 6 DEP-01](../../../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment).

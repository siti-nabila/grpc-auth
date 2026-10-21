# Session verifier and producer handoff

The default server uses EdDSA access verification and persistent primary session
state with the real Login producer and all six foundation consumers. Source wiring
is complete; consuming-client and deployment readiness remain mandatory before
release. See [deployment gate](foundation-activation-readiness.md). There is no
legacy token fallback or refresh rotation endpoint in this increment.

## Credential verification

Construct `pkg/jwt.NewAccessVerifier` once from the exact configured
ApplicationName issuer, Ed25519 public key and clock. The verifier owns a copy
of the public key. Only EdDSA is accepted. Required claims are canonical positive
decimal sub fitting PostgreSQL INTEGER, UUID sid/jti, exact iss, iat, exp and
positive integer auth_version. Optional legacy user_id must agree with sub.
Audience is intentionally ignored, including absence or a different audience.
Invalid signature/claims use generic auth.invalid_session / UNAUTHENTICATED.

Access lifetime is positive and at most 15 minutes. JWT time validation permits
30 seconds skew. No credential, role snapshot or permission grant is exposed by
VerifiedClaims, only immutable verified identity/version/timestamps. This
verifier does not create sessions or authorize a user by itself.

## Primary state and transaction boundary

`reader.NewSessionStateReader` accepts an injected primary query adapter or
feature-owned transaction. `GetSessionState(requestCtx, sid)` explicitly forwards
the request context and reads auth/session state in one join, without password,
token or signing-key columns. Missing handles fail closed; no pool fallback,
transaction lifecycle or lock acquisition exists inside the repository.

`authfeature.AuthenticateSession` requires a verified service caller, retrieves
primary state, and creates a Principal only if user/session are ACTIVE, user is
not deleted, sid/owner/sub agree, auth_version matches and entity version is
positive. Revoked/missing/inactive/expired state is denied. Repository failure
becomes common.dependency_unavailable / UNAVAILABLE with original cause retained;
cancellation/deadline retain their identity. No cache or synthetic state grants
access when primary data is unavailable.

Session expiry is strictly absolute: now must precede expires_at, lifetime is
at most seven days from creation, and token exp cannot exceed session expiry.
JWT skew never extends this database deadline. Reactivating a user does not
reactivate a revoked session. Domain Session maps existing UUID/user/status,
timestamps, revocation and bounded device metadata; it has no version column.
Auth entity version and auth_version are distinct.

Principal stores verified user/sid/jti/auth_version, authoritative restricted
state, user entity version and evaluation time. It grants no permanent
permissions. `PrincipalForState` must run again on the same snapshot as sensitive
data, or after mutation locks using the transaction's current state. An earlier
interceptor lookup alone does not authorize a later ReadCommitted data query.

The restricted exact allowlist is Me, ChangePassword and Logout. Me must emit
minimum identity/profile without role/application/permission fields. UpdateProfile,
ListUsers and RefreshSession are denied while restricted. The registered Me consumer implements minimum-response filtering. Unavailable
lifecycle methods are not registered.

## Real Login increment

The producer must use one feature-owned transaction and request context for
ACTIVE credential recheck, `writer.NewSessionWriter(tx).Create`, its bound
refresh-token writer, and `writer.NewAuditWriter(tx)` SESSION_CREATED audit.
Session UUID and refresh family/token UUIDs come from its cryptographic source;
refresh credential is stored as a hash only. SessionWriter validates an initial
ACTIVE session, absolute expiry and bounded metadata; it does not begin, commit,
rollback or lock. The producer must supply the same handle to all writers and
commit before emitting access/refresh/session credentials.

Session creation leaves user version/auth_version unchanged and creates no
session-created outbox event outside the agreed catalog. Password/account failure
uses generic LOGIN_FAILED, with independent safe failed-attempt audit policy.
Rollback on required write/commit failure prevents success response. Do not
persist credentials for response replay; response loss requires login again.
Producer-owned refresh writer/hash/issuer/limiter implementation and real Login
integration evidence remain mandatory prerequisites, not fixture proof here.

Tests: `rtk go test ./tests/pkg/jwt/... ./tests/features/auth_feature/...
./tests/repositories/transaction/...`. They use fixed keys, controlled clocks,
injected state/SQL fakes and request-context assertions, without a real database.
Opt-in PostgreSQL tests also exercise the complete registered production server;
these results do not certify external deployment readiness.

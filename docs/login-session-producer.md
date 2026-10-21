# Login session producer

## Configuration and activation

`env.yaml` remains the configuration source. `AccessIssuer.PrivateKeyFile` is one PKCS#8 Ed25519 private PEM, `PublicKeyFile` the matching PKIX public PEM, `KeyVersion` an explicit operational identifier. Issuer is exactly `ApplicationName`. `AuthLimiter.AccountHMACSecret` is separately provisioned, without defaults or reuse of JWT/service credentials. See [example](../env.example.yaml).

`configs.ReadConfig` parses these typed sections. `LoadLoginSecurity` validates required fields, reads the pair once, verifies match/signature and constructs immutable issuer/verifier before runtime resources. It never logs parser errors or file contents. The default startup calls `LoadLoginSecurity` before opening logger/pools/listener and composes the producer through `PrepareFoundation`; missing producer configuration fails startup. Provision actual keys outside Git; evidence contains only versions/status, never secret material. Rotation and client login ulang are coordinated with consumers; key version does not introduce a keyring or dual algorithms.

## Credential encoding and lifetime

Initial entropy generates independent UUIDv4 session, refresh-record, family and access jti identifiers. Refresh secret is 32 random bytes encoded as unpadded base64url. Persist only SHA-256 of the exact encoded refresh token as 32-byte BYTEA. Future strict refresh parser must digest that same canonical representation. No raw credentials in audit, event, logs, metric labels or idempotency storage.

Producer uses one UTC-second issuance timestamp. Session/initial refresh expire exactly seven days later. Access expires at the earlier of issuance +15 minutes or session expiry; required claims are canonical INTEGER `sub`, UUID `sid/jti`, exact `iss`, `iat/exp`, positive authoritative `auth_version`. Audience remains ignored; verifier skew 30 seconds never extends absolute database session expiry. Signing uses only EdDSA/Ed25519.

`GenerateInitialCredentials` takes injected entropy; `AccessIssuer.Issue` receives authoritative state/timestamps. Unit tests use deterministic entropy/key material and foundation verifier, not deployment secrets. Initial refresh issuance does not provide RefreshSession rotation/reuse, Logout or ChangePassword. Whole-server readiness remains owned by [foundation](../openspec/changes/archive/2026-10-10-auth-session-authorization-foundation/tasks.md).

## Persistence prerequisites

`LoginAccountReader` reads explicit credential/status/version/deleted fields from a supplied primary adapter for discovery or a supplied transaction for authoritative recheck. Both methods forward request context; neither creates a pool or transaction. Password data never enters public response/audit models.

`InitialRefreshWriter` requires the feature-owned transaction and validates UUID IDs, exactly 32 digest bytes, initial NULL consumed/revoked/replaced fields and expiry. `refresh_tokens` must have session/family UUID fields, token_hash BYTEA unique, session FK and self replacement FK/uniqueness; expiry must follow issuance. `sessions` has no version column and no family table is introduced. Session/refresh/audit writes share the same transaction. Default SQL-fake tests verify query arguments/handle/error/NULL state; dedicated test DB verifies actual constraints/atomicity. Historical schema inspection is not that evidence. A missing/mismatched schema is reported as a gap, without automatic migration, seed or development DB writes.

## Login command

`NewSessionLoginService` receives explicit candidate/tx-bound readers, transaction provider, repository binder, Argon2 verifier, issuer, entropy/clock and failure recorder/observer. It accepts only verified service context and normalized valid existing email/password. Unknown/mismatch/deleted/non-ACTIVE return generic `LOGIN_FAILED`/UNAUTHENTICATED; dependencies remain UNAVAILABLE, contention RESOURCE_BUSY/ABORTED, context cancellation/deadline retained. Legacy catalog mappings are unchanged.

Password proof precedes transaction. Feature acquires canonical user then new session lock, reads credentials/status/versions/restriction from that transaction and denies if state changed. Session, initial refresh and `SESSION_CREATED` audit are written on that handle; signing precedes commit, credential response follows successful commit. No pool fallback/network/concurrent transaction writes. Cleanup runs on error, cancellation and panic; ambiguous commit never returns credentials or claims certain rollback.

The producer validates the existing Argon2id v19 encoding and fixed parameters/salt/digest lengths before verifying. A corrupt stored hash is a dependency failure, including an empty digest; it can never authenticate by comparing two empty digests. The legacy verifier and registered handler are unchanged.

| Outcome | Persisted effects |
| --- | --- |
| Success | One ACTIVE session + initial refresh/family + SESSION_CREATED audit atomically |
| Precommit write/sign failure | Rollback issuance, no credentials |
| Credential/stale-proof denial | Independent LOGIN_FAILED audit after rollback; verified service actor, safe RPC target; original denial retained if audit fails |
| Busy/dependency/cancellation/panic | Safe corresponding error, no credential; cleanup as applicable |
| Repeat or response lost after commit | A subsequent Login creates a distinct session; previous committed session remains until expiry/revoke |

Login does not modify user entity/auth versions, create an uncatalogued outbox event or cache credential/idempotency replay. Failure audit takes request context, uses no account email/password/hash/raw token and does not create detached work when canceled; persistence failure invokes only a bounded local observer. ACTIVE must-change-password users can Login, with restriction signaled in response and enforced by foundation consumers. Full refresh rotation/reuse commit-on-denial, Logout and ChangePassword remain separate increments.

## Local limiter

| Group | Key | Refill / burst |
| --- | --- | --- |
| Login | Trusted IP or verified service fallback | 30/minute / 10 |
| Login | HMAC(trim/lowercase account) | 5/minute / 3 |
| Register | Trusted IP or verified service fallback | 5/minute / 2 |
| Me + ListUsers | Verified user + shared protected-read group | 120/minute / 20 |

Admission checks/consumes applicable buckets atomically under one store lock, including invalid/nil input before handler validation. Integer credits preserve exact refill boundaries. A rejected layer consumes no other bucket; rejected checks update existing idle timestamps. Max 10,000 entries, idle TTL 15 minutes, cleanup 1 minute. At capacity existing keys remain usable; new keys reject without partial allocation or evicting active exhausted quota. Store counters expose bounded quota/capacity totals, never identity labels. `RunCleanup` takes lifecycle context/ticks and stops on cancellation or closed ticks; host owns ticker shutdown and worker join.

Only verified rest-orc may supply one valid `x-client-ip`; missing/malformed/multiple/untrusted values use verified service ID fallback, with separate identity prefixes. gRPC peer IP is not browser identity. Real rest-orc forwarding sanitization remains EXT-02 readiness evidence. Account HMAC secret is copied on construction and never logged. RATE_LIMITED/RESOURCE_EXHAUSTED retains published catalog key/locale and standard RetryInfo, rounded up from maximum bucket wait; capacity guidance is one cleanup interval. The outer error encoder preserves these details. Attack rejection uses bounded aggregate telemetry, not per-request DB audit. Restart resets quota; cross-process/distributed limits are POST_MVP. Future refresh/admin/password groups retain MVP quotas in their own increments.

## Composition handoff

Load `LoginSecurity` before opening resources. `PrepareLoginProducer` receives startup context, validated security, one reusable DB pool/dialect/ORM config, clock and entropy; it builds the production Argon2 verifier, candidate/bound adapters, atomic Login and independent audit, response adapter, store and interceptors. All request reads override adapter context with the original request context. Credential reads and transactions disable SQL debug logging. Preparation opens no pool/listener/worker and does not take ownership of the supplied pool.

The default complete server composes Language → Error → Service → independent attempt audit → Session → Rate → authoritative handlers. Session exempts only Register/Login/TesRPC; Me/ListUsers/UpdateProfile require verified access/session, exact restricted allowlist, and consumer snapshot/mutation recheck. Rate then selects only the groups above. Register PENDING/token-empty and Profile commands remain foundation; TesRPC and list filter/cursor contracts remain. `SessionLoginHandler` is intentionally a component, not an independently registrable UserService. The producer stays a component of the complete foundation registration; no missing RPC is registered.

Foundation owns final handler delegation/activation, the one-minute cleanup ticker/cancellation/join lifecycle, actual key/client/IP/database readiness and login-ulang/rollback runbook. Handoff references [producer tasks/evidence](../openspec/changes/archive/2026-10-10-auth-login-session-producer/evidence.md). RefreshSession is not available merely because initial refresh credentials are issued.

`rest-orc`, frontend and deployment name separate integration owners. Frontend compatibility refers to the consuming application in its own repository; it does not require a frontend folder in `rest-orc`. This producer change implements only `grpc-auth`. External readiness is checked when those consumers are ready for coordinated activation.

| Foundation task | Concrete handoff | Remaining acceptance |
| --- | --- | --- |
| 7.1 | `NewSessionLoginService`, `BindLoginRepositories`, `SessionLoginHandler`, `AccessIssuer`, `PrepareLoginProducer`; production assembly integration in root `tests/integration/auth_reads` | Producer PostgreSQL atomicity/concurrency evidence delivered; foundation consumer wiring/acceptance remains |
| 7.2 | `ratelimit.Store`, `NewAuthRateInterceptor`, structured published RetryInfo; controlled clock/concurrent/race tests | Actual rest-orc sanitization, client decoder and caller readiness evidence |
| 7.3 | `LoadLoginSecurity`, example config, exact command/TTL/method matrix above | Actual provisioned keys, CA/hostname, callers, schema/ACTIVE platform admin and frontend token-empty/login-ulang compatibility |
| 7.4 | `PrepareLoginProducer` exposes handler, session/rate interceptors, cleanup store and aggregate audit-failure counter | One coordinated switch of all six methods, Register/Profile mutations, lifecycle ownership and full integration gate |

Before cutover, owners must record successful dedicated PostgreSQL tests, finish every mandatory foundation consumer, provision the matching issuer pair and separate HMAC/service secrets, and verify TLS/caller/client/IP compatibility. Plan a login-ulang window: existing HS256 or sid-less credentials will fail the new guard, and initial refresh issuance alone provides no refresh endpoint. Notify clients through the separately authorized release process and verify they retain session/expiry/restricted fields and handle retry guidance.

Rollback is coordinated across producer and all guarded consumers. Retain the previously approved deployment/config securely, choose the full compatible version set before release, and confirm session invalidation/login-ulang policy with the owner. A failed rollout must stop the switch and restore that version set as one operation; never enable a runtime JWT bypass or temporarily accept both algorithms. No deployment, session cleanup or database mutation is authorized by this document. All external activation evidence remains BLOCKED until its owners supply actual results.

The user-approved gate separation on 2026-10-10 completes backend tasks 7.1–7.4
using `PrepareFoundation` and registered-RPC integration. The table above records
the original producer handoff. External sanitization, consuming-client handling,
actual deployment rotation/readiness and coordinated release remain mandatory
[Phase 6 deployment gate DEP-01](grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment); no deployment occurs.

# Foundation source and deployment readiness

The user approved separating backend source completion from deployment on
2026-10-10. The default grpc-auth server now composes all six existing RPCs with
real Login, EdDSA-only session verification and authoritative consumers. External
readiness remains **BLOCKED** at mandatory Phase 6 DEP-01. This does not close Phase 1
or Phase 3 overall, add management/lifecycle RPCs or authorize deployment.

| Gate | Backend evidence | Deployment requirement |
| --- | --- | --- |
| EXT-01 / SAF-07 | Published api-contracts v0.0.0-20261010071509-42a9e6fa23ad; workspace and standalone checks | Preserve tested version set |
| 7.1 / SAF-08 | Real Login session/refresh/SESSION_CREATED transaction and EdDSA issuer | Compatible client storage and login ulang |
| 7.2 | Controlled-clock quotas/cap/idle cleanup, atomic buckets, fallback, RetryInfo and race checks; actual local caller proof | Trusted proxy/IP sanitization and client RetryInfo handling |
| 7.3 | Actual local key sign/verify/HMAC, CA/hostname/TLS pair, caller allowlists/expiry; rotation boundary tests; read-only schema/admin inspection | Reconfirm actual deployed keys/rotation/caller/schema/minimal ACTIVE admin |
| 7.4 / SAF-10 | Complete registered production pipeline over in-memory TLS, controlled SQL and dedicated PostgreSQL; startup failure and cleanup worker join | Coordinate the release with compatible clients |
| Phase 6 DEP-01 / SAF-09 | Backend tests cannot satisfy this gate | Actual EXT-02 consuming-client and deployment evidence; release/rollback approval |

Fresh source/test commands and identity are recorded in
[server-verification.json](../openspec/changes/archive/2026-10-10-auth-session-authorization-foundation/server-verification.json).
Earlier component results in consumer-verification.json remain historical.

## Startup and lifecycle

Startup validates the Ed25519 issuer pair, separate HMAC secret, TLS certificate
pair and caller policy before logger/pools/listener. `Database.User` must use
PostgreSQL. Pool creation failures return through startup cleanup. Foundation
queries disable SQL debugging, share the primary pool and preserve request
context. The fixed interceptor chain is Language → Error → Service → independent
attempt Audit → Session → Rate → authoritative handlers.

Register/Login/TesRPC are exempt only from the user JWT; every RPC requires
service authentication over TLS. Login emits credentials only after its atomic
commit. Register returns PENDING metadata and an empty token with no credentials.
Me/ListUsers recheck inside primary read-only RepeatableRead snapshots. Profile
rechecks after ordered user/session locks. There is no HS256/sid-less fallback,
partial registration, bypass flag or synthetic session.

The server owns the one-minute limiter cleanup ticker/worker. Graceful shutdown
drains RPCs, cancels and joins the worker, and closes outbound clients. Listener
failure performs the same worker/client cleanup. The startup owner then closes
pools in reverse order. Constructor failure starts no worker. Repeated stops are
safe. Initial refresh issuance does not create a RefreshSession endpoint.

## Actual local evidence

[activation-readiness.json](../openspec/changes/archive/2026-10-10-auth-session-authorization-foundation/activation-readiness.json)
records local sign/verify/HMAC, TLS and caller configuration checks without
credential contents or private-value hashes. Local setup preserves the existing
self-signed CA and uses a separate server certificate with localhost/127.0.0.1/::1
SANs. The server certificate and opaque caller credential expire on 2027-01-08
UTC. Private keys/config/backups and caller token are excluded from Git.

[Local caller verification](../openspec/changes/archive/2026-10-10-auth-session-authorization-foundation/grpc-auth-caller-verification.json)
uses actual local TLS material and the production verifier. All six methods are
allowed; missing/duplicate/unknown credentials, non-TLS peers, unlisted
method/namespace, cancellation and exact expiry are denied. The local credential
has no Previous entry; actual deployment rotation remains unverified.

[Read-only schema/admin preflight](../openspec/changes/archive/2026-10-10-auth-session-authorization-foundation/grpc-auth-readiness-preflight.json)
used RepeatableRead READ ONLY and rollback: one ACTIVE canonical platform admin,
one unrestricted admin with iam.user.read, one safe default member, historical
columns unchanged, search table/pg_trgm and two search triggers present, zero
profiles missing a search row. Metadata inspection does not certify production
search-trigger behavior or deployment state. Dedicated env-test.yaml tests use
only owned fixture schemas in grpc_auth_test; no baseline database writes,
migration, seed or Docker occurs.

## Phase 6 deployment checklist and rollback

User moved deployment readiness to the final phase on 2026-10-10. Its authoritative
unchecked task is [Phase 6 DEP-01](grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment).
Foundation has **36/36 backend tasks complete**; the former task 9.1 is removed
from its checklist, transferred in full and not marked PASS. Historical reports
with 36/37 describe the previous task allocation.


1. Consumer owner verifies TLS CA/hostname, secure service-token injection and
   expiry/rotation with the actual deployment version set. Current/previous
   overlap must be explicit and bounded; do not publish secrets in evidence.
2. rest-orc owner demonstrates trusted client-IP/proxy sanitization and safe
   RetryInfo mapping. Missing trusted IP uses the verified service-ID fallback.
3. Consuming application verifies PENDING/token-empty registration, explicit
   Login, session/expiry/restriction fields, secure credential storage and login
   ulang when legacy credentials fail. Browser cookie/CSRF belongs to that
   application; no frontend folder in rest-orc is required.
4. Deployment owner rechecks actual key/HMAC/TLS/caller and schema/minimal ACTIVE
   administrator readiness, records compatible versions and coordinates a
   login-ulang release window. Phase 6 DEP-01 remains unchecked until these results pass.
5. Failed readiness holds deployment. Failed rollout stops serving or uses a
   fully security-compatible version set; never restore HS256/sid-less
   acceptance, revoked sessions or old auth_version. No deployment or runtime
   credential invalidation is performed by this change.

User deferred rest-orc work. Full refresh/reuse/logout/password lifecycle,
management RPCs and management idempotency remain follow-ups. RabbitMQ delivery
is not needed for required pending outbox writes.

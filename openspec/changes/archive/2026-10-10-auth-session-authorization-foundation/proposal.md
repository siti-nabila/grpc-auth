# Proposal

## Why

Transaction foundation sudah diimplementasikan dan diarsipkan, tetapi request protected masih memakai JWT HS256 dan context berisi user ID tanpa persistent session atau permission authoritative. Sisa Phase 1 diperlukan sebelum command management Phase 2 dapat dibuka dengan actor terverifikasi dan persistence keamanan yang atomik.

## What Changes

- Autentikasi semua RPC menggunakan opaque service token melalui TLS, terpisah dari bearer JWT user; validasi caller, expiry, exact full-method allowlist, dan permission namespace.
- Persistent session reader/writer dan authenticated principal yang memeriksa user ACTIVE, session ACTIVE, ownership, absolute expiry, dan auth_version pada primary database.
- Resolver permission dari assignment/role aktif serta application access; `ListUsers` memerlukan `iam.user.read`, sedangkan `Me` dan `UpdateProfile` menggunakan ownership diri sendiri.
- Hubungkan caller/user terverifikasi ke transaksi dan audit/outbox command existing yang termasuk increment ini. Idempotency management memakai fondasi existing saat consumer management tersedia; jangan menambah general idempotency atau credential replay ke Register/Login/profile.
- **BREAKING pada activation gate:** protected pipeline tidak menerima token legacy tanpa sid; Register target PENDING/token kosong. Implementasi pipeline source dilakukan bersama real issuer/contract dan local readiness; deployment menunggu client Phase 3. Tidak ada fallback JWT lama atau dual-algorithm.
- Bedakan foundation code/test readiness dari activation readiness. Backend completion memerlukan guard terhubung ke RPC dan integration tests. Production deployment tetap menunggu gate lintas service yang dicatat terpisah.

## Capabilities

### New Capabilities

- `service-authentication`: TLS, opaque caller credential, rotation, method/namespace trust boundary, dan service identity terverifikasi.
- `session-authentication`: persistent session contract dan authoritative user/session principal, restricted policy, serta activation dependency pada session-bearing token issuer.
- `authoritative-authorization`: permission union dari primary snapshot, exact-method policy, ownership, dan transaction recheck.

### Modified Capabilities

- `command-persistence`: tambahan requirement production wiring verified actors untuk Register dan UpdateProfile serta kontrak consumer Login pada activation gate; requirement foundation yang sudah tersinkron tetap dipertahankan.

## Impact

**MVP_REQUIRED / grpc-auth:** server/config/interceptors, sessions request context, JWT verification boundary, feature auth/profile/user, typed domain dan bound reader/writer untuk session/RBAC, serta tests. Pertahankan loader `env.yaml`, ORM v1.7.0, transaction ownership features, reusable pool, locale/error boundary dan TesRPC/ListUsers pagination existing. Source/graph baseline commit `b89f37270d019c8d0d7823a3ff5d3526874d2232`; module api-contracts `v0.0.0-20261010044649-bfbbeb282ec7` baru menyediakan error fondasi, bukan RPC/session fields target.

**EXTERNAL_REPOSITORY / api-contracts (EXT-01):** safe additive user/session/registration fields, generic authentication/security error catalog dan mapping; published version serta standalone build wajib sebelum dependent integration. Proposal ini tidak mengizinkan edit sibling, generated file, dependency pin, atau migration.

**EXTERNAL_REPOSITORY / rest-orc dan frontend (EXT-02):** service credential/TLS, register response baru, cookie/CSRF/IP sanitization, single-flight refresh, dan login ulang. Implementasinya di repository masing-masing.

**MVP_REQUIRED / prerequisite increment Phase 3:** session-creating Login dan issuer EdDSA dengan claims sub/sid/jti/iss/iat/exp/auth_version, token/refresh response, key readiness dan basic auth limiter. Producer harus memakai production session/persistence contracts change ini dan diverifikasi melalui real Login integration sebelum protected activation. Tidak membuat session dummy, issuer test-only, runtime bypass, atau token-only login yang dianggap session-ready.

**MVP_REQUIRED / follow-up Phase 2:** CRUD/status user/role, role-permission mapping, grant/revoke role/application, last-admin business guards, catalog/audit/session management RPC. Policy/transaction consumer contract disiapkan di sini; RPC tersebut tidak ditambahkan atau dianggap PASS oleh fixture.

**MVP_REQUIRED / follow-up Phase 3:** full refresh rotation/reuse/logout/change-password/revoke lifecycle dan coordinated EdDSA deployment cutover. TTL yang sudah diputuskan tetap access 15 menit, session/refresh absolute 7 hari, skew JWT 30 detik, issuer exact ApplicationName, audience sengaja tidak divalidasi. Tidak ada sliding, refresh grace/replay, atau HS256 fallback setelah cutover.

**POST_MVP:** RabbitMQ relay/consumer, CLM resource integration, UI, distributed limiter, CRUD definisi permission/application, MFA/SSO/multi-tenant. Pending outbox tidak memerlukan broker.

Acuan: [MVP sections 3–6 dan Phase 0–6](../../../../docs/grpc-auth-mvp-spec.md), [aturan bisnis/schema](../../../../docs/user-management-spec.md), [foundation evidence](../2026-10-10-auth-transaction-foundation/evidence.md). Bukti planning bukan PASS implementasi; Phase 1 tetap terbuka sampai required integration/activation evidence tersedia.

## Approved gate separation — 10 Oktober 2026

User explicitly approved completing grpc-auth wiring/tests now and retaining
rest-orc readiness as a mandatory deployment gate. No rest-orc implementation is
added to this change. Backend prerequisites remain the real Login producer,
published contract, valid local keys/TLS/caller, basic limiter and inspected local
schema/admin. All six RPCs switch together in source; no legacy fallback or
runtime bypass is allowed. EXT-02/actual deployment readiness is preserved as a
unchecked Phase 6 DEP-01 in the global MVP spec, outside this increment; backend completion does not permit deployment
or claim Phase 1/3 production completion.

## Deployment readiness moved to final phase — 10 Oktober 2026

User moved all deployment readiness from foundation task 9.1 to mandatory
[Phase 6 DEP-01](../../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment).
Foundation completes on its implemented backend requirements and verification;
no deployment task remains in this change. External readiness is still NOT PASS
and cannot be satisfied by backend fixtures. Phase 1–5 source work retains its
security/contract/test prerequisites. No code, deployment or archive is performed
by this planning revision.

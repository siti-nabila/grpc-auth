# Proposal

## Why

Login production sudah membuat persistent session dan initial refresh credential, tetapi belum ada RPC untuk rotasi, logout, mengganti password atau mengelola sesi. Increment ini menutup lifecycle tersebut agar credential dapat diperbarui dan dicabut dengan ownership, authorization serta persistence atomik yang teruji.

## What Changes

- Tambahkan `/auth.v1.AuthService/RefreshSession`, `/auth.v1.AuthService/Logout`, dan `/auth.v1.AuthService/ChangePassword` sesuai spec MVP.
- Tambahkan `/session.v1.SessionService/ListMySessions`, `RevokeMySession`, `RevokeAllMySessions`, `ListUserSessions`, `RevokeUserSession`, dan `RevokeAllUserSessions`; seluruh nama terakhir memakai prefix service yang sama.
- Terapkan strict single-use refresh rotation, consumed-token reuse commit-on-denial, logout dengan tepat satu mode credential, serta invalidasi global setelah password/revoke-all.
- Terapkan current permission/ownership, required audit/outbox dan persistent idempotency khusus kedua admin revoke RPC sejak command diaktifkan.
- Perluas admission limiter memakai angka existing spec; sambungkan seluruh RPC ke default server tanpa membuka fallback legacy.
- Pisahkan prerequisite published `api-contracts` dari implementasi grpc-auth. Readiness consuming-client/deployment tetap mandatory Phase 6 DEP-01, di luar checklist completion backend.

## Capabilities

### New Capabilities

- `refresh-token-rotation`: refresh credential terverifikasi, single-use rotation, reuse revocation, absolute expiry dan response-loss.
- `session-management`: logout serta enam RPC session self/admin, safe list, ownership, atomic single/global revoke dan admin replay.
- `password-change`: current-password proof, restricted self-service, atomic password/counter/session invalidation dan login ulang.

### Modified Capabilities

- `local-auth-rate-limiting`: tambahan group admission RefreshSession, ChangePassword, session reads dan admin session mutations; requirements existing tetap berlaku.

## Impact

**MVP_REQUIRED / owner grpc-auth:** `internal/handler`, `internal/features/auth_feature`, feature session yang dibuat sesuai kebutuhan, `internal/repositories/domain`, `reader`, `writer`, `internal/interceptors`, `internal/configs`, `internal/ratelimit`, `pkg/types`, dan root `tests/`. Reuse primary pool, transaction provider, issuer/verifier, failure audit, authorization snapshot dan limiter existing. Tidak ada Go/SQL/config runtime yang diubah saat proposal.

**EXTERNAL_REPOSITORY / owner api-contracts / prerequisite:** sembilan RPC beserta request/response, safe session DTO/PageQuery/PageMeta, credential mode Logout, refresh-reuse error, session/version/idempotency error yang belum tersedia, localized mapping dan published module version. Rincian acceptance contract di [design](design.md#external-contract-prerequisites). Jangan mengedit sibling/generated/module cache atau pin dependency otomatis; implementasi terkait BLOCKED sampai deliverable terpisah tersedia dan scope dependency update diotorisasi.

**EXTERNAL_REPOSITORY / Phase 6:** rest-orc/browser single-flight, TLS/service caller, IP sanitization, RetryInfo, credential storage/login ulang dan actual deployment rotation/schema/admin tetap [DEP-01](../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment). Tidak ada pekerjaan rest-orc dalam increment ini.

**Compatibility:** enam RPC existing, field number/type, TesRPC, auth_id/last_id pagination dan role_codes numerik tetap. Register tetap PENDING/token kosong; Login tetap EdDSA dengan published additive fields. Endpoint baru additive; tidak ada grace refresh, sliding expiry, HS256/sid-less fallback atau credential replay.

**Follow-up MVP_REQUIRED:** user/status management, role/permission mapping, application access/catalog, EvaluateAccess dan audit read tetap increment lain; admin session guard memakai resolver existing dan tidak menunggu seluruh management CRUD selesai. Suspend/deactivate integration tetap scope user-status increment.

**POST_MVP:** RabbitMQ relay/consumer, CLM, UI, distributed limiter. DeleteRole/reset-password/permission-application CRUD tidak ditambahkan.

Acuan: [spec MVP §§3–9](../../../docs/grpc-auth-mvp-spec.md), [aturan bisnis/AC](../../../docs/user-management-spec.md), serta [backend foundation evidence](../archive/2026-10-10-auth-session-authorization-foundation/evidence.md). Proposal siap bukan evidence implementation PASS atau penutupan seluruh Phase 2/3.

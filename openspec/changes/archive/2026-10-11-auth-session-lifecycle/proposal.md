# Proposal

## Why

Login production sudah membuat persistent session dan initial refresh credential, tetapi belum ada RPC untuk rotasi, logout, mengganti password atau mengelola sesi. Increment ini menutup lifecycle tersebut agar credential dapat diperbarui dan dicabut dengan ownership, authorization serta persistence atomik yang teruji. ListUsers juga belum menerapkan semua filter contract, belum mengisi seluruh field response dan hanya menerima sort ID; revisi ini menambahkan sort dinamis serta total user unik yang konsisten.

## What Changes

- Tambahkan `/auth.v1.AuthService/RefreshSession`, `/auth.v1.AuthService/Logout`, dan `/auth.v1.AuthService/ChangePassword` sesuai spec MVP.
- Tambahkan `/session.v1.SessionService/ListMySessions`, `RevokeMySession`, `RevokeAllMySessions`, `ListUserSessions`, `RevokeUserSession`, dan `RevokeAllUserSessions`; seluruh nama terakhir memakai prefix service yang sama.
- Terapkan strict single-use refresh rotation dengan insert successor sebelum conditional consume, consumed-token reuse commit-on-denial, serta invalidasi global setelah password/revoke-all.
- Logout menerima tepat satu credential: access valid atau refresh terbaru unconsumed; consumed refresh ditolak. Refresh terbaru revoked dapat membuktikan repeat Logout changed=false sampai expiry, khusus Logout.
- Tambahkan immutable generation `sessions.auth_version` dari account saat Login; refresh dan terminal Logout memeriksa generation tersebut terhadap user. Siapkan prerequisite migration SQL terpisah tanpa menjalankannya pada database aplikasi; backfill hanya sesi ACTIVE, generation revoked legacy yang tidak diketahui tetap NULL dan tidak mengotorisasi terminal Logout.
- Rollback seluruh transaksi bisnis pada setiap error sebelum commit, termasuk business guard/read/conditional update/cancellation/panic; setelah rollback, catat alasan aman melalui transaksi audit terpisah tanpa mengganti error asli.
- Perluas ListUsers dengan sort seluruh scalar publik dan nama alfabetis terkecil dari roles/application_access, ordered multi-sort, cursor terikat query dan compatibility cursor numerik untuk sort ID. Isi seluruh safe response fields, wire statuses/application_code, role OR dan search/filter lintas kategori AND, serta total user unik sebelum cursor/limit dalam satu primary snapshot.
- List session memakai default created_at DESC, id DESC dan primary sort created_at/last_seen_at/expires_at ASC/DESC.
- Terapkan current permission/ownership, required audit/outbox dan persistent idempotency khusus kedua admin revoke RPC sejak command diaktifkan.
- Perluas admission limiter memakai angka existing spec; sambungkan seluruh RPC ke default server tanpa membuka fallback legacy.
- Pisahkan prerequisite published `api-contracts` dari implementasi grpc-auth. Readiness consuming-client/deployment tetap mandatory Phase 6 DEP-01, di luar checklist completion backend.

## Capabilities

### New Capabilities

- `refresh-token-rotation`: refresh credential terverifikasi, single-use rotation, reuse revocation, absolute expiry dan response-loss.
- `session-management`: logout serta enam RPC session self/admin, safe list, ownership, atomic single/global revoke dan admin replay.
- `password-change`: current-password proof, restricted self-service, atomic password/counter/session invalidation dan login ulang.
- `user-list-sorting`: public-field sort registry, composite cursor, active collections, filter/search composition dan unique-user total dalam snapshot authoritative.

### Modified Capabilities

- `local-auth-rate-limiting`: tambahan group admission RefreshSession, ChangePassword, session reads dan admin session mutations; requirements existing tetap berlaku.
- `authoritative-authorization`: perluasan ListUsers yang disetujui eksplisit dengan default ID ASC dan legacy numeric cursor tetap kompatibel; exact permission dan self ownership tidak berubah.

## Impact

**MVP_REQUIRED / owner grpc-auth:** `internal/handler`, `internal/features/auth_feature`, feature session yang dibuat sesuai kebutuhan, `internal/repositories/domain`, `reader`, `writer`, `internal/interceptors`, `internal/configs`, `internal/ratelimit`, `pkg/types`, dan root `tests/`. Reuse primary pool, transaction provider, issuer/verifier, failure audit, authorization snapshot dan limiter existing. Tidak ada Go/SQL/config runtime yang diubah saat proposal.

**EXTERNAL_REPOSITORY / owner api-contracts / prerequisite:** sembilan RPC/DTO/errors tersedia lokal tetapi belum published; tambahkan `UserListItem.created_at` secara additive, selaraskan docs Logout/session/ListUsers, regenerate dan verifikasi. Publication yang sudah diotorisasi menjadi workstream apply: commit scoped changes, push normal ke branch existing `feat/user-err`, tanpa tag/release/merge/force-push, lalu buktikan module dapat diambil berdasarkan commit. Pembaruan dependency grpc-auth tetap task 1.2. Rincian di [design](design.md#external-contract-prerequisites); tahap propose ini hanya mengubah artifact change, tanpa publication atau dependency update.

**EXTERNAL_REPOSITORY / Phase 6:** rest-orc/browser single-flight, TLS/service caller, IP sanitization, RetryInfo, credential storage/login ulang dan actual deployment rotation/schema/admin tetap [DEP-01](../../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment). Tidak ada pekerjaan rest-orc dalam increment ini.

**Compatibility:** enam RPC existing, field number/type, TesRPC dan role_codes numerik tetap. ListUsers mempertahankan default ID ASC, alias auth_id dan numeric last_id pada mode ID; dynamic multi-sort/cursor, filter yang belum diterapkan, response enrichment dan unique-user total adalah perluasan eksplisit yang disetujui. Tidak menerima SQL/kolom internal dari request. Register tetap PENDING/token kosong; Login tetap EdDSA dengan published additive fields. Endpoint baru additive; tidak ada grace refresh, sliding expiry, HS256/sid-less fallback atau credential replay.

**Follow-up MVP_REQUIRED:** user/status management, role/permission mapping, application access/catalog, EvaluateAccess dan audit read tetap increment lain; admin session guard memakai resolver existing dan tidak menunggu seluruh management CRUD selesai. Suspend/deactivate integration tetap scope user-status increment.

**POST_MVP:** RabbitMQ relay/consumer, CLM, UI, distributed limiter. DeleteRole/reset-password/permission-application CRUD tidak ditambahkan.

Acuan: [spec MVP §§3–9](../../../../docs/grpc-auth-mvp-spec.md), [aturan bisnis/AC](../../../../docs/user-management-spec.md), serta [backend foundation evidence](../2026-10-10-auth-session-authorization-foundation/evidence.md). Proposal siap bukan evidence implementation PASS atau penutupan seluruh Phase 2/3.

Main specs tidak di-sync pada tahap propose. Delta specs menjadi target implementasi; sync/archive dilakukan setelah implementasi dan verifikasi selesai. Revisi/publication contract saja tidak menutup lifecycle atau Phase 6 DEP-01.

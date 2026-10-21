# Proposal

## Why

Increment B/C/D sudah mengelola role, assignment dan akses aplikasi, tetapi belum menyediakan RPC untuk membaca union permission beserta role sumbernya. Increment E membuka pembacaan self/admin melalui resolver dan primary snapshot existing agar consumer dapat menjelaskan akses tanpa menafsirkan JWT atau role numerik.

## What Changes

- Tambahkan `/user.v1.UserService/GetMyEffectivePermissions`: target selalu verified self, active unrestricted session, tanpa permission administratif.
- Tambahkan `/user.v1.UserService/GetUserEffectivePermissions`: target numerik eksplisit dan exact current `iam.user.read`; service caller juga memerlukan namespace IAM untuk operasi admin.
- Keduanya mengembalikan union permission unik dan contributing role ID/string code, diurutkan deterministik dari satu primary snapshot bersama actor authority dan target status/version. Assignment/role/application harus ACTIVE; permission bisnis tetap memerlukan application access ACTIVE. Tidak ada bypass role name, numeric role-code, JWT atau cache.
- Keputusan user 14 Oktober 2026: `application_code` opsional; kosong mencakup semua namespace yang diizinkan caller, kode valid unknown/inactive menghasilkan daftar kosong, malformed ditolak. Namespace eksplisit yang tidak diizinkan tetap ditolak sebelum lookup.
- Keputusan user: admin boleh membaca target PENDING/SUSPENDED/DEACTIVATED yang belum deleted. Tampilkan union assignment/role yang masih ACTIVE dengan gerbang aplikasi existing, mengabaikan hanya status akun target. Response menyertakan `user_status`; kontribusi ini merupakan tampilan konfigurasi yang tersimpan, bukan izin akses atau rekonstruksi audit historis. REVOKED/DISABLED tidak disertakan. Status akun/principal untuk otorisasi caller tetap wajib ACTIVE.
- Tambahkan admission protected-read 120/menit burst 20, thin handlers dan registrasi tepat 35 methods; pertahankan baseline 33. Tidak ada mutation, version increment, session invalidation, success audit/outbox atau persistent replay untuk read ini; failure audit existing tetap berlaku.
- Implementasikan self/admin sebagai dua fungsi snapshot dengan dependency eksplisit, mengikuti ReadUserDetailSnapshot existing. Perluas SessionReadHandler yang sudah menerima provider/clock; tidak menambah read-service constructor, interface service atau binder baru.

## Capabilities

### New Capabilities

- `effective-permission-reads`: pembacaan self/admin, filter namespace, safe role-source union, tampilan target nonaktif, consistent snapshot, error/admission dan compatibility.

### Modified Capabilities

Tidak ada requirement existing yang diganti. Capability ini memakai [authoritative authorization](../../../specs/authoritative-authorization/spec.md), snapshot/session/service authentication dan limiter existing. Tampilan target nonaktif tidak mengubah keputusan akses authoritative maupun restricted allowlist.

## Impact

| Scope | Owner / deliverable | Prerequisite / status proposal |
| --- | --- | --- |
| EXISTING | grpc-auth: resolver, permission reader, primary RR snapshot, auth/session/transaction owner, B/C/D, limiter dan failure audit | Tersedia di working tree; pertahankan seluruh perubahan user |
| MVP_REQUIRED E | grpc-auth: input/result domain, dua fungsi snapshot feature, perluasan handler existing, composition/interceptor admission, tests dan API/evidence | NOT_STARTED; backend transport menunggu contract E |
| EXTERNAL_REPOSITORY EXT-01 | api-contracts: dua RPC/messages additive, generation/compatibility/error/locale tests dan published module | Belum ada pada proto checkout; pin grpc-auth `v0.0.0-20261013075041-21082d7a243d`. Scope sibling perlu eksplisit saat apply; publication tetap memerlukan otorisasi Git |
| EXTERNAL_REPOSITORY consumers | rest-orc/UI: REST dan tampilan status/contributing roles | Tidak diedit dalam change ini; readiness deployment tetap Phase 6 DEP-01 |
| POST_MVP / increment lain | Relay/CLM/resource policy/distributed limiter; EvaluateAccess dan ListAuditLogs di Phase 4 | Bukan prerequisite E atau placeholder |

Area terencana: `internal/features/user`, `internal/repositories/domain`, reader existing bila perlu, `internal/handler`, `internal/configs`, `internal/interceptors`, root `tests/` dan dokumentasi. Tidak ada schema/migration, credential/config baru, framework/refactor luas, local proto/pb atau perubahan RPC/field/error existing.

Traceability: [Phase 2 E](../../../../docs/phase-2-management-plan.md#8-increment-e--effective-permission-reads), [matriks MVP](../../../../docs/grpc-auth-mvp-spec.md#42-matriks-full-rpc), AC-09/10/13/23/31 dan M-02/03/05/06/09/10/11. Artifact ini planning-only; tidak membuktikan implementation, publication, acceptance, atau deployment PASS.

## Implementation status

Backend E COMPLETE; published contract, runtime and required gates PASS. Scope/status tables above record the proposal baseline. See [final evidence](evidence/progress.md) and [API](../../../../docs/effective-permission-api.md). Main spec synced and change archived on 14 October 2026. DEP-01 deployment remains separate.

# Proposal

## Why

Phase 2 A–E sudah menyediakan administrasi akses dan pembacaan permission efektif, tetapi consumer belum memiliki RPC untuk mengevaluasi satu permission atas identitas user yang memanggil. Increment Phase 4 ini membuka evaluasi authoritative melalui resolver dan primary snapshot existing, dengan hasil deny yang dibedakan dari kegagalan autentikasi atau dependency.

## What Changes

- Tambahkan `/authorization.v1.AuthorizationService/EvaluateAccess`, dengan `application_code` dan `permission_code` wajib; identitas selalu verified self, tanpa target `user_id` pada request.
- Wajibkan TLS, exact-method service allowlist, requested-namespace allowlist serta user/session ACTIVE dan unrestricted. Tidak memerlukan `iam.user.read` atau permission administratif lain untuk mengevaluasi permission milik sendiri.
- Baca ulang user/session/version, application/access dan kontribusi permission pada satu primary RepeatableRead READ ONLY snapshot. Pakai `ResolvePermissions`/`HasPermission` existing; hanya assignment/role/application ACTIVE dan mapping `deleted_at IS NULL` yang dapat berkontribusi. Permission bisnis juga memerlukan application access ACTIVE; tidak ada bypass role admin, JWT role snapshot, cache atau wildcard.
- Kembalikan `allowed`, bounded `reason`, `user_id`, `sid`, `user_version` dan `evaluated_at`. Identity/session/dependency failures tetap gRPC error; response bukan tiket akses yang boleh disimpan atau dipakai ulang untuk aksi berikutnya.
- Keputusan user 18 Oktober 2026: application code valid yang belum terdaftar menghasilkan `allowed=false`, `PERMISSION_MISSING`. Aplikasi terdaftar nonaktif menghasilkan `APPLICATION_INACTIVE`; business access absent/REVOKED menghasilkan `APPLICATION_ACCESS_REQUIRED` sebelum evaluasi exact permission.
- Daftarkan satu service/method baru: baseline 35 methods/seven services menjadi 36 methods/eight services. Gunakan protected-read quota bersama 120/menit burst 20 dan safe independent denial/failure audit setelah snapshot selesai. Allowed read tidak menulis success audit, outbox, idempotency result atau state bisnis.

## Capabilities

### New Capabilities

- `access-evaluation`: kontrak single-permission self evaluation, validation, service namespace boundary, authoritative gates/reasons, snapshot/error cleanup, admission, denial audit dan regression evidence.

### Modified Capabilities

Tidak ada requirement existing yang diganti. Change memakai [authoritative authorization](../../../specs/authoritative-authorization/spec.md), [service authentication](../../../specs/service-authentication/spec.md), [effective permission reads](../../../specs/effective-permission-reads/spec.md) dan [local limiter](../../../specs/local-auth-rate-limiting/spec.md). Tampilan permission target nonaktif pada increment E tetap bersifat deskriptif dan tidak dipakai sebagai authorization shortcut.

## Impact

| Scope | Owner / planned deliverable | Prerequisite / status saat proposal |
| --- | --- | --- |
| EXISTING | grpc-auth: verified caller/principal, session checks, resolver, permission/application/access readers, snapshot provider/transaction owner, limiter dan safe attempt recorder | Terlihat di source; preserve working-tree changes, termasuk rename fixture `schema_fixture.go` |
| MVP_REQUIRED | grpc-auth: feature `authorization`, input/result domain, thin gRPC handler, explicit composition/admission, deny audit, tests dan API/evidence | NOT_STARTED; transport BLOCKED pada EXT-01 sampai contract tersedia |
| EXTERNAL_REPOSITORY EXT-01 | api-contracts: AuthorizationService/messages/reason enum, generation, compatibility/error/locale checks dan published module | Belum tersedia pada checkout proto yang diperiksa; grpc-auth pin `v0.0.0-20261014032725-545701c82e22`; pekerjaan sibling/publication memerlukan scope/otorisasi tersendiri saat apply |
| EXTERNAL_REPOSITORY consumers | rest-orc dan service domain: panggil evaluasi menjelang aksi serta tetap cek resource policy sendiri | Tidak diedit di change ini; consuming-client/deployment readiness tetap Phase 6 DEP-01 |
| Increment lain / POST_MVP | ListAuditLogs dan full mutation-matrix reconciliation adalah increment Phase 4 terpisah; metrics penuh Phase 5; relay/CLM integration/distributed limiter POST_MVP | Bukan prerequisite atau placeholder pada RPC ini |

Area implementasi terencana: `internal/features/authorization/`, `internal/repositories/domain/`, existing reader adapters, `internal/handler/`, `internal/configs/`, session/rate/security-attempt boundaries, root `tests/` serta dokumentasi. Reuse pool, provider dan clock existing; tidak ada schema/migration, secret/config runtime baru, proto lokal, framework atau refactor luas.

Traceability: [matriks full RPC MVP](../../../../docs/grpc-auth-mvp-spec.md#42-matriks-full-rpc), [Phase 2 follow-up](../../../../docs/phase-2-management-plan.md#3-urutan-increment), AC-09/10/11/12/13/23/27/31, M-01/02/03/05/06/07/09/10/11 dan EXT-01. Dokumen ini planning-only; tidak meluluskan implementasi, publication atau deployment.

## Implementation status

Backend COMPLETE on 18 October 2026; all 24 tasks and required workspace/standalone contract, default/race/vet/build and dedicated PostgreSQL normal/race gates PASS. Planning baseline tables above are historical. See [implementation evidence](evidence/progress.md) for immutable dependency/source identity and actual results. Phase 6 DEP-01 remains separate; main specs were synced and this change was archived on 18 October 2026.

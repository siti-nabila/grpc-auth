# Proposal

## Why

Reader/writer grpc-auth belum sepenuhnya memakai ORM: assignment/application access mengeksekusi INSERT/UPDATE manual, role mapping memakai DELETE/INSERT manual, dan beberapa reader menyusun SELECT subquery sebagai string. Pengguna meminta seluruh persistence melalui ORM, dengan penghapusan mapping berupa soft delete agar row tetap tersimpan.

## What Changes

- Refactor reader/writer agar SELECT/INSERT/UPDATE, termasuk nested SELECT, dibangun oleh ORM; hapus jalur eksekusi SQL manual di repository. Predicate, projection, aggregate dan join expression tetap boleh mengikuti API ORM existing dengan whitelist dan parameter binding; statement atau subquery SELECT utuh tidak boleh ditulis manual.
- Lengkapi sibling ORM dengan komposisi query untuk EXISTS/NOT EXISTS, IN subquery dan derived joins, serta kemampuan INSERT yang mengembalikan hasil affected rows. Pertahankan API, DryRun, placeholder, quoting, context dan transaction ownership existing untuk PostgreSQL/MySQL/Oracle.
- Tidak menambah API hard DELETE. Keputusan pengguna 14 Oktober 2026: `role_permissions.deleted_at` nullable; mapping dicabut diberi timestamp, mapping dipasang kembali menjadi NULL, dan authorization/read hanya memakai mapping yang belum dihapus.
- ReplaceRolePermissions mempertahankan final-set semantics, row pasangan unik, mapping yang tidak berubah, version/no-op/replay dan audit/outbox atomik. CreateRole memakai initial mapping nondeleted. Semua timestamp mutation berasal dari clock feature existing.
- **Storage compatibility prerequisite:** tambahkan kolom nullable melalui migration additive yang direview terpisah dari eksekusinya. Existing row tetap aktif/NULL; composite primary key tetap. Binary lama yang mengabaikan tombstone tidak aman untuk rollback setelah soft delete dipakai.
- Pertahankan inventory 35 methods, protobuf/error catalog, authorization, limiter, pagination/cursor dan credential policy. Implementasi hanya dimulai setelah request apply baru.

## Capabilities

### New Capabilities

- `repository-orm-persistence`: construction/execution melalui ORM, komposisi nested query, affected-row/error behavior, ownership transaksi dan prerequisite dependency/schema yang dapat diuji.

### Modified Capabilities

- `role-administration`: final-set mapping menggunakan soft delete/restore dan atomic failure mencakup kedua operasi itu.
- `authoritative-authorization`: mapping dengan deleted_at terisi tidak memberi authority, termasuk setelah role diaktifkan kembali.
- `effective-permission-reads`: union dan contributing sources mengabaikan mapping yang soft-deleted, termasuk target nonaktif.

## Impact

| Scope | Owner / deliverable | Prerequisite / status |
| --- | --- | --- |
| EXISTING | grpc-auth: feature-owned transaction/RR snapshot, reader/query builder, conditional ORM Updates, B/C/D/E, root tests dan dedicated PostgreSQL harness | Ada di working tree; seluruh user hunks dipertahankan |
| MVP_REQUIRED correction | grpc-auth: reader/writer/domain transaction capability, role mapping timestamp, schema script/fixture, authorization/role/catalog/list regression dan documentation | NOT_STARTED; ORM/schema compatibility wajib sebelum activation |
| EXTERNAL_REPOSITORY | `/Users/bleki/Desktop/mini-projects/orm`: additive query-composition dan insert-result APIs, multi-dialect tests, release artifact | Scope refactor sibling disetujui pengguna; belum diimplementasikan. Publication/commit/push/version tetap pekerjaan eksplisit, bukan izin dari proposal |
| EXTERNAL_REPOSITORY unchanged contract | api-contracts pinned `v0.0.0-20261014032725-545701c82e22`; REST/UI | Tidak ada protobuf, catalog atau client contract change yang direncanakan |
| MVP_REQUIRED increment lain | EvaluateAccess dan ListAuditLogs | Phase 4 follow-up; bukan POST_MVP atau dependency refactor ini |
| POST_MVP | Relay/CLM/UI/distributed limiter | Di luar scope |

Source terdampak: `internal/repositories/{reader,writer,domain}`, `pkg/database/transaction.go`, `internal/features/role/{create,permissions}.go`, tests dan `docs/sql/`; sibling `orm/{query,orm}` serta `orm/test_scenarios/`. Existing soft-delete auth dan lifecycle ACTIVE/REVOKED assignment/application/session tidak diganti mekanismenya. Tidak ada DeleteRole/CRUD permission baru, generic repository framework, koneksi baru atau migration otomatis.

Traceability: [spec MVP](../../../../docs/grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence), [spec user management](../../../../docs/user-management-spec.md#15-acceptance-criteria-dan-pengujian), AC-09/10/13/21–26/31, M-02/05/07/08/09/10/11 dan local ORM-01..07. Proposal ini bukan bukti runtime PASS; deployment actual tetap Phase 6 DEP-01.

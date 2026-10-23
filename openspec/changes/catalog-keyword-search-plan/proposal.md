# Proposal

## Why

Pencarian catalog saat ini memakai `ILIKE` pada field bisnis sehingga belum mendukung dokumen keyword full-text yang komposisinya dikelola database. Ketentuan [catalog-keyword-search-plan](../../../docs/catalog-keyword-search-plan.md) menyatukan pencarian role, permission, application, dan assignment role user tanpa membuat frontend mengetahui kolom sumber.

## What Changes

- Tambahkan proyeksi `role_search`, `permission_search`, `application_search`, dan `user_role_search`, beserta function/trigger, backfill, index, verifikasi, dan rollback sebagai migration baru yang direview.
- Database memiliki komposisi dokumen dan normalisasi titik/hyphen/whitespace. `fts_lexeme_text` diturunkan dari lexeme vector; perluasan sumber cukup melalui function/trigger/backfill tanpa perubahan binary grpc-auth.
- Empat list menerima logical search field `keyword` dengan mode explicit `FULL_TEXT` atau `FULL_TEXT_TRIGRAM`; mode gabungan memakai prefix semua token dengan AND, atau substring literal. Tidak menambah toleransi typo atau ranking.
- Pertahankan `CONTAINS`/`PREFIX`, default mode, explicit fields, filter, sort, projection, dan cursor caller lama. Cursor jalur keyword baru terikat pada query/scope serta versi semantics; numeric cursor hanya tetap pada jalur lama.
- Verifikasi baseline users email/name/phone/digit-only phone dan selaraskan fixture; formula dan contract users tidak otomatis diubah. Tambahkan regression, isolation, transactional trigger, concurrency, dan performance gates.

## Capabilities

### New Capabilities

- `catalog-search-projections`: proyeksi search yang sinkron secara transaksional, tokenisasi database, perluasan dokumen tanpa perubahan Go, dan cutover setelah verifikasi.

### Modified Capabilities

- `iam-catalog-reads`: perluas requirement search tiga catalog dan cursor untuk jalur keyword; hapus penolakan full-text hanya bagi logical field baru, pertahankan jalur compatibility.
- `user-role-assignment`: perluas search/pagination `ListUserRoles` melalui proyeksi nama role tanpa mengubah ownership target, histori assignment, atau mutation behavior.

## Impact

| Klasifikasi / owner | Scope dan prerequisite |
| --- | --- |
| EXISTING / grpc-auth | Keempat RPC telah memiliki handler, feature, reader bersama, authorization dan primary snapshot. Domain masih hanya menerima contains/prefix; protobuf mode FTS sudah dipetakan. |
| MVP_REQUIRED / grpc-auth | `/role.v1.RoleService/ListRoles`, `/permission.v1.PermissionService/ListPermissions`, `/application.v1.ApplicationService/ListApplications`, `/user.v1.UserService/ListUserRoles`; domain/reader catalog, cursor, fixture dan tests, migration search terpisah. Secure admission dan transaction foundation tetap wajib. |
| EXTERNAL_REPOSITORY / rest-orc | Caller memilih `fields=[keyword]` dan `FULL_TEXT_TRIGRAM` secara explicit setelah schema/backfill siap. Implementasi REST/UI serta deployment lintas service tetap pekerjaan terpisah, dilacak pada Phase 6 DEP-01. |
| EXTERNAL_REPOSITORY / api-contracts dan ORM | Tidak ada RPC, generated code, dependency update atau sibling edit yang direncanakan. ORM terpakai sudah menyediakan join dan predicate berparameter; builder FTS generiknya belum aman untuk requirement ini dan tidak dipakai langsung. Gap tambahan yang terbukti harus menjadi scope terpisah sebelum implementasi terkait. |
| POST_MVP / di luar increment | Typo similarity, relevance ranking, perubahan search `ListUserApplicationAccess`, penghapusan compatibility, permission/application CRUD, relay/CLM/UI. |

Tidak ada code atau SQL implementasi yang diubah pada tahap proposal. Schema live, wrapper trigger users, biaya fan-out rename dan hasil PostgreSQL belum diverifikasi; semua gate implementasi baru berstatus belum dijalankan, bukan PASS.

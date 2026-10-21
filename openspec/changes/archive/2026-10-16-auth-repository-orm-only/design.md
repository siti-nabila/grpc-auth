# Design

## Context

Motivasi dan scope ada di [proposal](proposal.md). Planning baseline: 14 Oktober 2026, grpc-auth Go 1.26.2, api-contracts `v0.0.0-20261014032725-545701c82e22`, ORM `v1.7.0`; 35 registered methods. B/C/D/E dan refactor user berada dalam working tree, sehingga HEAD saja bukan identitas deliverable. Graph grpc-auth dibangun pada `bf6be877296b8f4191f2e2db5f744b23ab636ed7`, graph ORM pada `90b9cdd8860578725f7402f12fbaa86af83cb4f1`; keduanya dipakai untuk routing, lalu source aktual diperiksa.

| Area aktual | Temuan source / batas perubahan |
| --- | --- |
| `writer/application_access.go`, `writer/user_role_assignment.go` | Activate/Revoke memakai StatementWriter.ExecContext untuk INSERT/UPDATE dan managementAffected untuk strict affected-row validation |
| `writer/role_management.go` | CreateRole/UpdateRole sudah ORM; ReplacePermissions menghapus seluruh mapping lalu insert satu per satu; DELETE/INSERT manual |
| `reader/user_list_query.go` | Correlated SELECT MIN untuk collection sorting; EXISTS SELECT untuk role/application filter; total/page/cursor harus tetap identik secara semantik |
| `reader/role_management.go`, `reader/catalog.go` | Correlated affectedRoleUsers expression dipakai projection/sort/filter cursor; counts memakai ACTIVE assignment dan join auth, tanpa menambah predicate status user baru |
| `reader/registration.go`, `writer/refresh_revocation.go` | NOT EXISTS mapping dan IN sessions ditulis sebagai full nested SELECT string |
| `reader/authorization.go`, `reader/role_management.go` | Permission contribution/RolePermissions belum memfilter mapping soft-deleted |
| `domain/transaction.go`, `pkg/database/transaction.go` | Optional StatementWriter dan native ExecContext menjadi escape manual query; provider menjaga primary lease dan isolation, lifecycle di feature |
| SQL/schema | `docs/sql/user-management/01_schema_backfill.sql` dan PostgreSQL fixture role_permissions hanya role_id/permission_id dengan composite PK; schema aplikasi live belum diperiksa |
| Sibling ORM | Updates mengembalikan sql.Result; Create/CreateWith.Exec hanya error; query belum punya composable subqueries; tidak ada hard-delete builder sesuai konsep pengguna |

Legacy `reader/auth.reader.go` juga menyimpan contoh raw SELECT yang dikomentari. Audit apply mencakup seluruh production reader/writer, bukan hanya daftar di atas. SQL migration, fixture setup dan ORM internal SQL generation berada di boundary yang berbeda.

Parent go.work saat ini memuat grpc-auth/api-contracts/error-package/rest-orc, **tidak memuat sibling ORM**. Tes ORM workspace dari checkout gagal dengan module-not-in-workspace sebelum kode dijalankan; draft ORM yang sempat dimulai telah dikembalikan seluruhnya. Tidak ada evidence implementasi baru. Apply menggunakan temporary workfile dengan member asli ditambah ORM; tidak mengubah parent go.work, tidak mengedit cache dan tidak menganggap workspace membuktikan published release.

## Goals / Non-Goals

**Goals:** Hilangkan SQL statement manual pada production repository dengan API ORM kecil; pertahankan query semantics, strict affected rows, error/cause, request context dan transaction ownership. Terapkan soft delete/restore mapping dalam transaksi/version/audit/outbox existing tanpa mengubah public wire contract.

**Non-Goals:** Hard DELETE API, automatic/global soft-delete scopes ORM, ORM baru, generic repository framework, join setiap request dengan pool baru, DeleteRole/permission CRUD, pagination DSL baru, EvaluateAccess/ListAuditLogs, publication Git otomatis, eksekusi migration/deployment atau refactor sibling lain.

## Decisions

### 1. Komposisi query kecil di ORM

Tambahkan fluent `WhereExists(child)`, `WhereNotExists(child)`, `WhereInQuery(column, child)` dan `LeftJoinQuery(child, alias, on)` pada builder existing. Setiap child berasal dari UseModel/Select/Join/Where/GroupBy existing. Simpan snapshot child; compile pada build parent memakai dialect/placeholder mode parent dan offset argument yang benar. EXISTS/IN dapat digunakan pada SELECT maupun Updates. Invalid/nil child atau alias derived table invalid menjadi error sebelum execution, bukan predicate yang hilang.

Child, count/page clone dan repeated DryRun tidak boleh saling mengubah. Parent-child berbeda dialect atau config tidak boleh menghasilkan SQL campuran: rendering placeholder/quoting mengikuti parent; konfigurasi incompatible yang tidak bisa diterapkan aman ditolak. Tidak menambah parser SQL, raw query passthrough atau manual replace `$1`/`:1` di repository. API baru harus mempertimbangkan Oracle derived alias tanpa `AS`, named/numbered placeholder offsets termasuk setelah SET update, identifier quoting, grouped/complex count, limit/offset dan selected-column scan.

Untuk ListUsers, derived grouped query per user menghasilkan MIN role name dan MIN application name dengan predicate ACTIVE yang sama. LEFT JOIN menghasilkan null key untuk user tanpa eligible collection; scalar expressions memakai alias hasil join untuk projection/order/cursor. EXISTS child menjaga filter roles sebagai OR dalam set kode dan AND lintas kategori. Jangan mengganti dengan query ID batch atau in-memory pagination karena itu mengubah total/continuation dan skala memory.

Untuk role summaries/catalog, derived grouped query per role menghasilkan COUNT DISTINCT user_id dengan join auth dan ACTIVE assignment yang sama; COALESCE ke nol mempertahankan role tanpa user. Gunakan builder helper yang sama untuk detail/catalog projection, sort, count dan cursor. Jangan menambah account/role/app predicates pada affected-user count yang mengubah definisi existing. Untuk registration, NOT EXISTS hanya terhadap mapping nondeleted; untuk revoke user refresh, IN child memilih semua session milik user, mempertahankan terminal/token scope existing.

Alternatif: string SELECT di Where/Select atau build manual di adapter akan mempertahankan bypass yang ditolak pengguna. Generic expression DSL/scalar subquery APIs tidak ditambahkan tanpa consumer nyata; empat kemampuan komposisi cukup untuk temuan saat ini.

### 2. INSERT result memakai pipeline existing

Tambahkan `CreateWith(model).ExecResult() (sql.Result, error)` yang memakai preparation/build/logging existing dan execution tanpa RETURNING, mengikuti batas Exec. SqlTransactionAdapter.CreateWith existing mempertahankan request context. ORM consumer interface grpc-auth memerlukan kemampuan insert-result kecil pada handle supplied; leasedTransaction meneruskannya ke adapter ORM pada lease yang sama, tanpa native SQL passthrough.

Create, Exec, Scan, ScanInto dan DryRun existing tetap kompatibel; ExecResult menolak returning intent yang tidak sesuai sebelum DB call. Tidak otomatis mengisi primary ID untuk mode ini. Insert-specific structs mencantumkan hanya kolom write yang dibutuhkan, sehingga database defaults dan generated keys tidak berubah. API harus mempertahankan identity/cause error, nil result dan driver RowsAffected error untuk keputusan consumer; tidak memalsukan count=1.

Test melalui CreateWith.DryRun dan ExecResult memakai model tanpa returning untuk SQL/args/mode yang sama, bukan API DryRun test-only. Multi-dialect input/tag/null/zero semantics dan transaction adapter diuji. Alternatif memakai Create lalu return true akan menghilangkan verifikasi rows affected yang sudah menjadi kontrak assignment/application writes.

### 3. Writer assignment dan application access tetap conditional

Ganti INSERT manual dengan insert-specific model melalui ExecResult. Ganti UPDATE manual dengan patch structs melalui UseModel.WithContext.Where.Updates. Patch activate menyertakan status/granted_by/granted_at serta nullable revoked_by/revoked_at/revoke_reason yang sengaja di-set NULL; patch revoke hanya mengganti status/provenance revoke, tanpa menimpa grant history. Include identity pasangan/row dan status-before pada WHERE existing. managementAffected tetap membedakan 0/1/invalid/missing result/RowsAffected error; feature memutuskan no-op/conflict, version dan final commit.

Setelah seluruh consumer dimigrasikan, hapus StatementWriter dan native ExecContext pada leasedTransaction. Pertahankan native transaction hanya untuk cleanup finalization existing; itu bukan query execution escape. Tidak membuat constructor/binder/layer baru.

### 4. Soft delete mapping menggunakan diff feature

Pengguna memilih deleted_at nullable, bukan hard delete atau status ACTIVE/REVOKED baru. Tambahkan domain mapping row dengan role_id, permission_id, deleted_at. Reader existing menyediakan inventory pasangan lengkap, termasuk tombstone, pada primary transaction yang sama. Feature pemilik command menghitung diff set canonical setelah locks dan validation; writer hanya menjalankan persistence operations yang ditentukan feature.

| Pasangan existing / final intent | Efek |
| --- | --- |
| Aktif dan masih diminta | Tidak ditulis ulang |
| Aktif dan tidak diminta | Conditional UPDATE deleted_at = waktu command |
| Deleted dan diminta kembali | Conditional UPDATE deleted_at = NULL pada pasangan yang sama |
| Tidak pernah ada dan diminta | INSERT pasangan baru dengan deleted_at NULL |
| Deleted dan tetap tidak diminta | Tidak ditulis ulang; timestamp lama dipertahankan |

Gunakan satu domain change set dengan sorted removed/restored/inserted IDs, diteruskan pada existing role writer; hindari boolean-mode controller. Writer memvalidasi shape/IDs/counts tetapi tidak memutuskan final-set policy. Timestamp berasal dari injected feature clock `now`, diteruskan untuk CreateRole/ReplaceRolePermissions; repository tidak membaca wall clock. Initial mapping CreateRole hanya inserted IDs pada role baru. Effective replacement memiliki satu version increment; set aktif identik tetap no-op walaupun terdapat tombstone. Set kosong sah dan hanya menghapus secara logis pasangan aktif.

Composite PK(role_id,permission_id) tetap mencakup tombstones agar regrant tidak membuat duplikat. Tidak menambahkan history table dan tidak merekonstruksi mapping yang pernah hard-deleted sebelum migration. deleted_at mencatat soft deletion terkini; regrant menghapus tombstone, audit existing tetap sumber peristiwa perubahan.

### 5. Semua permission reads memakai pasangan nondeleted

Authorization contribution reader dan RolePermissions memakai `rp.deleted_at IS NULL`; DefaultMember memeriksa ketiadaan **active mapping**, bukan ketiadaan historical row. GetRole menampilkan current permissions saja; permission catalog tetap menampilkan catalog registered permission terlepas dari association tombstone. GetMy/GetUserEffectivePermissions mempertahankan sorted source union, filter caller namespace dan active assignment/role/application/access gates, termasuk descriptive view target nonactive. Tidak memaparkan deleted_at/provenance baru dalam protobuf.

Role enable tidak memulihkan mapping deleted. Restored mapping mengikuti existing app/access gate. Current actor recheck sebelum mutation/replay membaca reader nondeleted yang sama; stale privilege tidak menjadi bypass. Soft delete mapping tidak mempengaruhi last-admin definition berbasis protected IAM role existing; system/IAM role mutation protections tetap wajib, termasuk no-op.

### 6. Security, lifecycle dan atomicity dipertahankan

Exact full methods utama: `/role.v1.RoleService/CreateRole` memerlukan iam.role.create, `/role.v1.RoleService/ReplaceRolePermissions` iam.role.permission.update, `/role.v1.RoleService/GetRole` dan ListRoles iam.role.read; `/user.v1.UserService/ListUsers` dan GetUserEffectivePermissions iam.user.read; GetMyEffectivePermissions memakai verified self tanpa admin permission. Semua mengikuti TLS, service token exact method/namespace, ACTIVE unrestricted actor/session serta verified principal; policy Register/Login/RefreshSession/TesRPC dan restricted Me/ChangePassword/Logout tetap existing. Tidak ada trust pada user_id/service/roles dari browser atau JWT role claim.

Begin/Commit/Rollback/TryLock tetap di feature/features-common; provider infrastructure hanya menyediakan primary handle/lease. PostgreSQL advisory TryLock retained sampai finalization; shared protocol idem -> last-admin/policy -> account/role-code -> user -> role -> application -> session dan canonical IDs tidak berubah. Mutations recheck actor/target setelah locks; reader RPC RR snapshot read-only tetap. Error acquired=false RESOURCE_BUSY/ABORTED berbeda dari dependency UNAVAILABLE. Cancellation/deadline dan panic/commit cleanup termasuk commit-on-denial refresh reuse tidak diubah.

| Command / outcome | Business write | Version / credential | Audit / event / result |
| --- | --- | --- | --- |
| CreateRole efektif | Role dan initial mapping | Role version1; user/auth/session tetap | ROLE_CREATED + iam.role.created.v1 + keyed safe result atomik |
| ReplaceRolePermissions efektif | Mapping diff dan role version | Role +1 sekali; user/auth/session tetap | ROLE_PERMISSIONS_CHANGED + iam.role.permissions_changed.v1 + keyed safe result atomik |
| Same active set, current version | Tidak ada mapping write | Tidak ada increment; stale request tetap VERSION_CONFLICT | Satu NO_OP audit dan keyed safe result; tanpa event |
| Replay unexpired | Tidak ada write atau refresh deleted_at | Original safe result; current actor harus tetap berwenang | Tidak ada audit/event/version tambahan |
| Grant/Revoke role atau app | Patch ORM mengganti operasi SQL setara | Existing entity version/compound policy; auth_version/sesi tetap | Existing audit/outbox/idempotency dan atomicity tetap |
| Permission/catalog/list reads | Tidak ada mutation | Tidak ada increment/invalidation | Tanpa success audit/outbox/replay; existing failure audit independen |

CreateRole/ReplaceRolePermissions key wajib; scope actor + exact full method + canonical role-code/role target + key, normalized final set/presence/expected version, safe retention24h. Actor authorization sebelum replay, replay sebelum expected-version recheck; same scope berbeda payload ABORTED. Failure soft-delete/restore/insert/version/audit/outbox/result rollback bersama; failure audit independen setelah cleanup tidak menutupi error asli. Commit uncertainty tidak diklaim rollback pasti. Credential hanya response Login/Refresh; logger/debug sensitif tetap disabled. Admission protected read120/min burst20 dan admin mutation30/min burst5 serta buckets/cap/TTL/cleanup existing tetap.

Register PENDING/token-empty dan client adjustment yang sudah disepakati dipertahankan. Access15m/session7d absolute/skew30s/issuer tepat/audience ignored/EdDSA-only, strict single-use refresh, restriction dan global invalidation password/suspend/deactivate tetap existing.

## Risks / Trade-offs

- Missing schema atau reader lama menganggap tombstone aktif → migration additive prerequisite dan coordinated binary activation; jangan deploy code baru sebelum schema siap atau mencampur writer baru dengan reader lama.
- Perubahan query shape mengubah null key, total atau cursor → derived joins memakai definisi existing dan satu query helper; uji pagination beberapa page, nullable ties, overlapping rows dan count sebelum/ sesudah.
- ORM composition salah menggeser placeholder dalam derived/WHERE/UPDATE → tests PostgreSQL/MySQL/Oracle, nested groups dan parameter SET lebih dahulu, invalid-child negatives dan repeated DryRun/count immutability.
- Restore/new insert/soft-delete conditional gagal → per-pair RowsAffected validation dan feature rollback; shared role locks/unique PK menghindari lost update, DB constraints tetap backstop.
- Storage tombstones bertambah → dipertahankan sesuai user decision, tanpa physical cleanup/retention job baru. History sebelum migration tidak dibuat kembali.
- Workspace memakai versi local yang belum released → pin hanya published version actual; standalone gate BLOCKED jika belum tersedia, bukan replace/cache workaround.
- Archived B/C/D/E evidence memakai source lama → tidak diedit sebagai bukti refactor; capture manifest/evidence baru dan ulangi affected backend gates.

## Migration Plan

1. Pada apply, siapkan SQL preflight dan additive migration `docs/sql/role-permission-soft-delete/01_add_deleted_at.sql`, terpisah dari fresh baseline schema. Tambahkan `deleted_at timestamptz NULL` tanpa default, pertahankan existing row/PK; preflight menolak incompatible existing column, tidak drop/recreate data. Update fresh schema docs dan fixture untuk parity. Script keberadaan bukan bukti live schema siap.
2. Lengkapi ORM, unit/DryRun/mocked tests; gunakan temporary workspace membership yang memasukkan ORM. Persiapkan reviewed release diff. Commit/push/tag/publication belum diotorisasi oleh proposal; mintakan scope Git hanya sesudah perubahan konkret dan verified. Versi final belum dipilih/diinvent.
3. Refactor grpc-auth terhadap source ORM yang sama, fixtures/schema dan permission readers/writers; jalankan mock dan dedicated-test gates. Tidak menjalankan migration pada DB aplikasi/user. Hanya fixture-owned schemas pada explicitly opt-in dedicated PostgreSQL suite.
4. Setelah ORM benar-benar published dan diotorisasi, pin immutable release dan ulangi GOWORK=off tests/vet/race/build. Catat commit/checksum/source manifest. Backend completion membutuhkan seluruh required gates, bukan actual deploy.
5. Actual schema/key/TLS/caller/consumer deployment tetap DEP-01. Schema expansion harus mendahului coordinated code activation; old readers/writers dihentikan sebelum tombstone writes. Sebelum aktivasi, binary rollback ke baseline mungkin dengan kolom additive tetap. Sesudah tombstones dipakai, rollback hanya ke binary yang tetap menghormati deleted_at; jangan clear tombstones/drop column atau menghidupkan permission untuk memudahkan rollback. Stop traffic dan forward-fix bila compatible rollback artifact belum ada.

## Acceptance ledger

Status apply aktual ada pada [evidence ledger](evidence/progress.md): seluruh required gates PASS; ORM published/pinned dan standalone/workspace final verification selesai. Setiap baris mencatat command, mode/env aman, waktu, source/release identity serta expected/actual. Source audit/planning validation tidak meluluskan functional gate.

| ID | Scope / owner / AC | Prerequisite | Required test / evidence |
| --- | --- | --- | --- |
| ORM-01 | Production query construction; grpc-auth; user ORM-only decision, M-09 | APIs ORM yang cukup | AST/source inventory seluruh reader/writer dan transaction escape; ORM model/binding SQL shape; zero complete manual statements/subqueries |
| ORM-02 | Generic query/result behavior; ORM; external library contract | Supported three dialects | Existing and new DryRun query/args/mode/quoting/count immutability; mocked execution/context/affected counts/cause; default API regressions |
| ORM-03 | Schema and soft-delete/restore; grpc-auth; AC-21/22, M-07/08 | Nullable schema + released/source-compatible ORM | Migration preservation/PK parity; A/B -> B/C -> A/B; row identity/timestamps, empty/no-op/replay/stale, no extra user/auth/session effect |
| ORM-04 | Authority and safe current views; grpc-auth; AC-09/10/13/23/31 | All mapping readers filter deleted_at | Sole/overlap actor and target mappings, role enable, app/access gates, registered E reads including nonactive target; next snapshot with original JWT |
| ORM-05 | ListUsers/role catalog; grpc-auth; existing sorting/cursor contract | Composed query helpers | SQL args/context; unique count, MIN/COALESCE/null keys, multi-sort/ties/numeric and composite continuation, filters/search/projection; no silent truncation |
| ORM-06 | Atomicity/concurrency; grpc-auth; AC-24–26, M-09/10 | Feature locks and isolated DB suite | Fail each soft-delete/restore/insert/version/audit/outbox/result/commit; actor loss/replay, two physical DB connections/barriers, no-op races, cancellation/panic/pool cleanup; existing refresh regressions |
| ORM-07 | Deliverable compatibility; both repositories; M-10/11 and publication prerequisite | Immutable published ORM + schema fixture | Targeted/full tests, vet/race/build as relevant; service workspace and GOWORK=off, published checksum; inventory35, redaction, scope/user-hunk preservation; final strict OpenSpec |

## Open Questions

Tidak ada keputusan bisnis tersisa: scope sibling dan deleted_at sudah diputuskan pengguna. Release version/commit/checksum dan actual deployed schema baru dapat dicatat dari hasil apply/publication atau DEP-01; tidak diisi dari asumsi. Nama exported API proposal dapat disesuaikan saat implementasi tanpa mengubah semantics/test gates di atas.

# Design

## Context

Motivasi dan scope ada pada [proposal](proposal.md); perilaku ada pada [self-access-snapshot](specs/self-access-snapshot/spec.md). Keputusan user berasal dari [plan Me](../../../../docs/me-access-snapshot-plan.md).

Inspeksi working tree 22 Oktober 2026 menemukan:

| Area | Existing | Perubahan yang diperlukan |
| --- | --- | --- |
| `internal/configs/register.service.go` → `internal/handler/session_reads.go` | Me diteruskan ke `ReadUserDataSnapshot`; handler tidak menerima target user | Pertahankan registrasi, principal dan constructor snapshot/clock |
| `internal/features/auth_feature/read_snapshot.go` | Satu primary read-only Repeatable Read; session recheck; identity/profile; legacy roles; explicit commit | Isi `created_at`, collection baru, map permission dan concat phone sebelum finalisasi snapshot yang sama |
| `internal/repositories/domain/user_identity.go` dan reader identity | Safe identity belum memiliki `created_at`; legacy `ActiveRoles` memeriksa assignment/role ACTIVE | Tambahkan safe user creation time; reader scoped baru untuk metadata dengan gate aplikasi/access, tanpa mengubah reader legacy/list globally |
| `internal/repositories/reader/authorization.go` + `internal/features/common/authorization.go` | Reader mengecualikan `rp.deleted_at`; resolver union/gates/namespace consistency tersedia | Gunakan pada tx Me; filter namespace service caller sebelum materialisasi permission map |
| `internal/features/user/effective_permissions.go` | Permission RPC memakai snapshot sendiri dan namespace filter | Reuse pola/resolver, bukan memanggil RPC atau feature yang membuka snapshot kedua |
| `internal/repositories/reader/user_list_collections.go` | Metadata role/application untuk list; scope/filter khusus list | Jadikan referensi mapping; jangan memanggil list atau mengubah semantik list agar cocok dengan Me |
| api-contracts `UserData` | Tag 1–14 tersedia; role/app messages sudah ada; map belum ada | Tambah map tag 15 di repository contract; generate, tidak edit generated code manual |
| `tests/integration/auth_reads/test_scenarios/scenarios.go` | SQL driver fake memeriksa ordering, isolation/read-only, error dan response minimum | Perluas fixtures/assertions mengikuti kolom/read baru; jangan menghapus regression legacy |

Graph `graphify-out/graph.json` pada commit `2462cb09d0f8e7eef3b17a70456451b8bd2dd6ec` dipakai untuk routing. Hub snapshot/authorization/effective-permission dikonfirmasi pada source di atas. Working tree memiliki perubahan country-code dan perubahan lain milik user yang harus dipertahankan. Config OpenSpec memuat beberapa baseline lama; source aktual dan keputusan user tentang country code/Me tetap menjadi acuan change ini.

## Goals / Non-Goals

**Goals:** satu view konsisten; reuse session/permission policy; contract additive untuk map; perubahan phone terisolasi pada Me; test failure-safe dan namespace boundary yang dapat dibuktikan.

**Non-Goals:** database migration/backfill; perbaikan data phone lama; perubahan login/JWT/refresh; search catalog; HTTP/FE/CLM implementation; penghapusan RPC permission/evaluation; caching atau permission version; publication/pin update otomatis.

## Decisions

### 1. Perluas feature snapshot existing dengan lifecycle yang sama

Call path tetap registered Me → session read handler → auth feature snapshot → tx-bound readers. Feature memanggil injected `BeginSnapshot(requestCtx)`, yang memakai shared pool dan `sql.LevelRepeatableRead`/read-only. Semua pembacaan dilakukan sequential pada handle tx yang sama; tidak membuat pool reader atau goroutine pada shared transaction.

Urutan implementasi:

1. Pastikan verified service caller dan context valid.
2. Begin snapshot, buat transaction owner dan defer cleanup dari feature.
3. AuthenticateSession dari tx dan `AuthorizeMethod` exact Me; owner hanya principal terverifikasi.
4. Baca identity termasuk `auth.created_at`, lalu profile termasuk nullable country code.
5. Bentuk safe scalar response dan concat phone literal.
6. Bila restricted, lewati seluruh privilege read. Bila unrestricted, baca legacy roles, scoped role/app metadata dan permission contributions pada tx yang sama.
7. Filter namespace menggunakan server-owned `Caller.AuthorizeNamespace`, resolve active contributions, isi `permissions[code]=true`.
8. Commit/finalisasi eksplisit dari feature; return hanya bila seluruh proses sukses. Deferred cleanup dan outer recovery meniadakan partial response pada error/panic.

Alternatif agregasi `GetMyEffectivePermissions`/list roles/apps ditolak karena membuka auth/read snapshot terpisah dan menambah public method permission/admission coupling. Feature `ReadMyEffectivePermissionsSnapshot` tidak dipanggil dari Me karena memiliki lifecycle sendiri.

`Begin`/`Commit`/`Rollback` tetap dimiliki `internal/features/` atau `features/common`; repository hanya menerima tx. Me adalah read sehingga tidak memakai TryLock, expected version, last-admin guard, command fingerprint atau idempotency persistence. Tidak ada key-order/commit-on-denial perubahan pada mutation/refresh; keduanya tetap existing. Provider/ORM hanya mengimplementasikan lifecycle teknis yang diminta feature.

### 2. Kontrak aman dan mapping terpisah dari list/detail

api-contracts menambahkan `map<string, bool> permissions = 15` pada `UserData`. Request Empty, service/RPC dan tag/type 1–14 tetap; tag 15 harus diperiksa masih kosong sebelum generator. Reuse `UserRoleAssignment` dan `UserApplicationAccess` existing. Tidak menambah session metadata, permission sources, new status enum atau wrapper response.

`created_at` berasal dari `auth.created_at`, bukan waktu profile, waktu request atau fallback current clock. Domain/reader identity ditambah kolom ini dan mock fixtures diperbarui. Proto timestamp dibuat dari nilai sumber, mengikuti pola safe user detail/list existing.

Permission map berisi grant `true` saja dan tidak mempunyai urutan wire yang dijanjikan. Empty map adalah hasil valid hanya setelah semua dependency yang diperlukan berhasil. Proto repeated/map yang empty dapat tidak tampil pada JSON sesuai marshaller; FE/REST adapter nantinya tidak boleh membedakan null/omitted sebagai grant. Tidak menambah false entries untuk seluruh catalog.

Alternatif response gabungan baru atau nested permission source ditolak karena contract UserData sudah memiliki safe profile/role/app fields dan user memilih lookup `map[string]bool`.

### 3. Namespace dan collection metadata tidak bergantung pada adanya permission

Tambahkan small safe reader/model untuk scoped Me metadata pada tx existing. Role query memerlukan assignment ACTIVE, role ACTIVE, application ACTIVE, serta business access ACTIVE. Application query memerlukan access/application ACTIVE. Pilih hanya namespace yang diizinkan service caller. Deduplikasi by ID dan urutkan ID ascending untuk response stabil.

Role tanpa mapping permission tetap boleh ada pada `roles`. Application grant aktif tanpa effective permission tetap ada pada `application_access`. Karena itu jangan membangun kedua collection hanya dari join permission: join tersebut tidak merepresentasikan metadata tanpa mapping. IAM tidak memerlukan application grant untuk permission/self-service; jangan membuat grant IAM sintetis pada collection aplikasi.

Legacy `role_codes` dan `role_names` tetap memakai mapping metadata `ActiveRoles` existing: numeric role code dan stored role name dengan urutan role ID. Jangan menggantinya dengan string RBAC code atau menjadikannya permission. Namespace/gates ketat pada collection baru dan permission map tidak dibypass oleh keberadaan metadata legacy; restricted mengosongkan seluruh collection lama dan baru. Ini mempertahankan consumer legacy sekaligus memberi contract scoped yang eksplisit untuk consumer baru.

Gunakan `NewPermissionReader(tx).Permissions(...)` dan `ResolvePermissions` existing, dengan namespace filter berdasarkan `ApplicationCode` server-owned. Pertahankan `deleted_at IS NULL`, namespace/application ID consistency, ACTIVE gates dan IAM exception yang sudah diuji. Jangan mengubah resolver shared untuk mengakomodasi response mapping.

Alternatif reuse reader list tanpa gate tambahan ditolak karena list hanya memfilter sebagian metadata dan tidak membatasi namespace service. Alternatif deriving roles dari resolved permission ditolak karena menghilangkan role tanpa mapping.

### 4. Restricted branch dan error boundary tetap fail-closed

Me tetap membutuhkan service token opaque melalui TLS dan allowed full method `/user.v1.UserService/Me`, lalu JWT/session ACTIVE. Tidak memerlukan permission admin. Identitas service tidak berasal dari payload/browser. Caller namespace merupakan batas tampilan collection baru, bukan pilihan FE.

Restricted user masih boleh Me, ChangePassword dan Logout saja. Response Me minimum mengisi safe profile/status/entity version/created_at/restriction marker; tanpa legacy roles, roles/apps baru atau permissions. Branch ini tidak memanggil privilege readers sehingga dependency privilege tidak diperlukan untuk minimum.

| Kondisi | Hasil |
| --- | --- |
| Credential/user/session invalid | Existing generic authentication catalog → UNAUTHENTICATED |
| Service method/namespace authority | Existing verifier/filter policy; disallowed response namespace dihilangkan, bukan diperluas dari input |
| Begin/identity/profile/legacy role/scoped collection/permission read atau commit gagal | Dependency error dengan cause → UNAVAILABLE; response nil |
| Cancellation/deadline | CANCELED/DEADLINE_EXCEEDED, context tetap dipropagasikan |
| Panic | Cleanup snapshot; safe INTERNAL; response nil |
| Successful empty permitted set | Safe response dengan no granted keys, bukan error dan bukan grant default |

Gunakan shared dependency/error mapper dan locale existing; jangan membuat error string/catalognya sendiri. Preserve independent failure audit setelah cleanup dan shared protected-read limiter existing (120/minute, burst 20) beserta bounded RetryInfo/telemetry. Tidak membuat bucket, method registration atau label cardinality baru.

Alternatif fallback empty map/stale cache ditolak karena menyembunyikan gangguan dependency dan membuat consumer menafsirkan state yang tidak authoritative.

### 5. Concat phone literal hanya di response Me

`phone = profile.CountryCode.String + profile.Phone`; nullable country code dianggap empty. Lakukan di mapping Me, termasuk restricted response. `country_code` tetap terpisah. Jangan memanggil formatter, validator/normalizer profile mutation, helper shared detail/list atau menyimpan hasilnya kembali.

| Country code | Stored phone | Me.phone |
| --- | --- | --- |
| `+62` | `81234567890` | `+6281234567890` |
| `+62` | `081234567890` | `+62081234567890` |
| `+62` | `+6281234567890` | `+62+6281234567890` |
| NULL / empty | `+6281234567890` | `+6281234567890` |

Tidak ada default +62, separator atau penghapusan prefix. Empty phone juga mengikuti concat literal; tidak ada pengecualian tersembunyi. User akan memperbaiki data lama melalui pekerjaan terpisah.

Alternatif normalisasi nomor atau concat di rest-orc ditolak sesuai keputusan user. Wire field tetap kompatibel, tetapi semantics Me.phone berubah; consumer harus memperlakukan nilai ini sebagai hasil concat dan tidak menambahkan country code lagi.

### 6. Tetap read-only dan bukan credential lintas service

Tidak ada command baru. Matrix efek Me: user/entity/auth_version, assignment/access, session/refresh, success audit, outbox dan idempotency result semuanya tidak berubah. Incidental Idempotency-Key tidak menghasilkan replay; response loss diikuti read baru. Failure audit/limiter tetap mengikuti mekanisme existing yang independen dari response snapshot.

rest-orc/CLM kelak mengambil Me langsung memakai credential request saat ini dan menentukan permission route di server. Browser map tidak diterima sebagai grant; backend protected RPC tetap mengotorisasi dan CLM tetap memeriksa resource. Tidak ada janji atomicity lintas Me dan transaksi CLM, atau pembatalan request in-flight setelah revoke.

Alternatif cache berdasarkan user `version` ditolak: role/mapping/app changes dapat mengubah permission tanpa mengubah entity version atau auth_version.

## Risks / Trade-offs

- [Legacy phone mempunyai prefix ganda] → hasil literal tetap sesuai keputusan user; dokumentasikan contoh dan verifikasi consumer tidak concat ulang. Tidak memperbaiki row otomatis.
- [Permission/scoped metadata berasal dari query berbeda] → semua reader memakai handle snapshot yang sama; PostgreSQL two-connection test membuktikan konsistensi, bukan hanya mock ordering.
- [Legacy role metadata tidak sama dengan collection scoped baru] → pertahankan compatibility mapping existing dan nyatakan hanya collection scoped/map yang dipakai consumer baru; legacy tidak memberi izin.
- [Jumlah role/app/permission besar] → tanpa truncation atau pagination pada Me; ukur query count/hasil representatif. Optimasi tidak boleh membuka scope atau mengganti primary state dengan cache.
- [Workspace memberi kesan contract sudah published] → catat exact checkout/generated output dan pisahkan verification workspace dari publication/standalone. Jangan menandai `GOWORK=off` PASS sebelum pin yang benar tersedia.
- [Rollback ke server lama menghilangkan map] → consumer baru harus fail-closed bila backend pengayaan yang diperlukan tidak tersedia; koordinasikan rollback consumer/backend, tanpa credential atau authorization fallback.

## Migration Plan

1. Pada tahap implementasi terotorisasi, owner api-contracts menambah tag 15, menjalankan `make proto`, compatibility/descriptor/binary/proto-JSON tests dan static checks. Tidak mengedit module cache/generated manual.
2. Dengan contract lokal yang kompatibel pada workspace, implementasikan pengayaan Me, safe readers dan tests grpc-auth. Tidak ada schema migration atau data write.
3. Jalankan targeted/full/vet kedua repository, targeted race, PostgreSQL opt-in dedicated DB, serta regression rest-orc tanpa mengubah source consumer. Catat hasil dan blockers.
4. Publication/pin version consumer menunggu permintaan terpisah sesuai plan. Setelah tersedia, owner verifikasi grpc-auth dengan `GOWORK=off`; standalone yang belum dapat resolve tidak diklaim PASS.
5. Actual rest-orc/CLM mapping, safe failure handling dan phone compatibility dilakukan sebagai follow-up consumer. Mandatory deployment gate tetap Phase 6 DEP-01; tidak deploy dari workflow proposal/apply backend ini.
6. Rollback terkoordinasi mempertahankan TLS, EdDSA/session/current authorization; tidak memulihkan legacy credentials atau revoked state. Karena read-only, tidak ada database rollback. Jangan memakai fallback privileged map dari browser atau snapshot lama.

## Acceptance dan Evidence Ledger

Kriteria dan AC subset ada pada [spec traceability](specs/self-access-snapshot/spec.md#traceability). MAS-EXT-01 dan MAS-01..06 PASS pada local workspace; hasil command dan source fingerprints ada pada [evidence](evidence/progress.md). Implementasi harus menambahkan evidence berisi source/module versions, waktu, command tanpa secret, mode/environment dan expected/actual result; failed/unavailable required gate dicatat FAIL/BLOCKED, bukan PASS karena suite di-skip.

| Criterion | Owner / prerequisite | Verification yang harus dicatat | Status |
| --- | --- | --- | --- |
| MAS-EXT-01 | api-contracts; tag 15 available | Generator, descriptor tags/type preservation, map binary/proto-JSON roundtrip, full test/vet | PASS |
| MAS-01 | grpc-auth; existing service/session pipeline | Allowed Me/no-admin, spoofed identity, invalid/expired/revoked session, restricted no-privilege branch, safe fields | PASS |
| MAS-02 | grpc-auth; active/gated readers + resolver | Caller namespace subset, dedup/overlap, soft-delete, no-mapping metadata, legacy mapping | PASS |
| MAS-03 | grpc-auth; snapshot provider + dedicated PostgreSQL opt-in | SQL-fake ordering + two distinct backend connections/barriers, committed-change visibility, finalization/cancel/panic/pool cleanup | PASS |
| MAS-04 | grpc-auth; nullable country code existing | Table concat examples, NULL/empty/empty-phone cases, profile/detail/list and no-write regression | PASS |
| MAS-05 | grpc-auth; existing protected-read/failure audit pipeline | Unchanged inventory/admission/RetryInfo/locale/redaction; repeated read has no versions, audit-success/outbox/replay effects | PASS |
| MAS-06 | grpc-auth + api-contracts; local compatible workspace, rest-orc available for regression | Targeted/full test and vet, race, consumer regression; compile/build when wiring/build path changes | PASS |
| MAS-EXT-02 | Separate publication/pin owner request | Published module/checksum/source and standalone grpc-auth GOWORK=off test/vet | DEFERRED_TO_SEPARATE_REQUEST |

PostgreSQL scenarios extend `tests/integration/postgres` with synthetic owned schema and explicit opt-in/DSN/expected DB guard. Connection A establishes the Me snapshot; connection B commits controlled role/mapping/app/access or user/session changes at a barrier. Assert internally consistent response, next-call visibility, proper denial where principal is now invalid, and zero business side effects. Use two backend IDs and barriers/channels rather than sleep-based proof. No Docker, app database, migration or seed execution in this proposal.

Relevant default test homes are `tests/integration/auth_reads`, `tests/features/auth_feature`, `tests/repositories/reader`, and `tests/handler`; keep root `tests/` runner/scenario convention. Prefer existing SQL fake and resolver fixtures to new test frameworks. Contract tests follow api-contracts' own test layout during its separately scoped implementation.

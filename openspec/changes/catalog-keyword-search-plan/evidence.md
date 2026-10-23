# Catalog keyword search evidence

Tanggal: 23 Oktober 2026, verifikasi final sekitar 11:40–11:45 WIB (UTC+7).
Status: backend implementation PASS; application migration pada database `user`
selesai 23 Oktober 2026 sekitar 14:22 WIB. Consumer cutover tetap Phase6 DEP-01.

## Deliverable dan environment

- Repository HEAD `38e70b922e73da354485fd5693c9088c8f036e38`; deliverable berupa
  working-tree changes, tidak membuat branch/commit/push. Perubahan user yang
  sudah ada dipertahankan, termasuk hash fixture PostgreSQL global dan runner.
- Go `go1.26.2 darwin/arm64`; ORM terpakai `v1.8.0-support-subquery`;
  go.mod api-contracts `v0.0.0-20261018073250-b4ad807ad5fa`, parent workspace
  memakai sibling api-contracts. Tidak ada dependency/generated/contract change.
- PostgreSQL `16.13`, `pg_trgm 1.6` pada schema `public` di database test
  khusus yang lolos LoadConfig dan read-only identity check harness.
- Awal koneksi ditolak sandbox (`EPERM`), lalu berhasil melalui execution
  approval. Index test awal gagal karena extension belum tersedia. User
  kemudian secara eksplisit mengizinkan Codex memasang pg_trgm hanya di
  database test. Provisioning dilakukan sekali di luar harness setelah
  verifikasi identitas; harness/migration tidak otomatis memasang extension.
- Pada verifikasi implementasi awal, semua mutation/schema/test data berada dalam namespace disposable milik
  harness. Tidak membaca source account/credential, mengakses database aplikasi,
  menjalankan application migration, Docker, server deployment atau rest-orc.
  DSN/password tidak dicetak atau disimpan di evidence.

## Commands dan hasil

Environment opt-in `GRPC_AUTH_POSTGRES_TEST_ENABLED`, DSN dan expected database
disiapkan secara privat dari konfigurasi test lokal, tidak dari env.yaml.
Cache terpisah `/tmp/grpc-auth-catalog-go-cache` dipakai untuk default test/vet
setelah sandbox menolak sebagian file cache Go eksternal; cache bukan artifact
repository.

| Command | Actual result |
| --- | --- |
| `rtk go test ./tests/repositories/domain/role/...` | PASS, 52 tests pada 3 package |
| `rtk go test ./tests/repositories/role/...` | PASS, 57 tests pada 2 package |
| `rtk go test ./tests/handler/... ./tests/repositories/domain/role/... ./tests/repositories/role/...` | PASS, 512 tests pada 8 package |
| `rtk go test ./tests/features/catalog/... ./tests/features/role/... ./tests/features/user/...` | PASS, 873 tests pada 9 package |
| `rtk proxy go test -tags=postgres ./tests/integration/postgres -run '^TestPostgresConfiguration$' -count=1` dengan cache test terpisah | PASS tanpa database/network |
| `rtk proxy go test ./...` dengan cache test terpisah | PASS, seluruh default suite; postgres-tag suite tidak dipilih |
| `rtk proxy go vet ./...` dengan cache test terpisah | PASS, exit0 |
| `rtk proxy go test -race -tags=postgres ./tests/integration/postgres -count=1` | PASS seluruh suite PostgreSQL, 169.950s; dilakukan sebelum penambahan scenario baseline users final |
| `rtk proxy go test -race -tags=postgres ./tests/integration/postgres -run '^TestPostgresCatalogKeyword' -count=1 -v` | PASS pada family final termasuk baseline users dan pengukuran batch/window, 4.523s |
| `rtk proxy openspec validate catalog-keyword-search-plan --strict` | PASS |
| `rtk git diff --check`, local-link/anchor/whitespace checks | PASS |

Perubahan setelah full PostgreSQL run hanya menambah scenario/fixture users
terisolasi dan diagnostic timing; seluruh family final diuji ulang dengan race
detector. Production Go tidak berubah setelah targeted/default/full suite lulus.
Make build dan GOWORK=off dependency migration tidak diperlukan: startup/wiring,
registration/build path dan dependency version tidak diubah.

## Acceptance ledger

| Criterion | Expected dan observed | Evidence / status |
| --- | --- | --- |
| CKS-01 | Exact logical keyword/mode, raw Unicode200, empty/omitted no filter, input isolation, legacy defaults/scalars tetap; invalid combinations tidak memanggil feature | `TestKeywordCatalogs`, `TestKeywordHandlers`, existing serialized-cursor scenarios; PASS |
| CKS-02 | Empat PK/FK-cascade/nonnull projections, source formula sempit, separator target positions, literal parser dan vector-derived text; source extension tanpa Go update | `TestPostgresCatalogKeyword`, `TestPostgresCatalogKeywordQueries`, projection metadata dan GIN checks; PASS |
| CKS-03 | Count/page memakai join/predicate/args sama, no wildcard/empty-normalized broadening, prefix-only/text-only OR, no silent fallback | `TestKeywordSQL`, `TestPostgresCatalogKeywordQueries`, dynamic permission formula/backfill scenario; PASS |
| CKS-04 | Target/app/status tetap AND, user lain tidak bocor, opaque query-bound revision, legacy serialization, ASC/DESC nullable/tie-breaker/projection, offset/keyset dan authority/snapshot/error tetap | Domain/handler/feature scenarios, registered RPCs dan existing all-public-scalar sort harness pada keyword path; PASS |
| CKS-05 | Role/permission/app/assignment refresh, revoked-name fanout, reassign, no rebuild status/regrant/replay/no-op; source/projection/persistence rollback dan cancellation cleanup | `TestPostgresCatalogKeywordConcurrency`, `TestPostgresCatalogKeywordCommands` dan trigger scenarios, real two-backend lock barriers; PASS |
| CKS-06 | Supplied email/name/phone/digit-only formula, full/partial email +/underscore, formatted phone, auth/profile changes, insert/reassignment, unchanged users cursor | `TestPostgresCatalogKeywordUsers`; source wrapper bodies supplied user pada chat 23 Oktober 2026, fixture namespace-only; PASS |
| CKS-07 | Idempotent resumable batches, initial formula completeness checks, physical GIN indexes, retained rollback boundary dan performance shape | `TestPostgresCatalogKeywordQueries` dan performance results di bawah; PASS untuk workload test, actual rollout tetap gate external |
| CKS-08 | Backend, default/vet/postgres/race, clean scoped source diff, documentation dan evidence | Commands di atas; PASS |
| CKS-EXT-01 | Actual application schema/privilege/window, consumer explicit keyword mode dan rollback readiness | Database migration/verification PASS pada `user.public`; consumer cutover/rollback readiness tetap EXTERNAL, Phase6 DEP-01 |

Concurrency native lock barriers observed distinct backends:
77449/77450 rename->grant, 77453/77454 grant->rename,
77457/77458 rename->reassign, 77461/77462 reassign->rename,
77465/77466 backfill->rename, 77469/77470 rename->backfill.
Wait dependency diverifikasi melalui `pg_blocking_pids`, bukan sleep.
Cancellation membatalkan blocked insert, tidak meninggalkan source/projection,
dan koneksi/lock dapat dipakai kembali. Registered RPC failure juga memastikan
source version, successful audit, outbox dan idempotency result tidak tertinggal.

## Performance dan operational window

Workload sintetis mencakup 3.002 role, 2.001 assignment, selektivitas nama role
sekitar20%, dan fan-out rename satu role pada2.000 assignment. Ini memeriksa
bentuk query count/page dan biaya proyeksi; bukan pengukuran endpoint production
atau janji latency SLO. Tidak memaksa index scan pada tabel kecil.

| Measurement | Actual ms |
| --- | ---: |
| Install schema/functions/triggers, empty owned fixture | 7.754 |
| Install transactional GIN indexes, empty owned fixture | 3.597 |
| Backfill batch100 per source | 5.405 |
| Short prefix count / page | 0.867 / 0.019 |
| Substring count / page | 0.747 / 0.044 |
| Multi-token count / page | 0.541 / 0.043 |
| No-match count / page | 0.429 / 0.367 |
| Rename + refresh2.000 assignment | 15.006 |

Total projection index size: 4.136.960 bytes pada workload ini. Query plans
direkam melalui EXPLAIN ANALYZE BUFFERS FORMAT JSON; root count Aggregate,
page Limit, rename ModifyTable, batch Function Scan. Count shared-hit blocks
50–309; batch4366 hit/2 read. Tidak ada raw keyword pengguna dalam report.

Batch100 menjadi candidate yang terukur pada environment test; setiap batch
adalah transaksi terpisah dan continuation dipersist setelah commit. Installation
window test empty schema di bawah8ms dan index di bawah4ms tidak dijadikan
timeout universal. Paket memakai transactional non-concurrent builds agar
failure rollback bersih; deployment harus mengukur ulang data/index volume
actual serta menetapkan client lock_timeout/window sebelum menjalankan schema.
Runbook dan privilege checks ada pada
[migration package](../../../docs/sql/catalog-keyword-search/README.md).

## Source fingerprints

SHA256 dari file yang diverifikasi:

| File | SHA256 |
| --- | --- |
| `internal/repositories/domain/catalog_search.go` | `64cd3c3735ce0ea539c5d18d3b47a93b8d3c0c0034b1bcefc39ab4c77bbca7de` |
| `internal/repositories/reader/catalog_search.go` | `8f3770fdcaf2e8df1ded04a2b8416cb208ec13630e0bdad1453a41073c255a48` |
| `docs/sql/catalog-keyword-search/01_schema_functions_triggers.sql` | `7c65d1b920fa61d162000b6a25adc04c05db150614d59db6466d3caa7b489eb6` |
| `tests/integration/postgres/fixtures/catalog_users_baseline.sql` | `638371cafa60af98b9c3df64170ead7a566cb57936aa72dfb51f43d249b2ff99` |

## Application database operation — 23 Oktober 2026, sekitar 14:22 WIB

User secara eksplisit meminta pemasangan trigger pada database `user`.
Koneksi memakai konfigurasi aplikasi lokal secara privat; DSN, credential,
host dan isi row sumber tidak dicetak. Identitas `current_database()='user'`
dan `search_path=public,pg_catalog` diverifikasi sebelum operasi.

- Preflight `00_preflight.sql`: PostgreSQL16.13, pg_trgm1.6 sudah ada di `public`,
  source PK/FK INTEGER sesuai; belum ada projection/helper/trigger catalog yang
  bentrok. Schema privilege dan source SELECT/INSERT/UPDATE/TRIGGER tersedia.
  Connection role adalah runtime role dari konfigurasi aplikasi dan pemilik
  source tables. Tidak membutuhkan extension installation atau GRANT tambahan.
- Source volume: role7 (73.728 bytes), permissions24 (65.536 bytes),
  applications2 (40.960 bytes), user_role10 (81.920 bytes).
  Tidak ada relation lock dari backend lain saat preflight.
- Client menetapkan `lock_timeout=3s`, `statement_timeout=60s`.
  `01_schema_functions_triggers.sql` berhasil commit dalam60ms:
  empat projection dan sepuluh trigger catalog terpasang.
- `02_backfill.sql` dipasang; batch100/source, transaksi terpisah dan cursor
  dimajukan setelah commit. Dua panggilan (termasuk batch akhir kosong)
  memproses 7 role,24 permission,2 application,10 assignment.
- `03_indexes.sql` commit dalam13ms. Delapan GIN dan empat PK indexes
  terkonfirmasi `indisvalid=true`, `indisready=true`; semua sepuluh trigger
  `tgenabled=O`. Runtime role memiliki SELECT/INSERT/UPDATE pada empat projection.
- Seluruh12 resultsets `04_verify.sql` lulus: semua missing,stale,orphan dan
  duplicate_keys nol. Fingerprint function users serta seluruh trigger
  noninternal pada auth/profile identik sebelum dan sesudah migration.
  Definisi users aktual cocok dengan formula/wrapper yang diberikan user.

Operasi ini hanya pemasangan additive migration dan aggregate verification;
tidak menjalankan test mutation pada source aplikasi atau mengganti caller.

## Rollout boundary

Application migration/backfill/index/verification sudah dijalankan pada
`user.public` sesuai catatan operasi di atas. Caller opt-in explicit dan
deployment server belum dijalankan dalam operasi ini.
Jika rollback diperlukan, pindahkan caller ke mode lama dahulu dan pertahankan
proyeksi/trigger/index; cleanup destructive adalah pekerjaan terpisah. Baseline
users diuji dari definisi yang diberikan user dan kemudian dicocokkan dengan
metadata database aplikasi saat operasi. Main specs belum disinkronkan dan
change belum diarsipkan.

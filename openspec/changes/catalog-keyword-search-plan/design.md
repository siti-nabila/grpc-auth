# Design

## Context

Motivasi mengikuti [proposal](proposal.md); keputusan bisnis mengikuti [plan sumber](../../../docs/catalog-keyword-search-plan.md). Inspeksi 23 Oktober 2026 bersifat read-only, tanpa koneksi database, server, migration atau test aplikasi. Working tree memiliki perubahan user yang harus dipertahankan.

Graph AST pada commit `2462cb09d0f8e7eef3b17a70456451b8bd2dd6ec` dipakai untuk routing, kemudian source aktual diperiksa. Hub domain transaction/snapshot dan catalog cursor sudah memiliki tanggung jawab bersama; tidak dibuat fondasi transaksi atau pagination kedua. Konteks OpenSpec bertanggal 9 Oktober adalah baseline historis: keempat RPC sudah mempunyai source handler/feature/wiring saat inspeksi ini.

| Observasi source | Implikasi |
| --- | --- |
| `internal/handler/catalog.go`, role dan assignment handlers memakai `catalogQueryFromProto`; `searchModeFromProto` sudah memetakan FTS | Tidak perlu RPC, enum atau protobuf baru. Domain masih menolak FTS. |
| `internal/repositories/domain/catalog.go` memvalidasi field publik, trim keyword, contains/prefix, batas 200 Unicode | Logical keyword harus dipisahkan dari registry projection/sort dan dari aturan compatibility. |
| `internal/repositories/reader/catalog.go` memiliki `catalogBase`, count/page dan keyset bersama; assignment memanggil `readCatalog` | Tambahkan satu routing proyeksi dan predicate bersama; jangan menduplikasi empat paginator. |
| `catalog_cursor.go` memakai envelope v1 dan hash method/sort/filter/search/target; numeric ID shortcut tidak mengikat query | Jalur baru membutuhkan envelope/hash berversi dan tidak menerima numeric shortcut. Serialization/hashes legacy tetap. |
| `features/catalog/read.go`, `features/role/read.go`, `features/user/list_assignments.go` memiliki snapshot dan current authorization | Reader tetap hanya memakai handle snapshot terinjeksi; tidak menambah transaksi mandiri. |
| `reader/user_list_query.go` memakai vector prefix OR lexeme-text substring; users handler memaksa full_text_trigram | Gunakan sebagai acuan mekanisme, bukan menyalin builder token mentah atau mengganti formula users. |
| `features/user/user.go` menormalisasi whitespace users tanpa batas keyword eksplisit pada jalur yang diperiksa | Jangan mengarang atau menyamakan batas users menjadi 200; catat hasil verifikasi contract aktual pada implementasi. |
| ORM terpakai `v1.8.0-support-subquery`, diperiksa melalui `go list -m`, mempunyai join/Where berparameter | Builder `orm/query_page.go` menambahkan `:*` pada whitespace tokens dan tidak meng-escape wildcard cabang gabungan; tidak memenuhi requirement secara langsung. |
| Parent `go.work` memakai sibling api-contracts; go.mod menunjuk versi `v0.0.0-20261018073250-b4ad807ad5fa` | Source enum FTS tersedia; tidak mengubah dependency, generated code, atau menyamakan workspace dengan release proof. |
| `docs/sql/user-management/README.md` tidak mengganti formula/trigger users; fixture PostgreSQL belum membuktikan seluruh sumber users | Baseline function dari user dipertahankan dan wrappers/live metadata menjadi gate verifikasi yang spesifik. |

## Goals / Non-Goals

**Goals:** komposisi dokumen dimiliki database; input aman melalui contract dua kolom search; perluasan sumber tanpa perubahan binary; count/page/scope konsisten; rollout dan rollback dapat diperiksa.

**Non-Goals:** mengubah autentikasi, JWT, limiter, policy, writer orchestration, search users, application-access search, atau menjalankan perubahan database pada tahap proposal. Tidak ada dependency baru, search engine, ranking, similarity typo, atau generalisasi semua query.

## Decisions

### 1. Additive search routing mempertahankan compatibility

Hanya empat catalog menerima kombinasi exact `fields=[keyword]` + explicit `FULL_TEXT`/`FULL_TEXT_TRIGRAM`. FTS dengan fields omitted atau physical fields ditolak. `keyword` bukan field response/projection/sort. Mode omitted tetap contains pada jalur legacy; `ListUserApplicationAccess` tidak ikut routing baru.

Untuk jalur baru, validasi raw input sebelum normalisasi dengan maksimum 200 rune serta pemeriksaan teks existing. Setelah normalisasi database, panjang juga tidak melebihi 200 karakter Unicode. Domain membuat salinan input dan canonical whitespace representation untuk binding cursor; nilai yang dikirim ke database tetap merupakan teks, bukan sintaks tsquery. Punctuation tidak divalidasi berdasarkan nama field bisnis atau daftar token.

REST boundary milik rest-orc tetap menerima raw keyword sesuai plan; pekerjaan ini tidak mengedit REST. Domain legacy tetap trim dan memakai batas existing setelah trim. Empty/whitespace-only menghilangkan search; punctuation-only nonblank tetap query aktif meskipun normalisasi delimiter menghasilkan string kosong. Text branch dengan normalized string kosong harus false, bukan `ILIKE '%%'`.

Alternatif mengganti default seluruh caller ditolak karena mengubah cakupan search lama, termasuk description permission dan code assignment. Memberi keyword akses ke projection/sort ditolak karena bukan scalar publik.

### 2. Empat proyeksi 1:1 dengan formula database

Migration baru direncanakan di `docs/sql/catalog-keyword-search/`, terpisah dari migration yang sudah digunakan.

| Proyeksi / alias | PK sekaligus FK INTEGER, ON DELETE CASCADE | Formula awal |
| --- | --- | --- |
| `role_search rs` | `role_id` ke `role.id` | `role.role_name`, `role.code` |
| `permission_search ps` | `permission_id` ke `permissions.id` | `permissions.code` |
| `application_search aps` | `application_id` ke `applications.id` | `applications.code`, `applications.name` |
| `user_role_search urs` | `user_role_id` ke `user_role.id` | Nama role melalui `user_role.role_id` |

Semua mempunyai `fts_keyword TSVECTOR NOT NULL` dan `fts_lexeme_text TEXT NOT NULL`; tidak ada surrogate key/sequence. Buat GIN vector dan GIN `gin_trgm_ops` text. Preflight memeriksa PostgreSQL actual, key/FK, schema/extension `pg_trgm`, privilege dan index existing; schema operator class mengikuti lokasi extension actual. Tidak memasang extension otomatis di test harness atau memindahkan extension existing.

Database refresh function menggabungkan sumber nullable dengan spasi, memakai helper normalisasi catalog, lalu `to_tsvector('simple', ...)`. Text selalu `array_to_string(tsvector_to_array(fts_keyword), ' ')`. Tidak ada field sensitif/status policy pada proyeksi. Mapping Go hanya method ke tabel/alias/key/vector/text; formula sumber tidak disalin ke Go.

Alternatif memakai role_search untuk assignment ditolak karena formula awal berbeda. Alternatif concat raw untuk text ditolak karena menimbulkan semantics yang berbeda dari lexeme vector. Penyimpanan nama berulang pada assignment diterima sesuai plan, dengan biaya rename menjadi gate performance.

### 3. Normalisasi dan parser query dimiliki database

Tetapkan contract helper migration `catalog_search_normalize(TEXT) RETURNS TEXT`, nama/schema tetap berdasarkan schema sumber yang diverifikasi. Helper menyamakan `[.[:space:]-]+` dengan satu spasi, menangani null, lowercase dan whitespace. Hanya catalog menggunakannya; formula users tetap terpisah.

Dua helper query ber-signature tetap menghasilkan tsquery aman dari normalized plain text: `catalog_search_query(TEXT) RETURNS TSQUERY` untuk semua token dengan AND dan `catalog_search_prefix_query(TEXT) RETURNS TSQUERY` untuk prefix tiap lexeme dengan AND. Query parser tidak memakai websearch operator NOT/OR dari input pengguna. Prefix dibuat dari lexeme hasil tokenisasi database dengan escaping grammar tsquery yang benar, lalu suffix prefix ditambahkan pada lexeme ter-escape; raw keyword tidak pernah diteruskan sebagai tsquery. Kasus kosong dan semua metacharacter diuji pada PostgreSQL. Helpers function harus tidak menulis data dan tidak memakai dynamic SQL dari input.

Reader memakai API ORM existing `Join`/`Where` dan ekspresi statis berparameter. Mode gabungan mengelompokkan `vector match prefix-query OR text match escaped-normalized-substring`, kemudian AND dengan target/aplikasi/status. Escape backslash, `%` dan `_` setelah normalisasi database sebelum substring `ILIKE ... ESCAPE`; parameter binding tetap wajib. Empty normalized text atau empty vector tidak berubah menjadi unfiltered query. Mode FULL_TEXT memakai vector saja dengan plain token AND, tanpa text fallback atau prefix promise. Mode gabungan adalah prefix/substring, bukan typo similarity.

Tidak dipakai langsung builder generic ORM karena source yang benar-benar dipakai belum menormalisasi delimiter atau meng-escape token/wildcard secara aman. API predicate statis berparameter tersedia sehingga tidak memerlukan sibling edit. Jika implementasi menunjukkan API tersebut tidak cukup, catat gap konkret dan hentikan bagian dependen sampai scope ORM disepakati; jangan membuat raw SELECT/subquery/ExecContext atau mengedit module cache.

### 4. Trigger menjaga sumber dan proyeksi dalam transaksi yang sama

Refresh function melakukan UPSERT berdasarkan source key. Trigger hanya untuk insert/perubahan sumber relevan: nama/code role; code permission; code/name application; insert atau perubahan role_id assignment. Rename role memperbarui role document dan semua assignment termasuk revoked. Grant ulang/revoke dengan role sama dan perubahan status/version/reason saja tidak membangun ulang dokumen. FK menangani source deletion tanpa menambah endpoint delete.

Sinkronisasi dipilih di database agar seed dan administrative writes tercakup. Writer aplikasi tidak melakukan write search tersendiri. Trigger failure menggagalkan transaksi sumber; tidak ada best-effort projection. Tidak ada audit/version/event/idempotency tambahan hanya karena proyeksi berubah. Seluruh persistence bisnis existing tetap atomik dengan mutation asal.

Untuk race rename/grant, refresh assignment harus membaca nama dari role row yang dikunci minimal SHARE hingga transaksi selesai, sebelum mengunci/upsert projection row. Rename sudah memegang write lock role; urutan projection mengikuti role lalu key assignment ascending. Refresh memakai nilai dari locked current row, bukan nilai hasil read lama. Native row locks ini melengkapi advisory protocol feature existing; tidak menambah Begin/Commit/Rollback/TryLock di repository. Verifikasi dengan dua physical connections bahwa waiting refresh membaca versi current atau gagal secara transaksional pada isolation yang menolak concurrent update; hasil stale tidak diterima. Role rename tidak mengunci source assignment rows berlawanan dengan reassign, dan update banyak role memakai ID ascending. Interleaving administrative reassignment, source FK locking dan backfill wajib diuji sebelum gate concurrency lulus.

Feature tetap pemilik Begin/Commit/Rollback/TryLock. Protocol mutation existing idem -> last-admin/policy -> account/role-code -> user -> role -> application -> session dan ordered IDs tidak diubah. PostgreSQL advisory transaction lock existing dan semantics busy/cleanup tetap. Reader hanya memakai handle feature; tidak membuat transaksi/lock lifecycle sendiri. Cancellation, panic, operation/trigger failure rollback melalui owner existing; commit failure tidak diklaim sukses atau rollback pasti. Commit-on-denial refresh reuse tidak terkait dan tidak diubah.

Alternatif application dual-write ditolak karena akan melewatkan seed/admin writes dan menduplikasi orchestration. Background asynchronous refresh ditolak karena memperbolehkan stale keyword setelah source commit.

### 5. Query bersama, snapshot dan cursor

Tambahkan join search hanya untuk jalur keyword aktif melalui `catalogBase`, dipakai count dan page secara identik. Nonkeyword dan legacy lists tidak membutuhkan row search. PK/FK 1:1 menjaga item/total tidak berlipat. Assignment tetap `ur.user_id = target`; OR search wajib berada di dalam grouping agar tidak melewati scope.

Pertahankan sort/tie-breaker/null ordering, projection dan pagination metadata existing. Tidak ada relevance score. Envelope keyword baru memakai version2 dan hash dengan revision `catalog-keyword-v1`, canonical input keyword, field/mode, full method, target, filters, dan sort. Canonical input hanya menormalkan whitespace; delimiter normalization tetap di database. Equivalent separator/case spellings boleh mempunyai hash berbeda sehingga caller mulai ulang tanpa cursor; ini penolakan konservatif, bukan izin reuse lintas query. Page/limit/projection tetap di luar binding sebagaimana existing.

Jalur keyword menolak numeric last_id karena tidak mengikat query/target/revision. Envelope v1 dan numeric cursor tetap untuk compatibility dengan byte serialization/hash lama. Revision hanya ditambahkan pada hash/envelope jalur baru, bukan global. Perubahan parser/normalizer semantics selanjutnya menaikkan revision dan memerlukan rebuild/caller cursor reset. Tidak menjanjikan snapshot yang bertahan lintas request.

### 6. Admission dan errors tetap authoritative

| Full RPC | Current permission |
| --- | --- |
| `/role.v1.RoleService/ListRoles` | `iam.role.read` |
| `/permission.v1.PermissionService/ListPermissions` | `iam.permission.read` |
| `/application.v1.ApplicationService/ListApplications` | `iam.permission.read` |
| `/user.v1.UserService/ListUserRoles` | `iam.user.read`, target terpisah dari actor |

Seluruhnya tetap TLS, verified service token dengan exact method/namespace allowlist, ACTIVE nonrestricted user/session, dan current primary authorization sebelum target disclosure. Principal berasal dari context terpercaya; payload/keyword tidak menjadi identity atau policy. Gunakan snapshot terinjeksi yang sama untuk authority, target, total dan page. Dependency failure tetap `UNAVAILABLE`; malformed mode/field/cursor/length memakai existing `common.ErrBadRequest` -> `INVALID_ARGUMENT`; cancellation/deadline dan localized error boundary tetap. Tidak ada error catalog baru atau fallback empty success.

Protected-read quota existing 120/minute burst20, bounded buckets/capacity/cleanup dan RetryInfo tetap. Search tidak mempercayai IP/service identity dari frontend dan tidak menambahkan limiter baru. Read tidak menulis version/audit/outbox/result. Trigger maintenance tidak mengubah command/no-op/replay matrix sumber: effective command, NO_OP, replay dan failure tetap mengikuti persistence existing.

Register tetap PENDING/token kosong; credential hanya Login/Refresh. TesRPC, auth_version/session revocation, EdDSA-only, issuer exact, audience ignored, access 15 menit, session absolut7 hari, skew30 detik dan restricted allowlist tidak berubah. Tidak perlu mengulang implementasi credential/session sebagai dependency baru untuk increment search.

### 7. Users baseline dan batas verifikasi

Gunakan function users yang diberikan pada plan: email/name/phone/digit-only phone, tanpa address atau country_code. Cocokkan metadata actual read-only saat environment yang diizinkan tersedia, termasuk kedua wrapper auth/profile dan perubahan profile.user_id. Pertahankan PK/FK dan formula users; fixture sintetis harus meniru formula baseline dengan tepat. Regression mencakup email lengkap/fragment, at-sign, plus, underscore, phone asli/digit-only, update auth/profile dan users cursor.

Source users saat ini juga memiliki perakitan prefix mentah. Jangan mengklaim semua input khusus sudah aman atau memperbaiki seluruh users search sebagai side effect. Jika regression baseline gagal karena perilaku users pre-existing, catat reproduksi dan blocker, lalu buat keputusan scope terpisah untuk perbaikan sebelum menyatakan gate users PASS. Pada 23 Oktober 2026 user memberikan kedua body wrapper dari database: auth me-refresh seluruh `profile p WHERE p.user_id=NEW.id`, sedangkan profile me-refresh `NEW.id`. Fixture khusus meniru definisi ini dan formula baseline; verifikasi aktual dicatat di evidence, tanpa mengganti fixture global atau code users.

Default suite tetap mock/fake dan runner/scenario existing di `tests/`. Real PostgreSQL verification memakai suite existing terpisah build tag `postgres`, tiga environment opt-in dan dedicated test database sesuai [harness](../../../tests/integration/postgres/README.md). Tidak membaca env.yaml, memakai DB aplikasi/production, menjalankan Docker atau migration aplikasi. Fixture disposable meniru migration dalam namespace miliknya; schema-qualified helper harus dapat dipasang dalam namespace test tanpa fallback public. `pg_trgm` diprovision operator test, bukan dipasang otomatis harness. Tidak mencatat raw keyword, DSN, email/phone atau credential dalam evidence.

## Risks / Trade-offs

- [Rename fan-out pada banyak assignment] -> ukur latency transaksi, writes, ukuran index dan lock contention; rollout menunggu evidence representatif, tanpa angka performa yang dikarang.
- [Backfill lama menimpa update baru] -> trigger aktif, ordered source locking/read-current yang sama dengan refresh, batch idempotent dan verifikasi ulang seluruh proyeksi.
- [Native role lock menambah deadlock pada admin writes] -> diagram urutan source/projection, dua koneksi dengan barriers dan pengujian reassign/rename/grant; gate gagal jika stale/deadlock belum dianalisis.
- [Parser/wildcard dapat mengubah scope] -> helper plain-text database dan SQL argument assertions, zero-lexeme/literal/multi-token tests; OR selalu dikelompokkan.
- [Index tidak membantu keyword pendek] -> bandingkan EXPLAIN ANALYZE BUFFERS, count/page dan realistic data; tidak memaksa index scan pada tabel kecil.
- [Schema/function live berbeda dari plan] -> preflight aman dan evidence actual; jangan menganggap script existing membuktikan deployment.
- [Cakupan keyword lebih sempit dari legacy] -> caller opt-in explicit; matriks sumber didokumentasikan dan rollback memakai mode lama.

## Migration Plan

1. Siapkan preflight read-only, schema/functions/triggers, backfill, indexes, verifikasi dan rollback baru. Inspeksi actual schema/version/extension/wrapper serta pola lock dan volume sebelum memilih ukuran batch/jendela lock. Nilai operasional ini ditetapkan dari evidence tanpa mengubah semantics.
2. Pasang schema/functions/triggers sebelum reader/caller baru diaktifkan. Gunakan jendela locking pendek terukur saat pemasangan trigger dan awal backfill. Backfill idempotent per PK mencakup semua status; jangan menulis ulang migration user management. Untuk data besar gunakan batch dengan source locking dan verifikasi ulang; bila memilih CREATE INDEX CONCURRENTLY, jalankan terpisah dari transaction block dan tangani index invalid pada retry.
3. Verifikasi missing/orphan/duplicate/content, trigger atomicity, failure/rollback dan concurrency pada dedicated test database; ukur query plans/index/rename pada data representatif. Status gate belum PASS hingga hasil actual dicatat.
4. Deploy backend additive; caller lama tetap bekerja. Readiness consumer actual dan cutover rest-orc dilacak sebagai EXTERNAL_REPOSITORY pada Phase6 DEP-01, bukan checkbox backend yang membutuhkan sibling edit.
5. Setelah schema/backfill dan gate query siap, caller mengirim `keyword`, fields keyword dan mode FULL_TEXT_TRIGRAM explicit serta membuang cursor saat query/revision berubah.
6. Rollback caller ke contains/prefix dahulu. Backend versi lama tetap kompatibel dengan tabel/trigger additive. Pertahankan proyeksi selama masa compatibility; removal helper/trigger/index/table hanya lewat cleanup migration terpisah setelah semua dependency berhenti. Tidak menambahkan fallback runtime yang diam-diam memperluas hasil.

## Acceptance ledger

Evidence increment ini telah dijalankan; hasil aktual dan batasnya ada pada [evidence](evidence.md). Catat commit/module/PostgreSQL version, command, environment aman, waktu, expected/actual dan hasil pada evidence implementasi; jangan menyalin status PASS increment lama. PostgreSQL/performance gate menjadi BLOCKED bila prerequisite tidak tersedia, tanpa menghalangi penulisan code independen atau mengarang hasil.

| ID | Scope / owner | Prerequisite | Test / evidence wajib | Status |
| --- | --- | --- | --- | --- |
| CKS-01 | Validasi/compatibility empat list, grpc-auth | Enum existing dan source caller mapping | Handler/domain scenarios, Unicode/raw overflow, mode/fields, legacy serialized cursors; AC-21/23/31, M-05/06 | PASS (evidence) |
| CKS-02 | Schema/formula/normalisasi, grpc-auth | Reviewed migration, pg_trgm pada DB test | Target vectors/positions, lexeme-derived text, formula expansion vector-only/text-only tanpa binary change | PASS (evidence) |
| CKS-03 | Predicate count/page, grpc-auth | Helper dan ORM parameter API | SQL/args asserts, AND/OR grouping, literals/no-lexeme, both independent branches, no-keyword no join | PASS (evidence) |
| CKS-04 | Cursor/ownership, grpc-auth | Query revision + existing snapshot/auth | Offset/keyset ASC/DESC, ties/nulls, changed mode/filter/query/revision, cross-user/method, missing privilege/dependency failure; AC-04/10/31 | PASS (evidence) |
| CKS-05 | Trigger/concurrency, grpc-auth | Dedicated DB opt-in, two physical connections | Source insert/update/delete/reassign, rename/grant/revoke, failure rollback, same tx handle/counters/audit/event; AC-04/10/21/23/35 | PASS (evidence) |
| CKS-06 | Users regression, grpc-auth | Wrapper definitions dan baseline fixture | Email/phone/name, auth/profile updates dan reassignment, metadata/cursor comparison | PASS (evidence) |
| CKS-07 | Backfill/performance/rollback package, grpc-auth | Representative dedicated DB + migration candidate | Missing/orphan/content verification, concurrent backfill, EXPLAIN count/page, short/multitoken/substrings/no-match, rename/index cost, rehearsal rollback | PASS (evidence) |
| CKS-08 | Backend deliverable, grpc-auth | CKS-01..07 evidence | Targeted/default/vet/race relevant, clean scope diff dan command results | PASS (evidence) |
| CKS-EXT-01 | Consumer/schema actual cutover, rest-orc/operator, Phase6 DEP-01 | Backend/schema gates + separate consumer change | Explicit keyword mode, actual deployed readiness/cursor reset/rollback; tidak dihitung sebagai pekerjaan backend increment | EXTERNAL / NOT RUN |

Traceability bisnis mempertahankan [AC existing](../../../docs/user-management-spec.md#15-acceptance-criteria-dan-pengujian) dan [MVP full-method matrix](../../../docs/grpc-auth-mvp-spec.md). Local CKS IDs menambah acceptance pencarian; tidak mengganti makna AC/M atau menyatakan MVP/deployment selesai.

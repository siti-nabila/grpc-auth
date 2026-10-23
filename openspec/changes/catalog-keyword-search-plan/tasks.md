# Tasks

Scope checklist hanya backend `grpc-auth`; status checkbox mengikuti evidence implementasi. CKS IDs mengacu pada [acceptance ledger](design.md#acceptance-ledger). Setiap penyelesaian memerlukan evidence actual (commit/module, command, waktu, environment aman, expected/actual). PostgreSQL yang tidak tersedia berarti gate terkait BLOCKED; default unit work yang independen tetap dapat dilanjutkan. Jangan menjalankan server, Docker, DB aplikasi atau deployment sebagai bagian verifikasi.

## 1. Preflight dan baseline terarah

- [x] 1.1 Tambahkan preflight read-only di `docs/sql/catalog-keyword-search/` untuk key/FK source, tabel/index search existing, PostgreSQL/schema/pg_trgm/privilege dan definisi function/trigger users; verifikasi script tidak melakukan DDL/DML dan dokumentasikan output aman serta prerequisite yang belum tersedia (CKS-02/06/07).
- [x] 1.2 Cocokkan `refresh_user_profile_search` dan kedua wrapper auth/profile dengan baseline user, termasuk update email dan profile.user_id/name/phone; dokumentasikan delta actual dan reproduksi source users untuk email/phone khusus tanpa mengubah formula/PK atau menyamakan batas users menjadi200 (CKS-06).
- [x] 1.3 Tambahkan matriks caller/compatibility dan mapping empat RPC ke proyeksi/helper serta versi cursor pada dokumentasi search; verifikasi enum/mode tersedia di dependency terpakai, mapping Go tidak memuat daftar sumber bisnis, dan sibling/generated/go.mod tetap tidak berubah (CKS-01/03).

## 2. Normalisasi dan proyeksi database

- [x] 2.1 Buat migration baru untuk empat PK/FK 1:1, vector/text nonnull dan cascade delete sesuai design; verifikasi metadata/constraints melalui fixture disposable pada suite PostgreSQL opt-in, termasuk duplicate/orphan rejection dan nullable source (CKS-02).
- [x] 2.2 Implementasikan helper normalisasi catalog ber-signature tetap dan refresh formula awal setiap feature; verifikasi target vector/posisi `iam.role.create`, `iam auditor`, `application-clm`, separator berulang/case/Unicode dan text yang berasal dari array lexeme, tanpa mengubah formula users (CKS-02).
- [x] 2.3 Implementasikan helper plain-text AND dan prefix-AND yang membentuk tsquery dari lexeme ter-escape, beserta literal substring normalization/escaping contract; verifikasi quotes, backslash, percent, underscore, ampersand, exclamation, parentheses dan zero-lexeme pada dedicated PostgreSQL tanpa parser failure atau wildcard/operator injection (CKS-02/03).
- [x] 2.4 Tambahkan fixture/scenario/tag postgres untuk schema/formula/helpers di root `tests/integration/postgres/` mengikuti runner/testutils existing dan namespace milik harness; verifikasi guard-only test tanpa network serta dokumentasikan prerequisite pg_trgm yang diprovision operator dan larangan public fallback (CKS-02).

## 3. Trigger dan atomicity

- [x] 3.1 Tambahkan relevant-column triggers role/permission/application dan assignment insert/role_id, termasuk fan-out rename ke assignment ACTIVE/REVOKED; verifikasi source insert/update/delete/reassign/seed menghasilkan current documents, sementara status/version/reason-only dan same-role revoke/regrant tidak memerlukan rebuild (CKS-05).
- [x] 3.2 Terapkan urutan native role-row/projection locking pada refresh sesuai design dan dokumentasikan interaksinya dengan advisory lock feature existing; verifikasi dua physical connections dengan barriers untuk rename/grant, rename/revoke, regrant, administrative reassign dan source deletion, tanpa stale document atau deadlock yang belum dianalisis (CKS-05).
- [x] 3.3 Tambahkan failure/rollback/cancellation scenarios pada trigger dan source commands; verifikasi source/projection/business audit/outbox/result tidak committed parsial, cleanup/pool dapat dipakai kembali, counter/no-op/replay tidak bertambah akibat refresh, dan transaction lifecycle tetap dimiliki feature (CKS-05).
- [x] 3.4 Dokumentasikan event-to-refresh matrix dan jalankan targeted postgres scenarios untuk grup ini dengan opt-in dedicated DB; simpan backend IDs/command/version/hasil yang aman dan tandai BLOCKED bila environment tidak tersedia (CKS-05).

## 4. Backfill, index dan migration package

- [x] 4.1 Buat backfill idempotent/resumable semua status dengan trigger aktif dan source-lock/read-current yang konsisten; verifikasi pause/resume, concurrent rename/insert/update serta missing/orphan/duplicate/content checks pada fixture disposable (CKS-07).
- [x] 4.2 Tambahkan GIN vector dan trigram text indexes dengan schema operator class yang benar; dokumentasikan transaction boundary, invalid-index retry bila concurrent index dipilih, dan tetapkan batch/lock window dari pengukuran; verifikasi index metadata dan retry tanpa menyentuh migration lama (CKS-02/07).
- [x] 4.3 Lengkapi README urutan preflight/schema/trigger/backfill/index/verification/rollback dan script verifikasi; verifikasi rehearsal pada namespace test membuktikan kelengkapan serta rollback kompatibel, dan cleanup destructive terpisah dari rollback caller (CKS-07).

## 5. Domain dan transport keyword additive

- [x] 5.1 Perluas normalisasi domain catalog hanya untuk exact logical keyword + explicit FULL_TEXT/FULL_TEXT_TRIGRAM; verifikasi scenario di `tests/repositories/domain/role/` dan module assignment untuk raw/normalized200 Unicode, input isolation, empty/whitespace, invalid mixes/modes/projection/sort dan legacy defaults/fields/trim (CKS-01).
- [x] 5.2 Pertahankan protobuf mapping raw keyword melalui handler role/catalog/assignment tanpa memaksa mode global; tambahkan scenario di `tests/handler/` yang memverifikasi request/response/meta/context, nil/invalid input, mode unknown, no downstream call pada validation failure dan error identity (CKS-01).
- [x] 5.3 Perbarui dokumentasi backend dengan contoh protobuf keyword, source matrix, FULL_TEXT token AND versus combined prefix/text OR, batas input dan compatibility examples; verifikasi seluruh empat full RPC dan permission tepat serta ListUserApplicationAccess tidak berubah (CKS-01).

## 6. Reader bersama dan cursor

- [x] 6.1 Tambahkan routing proyeksi dan static bound predicate ORM pada jalur catalog bersama, tanpa memakai builder FTS mentah; verifikasi SQL/args count/page, aliases/joins, prefix-only/text-only, literal escaping, empty normalized guards dan no-keyword tanpa search join dalam scenario reader role/assignment (CKS-03).
- [x] 6.2 Tambahkan query/scope scenarios untuk aplikasi/status AND grouped search OR serta `ur.user_id=target`; verifikasi unique totals sebelum cursor, tidak ada duplikasi karena mapping, dan reviewer milik user456 tidak muncul pada user123 termasuk text-only match (CKS-03/04).
- [x] 6.3 Tambahkan keyword cursor envelope/revision terpisah sambil mempertahankan legacy bytes/hash; verifikasi changed keyword/mode/field/revision/filter/sort/target/method ditolak, numeric cursor ditolak hanya pada jalur baru, dan serialized legacy fixtures tetap terbaca (CKS-04).
- [x] 6.4 Verifikasi offset/keyset ASC/DESC, tied/null multi-sort, hidden projection values, terminal pages dan metadata pada targeted repository/domain scenarios; perbarui dokumentasi cursor reset tanpa menjanjikan snapshot lintas request (CKS-04).
- [x] 6.5 Pertahankan feature-owned primary snapshot dan current permission untuk empat list; verifikasi handler->feature->reader context/handle identity, target-existence disclosure, actor privilege change, restricted/service-denied/quota rejection, dependency error dan cancellation/commit failure tanpa partial result di tests feature/integration terkontrol (CKS-04).

## 7. Regression dan evidence database

- [x] 7.1 Selaraskan fixture users dengan formula email/name/phone/digit-only dan wrapper yang telah dikonfirmasi; jalankan regression user name/email lengkap/fragment, at-sign/plus/underscore, phone asli/digit-only, auth/profile update/reassignment dan cursor; dokumentasikan blocker pre-existing terpisah tanpa memperbaiki users di luar scope (CKS-06).
- [x] 7.2 Tambahkan PostgreSQL scenario perluasan formula permission untuk description beserta trigger/backfill; verifikasi binary/request yang sama menemukan term sumber tambahan melalui vector-only dan text-only secara terpisah, tanpa whitelist source di Go (CKS-02/03).
- [x] 7.3 Jalankan suite database opt-in pada database test khusus memakai guard existing, minimal dua physical connections/barriers dan fixture disposable; verifikasi formula/query/atomicity/concurrency/backfill/rollback grup2–4 dan6–7, catat actual results tanpa secret, tandai gate BLOCKED jika prerequisite unavailable (CKS-02..07).
- [x] 7.4 Ukur EXPLAIN ANALYZE BUFFERS untuk count/page, keyword pendek, substring, multi-token, no-match, serta index size dan role rename dengan banyak assignment pada data test representatif; dokumentasikan hasil/lock window dan trade-off tanpa memaksa index scan atau mengarang threshold (CKS-07).

## 8. Verifikasi deliverable backend

- [x] 8.1 Jalankan gofmt pada file Go berubah, targeted tests tiap module lalu `rtk go test ./...` dan `rtk go vet ./...`; verifikasi default suite tidak mengakses DB dan catat hasil actual, termasuk blocker environment atau failure pre-existing (CKS-08).
- [x] 8.2 Jalankan race detector pada suite concurrency/shared state yang terdampak termasuk opt-in postgres ketika environment tersedia; jalankan make build bila wiring/registration/build path berubah; verifikasi tidak ada race dan scope dependency/contract tetap, ulangi GOWORK=off bila dependency contract kemudian berubah secara eksplisit (CKS-05/08).
- [x] 8.3 Review source call sites feature-owned Begin/Commit/Rollback/TryLock dan mutation lock ordering tetap, ORM-only reader, no business-source whitelist, safe logs dan diff/status; verifikasi `rtk git diff --check` serta semua file/test berada dalam scope dan perubahan user tetap utuh (CKS-08).
- [x] 8.4 Isi evidence CKS-01..08 sesuai command/version/environment/waktu/expected/actual, validasi change dengan `rtk proxy openspec validate catalog-keyword-search-plan --strict`, dan dokumentasikan handoff external CKS-EXT-01 ke Phase6 DEP-01; verifikasi tidak ada klaim deployment/consumer PASS dari fixture dan tidak ada checkbox backend yang mengharuskan edit rest-orc (CKS-08).

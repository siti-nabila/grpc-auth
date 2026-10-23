# Tasks

## 1. Contract Permission Map — api-contracts Prerequisite

- [x] 1.1 Pada pekerjaan api-contracts yang diotorisasi terpisah dari root edit change ini, tambahkan `map<string, bool> permissions = 15` pada `proto/user/v1/user.payload.proto`; verifikasi tag 15 belum dipakai dan descriptor fields 1–14 serta request Empty/full method tetap identik (MAS-EXT-01).
- [x] 1.2 Owner contract menjalankan `rtk proxy make proto`, menambah descriptor/binary/proto-JSON roundtrip tests termasuk empty map dan pembacaan memakai descriptor lama, lalu menjalankan targeted/full tests dan vet; verifikasi generated diff hanya perubahan yang diharapkan dan tidak ada credential pada UserData (MAS-EXT-01).
- [x] 1.3 Dokumentasikan grant-true/absent-key semantics, safe fields dan breaking semantics Me.phone dalam dokumentasi contract yang relevan; catat exact source/generated version dan hasil test sebagai evidence dependency. Bila contract belum tersedia, tandai task backend yang memakainya BLOCKED; jangan membuat dummy struct, protobuf lokal atau mengedit module cache (MAS-EXT-01).

## 2. Safe Snapshot Readers — grpc-auth

- [x] 2.1 Tambahkan user creation timestamp pada safe identity model/reader (`internal/repositories/domain/user_identity.go`, `reader/user_identity.go`) dari `auth.created_at`; perbarui SQL fake fixtures dan targeted reader/auth-read tests untuk membuktikan context, tx handle dan timestamp sumber, tanpa query pool atau perubahan response list/detail (MAS-01).
- [x] 2.2 Tambahkan model/reader kecil untuk metadata Me roles dan application access pada handle tx existing; verifikasi targeted repository scenarios mencakup ACTIVE assignment/role/application/access, IAM exception yang relevan, role tanpa permission, app grant tanpa role, unique ID dan ordering ID ascending (MAS-02).
- [x] 2.3 Tambahkan scenario namespace subset pada reader/mapping boundary menggunakan server-owned caller authority; verifikasi new collections tidak memuat namespace di luar allowlist dan legacy ActiveRoles tetap memakai mapping numeric/name existing (MAS-02).
- [x] 2.4 Dokumentasikan safe-reader gates dan ownership snapshot pada komentar contract yang diperlukan; verifikasi tidak ada lifecycle/lock mandiri di repository, generated import pada domain, perubahan shared resolver atau duplikasi pagination/list reader (MAS-02/03).

## 3. Pengayaan Me dan Mapping Phone — grpc-auth

- [x] 3.1 Perluas `ReadUserDataSnapshot` untuk mengisi safe fields/created_at, scoped roles/apps dan `permissions[code]=true` melalui permission reader/resolver existing pada snapshot yang sama; verifikasi handler/auth-read scenarios untuk lengkap, no-admin, allowed-Me-only caller, empty union dan dedup overlap tanpa memanggil public permission RPC (MAS-01/02).
- [x] 3.2 Pertahankan recheck user/session/expiry/auth_version dari tx serta restricted branch yang melewati semua privilege readers; verifikasi stored privileges tidak bocor, caller spoofing tidak mengubah owner dan invalid principal menghasilkan catalog denial existing (MAS-01).
- [x] 3.3 Terapkan concat literal country-code + phone hanya pada mapping Me, termasuk restricted; tambahkan scenario seluruh contoh plan, NULL/empty country, empty phone dan prefix legacy, lalu verifikasi stored data serta profile/detail/list output tidak berubah (MAS-04).
- [x] 3.4 Tambahkan default SQL-fake failure scenarios untuk begin/identity/profile/legacy role/scoped role/app/permission read, commit, cleanup, cancellation/deadline dan panic; verifikasi `BeginSnapshot -> recheck -> reads -> Commit`, rollback/cleanup pada failure, response nil, cause/locale/context dan read-only Repeatable Read (MAS-03).
- [x] 3.5 Perbarui dokumentasi Me grpc-auth dan status plan untuk field lengkap, scope collection baru versus legacy metadata, permission map, restricted minimum serta phone concat; verifikasi contoh sesuai descriptor/spec dan tidak menyatakan deployment atau consumer integration telah selesai (MAS-01/02/04).

## 4. Regression Jalur Terdaftar dan Read-only Effects — grpc-auth

- [x] 4.1 Tambahkan/luaskan registered-path test Me untuk service TLS/exact method, valid/invalid/expired/revoked session, no-admin, caller namespace, safe fields dan restricted behavior; verifikasi method inventory unchanged dan caller tidak memerlukan allowlist GetMyEffectivePermissions agar Me berhasil (MAS-01/05).
- [x] 4.2 Uji mapping soft-delete, disabled/revoked sources, cross-application mismatch dan perubahan grant/revoke dengan token masih valid; verifikasi permission map mengikuti resolver existing tanpa role-code/name/JWT/browser bypass (MAS-02/05).
- [x] 4.3 Uji repeated Me/response-loss dengan incidental idempotency metadata dan unchanged user/auth versions, session/refresh, assignments/access, success-audit/outbox/idempotency result; verifikasi setiap retry membaca current state dan tidak melakukan persistent replay (MAS-05).
- [x] 4.4 Jalankan regression existing permission/evaluation, Register/Login/TesRPC, profile/detail/list, failure audit, locale/redaction dan shared limiter/RetryInfo; verifikasi perubahan Me tidak menambah credential, mengubah method policy atau mengubah quota existing (MAS-05).

## 5. PostgreSQL Snapshot dan Cleanup — grpc-auth Opt-in

- [x] 5.1 Tambahkan scenario Me pada suite `tests/integration/postgres` dengan build tag/explicit dedicated-test-DB guard existing dan synthetic owned-schema fixtures; verifikasi registered consumer dapat mengembalikan response lengkap serta concat tanpa mutation pada database test (MAS-03/04).
- [x] 5.2 Buat two-physical-connection barrier scenarios: connection B commit perubahan role/mapping soft-delete/app/access ketika connection A membaca Me; verifikasi satu snapshot konsisten, next-call visibility dengan token existing, serta user/session invalidation tetap menolak panggilan baru (MAS-02/03).
- [x] 5.3 Tambahkan real snapshot finalization/cancel/panic cleanup scenarios menggunakan pola wrapper test existing; verifikasi tidak ada partial response, koneksi kembali ke pool dan query berikutnya dapat berjalan tanpa state/version/audit/outbox berubah (MAS-03/05).
- [x] 5.4 Jalankan `rtk proxy go test -tags=postgres ./tests/integration/postgres` dan race mode terarah pada database test yang disediakan, lalu catat PostgreSQL version/backend IDs/command/waktu/expected-actual tanpa DSN atau secret; dependency tidak tersedia berarti gate BLOCKED, bukan skip/PASS. Jangan menjalankan Docker atau database/migration aplikasi (MAS-03/06).

## 6. Integration Verification dan Evidence

- [x] 6.1 Jalankan gofmt pada file Go yang berubah dan targeted test sesuai runner package reader/handler/feature/auth_reads yang disentuh; verifikasi semuanya lulus sebelum full suite dan catat hasil pada acceptance ledger (MAS-01..05).
- [x] 6.2 Jalankan `rtk go test ./...` dan `rtk go vet ./...` di grpc-auth serta api-contracts pada workspace dengan contract baru, kemudian targeted race untuk shared snapshot/collection mapping dan `make build` bila wiring/build path berubah; verifikasi versi/command/environment/expected-actual tercatat dengan jujur (MAS-06).
- [x] 6.3 Jalankan regression `rtk go test ./...` di rest-orc menggunakan workspace contract yang sama tanpa mengubah source consumer; verifikasi compile/mapping existing tetap kompatibel dan tandai runtime Me phone/consumer mapping sebagai follow-up, bukan klaim sudah terintegrasi (MAS-06).
- [x] 6.4 Lengkapi MAS-EXT-01/MAS-01..06 dengan evidence implementasi dan status aktual, validasi OpenSpec serta local links/traceability, review `git diff --check`/status dan pastikan perubahan user tetap utuh; verifikasi seluruh gate backend mandatory mempunyai hasil yang benar sebelum menyatakan backend selesai.

Publikasi contract/pembaruan pin, verification `GOWORK=off` setelah versi published tersedia (MAS-EXT-02), implementasi rest-orc/CLM dan mandatory deployment Phase 6 DEP-01 tetap dilacak sebagai follow-up di design/spec. Tidak menjadi checkbox completion backend change ini dan tidak dianggap PASS dari workspace tests. Hasil apply dan verifikasi dicatat pada evidence/progress.md.

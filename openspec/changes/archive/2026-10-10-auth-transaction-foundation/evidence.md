# Implementation evidence

## Baseline task 1.1

Inspeksi 9 Oktober 2026; hanya baseline berikut yang selesai, bukan implementasi fondasi atau AC MVP.

| Evidence | Actual |
| --- | --- |
| grpc-auth HEAD | `46dc4debb87281d8357a1c671365407ac8c374ca` |
| Graph built_at_commit | Sama dengan HEAD; graph-instruction.md tidak tersedia. Routing AST diverifikasi terhadap source; interface dispatch tidak dianggap sebagai edge lengkap handler-ke-finalizer |
| Structural hubs (degree) | domain/auth 40, domain/user 36, domain/profile 32, pkg/database/postgres 29; domain/role dan handler scenarios 28. Tidak memindahkan tanggung jawab hub ke framework baru |
| Module/runtime | github.com/siti-nabila/grpc-auth; Go directive 1.26.2; ORM v1.7.0; JWT/v5 v5.3.0; gRPC v1.82.1 |
| api-contracts checkout | HEAD `2473ca37d93f4d86bd7197adefdce91c9908e9d2`; module github.com/siti-nabila/api-contracts |
| Published pin baseline | grpc-auth go.mod `v0.0.0-20260803064555-df4684bcd60d`, berbeda dengan checkout |
| Workspace | Parent go.work memakai grpc-auth, api-contracts, error-package dan rest-orc; tidak memakai sibling ORM |
| Public methods | `/user.v1.UserService/Register`, Login, TesRPC bebas user JWT existing; `/profile.v1.ProfileService/UpdateProfile` protected. Full methods/source protobuf sesuai proposal |
| Existing changes | .gitignore modified; .agents/, .graphifyignore, docs/, openspec/ untracked; tidak ada tracked Go change saat baseline |

### Delta implementation yang diperlukan

- Register: duplicate/default-role reads masih pool; writer Begin memakai Db.Begin tanpa request context; profile/assignment memakai errgroup dan shared err; deferred finalizer berjalan sebelum recover. Jadikan feature pemanggil lifecycle/locks, bound checks/read/write satu tx, sequential writes dan success setelah commit.
- Profile: read mendahului Begin, return Update langsung membuat deferred cleanup membaca err lama; commit error tidak masuk unnamed return. Feature perlu hasil committed; handler saat ini memakai request by value dengan ID/user_id nol.
- Reader auth/profile/role belum menerima bound tx. Writer auth/profile/user_role menyimpan Tx dan mengembalikan handle lama bila masih non-nil; provider request-scoped dan pemutusan stale handle perlu diuji.
- Common finalizer commit berdasarkan err pointer dan membuang rollback error; belum aman untuk panic atau commit-then-denial.
- ORM v1.7.0 TryLock memakai QueryRowContext dan PostgreSQL pg_try_advisory_xact_lock; ReleaseLockQuery needed=false. Ini source evidence, bukan bukti PostgreSQL lifecycle.
- Constructor auth/profile membuat concrete dependencies langsung; test seams kecil diperlukan. Struktur impl.go, request context, reusable pool dan interceptor Language -> Error -> Token dipertahankan.
- tests/ memiliki runner handler/interceptor/helper/reader dan testutils.Run; belum ada suite feature transaction atau PostgreSQL. Legacy user/paginator tests dipertahankan. Tidak ada generator catalog: YAML di-embed oleh facade Go; protobuf generator hanya diperlukan jika proto berubah.

### Verification actual

`rtk proxy go test ./tests/... ./internal/features/user ./pkg/paginator` pada parent workspace lulus: enam package dengan test PASS; tujuh package scenario/helper tanpa test. Percobaan pertama sandbox gagal mengakses Go build cache, lalu rerun dengan akses cache yang disetujui lulus. Tidak menjalankan startup, server, database, migration atau Docker. Ini regression baseline, bukan F-01..F-12 PASS.

## Dependency readiness

| Gate | Status / actual |
| --- | --- |
| Task 1.2 / F-06 live schema | BLOCKED: metadata environment, constraints dan canonical-email/noncanonical/hash-length counts belum diambil; keberadaan SQL tidak membuktikan database live |
| Task 1.3 / B-01 | BLOCKED baseline: auth/user RESOURCE_BUSY belum ada; user dictionary belum ada; common.ErrServiceUnavailable -> UNIMPLEMENTED legacy. Pengguna telah mengizinkan perubahan api-contracts yang diperlukan; source/mapping/tests kini selesai sesuai rincian di bawah, publishing/pin tetap prerequisite terpisah |
| B-02 / F-12 | BLOCKED sampai test environment tersedia bagi proses verifikasi: pengguna menyatakan database test tersedia dan akan mengisi GRPC_AUTH_POSTGRES_TEST_ENABLED, GRPC_AUTH_POSTGRES_TEST_DSN, GRPC_AUTH_POSTGRES_TEST_DATABASE. Ketiganya UNSET pada pemeriksaan awal; tidak membaca env.yaml atau meminta secret di chat |
| B-03 | Conditional; belum ada proof gap ORM |
| B-04 | Mandatory follow-up security/command wiring sesuai design, bukan POST_MVP |

Task selain 1.1 tetap unchecked. Bukti database, pin published version, implementasi dan test fondasi belum tersedia.

## B-01 contract implementation setelah izin pengguna

Perubahan source api-contracts telah diizinkan eksplisit pengguna dalam sesi apply ini. Izin filesystem untuk repository di luar writable roots juga telah diberikan; tidak ada branch, commit, push, release atau dependency pin yang dijalankan.

| Contract | Actual |
| --- | --- |
| Auth contention | `auth.ErrResourceBusy`, key auth.resource_busy, app code RB, HTTP 409, gRPC ABORTED |
| User/profile contention | Dictionary user baru dan `user.ErrResourceBusy`, key user.resource_busy, app code RB, HTTP 409, `mapping.UserCodeMappings()` -> ABORTED |
| Dependency | `common.ErrDependencyUnavailable`, key common.dependency_unavailable, DU / 503 / UNAVAILABLE; mapping legacy common.ErrServiceUnavailable -> UNIMPLEMENTED unchanged |
| Cancellation | `common.ErrRequestCanceled`, RC / 499 / CANCELED; wrapped context.Canceled dan raw gRPC CANCELED memakai localized safe catalog |
| Deadline | Wrapped context.DeadlineExceeded memakai existing common.ErrDeadlineExceeded / DEADLINE_EXCEEDED |
| Safety / compatibility | en/id locale + regional/unknown-language fallback, distinct auth/user identity, metadata, wrapped error cause, detailed encoding idempotence dan generic unknown-error redaction diuji; tidak ada proto/generated/dependency changes |

File api-contracts berubah: README.md; pkg/dictionary/auth/errors.go dan errors.yaml; pkg/dictionary/common/errors.go dan errors.yaml; pkg/dictionary/user/errors.go dan errors.yaml (baru); pkg/grpcerror/encode.go; pkg/grpcerror/mapping/auth_err.go, common_err.go dan user_err.go (baru); tests/grpcerror/test_scenarios/grpcerror_scenarios.go dan transaction_errors.go (baru). YAML di-embed langsung, tidak ada catalog generator yang harus dijalankan; make proto tidak relevan karena protobuf tidak berubah.

Verification 9 Oktober 2026 pada workspace:

- `rtk proxy go test ./tests/grpcerror ./tests/dictionary` di api-contracts: PASS.
- `rtk proxy go test ./...` di api-contracts: PASS, tiga package test; package lainnya no test files.
- `rtk proxy go vet ./...` di api-contracts: PASS.
- `rtk proxy go test ./tests/... ./internal/features/user ./pkg/paginator` di grpc-auth sesudah contract berubah: PASS, enam package test. Ini consumer regression, bukan bukti new user mapping sudah terdaftar pada encoder grpc-auth atau lifecycle fondasi sudah diimplementasikan.
- Go files contract di-format dengan gofmt; diff --check api-contracts PASS. Perubahan user .DS_Store/.graphifyignore/graphify-out dipertahankan.

**Task 1.3 tetap unchecked / B-01 BLOCKED** karena versi baru belum dipublikasikan. Task 1.4 belum dijalankan: tidak menebak pseudo-version, menggunakan replace sementara, atau mengklaim workspace sebagai proof release. GOWORK=off terhadap published contract baru belum dapat dijalankan.

Pemeriksaan ulang setelah test: GRPC_AUTH_POSTGRES_TEST_ENABLED, GRPC_AUTH_POSTGRES_TEST_DSN dan GRPC_AUTH_POSTGRES_TEST_DATABASE tetap UNSET dalam proses tool; database tidak dihubungi. Task 1.2/F-06 dan required PostgreSQL gates tetap BLOCKED. Task 1.1 adalah satu-satunya task yang memenuhi seluruh acceptance task pada sesi ini (1/32).

## Task 1.2: live read-only PostgreSQL preflight

Pengguna memberikan koneksi PostgreSQL lokal secara eksplisit. Dua pemeriksaan memakai psql -X, ON_ERROR_STOP, connection/statement timeout, default_transaction_read_only=on serta BEGIN READ ONLY / ROLLBACK; tidak menjalankan migration atau setup fixture. Kredensial hanya diberikan ke child-process environment dan tidak disimpan dalam repository/report. Koneksi pertama diblokir sandbox; rerun akses localhost yang disetujui berhasil.

[Report metadata dan aggregate counts](postgres-preflight.json) mencatat waktu UTC, PostgreSQL 16.13, 70 required columns, 35 validated constraints serta 22 indexes pada public.auth/profile/role/user_role/audit_logs/outbox_events/idempotency_records. Seluruh required columns tersedia. Unique email, unique profile.user_id, unique assignment pair, numeric/string role uniqueness, composite idempotency PK, FK serta CHECK actor/version/status/hash/expiry/outbox terkonfirmasi. Audit ID memakai identity BY DEFAULT; entity IDs memakai sequence; auth PENDING/version=1/auth_version=1/must_change_password=false, role/assignment ACTIVE dan outbox attempts=0 sesuai metadata actual.

| Preflight | Actual |
| --- | --- |
| Auth rows | 17 |
| Noncanonical / canonical collision groups | 0 / 0 |
| Empty password hashes / hashes over 150 | 0 / 0 |
| Maximum stored hash length | 97 |
| Duplicate profile owners / assignment pairs | 0 / 0 |
| Active default iam.member numeric code 1 | 1 |

Actual auth.password adalah VARCHAR(150), sesuai keputusan MVP. Artifact 01_schema_backfill.sql saat inspeksi justru mendeklarasikan TEXT; perbedaan dicatat dengan source hash, tanpa mengubah SQL/mengulang migration. Constraint/type/default yang dipakai fondasi dicocokkan pada report; ini bukan bukti schema artifact pernah dijalankan persis sama.

**Task 1.2 selesai; preflight subset F-06 PASS.** F-06 normalization application tests dan seluruh correctness PostgreSQL suite tetap belum PASS. Database yang dapat dihubungi belum otomatis diotorisasi sebagai disposable test database: pertanyaan apakah database user khusus test masih menunggu jawaban. Jangan menjalankan write/DDL fixture sebelum keputusan itu tersedia.

Progres kini 2/32; task 1.3/1.4 masih BLOCKED pada published contract version dan required PostgreSQL execution tetap menunggu dedicated-test confirmation/configuration. Catatan UNSET/live metadata belum diambil pada bagian sebelumnya merupakan baseline historis; bagian ini menggantikan status live schema task 1.2 tersebut.

## Tasks 2.1–2.4: isolated opt-in PostgreSQL harness

Pengguna mengonfirmasi database `user` adalah database aplikasi, **bukan test**. Pengguna kemudian memberi izin eksplisit membuat database terpisah `grpc_auth_test`. Provisioning dilakukan di luar suite dengan CREATE DATABASE hanya bila belum ada; tidak reset/drop database existing atau mengubah database aplikasi. Database baru PostgreSQL 16.13 terverifikasi public tables=0.

Implementasi ada di [tests/integration/postgres](../../../tests/integration/postgres/README.md), seluruh Go files memakai build tag postgres. Default go list/go test tidak memilih package ini. Config mewajibkan tiga test environment variables tanpa env.yaml/PGDATABASE fallback; DSN keyword/URI harus secara eksplisit menunjuk database expected yang sama. Database aplikasi user serta postgres/template0/template1 ditolak sebelum network. Quoted/escaped values dan duplicate dbname mengikuti semantik pq (last wins); invalid/mismatched DSN menghasilkan error redacted. Setelah connect, identity diverifikasi ulang lewat transaksi READ ONLY; failure menutup pool.

Fixture DDL test-only dicocokkan dengan metadata: 70 required columns, 35 validated constraints dan 22 indexes pada tujuh tabel fondasi, plus applications sebagai FK prerequisite role. Seed sintetis memakai fixture@example.test, marker hash yang tidak dapat dipakai login, profil dan assignment member; tidak menyalin row/credential aplikasi. CREATE SCHEMA + fixture DDL/seed atomik, namespace existing tidak diadopsi. DSN startup search_path berlaku pada setiap physical/replacement connection dan tidak fallback public. Cleanup memakai owned namespace privat dan verified dedicated DB, menutup pool dan drop hanya schema miliknya; failure cleanup dipertahankan sebagai safe error.

[Actual harness report](postgres-harness.json) mencatat Go 1.26.2 darwin/arm64, ORM v1.7.0, source HEAD, PostgreSQL version, timestamp dan results. Test membuktikan pg_backend_pid berbeda pada A/B, same-key contention, independent-key progress sebelum holder selesai, lock reacquired setelah commit, replacement connection tetap di namespace, setup collision tidak merusak owner, canceled setup tidak meninggalkan namespace, serta controlled holder cancellation/acknowledged rollback tanpa sleep. Cancellation ini membuktikan kemampuan harness; belum membuktikan feature request-context BeginTx, automatic cancellation atau panic cleanup production.

Verification actual:

- Guard-only `go test -tags=postgres ./tests/integration/postgres -run '^TestPostgresConfiguration$' -count=1`: PASS, tanpa network/database.
- Default `go list ./...`: package PostgreSQL tidak terpilih. Requested `TestPostgresEnvironment` dengan test variables dihapus: FAIL sesuai harapan dengan missing opt-in, bukan SKIP/PASS.
- Opted-in `go test -tags=postgres ./tests/integration/postgres -count=1`: PASS, delapan scenarios pada configuration/environment/harness.
- `go test -race -tags=postgres ./tests/integration/postgres -count=1`: PASS.
- `go vet -tags=postgres ./tests/integration/postgres/...`: PASS.
- Default `go test ./...` dan `go vet ./...`: PASS.
- Post-run read-only inspection: remaining fixture schemas=0, public tables=0 pada grpc_auth_test. Tidak ada migration atau baseline application write.

Test DSN/credential hanya diberikan melalui environment child process; tidak disimpan dalam source, README atau report. README menyertakan opt-in/race commands tanpa secret, ownership/cleanup, prerequisites dan batas evidence.

**Tasks 2.1–2.4 selesai, progres 6/32. B-02 environment readiness dan F-12 opt-in harness PASS.** Status B-02 sebelumnya merupakan baseline historis; database aplikasi tetap read-only dan tidak dianggap dedicated test database. F-01..F-11, production feature transaction/locks, persistence repositories dan end-to-end RPC belum PASS. Task 1.3/1.4 masih membutuhkan published contract version. Pertanyaan commit/push 13 file contract ke origin/dev menunggu keputusan eksplisit sesuai AGENTS.md api-contracts.

## Tasks 1.3–1.4: published contract receipt and standalone blocker (10 Oktober 2026)

Pengguna mengizinkan branch feat/user-err, commit dan push 13 file contract ke https://github.com/siti-nabila/api-contracts.git. Commit 13eee3ea6d7ce6619999f05c52c2e72bc6a5eaf9 berhasil dipush; Go resolver tanpa workspace mengonfirmasi versi v0.0.0-20261010031542-13eee3ea6d7c. Catalog/mapping source dan version receipt tersedia.

grpc-auth go.mod/go.sum kini menunjuk versi tersebut (serta required indirect error-package v0.0.9). UserCodeMappings didaftarkan di internal/configs/server.go dan pkg/helpers/grpc.go. Scenario interceptor baru menguji wrapped auth/user busy, dependency unavailable, cancellation, deadline, legacy unavailable/duplicate dan unknown-error redaction melalui chain serta helper production untuk en/id/id-ID/fr; detailed errors tetap idempotent.

Actual verification pada parent workspace: targeted tests interceptor/helper PASS; full go test ./... PASS; go vet ./... PASS; targeted race tests PASS; make build PASS tanpa menjalankan server. git diff --check PASS. Tidak ada database/migration/protobuf changes pada langkah ini.

GOWORK=off go test ./... dan go build ./... FAIL dengan undefined: errorpackage.ParseLanguageCode pada published api-contracts/pkg/locale/locale.go:27. Fungsi tersedia pada local error-package HEAD e06b69ef855d0f4055eb7dd0fa28b36a08535ba7 tetapi tidak pada pin v0.0.9. Go resolver mengonfirmasi commit tersebut sudah tersedia sebagai v0.0.10-0.20260927130242-e06b69ef855d. Ini gap dependency existing yang sebelumnya tertutup parent workspace, bukan kegagalan mapping baru.

Perbaikan konkret disiapkan pada api-contracts/go.mod dan go.sum: pin error-package ke versi published tersebut, tanpa source/protobuf change atau edit error-package. Standalone GOWORK=off go test ./... dan go vet ./... api-contracts PASS; diff --check PASS. Dua file dependency belum di-commit/push; menunggu izin publikasi tambahan pada branch feat/user-err. Tidak memakai replace atau pin override service untuk menutupi release contract yang tidak dapat dibuild sendiri.

Task 1.3/1.4 tetap unchecked, B-01/F-05 integration release gate BLOCKED sampai dependency fix dipublikasikan, service dipin ke commit contract yang diperbaiki, dan standalone gates diulang. Progres tetap 6/32. Bagian ini menggantikan status publikasi lama pada ledger historis.

## Tasks 1.3–1.4 completed: dependency fix published (10 Oktober 2026)

Pengguna secara eksplisit mengizinkan push perbaikan api-contracts. Hanya go.mod/go.sum di-commit sebagai 42732cd6f546564fd58a457593ce4346c99ef01a dan dipush ke origin feat/user-err (https://github.com/siti-nabila/api-contracts.git). File metadata/graph user tidak di-stage; tidak membuat PR/tag/release atau mengubah source error-package.

grpc-auth kini pin api-contracts v0.0.0-20261010034410-42732cd6f546 dengan transitive error-package v0.0.10-0.20260927130242-e06b69ef855d. GOWORK=off go list -m -json mengonfirmasi kedua versi published beserta checksums; tidak memakai replace/module-cache edits.

Verification actual pada pin final:

- Parent workspace go test ./... PASS, termasuk interceptor/helper scenarios baru; make build PASS. go vet ./... serta targeted race interceptor/helper PASS pada langkah integrasi yang sama (source encoder tidak berubah sesudahnya).
- GOWORK=off go test ./... PASS.
- GOWORK=off go vet ./... PASS.
- GOWORK=off make build PASS tanpa menjalankan server.
- GOWORK=off go test -race ./tests/interceptors ./tests/pkg/helpers PASS.
- api-contracts GOWORK=off go test ./... dan go vet ./... PASS sebelum publikasi; commit hanya mencakup pin yang diuji.
- git diff --check PASS; protobuf/generated tidak berubah, database/migration tidak dijalankan. Build binary hanya artifact ignored existing bin/. Perubahan grpc-auth belum di-commit/push.

Task 1.3 dan 1.4 selesai, progres 8/32. B-01 release prerequisite PASS; F-05 encoding integration subset PASS (ABORTED auth/user, UNAVAILABLE, CANCELED, DEADLINE_EXCEEDED, en/id/regional/fallback, cause/redaction, unchanged legacy mapping). F-05 actual lock-contention boundary tetap menunggu task 5.3; tidak mengklaim seluruh F-05 atau transaction foundation selesai. Status BLOCKED pada bagian sebelumnya historis dan digantikan bukti ini. Langkah berikutnya task 3.1 transaction provider/bound repositories; task 3–8 masih pending.

## Tasks 3–8 completed: transaction and persistence foundation (10 Oktober 2026)

Bagian ini menggantikan status task 3–8 NOT_STARTED pada catatan historis, bukan
mengubah baseline source/design. [Report gates aktual](foundation-verification.json)
mencatat waktu UTC, source HEAD, working-tree implementation, SHA256 setiap Go
file yang berubah, module checksums, waktu tiap command, exit code dan expected/
actual results. grpc-auth HEAD tetap 46dc4debb87281d8357a1c671365407ac8c374ca;
implementasi service belum di-commit atau dipush.

### Published idempotency contract

Pengguna secara eksplisit menyetujui commit/push empat file tambahan ke
https://github.com/siti-nabila/api-contracts.git branch feat/user-err:
pkg/dictionary/user/errors.go, errors.yaml, pkg/grpcerror/mapping/user_err.go,
dan tests/grpcerror/test_scenarios/transaction_errors.go. Commit bfbbeb282ec7
berhasil dipush; service pin final
v0.0.0-20261010044649-bfbbeb282ec7 ter-resolve dengan GOWORK=off.
user.ErrIdempotencyConflict / user.idempotency_conflict / IC / HTTP 409 / gRPC
ABORTED mempunyai locale en/id, wrapped-cause dan encoder tests. Standalone
api-contracts go test ./... dan go vet ./... PASS sebelum commit; tidak ada
protobuf/generated change. File metadata/graph user di sibling tetap tidak
termasuk commit. Pin error-package tetap
v0.0.10-0.20260927130242-e06b69ef855d; ORM tetap v1.7.0.

### Ownership, existing commands dan locking: tasks 3.1–5.3

Production provider di pkg/database/transaction.go memakai Db.BeginTx(requestCtx),
Read Committed, reusable pool serta ORM existing dengan debug query logging off.
Begin/Commit/Rollback/TryLock caller berada di features/common atau feature;
source scan tidak menemukan lifecycle/lock call dalam internal/repositories.
Writer auth/profile/user_role dan role legacy menerima immutable supplied tx;
writer role belum mempunyai public consumer dan tidak berarti CreateRole selesai.
Legacy AuthWriterOld/ProfileWriterOld hanyalah interface tidak terpakai, tanpa
implementation/call site. Bound auth/profile/default-role readers tidak fallback
pool. Constructor seams adalah dependency assembly yang dipakai production,
bukan test-only production switch.

TransactionOwner memanggil Commit eksplisit, rollback unfinished scope pada
failure/cancellation/panic, mempertahankan operation cause bersama cleanup error,
dan menolak closed handle. Panic tidak membocorkan payload. Commit error tidak
mengembalikan credential/success dan tidak memicu retry mutation. Commit unknown
outcome tetap unknown: controlled failure sebelum real commit bukan proof bahwa
network failure pasti rollback.

Register/Login normalize trim/lowercase sebelum validation/lookup. Hash Register
sebelum begin, policy/account locks sebelum bound duplicate/default-member reads,
auth/profile/assignment writes sequential, token generation sebelum commit dan
response hanya sesudah successful commit. Hash panic, nil token, cancellation,
busy, dependency/deadline, duplicate, setiap read/write/token/commit failure serta
panic diuji dengan strict sequence/context/handle/call-count assertions. Legacy
JWT HS256/expiry/status behavior tidak diubah menjadi security target MVP.

Profile mengambil principal context, user lock, bound owner read, full replacement
name/address/phone, dan explicit commit. Payload ID tidak mengalihkan target;
handler memetakan actual committed profile ID/user ID. Missing/nil principal,
missing/mismatched owner, busy, read/write/commit/panic/cancellation paths diuji.
ListUsers/TesRPC dan existing pagination/interceptor behavior tetap lulus regression.

Canonical keys shared mengurutkan category dan numeric IDs, sorted/deduplicated
sets, account/key digest tanpa raw sensitive lock labels. Acquired=false terpisah
dari normalized DB error/deadline/cancellation. en/id/id-ID/fr boundary tests
melalui chain/helper production membuktikan ABORTED auth/user/idem vs UNAVAILABLE,
CANCELED, DEADLINE_EXCEEDED, unchanged legacy mappings, identity/cause/redaction.

Actual PostgreSQL scenarios memakai production provider/owner/repositories dan
backend A/B berbeda. Same key busy sebelum terminal outcome, different keys/users
berjalan, uncommitted rows invisible, lalu scope reacquired setelah commit,
rollback, operation error, panic dan cancellation. Pool checkout mengakui automatic
database/sql cancellation cleanup sebelum recheck; tidak memakai sleep. Discovery
predecessor fixture rollback/restart memakai fresh handle dan bounded context,
complete ordered lock set lalu state recheck. Ini proof restart protocol, bukan
last-admin command. ORM gap B-03 tidak ditemukan.

PostgreSQL Register membuktikan canonical-email exclusion, token tidak keluar
sebelum commit, next duplicate setelah commit, unique-constraint bypass protection,
zero partial auth/profile/assignment rows setelah injected required failure dan
successful fresh retry. Profile membuktikan same-user contention, different-user
independence, owner/response identity, full replacement dan rollback/retry, tanpa
kenaikan auth_version pada path profile. Preflight canonical collisions=0 dan
noncanonical=0 tetap prerequisite metadata/read-only yang sudah dicatat.

### Safe persistence dan replay: tasks 6.1–7.5

[Contract implementation](../../../docs/transaction-foundation.md) menjelaskan
verified actor precondition, bounds, typed safe DTOs, catalog envelope, pending
metadata, lock/replay/version order, fixed retention dan keyring responsibility.
AuditActor constructors memvalidasi identity saja; authentication dilakukan
consumer. Tidak memasang fake actor_service atau stores ke Register/self-profile.
Actor nil, schema overflow, unknown credential JSON fields, non-UUID session ID,
free event reason, invalid event/catalog/version dan nil tx ditolak. Audit/outbox
unit driver tests memverifikasi exact arguments, context, supplied transaction,
nullable fields, safe serialization dan preserved write causes. Catalog examples
mencakup user/role/batch/compound/session terminal revision 2. Outbox selalu pending
published_at NULL, attempts 0; tidak ada broker call atau invented event.

Idempotency scope memvalidasi actor/full method/canonical target/key dan schema
bounds. Alias application harus di-resolve consumer ke authoritative ID; scope
validator menolak alias textual. Fingerprint typed memuat normalized fields,
expected version, presence dan cloned sorted/deduplicated sets, tanpa trace/deadline
atau unique service dimension. Password-bearing future consumer memakai HMAC
versioned injected keyring, key copied defensively dan tanpa default secret/config
read. Old version dapat memverifikasi result unexpired setelah active key berubah;
missing retained key fail closed UNAVAILABLE, tepat saat expiry old key tidak
lagi diperlukan. Consumer wajib mempertahankan key untuk seluruh unexpired 24h.
Raw password/login hash/token bytes bukan request_hash format yang diterima store.

Bound store memetakan missing record ke nil, menyimpan typed safe result, melarang
unexpired overwrite, dan mengganti expired record dengan complete composite PK
plus expires_at condition dalam tx/lock yang sama. Unit SQL driver tests membuktikan
query/arguments, missing/expiry/update/unexpired/read/write error dan credential
hash rejection tanpa independent lifecycle atau pool fallback. Safe result decoder
menolak token/unknown fields serta trailing JSON. Replay identity/permission adalah
responsibility consumer sebelum lookup; stale expected-version check sesudah replay.

Actual PostgreSQL consumer fixture membuktikan:

- Mutation + audit + required event + safe result terlihat bersama sesudah commit.
- Repeated request/lost response mengembalikan saved result sebelum stale-version
  check; revoked fixture permission tetap menolak sebelum replay.
- Same scope/key different payload menghasilkan user.idempotency_conflict tanpa
  write; same key different target menjadi independent committed result.
- Concurrent claims pada backend berbeda menghasilkan busy loser tanpa writes,
  tepat satu effective mutation/audit/event/result, lalu successful replay.
- No-op baru changed=false: satu NO_OP audit, tanpa event/version increment;
  replay tidak menambah efek.
- Fixed clock sebelum expires_at masih replay; tepat 24h record diganti atomik
  dan window baru tetap fixed 24h, bukan sliding saat replay.
- Actual FK audit failure, duplicate-PK required outbox failure, duplicate-PK
  idempotency write failure dan injected post-mutation failure me-rollback semua
  state/result. Fresh retry sesudah tiap failure berhasil. Profile/access fixture
  tidak menaikkan auth_version; payload persistence tidak membawa credential keys.
- Security lifecycle fixture mengubah status SUSPENDED/auth_version, required
  audit/status event/result, commit eksplisit lalu denial. State tetap committed
  meski returned error. Required audit/outbox failure rollback status/counter/
  profile/rows, mempertahankan dependency failure dan tidak mengklaim revocation.

Fixture security itu hanya pembuktian terminal ownership; tidak mengimplementasikan
RefreshSession, token rotation, session store, JWT enforcement atau actual privileged
status command. Session terminal event di unit contract test juga bukan revocation
end-to-end. Clock/UUID nonzero disediakan consumer (secure production source saat
wiring B-04, fixed synthetic source dalam suite), bukan default secret/runtime config
baru di repository.

### Criterion ledger aktual

| Criterion | Result dan actual proof |
| --- | --- |
| F-01 | PASS foundation: strict Register scenarios + actual canonical contention/atomic rollback/token-after-commit; AC-02 PENDING tetap outstanding |
| F-02 | PASS foundation: strict profile/handler + PostgreSQL owner/identity/independence/rollback |
| F-03 | PASS: production request-context owner/bound paths + actual lifecycle/pool/cancellation/panic; unknown commit outcome tetap dibatasi |
| F-04 | PASS foundation: ORM v1.7.0 backend/barrier/lifecycle/order/restart fixture; real last-admin/refresh follow-up |
| F-05 | PASS: published auth/user/idem mappings, actual feature busy/error paths, locale/cause/redaction, unchanged legacy mappings |
| F-06 | PASS: read-only metadata/count report + canonical Register/Login validation/lookup/unique tests; SQL artifact/live type difference tetap tercatat |
| F-07 | PASS capability: typed verified audit + bounds/credential rejection + actual atomic FK failure rollback |
| F-08 | PASS persistence subset: catalog envelope user/role/batch/compound/session examples + actual pending event/atomic failure; belum delivered |
| F-09 | PASS repository subset: actual scoped replay/conflict/different target/parallel claims/exact effect counts; belum admin RPC |
| F-10 | PASS capability: fixed 24h replacement/no-op + versioned retained HMAC key tests/safe response |
| F-11 | PASS lifecycle subset: actual security fixture commit-then-denial vs required-write rollback; AC-17 real refresh outstanding |
| F-12 | PASS: explicit dedicated DB opt-in, tagged suite, default exclusion, namespace ownership and zero leftover fixture schemas/public tables |

### Final gates tasks 8.1–8.3

Final report menggunakan PostgreSQL 16.13 dan Go 1.26.2 darwin/arm64; module
versions/checksums dan implementation file hashes tercantum dalam JSON. Semua
changed Go files gofmt; targeted scenarios PASS sebelum full gates.

- Workspace go test -json ./... PASS: 146 test/subtest PASS events, zero FAIL.
- GOWORK=off go test -json ./... PASS: 146 PASS events, zero FAIL.
- Workspace dan GOWORK=off go vet ./... PASS.
- GOWORK=off relevant feature/repository/handler/interceptor race suite PASS.
- Workspace make build dan GOWORK=off make build PASS, tanpa menjalankan server.
- Tagged PostgreSQL go vet PASS.
- GOWORK=off complete opted-in PostgreSQL race suite -count=1 PASS: 43 test/subtest
  PASS events, zero FAIL; ~6.1s command wall time.
- Post-run READ ONLY inspection: grpc_auth_test remaining fixture schemas=0 dan
  public tables=0. Default go list tidak memilih PostgreSQL package.
- Changed-source supplied-password scan PASS; report tidak menyimpan DSN/credential.
- git diff --check PASS; OpenSpec strict validation PASS. User .gitignore/docs/
  planning/graph changes dipertahankan; binaries hanya ignored build artifact.

**32/32 scoped tasks selesai; foundation increment siap review.** B-01/B-02 PASS,
B-03 tidak terpicu. B-04 tetap mandatory MVP follow-up: service token opaque per
caller via TLS, verified principals, Register PENDING/admin activation, persisted
session/authoritative permission checks, JWT/refresh security cutover, management
RPC/version policy dan atomic production stores wiring saat command dibuka.
Tidak mengklaim seluruh Phase 1, AC-17/19/20/33–35, event delivery atau MVP selesai.
Tidak menjalankan migration/reset/application startup, menulis database aplikasi,
mengubah ORM/error-package/generated files, atau commit/push grpc-auth.

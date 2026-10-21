# Tasks

Implementasi dan seluruh gate selesai. Hasil tersedia pada [verification evidence](evidence/verification.md) dan [coverage mapping](evidence/coverage.md). Ikuti [design](design.md), [proposal](proposal.md) dan AGENTS.md grpc-auth. Specs sengaja skipped karena tidak ada perubahan requirement. Semua command memakai RTK; test baru mengikuti root tests/ dan testutils.Run. Checklist ditandai berdasarkan hasil verifikasi nyata.

## 1. Baseline apply dan characterization

- [x] 1.1 Rekam working-tree status/hashes, HEAD, contract pin dan inventory source/scenario dalam evidence change ini, termasuk untracked assignment C; verifikasi manifest mencakup perubahan existing tanpa reset/stash/commit atau pembacaan secret.
- [x] 1.2 Jalankan baseline default `go test ./...` pada workspace dan GOWORK=off, serta registered-server inventory test; verifikasi exact30 full methods sesuai design dan catat pre-existing/environment failure terpisah sebelum mengedit production.
- [x] 1.3 Petakan active coverage ECS-01..06, legacy-only scenarios dan baseline serialized cursor fixtures; tambahkan hanya gap characterization yang diperlukan di tests/, lalu verifikasi targeted tests lulus sebelum refactor terkait dimulai.

## 2. Tahap pertama: legacy dan dependency cleanup

- [x] 2.1 Pisahkan TesRPC dari UserHandler legacy dan sambungkan adapter kecil ke sessionUserServer; verifikasi response protobuf, service authentication, locale/error boundary dan exact30 registration tetap melalui handler/registered-server tests.
- [x] 2.2 Hapus handler/feature auth/profile token-only dan TokenInterceptor yang tidak mempunyai consumer production, beserta leaf internal eksklusifnya; verifikasi reference/removal map dan pindahkan safety coverage yang masih relevan ke jalur aktif sebelum menghapus legacy-only test/mocks/fixtures.
- [x] 2.3 Kecilkan dependency userService/NewUserService sesuai SearchUsers yang masih dipakai ReadUsersSnapshot; verifikasi list-query scenarios dan collocated test existing yang masih relevan lulus serta shared bound readers/writers dan exported pkg APIs tetap tersedia.
- [x] 2.4 Perbarui hanya dokumentasi runtime yang terdampak penghapusan legacy dan catat test-retirement map ECS-01; verifikasi local links dan dokumentasi tetap menyatakan PENDING/token-empty, EdDSA, session-aware runtime tanpa mengedit archive/main specs historis.
- [x] 2.5 Jalankan gofmt file tahap ini, targeted tests auth/profile/user/handler/interceptor/integration, kemudian `go test ./...`; verifikasi tidak ada coverage aktif hilang, public method hilang atau hunks C tertimpa sebelum tahap kedua.

## 3. Tahap kedua: management, assignment dan shared authority

- [x] 3.1 Implementasikan helper LockDiscoveredAuthority di features/common sesuai bounded discovery design; verifikasi strict scenarios stable/missing/predecessor/unstable exhaustion, busy versus dependency, cancellation, same-handle locks dan tanpa lifecycle di helper tersebut.
- [x] 3.2 Gunakan helper pada user-management, role-management dan assignment saja; verifikasi outer retry maksimal3, inner checks maksimal3, rollback sebelum restart, canonical lock order dan tidak ada mutation/replay sebelum current authority melalui feature mocks dan concurrency scenarios existing.
- [x] 3.3 Ubah AuthorizeCurrentPermission untuk mengembalikan verified principal, perbarui seluruh caller dan hilangkan duplikasi authority yang identik; verifikasi exact permission, restricted/self policy, stale actor/session dan dependency/error precedence pada feature/read/handler tests.
- [x] 3.4 Pisahkan replay/fresh execution secara lokal dan satukan commit sukses pada tiga management transaction owners; verifikasi mandatory/optional keys, password fingerprint, role error remapping, safe-result validation, replay sebelum target-version, no duplicate persistence dan commit-uncertainty cleanup.
- [x] 3.5 Sederhanakan assignmentBatch/finishAssignment dan bagian validasi/persistence panjang user/role commands tanpa menyatukan grant/revoke loops; verifikasi all-or-nothing, pair reactivation identity, unknown-role failure, absent-pair no-op, sekali version/event, self-admin dan last-admin safeguards.
- [x] 3.6 Lengkapi hanya gap tests tahap ini dan catat ECS-02/03 serta perubahan helper internal; jalankan gofmt, targeted common/user/role/catalog/assignment/read tests, full default suite dan targeted opt-in PostgreSQL management/assignment/last-admin races dengan dua koneksi/barriers; verifikasi outcome dan error/cleanup evidence sebelum tahap ketiga.

## 4. Tahap ketiga: auth, session dan profile existing

- [x] 4.1 Pecah pending Register dan session Login menjadi preparation, locked recheck dan persistence yang fokus dengan helper existing; verifikasi PENDING/default member/token-empty, canonical email, generic login failure, signing/write/commit failures serta failure audit sesudah cleanup melalui targeted auth tests.
- [x] 4.2 Pecah ChangePassword dan UpdateProfile pada current proof, mutation/invalidation serta evidence stages; verifikasi restricted allowlist, ownership, mask/full-replacement compatibility, version/auth_version, hash/proof recheck, no-op dan audit/outbox rollback pada auth/profile tests.
- [x] 4.3 Sederhanakan Refresh proof guards dan orchestration dengan rotation/reuse branches eksplisit; verifikasi issuer/algorithm/lifetimes, stale generation/restricted denial, successor-before-consume, lost response, required-write failures dan commit-before-reuse-denial pada refresh tests.
- [x] 4.4 Rapikan Logout, self/global session revoke, admin revoke dan session list berdasarkan tahap lokal tanpa menambahkan retry/discovery protocol baru; verifikasi credential-mode ownership, single no-op versus global invalidation/count rules, session event revision2, snapshot, replay dan expected-version tests.
- [x] 4.5 Lengkapi gap strict context/order/clock/panic/cleanup tests dan dokumentasikan helper/flow yang berubah pada evidence ECS-04; verifikasi test baru tetap di tests/ dan tidak ada universal command runner, changed time source atau test-only production branch.
- [x] 4.6 Jalankan gofmt, targeted auth/profile/session/handler/interceptor tests dan full default suite; jalankan targeted opt-in PostgreSQL refresh/reuse/password/profile/session concurrency regression, verifikasi committed denial dan atomic failures sebelum tahap keempat.

## 5. Tahap keempat: query, adapter dan composition

- [x] 5.1 Pecah Catalog.Normalize dan validasi user-list yang masih mencampur tanggung jawab, memakai helper lokal dan validator existing; verifikasi input cloning, defaults, presence, field/status/mode rejection, projection, multi-sort dan unchanged normalized cursor fixtures.
- [x] 5.2 Sederhanakan reader catalog/assignment/user-list query stages tanpa mengubah SQL semantics/query count; verifikasi DryRun/sqlmock arguments, literal search, distinct totals, collation, nulls-last, tie-breakers, numeric/composite cursors, target binding, offset fallback dan terminal pages.
- [x] 5.3 Rapikan mapping handler yang terdampak dan delegasikan wiring PrepareFoundation per feature mengikuti prepareLifecycle/prepareRoles; verifikasi constructor fail-fast, single primary pool, synchronized entropy, cleanup, exact30 methods dan unchanged interceptor order menggunakan configs/handler/registered-server tests dan build.
- [x] 5.4 Lengkapi hanya gap query/backward-cursor/startup regression dan perbarui dokumentasi runtime yang terdampak serta evidence ECS-05/06; verifikasi local links dan tidak ada rename massal, policy registry, generated edit, dependency/config/schema change atau business logic pada handler/composition.
- [x] 5.5 Jalankan gofmt, targeted domain/repository/catalog/user/session/handler/configs/integration tests dan full default suite; jalankan targeted opt-in PostgreSQL list/snapshot/scalar-sort tests, verifikasi safe metadata, null/tie ordering dan authority/data satu snapshot sebelum final gates.

## 6. Final integration gates dan evidence

- [x] 6.1 Jalankan `go test ./...`, `go vet ./...`, `go test -race ./...` dan `make build` pada parent workspace menggunakan writable GOCACHE bila perlu; verifikasi seluruh gate PASS tanpa menjalankan server atau memasukkan binary ke perubahan.
- [x] 6.2 Ulangi full test/vet/race/build dengan GOWORK=off pada published dependency existing; verifikasi hasil standalone PASS tanpa go.mod/go.sum atau sibling changes.
- [x] 6.3 Jalankan seluruh opt-in PostgreSQL suite normal dan race pada workspace dan GOWORK=off memakai env-test.yaml secara privat dan dedicated DB fixture existing; verifikasi no unexpected skips, dua physical connections, lock/pool cleanup, last-admin cross-command races, idempotency, refresh reuse dan atomic rollback. Gate unavailable dicatat BLOCKED, bukan PASS; tanpa Docker, application DB atau application migration.
- [x] 6.4 Review readability dan final diff terhadap recorded working-tree baseline, lalu jalankan `git diff --check` dan status; verifikasi ECS-01..06, active coverage tetap, satu discovery algorithm, dependency terpakai, direct feature lifecycle, tidak ada helper/abstraction berlebih, secret/artifact/scope leak atau kehilangan hunks C.
- [x] 6.5 Lengkapi evidence command/mode/waktu/exit/scenario/source identity dan task-retirement mapping, validasi `openspec validate auth-existing-code-simplification --strict` serta local links; verifikasi tasks hanya dicentang berdasarkan hasil nyata, main specs/archive untouched dan DEP-01 tetap terpisah tanpa klaim deployment readiness.

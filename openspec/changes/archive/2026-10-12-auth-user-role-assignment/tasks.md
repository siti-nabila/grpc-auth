# Tasks

Status implementasi C: **COMPLETE — 36/36**. Checklist ditutup berdasarkan
[evidence aktual](evidence.md); planning artifacts sendiri bukan backend PASS. Ikuti [design](design.md),
[acceptance ledger](design.md#acceptance-ledger), dan
[delta spec](specs/user-role-assignment/spec.md). Publication C dan dedicated
PostgreSQL verification adalah required gates; unavailable berarti BLOCKED.
Actual deployment tetap Phase6 DEP-01 di luar checklist backend ini.

## 1. Additive contract dan published dependency — api-contracts

- [x] 1.1 Rekam baseline 27 full methods/descriptors, existing user assignment tags1–5, generator/error/locales dan sibling dirty paths sebagai contract inventory C; verifikasi snapshot compatibility membedakan tepat tiga method baru dan mempertahankan proto/response.json milik user.
- [x] 1.2 Tambahkan ListUserRoles/GrantUserRoles/RevokeUserRoles serta dedicated filter/item/request/response sesuai wire plan design, PageQuery/PageMeta dan expected-version presence; jalankan generator resmi make proto dan targeted descriptor/presence/field-tag tests tanpa mengedit generated output manual.
- [x] 1.3 Tambahkan semantic errors target grant inactive, disabled role, business access required dan self-admin-role revoke dengan reuse existing conflicts/last-admin; verifikasi dictionary unique codes, seluruh supported locales dan gRPC INVALID_ARGUMENT/NOT_FOUND/FAILED_PRECONDITION/PERMISSION_DENIED/ABORTED mappings tanpa legacy remap.
- [x] 1.4 Dokumentasikan API C termasuk17 sortable public fields, historical source/NULL, input/default/cursor/replay dan errors di contract repo; jalankan targeted/full tests, go vet ./..., regeneration dua kali dan compatibility diff check terhadap baseline27.
- [x] 1.5 Publikasikan contract C memakai otorisasi repo/branch Git yang masih berlaku, tanpa mengikutkan perubahan user; rekam commit/pseudo-version/module sums dan buktikan standalone module resolution/download berhasil sebelum backend wiring (URA-EXT-01).
- [x] 1.6 Perbarui go.mod/go.sum grpc-auth ke published C secara eksplisit; jalankan baseline test/build workspace serta GOWORK=off dan rekam resolved module/checksums yang sama, bukan local replace sebagai bukti deliverable.

## 2. Assignment domain dan persistence atomik — grpc-auth

- [x] 2.1 Tambahkan domain item/query/mutation input dan validasi C: positive INTEGER IDs, present positive int64 version, raw1–20 roles lalu set canonical, reason3–500 dan exactly-one metadata key; targeted scenario tests memverifikasi nil/zero/overflow/duplicate/empty/21 entries/UTF-8/key errors dan normalized fingerprint payload.
- [x] 2.2 Tambahkan dedicated schema parity assertions untuk user_role pair uniqueness, FK/status/provenance/revocation checks, nullable legacy timestamps, auth.version dan admin_guard singleton; bandingkan archived B metadata dengan owned test schema dan catat mismatch tanpa menjalankan app migration atau membaca row bisnis.
- [x] 2.3 Implementasikan transaction-bound assignment reader untuk requested role/application/access/pair state dan reuse current actor/session/permission serta ManagementPolicyReader.AdminCount; query/argument/context/error tests membuktikan same-handle reads, registered absent pair dibedakan dari missing role, dan tidak ada lifecycle/lock/pool fallback.
- [x] 2.4 Implementasikan insert ACTIVE, regrant existing REVOKED dengan ID sama/clear revocation, revoke ACTIVE preserve grant metadata, serta conditional user version increment exactly-one-row; targeted writer tests cover binding, metadata, row-count conflict, FK/unique/error dan tanpa auth_version/session/app-access writes.
- [x] 2.5 Jalankan opt-in dedicated PostgreSQL persistence cases untuk unique pair, revoke/regrant identity/source, null legacy source, zero-row version conflict, raw batch rollback dan later required-write/deferred-constraint failures; verifikasi actual rows, versions, resource cleanup dan no duplicate success persistence.
- [x] 2.6 Rekam schema/domain/persistence evidence dengan command/environment aman/duration/expected-actual dan updated source references; dokumentasikan storage transition matrix dan verifikasi links serta URA-04/05/08 prerequisite status sesuai hasil nyata.

## 3. Protected historical listing dan complete query

- [x] 3.1 Implementasikan ListUserRoles safe assignment query memakai dedicated item17 fields, left-joined nullable access/source, registered role/app metadata dan unique pre-cursor total; SQL/fixture tests cover all target account states, ACTIVE/REVOKED/disabled/system roles, absent pairs, zero results dan nullable source tanpa credential disclosure.
- [x] 3.2 Implementasikan application/status OR-AND filters, literal CONTAINS/PREFIX search, normalized public registry/projection dan ASC/DESC multi-sort/nulls-last/id tie-break; targeted tests cover seluruh17 fields dua arah, duplicate/unknown/injection fields, unknown statuses/modes, wildcard literal, limit bounds dan unique total.
- [x] 3.3 Implementasikan typed opaque method/target/query-bound cursor, numeric id-only compatibility, hidden sort projection dan deterministic offset fallback; domain/repository tests cover mixed tied/null keys, page boundaries, changed target/query/method, malformed values, mandatory id/role_id dan terminal cursor.
- [x] 3.4 Implementasikan feature-owned primary read snapshot untuk current user/session/iam.user.read sebelum target disclosure, target/version/page/total/source pada satu handle; strict feature mocks verify BeginSnapshot→authority→target/data→Commit serta rollback/empty-result pada every read/cancel/panic/commit failure.
- [x] 3.5 Jalankan two-physical-backend PostgreSQL list snapshot barriers dengan perubahan authority/target-version/assignment/source/query-total secara concurrent; assert backend PID berbeda, response prior snapshot konsisten, fresh read memakai current authority, all17/mixed/null keyset checks dan pool/lock cleanup.
- [x] 3.6 Dokumentasikan ListUserRoles fields/defaults/filter/search/projection/cursor binding dan historical-vs-effective semantics; rekam actual URA-02/03 evidence/scenario mappings dan verifikasi examples serta local links sesuai implemented query.

## 4. Atomic grant/revoke, replay dan last-admin protection

- [x] 4.1 Buat narrow assignment service/dependency binding di feature user memakai existing provider/owner/common utilities; strict mocks memverifikasi Begin(requestCtx)→canonical idem/last-admin/policy/user/role/app/session locks→locked recheck, missing-predecessor rollback/restart bounded3, same-handle use dan acquired=false versus dependency failure.
- [x] 4.2 Implementasikan GrantUserRoles dengan current authority/version, seluruh role/target/app/access eligibility sebelum writes, IAM access exception, deduplicated insert/reactivate/no-op, satu version/audit/event/result outcome; targeted feature scenarios cover success, unknown/disabled/missing-access/inactive-target/mixed-invalid batch serta immutable credential/app state.
- [x] 4.3 Implementasikan RevokeUserRoles untuk semua nondeleted account states dan disabled role/app, registered missing pair no-op, unchanged revoked metadata, unknown role whole-batch rejection dan sorted actual changed IDs; targeted feature scenarios assert no absent insert, requested pairs saja, one effective version dan full no-op audit/result policy.
- [x] 4.4 Terapkan self-platform-admin revoke denial termasuk nominal no-op dan reuse shared last-admin key/count/singleton dengan ChangeUserStatus; unit scenarios verify exact errors, auth-before-target, post-lock recount, no target writes on denial dan failure count menjadi dependency error (URA-06).
- [x] 4.5 Integrasikan mandatory scoped key/fingerprint/current-auth-before-replay/replay-before-version,24h retention, conflict/different-target/method/expiry handling dan safe original response; strict scenarios verify reordered/duplicate roles canonical, stale retained replay, changed payload denial, one NO_OP audit versus zero replay audit dan no credential in persisted data.
- [x] 4.6 Lengkapi effective USER_ROLES_GRANTED/REVOKED audit dan satu pending batch event sesuai catalog/user aggregate version; failure injection pada insert/reactivate/revoke/version/audit/outbox/result/later guard/read/zero-row/cancel/panic/uncertain commit membuktikan all-business rollback, independent audit setelah cleanup, bounded audit-failure telemetry dan no broker prerequisite.
- [x] 4.7 Jalankan dedicated PostgreSQL dua-backend same-pair same/different-key contention, real commit/response-lost replay, expiry/rollback, actor authority change, role/app status race, revoke/revoke dan revoke/status last-admin races kedua arrival order; assert unique row/event counts, minimal satu eligible admin, retry semantics, PID/barrier overlap dan pool InUse0/lock reuse.
- [x] 4.8 Jalankan original-Login-JWT/primary resolver scenarios sebelum/sesudah grant/revoke termasuk overlapping permissions, business access gates dan regrant; assert byte-equal credential/auth_version/session/refresh/app-access rows, current contributing roles, lalu dokumentasikan mutation/replay/event matrix dan URA-04..09 evidence tanpa mengklaim RPC D/E atau CLM action sudah tersedia.

## 5. Handler, production registration dan shared admission

- [x] 5.1 Tambahkan gRPC handler C untuk transport mapping, input/key/version validation, safe list/query/timestamp presence dan command responses; targeted handler tests verify nil requests, overflow/presence, normalized inputs/context, dependency calls/count/errors, projection dan no sensitive fields.
- [x] 5.2 Wire assignment dependencies sekali pada existing foundation/sessionUserServer dan tambahkan exact method/session/service-namespace routing; registered integration tests verify tepat30 RPC, required TLS/service-token/current user permissions, restricted denial, actor-target separation dan existing27 methods tetap terdaftar.
- [x] 5.3 Tambahkan three full methods pada shared verified protected-read120/min burst20/admin-mutation30/min burst5 groups; deterministic interceptor/race scenarios verify old/new method exhaustion, applicable invalid input, bounded capacity/cleanup, RetryInfo dan rejected requests tanpa business effects.
- [x] 5.4 Integrasikan semantic catalog/locale mapping dan existing security-attempt audit untuk C; registered tests cover every planned error, cause/deadline/cancellation, denial before target/replay, rollback-before-independent-audit dan audit failure preserving original safe response.
- [x] 5.5 Jalankan registered production-path integration untuk list/grant/revoke success, no-op/replay/conflicts, mixed-invalid atomic batches, same-session fresh permission state dan last-admin/self denial; opt-in PostgreSQL menggunakan real wiring dan actual persisted counts, default integration memakai controlled fakes tanpa external DB/network.
- [x] 5.6 Dokumentasikan API/inventory30 dan reconciliation Phase2 C status dengan URA-01/10 serta actual evidence; verify source registration/constants/descriptor matrix konsisten, Me/GetUser/ListUsers historical defaults tidak berubah dan Phase6 DEP-01 tetap OPEN terpisah.

## 6. Deliverable regression dan completion evidence

- [x] 6.1 Jalankan gofmt hanya changed Go files, targeted suites lalu go test ./..., go vet ./..., go test -race ./... dan make build pada parent workspace; ulangi full gates GOWORK=off memakai published C, rekam actual command/duration/result dan investigasi failure sebelum PASS (URA-11).
- [x] 6.2 Jalankan full opt-in dedicated PostgreSQL suite normal/race secara berurutan pada deliverable workspace dan GOWORK=off; existing refresh/reuse/password/logout/status/role/user tests serta new C scenarios wajib benar-benar execute, dengan no skip-as-PASS dan no Docker/app DB writes.
- [x] 6.3 Rekonsiliasi every delta scenario ke test/evidence, acceptance URA-01..11/EXT-01 ke AC asli dan actual status; record source/module/generated hashes serta scope/secret/transaction-ownership/test-layout review, strict OpenSpec validation, local links/anchors, git diff --check dan status.
- [x] 6.4 Finalisasi evidence C hanya setelah seluruh required backend/contract/DB gates PASS; verify checklist count dan implementasi terpisah dari planning completion, dependency versions reproducible, perubahan user tetap utuh, D/E belum diklaim dan deployment/consumer readiness hanya Phase6 DEP-01.

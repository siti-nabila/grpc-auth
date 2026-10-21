# Design

## Context

Motivasi dan batas product: [proposal](proposal.md#why). Acuan kerja adalah
[plan](../../../../docs/auth-mutation-audit-reconciliation-plan.md) dan
[AGENTS.md root](../../../../AGENTS.md), ditambah instruksi lokal pada scope file.
User mengizinkan penyesuaian fungsi untuk reuse dengan flow bisnis lama tetap
berjalan; izin ini dibatasi pada consumer nyata dan minimum perubahan.

### Baseline inspeksi source

Inspeksi berikut read-only pada 18 Oktober 2026; tidak ada test aplikasi/DB baru
pada proposal. Ini adalah routing dan observed structure, bukan acceptance PASS.

| Area | Fakta source yang diamati | Konsekuensi design |
| --- | --- | --- |
| Graph dan freshness | Graph built_at_commit 2462cb09d0f8e7eef3b17a70456451b8bd2dd6ec; domain auth/outbox/user_role/snapshot/transaction adalah hub degree tertinggi; working tree lebih baru dan collision Go diketahui | Validasi source; graph tidak membuktikan kelengkapan relasi/caller |
| Transaction owner | [common/transaction.go](../../../../internal/features/common/transaction.go): Finish tidak commit; Commit menutup owner dan mempertahankan kemungkinan unknown outcome tanpa retry | Pertahankan explicit commit, cleanup dan original failure; test commit acknowledgement terpisah dari rollback yang terbukti |
| Management | [user transaction](../../../../internal/features/user/management_transaction.go), [role transaction](../../../../internal/features/role/transaction.go), [application transaction](../../../../internal/features/application/transaction.go) mengotorisasi current actor sebelum replay dan explicit commit | Petakan outcome/caller existing; jangan membangun command framework baru |
| Refresh | [refresh.go](../../../../internal/features/auth_feature/refresh.go) explicit Commit sebelum ErrRefreshTokenReused; [refresh_reuse.go](../../../../internal/features/auth_feature/refresh_reuse.go) menulis reuse audit dan conditional terminal audit/event | Denial transport tidak menentukan rollback security outcome |
| Safe persistence | [audit.go](../../../../internal/repositories/domain/audit.go), [outbox.go](../../../../internal/repositories/domain/outbox.go), [idempotency.go](../../../../internal/repositories/domain/idempotency.go) sudah menjadi shared domain contracts | Gunakan safe types existing; jangan membuat persistence/DTO parallel |
| Reuse existing | AcquireLocks, TransactionOwner, Replay/Fingerprinter, UserEvent/RoleEvent/SessionRevokedEvent sudah shared; managementLocks dan roleLocks membungkus busy error namespace masing-masing | Similarity baris belum membuktikan semantik identik; preserve error identity saat mengevaluasi reuse |
| Handler dan wiring | [user management handler](../../../../internal/handler/user_management.go) memisahkan key wajib/opsional dan expected-version presence; [foundation](../../../../internal/configs/foundation.go) merangkai handler serta independent attempt interceptor | Perubahan helper harus diuji sampai transport boundary; registry existing dipertahankan |
| Existing test | [role matrix](../../../../tests/features/role/test_scenarios/management.go) dan [app matrix](../../../../tests/features/application/test_scenarios/access.go) sudah menginjeksi audit/outbox/save/error/panic/cancel; [PostgreSQL runner](../../../../tests/integration/postgres/runner_test.go) memuat lifecycle/management/concurrency suites | Reuse case/fixture dan identifikasi missing assertion; jangan menganggap semua kasus belum diuji |
| Dedicated DB evidence | [commands.go](../../../../tests/integration/postgres/test_scenarios/commands.go) memiliki commitFailureTransaction yang mengembalikan error sebelum underlying commit | Case itu membuktikan simulated rollback path; bukan kehilangan acknowledgement sesudah actual commit |
| Dependency | go.mod: api-contracts v0.0.0-20261018073250-b4ad807ad5fa dan ORM v1.8.0-support-subquery; source proto sibling dibaca tanpa edit | Published standalone verification wajib; tidak ada planned contract/generator/dependency change |

Observation prioritas: membedakan assertion existing untuk failed commit sebelum
underlying commit dari unknown committed outcome; memetakan precise count/
correlation/secret exclusion per mutation; memeriksa duplicate attempt pada
Login/reuse; membuktikan semua caller tetap sama bila reuse helper dilakukan.
Belum ada klaim bug production atau coverage gap menyeluruh dari sampling ini.

## Goals / Non-Goals

**Goals:** satu matrix 21 mutation methods dengan expected/actual/test identity;
perbaikan minimum untuk reproduced gap; regression pada 37 methods/sembilan
services; evidence MAR-01..07 pada source deliverable dan dependency yang sama.

**Non-Goals:** architecture/framework baru, mode flags untuk menyatukan flow
berbeda, protobuf/schema/catalog/dependency baru, refactor luas, patch Graphify,
Phase 5 metrics overhaul, REST/UI, production retention/deployment atau delivery
broker. Inventaris dan fix tidak menyalin private runtime config ke artifact.

## Decisions

### 1. Perluas skenario pada capability existing

Delta [command-persistence](specs/command-persistence/spec.md) mempertahankan
statement dan 13 skenario existing pada lima requirement, lalu menambah 21
skenario production-conformance. Enam requirement lain dan feature specs tetap
berlaku. Alternative membuat capability audit baru ditolak karena persistence
contract sudah memiliki owner; requirement bisnis baru tidak diperlukan.

### 2. Pertahankan authority dan boundary transport existing

Semua RPC memerlukan TLS dan verified opaque x-service-token dengan exact method
serta namespace allowlist. U di matrix berarti verified bound current user
session; kecuali Me/ChangePassword/Logout, restricted actor ditolak. Register,
Login, RefreshSession dan TesRPC mengikuti credential modes yang sudah ada,
bukan bypass service authentication. Self targets berasal dari proof/principal;
admin targets tetap divalidasi terhadap current permission dan business guards.

Permission exact pada matrix; app grants/revokes memerlukan kedua permission
AND. Primary account/session/auth_version dan permission dibaca ulang pada
snapshot/tx yang ditetapkan consumer. Numeric role/name, browser metadata dan
payload actor tidak dipercaya. Negative transport tests membedakan
UNAUTHENTICATED, PERMISSION_DENIED, INVALID_ARGUMENT, business ABORTED/conflict,
UNAVAILABLE, CANCELED/DEADLINE_EXCEEDED dan redacted INTERNAL melalui registry
existing dengan cause/locale tetap. Tidak ada remap global atau false allow/deny.
Alternative mengganti policy dengan shared privileged-role shortcut ditolak.

### Mutation outcome matrix

Expected behavior berikut berasal dari matriks MVP, bukan klaim actual PASS.
Audit dan outbox pada tabel berlaku pada effective change. R/N mengikuti
[spec MVP](../../../../docs/grpc-auth-mvp-spec.md#6-kontrak-mutasi-audit-dan-transactional-outbox).
G: valid management no-op menulis satu NO_OP tanpa version/event; result disimpan
jika key dipakai. C: create/replay contract, tanpa fabricated create no-op.
T: terminal session change/replay contract. Replay selalu memerlukan authority
saat ini; key bukan izin. Profile identical replacement mempunyai NO_OP sendiri.

| Full RPC | Verified user/permission atau ownership | Counter effective change | Audit dan pending event | Key / no-op / replay |
| --- | --- | --- | --- | --- |
| `/user.v1.UserService/Register` | verified service; tanpa user JWT | user/auth initial 1 | USER_REGISTERED; PENDING, tanpa credential/session; R: iam.user.created.v1 | duplicate policy; tanpa replay |
| `/user.v1.UserService/CreateUser` | U + iam.user.create | user/auth initial 1 | USER_CREATED; R: iam.user.created.v1 | wajib; C/replay |
| `/profile.v1.ProfileService/UpdateProfile` | U + self owner; unrestricted | user +1; auth tetap | USER_PROFILE_UPDATED; R: iam.user.profile_updated.v1 | identical: NO_OP; tanpa replay |
| `/user.v1.UserService/UpdateUser` | U + iam.user.update | user +1; auth tetap | USER_PROFILE_UPDATED; R: iam.user.profile_updated.v1 | opsional; G/replay bila key |
| `/user.v1.UserService/Login` | verified service + valid login credential | user/auth tetap | SESSION_CREATED; N | session baru; tanpa replay |
| `/auth.v1.AuthService/RefreshSession` | valid refresh proof; unrestricted | rotation: tetap; terminal reuse: session revision 2 | Rotation: SESSION_REFRESH_ROTATED. Reuse: REFRESH_REUSE_DETECTED dan SESSION_REVOKED jika transisi efektif; Rotation N; reuse R: iam.session.revoked.v1 jika perubahan | strict rotation/reuse; tanpa replay |
| `/auth.v1.AuthService/Logout` | tepat U atau refresh proof; current owner | session revision 2 | SESSION_REVOKED; R: iam.session.revoked.v1 | session-state; tanpa replay |
| `/session.v1.SessionService/RevokeMySession` | U + session owner | session revision 2 | SESSION_REVOKED; R: iam.session.revoked.v1 | session-state; tanpa replay |
| `/session.v1.SessionService/RevokeUserSession` | U + iam.session.revoke | session revision 2 | SESSION_REVOKED; R: iam.session.revoked.v1 | wajib; T/replay |
| `/auth.v1.AuthService/ChangePassword` | U + current password; restricted boleh | user/auth +1; global revoke | PASSWORD_CHANGED dan USER_SESSIONS_REVOKED; R: satu iam.user.sessions_revoked.v1, reason_code PASSWORD_CHANGED | password/global invalidation; tanpa replay |
| `/session.v1.SessionService/RevokeAllMySessions` | U + self | user/auth +1; global revoke | USER_SESSIONS_REVOKED; R: satu iam.user.sessions_revoked.v1 | valid self call: global invalidation |
| `/session.v1.SessionService/RevokeAllUserSessions` | U + iam.session.revoke | user/auth +1; global revoke | USER_SESSIONS_REVOKED; R: satu iam.user.sessions_revoked.v1 | wajib; G/replay |
| `/user.v1.UserService/ChangeUserStatus` | U + iam.user.status.update | user +1; auth +1 hanya suspend/deactivate | USER_STATUS_CHANGED; R: iam.user.status_changed.v1; sessions_revoked=true saat suspend/deactivate | wajib; G/replay |
| `/role.v1.RoleService/CreateRole` | U + iam.role.create | role initial 1 | ROLE_CREATED; R: iam.role.created.v1 | wajib; C/replay |
| `/role.v1.RoleService/UpdateRole` | U + iam.role.update | role +1; user/auth tetap | ROLE_UPDATED; R: iam.role.updated.v1 | opsional; G/replay bila key |
| `/role.v1.RoleService/ChangeRoleStatus` | U + iam.role.update | role +1; user/auth tetap | ROLE_UPDATED; R: iam.role.updated.v1 | wajib; G/replay |
| `/role.v1.RoleService/ReplaceRolePermissions` | U + iam.role.permission.update | role +1; user/auth tetap | ROLE_PERMISSIONS_CHANGED; R: iam.role.permissions_changed.v1 | wajib; G/replay |
| `/user.v1.UserService/GrantUserRoles` | U + iam.user.role.grant | user +1; auth tetap | USER_ROLES_GRANTED; R: satu iam.user.roles_granted.v1 per batch | wajib; G/replay |
| `/user.v1.UserService/RevokeUserRoles` | U + iam.user.role.revoke | user +1; auth tetap | USER_ROLES_REVOKED; R: satu iam.user.roles_revoked.v1 per batch | wajib; G/replay |
| `/application.v1.ApplicationService/GrantApplicationAccess` | U + iam.user.application.grant AND iam.user.role.grant | user +1; auth tetap | APPLICATION_ACCESS_GRANTED; R: iam.user.application_access_granted.v1 | wajib; G/replay |
| `/application.v1.ApplicationService/RevokeApplicationAccess` | U + iam.user.application.revoke AND iam.user.role.revoke | user +1; auth tetap | APPLICATION_ACCESS_REVOKED; R: iam.user.application_access_revoked.v1 | wajib; G/replay |

Role/permission/app/assignment changes tidak menaikkan user auth_version atau
mencabut sesi. Password/global revoke/suspend/deactivate menginvalidasi global;
reactivate tidak memulihkan sesi lama. Single-session event aggregate adalah
session UUID/revision 2; global event memakai user version, tanpa fan-out event
per session. Compound app events tidak membuat role event tambahan.

### 3. Pertahankan satu transaction dan ordered locks

Call sites Begin/Commit/Rollback/TryLock berada di features atau features/common;
transaction provider mengimplementasikan adapter teknis di infrastructure.
Primary pool dibuat sekali; request context diteruskan. State read/write sesudah
lock memakai tx bound yang sama, tanpa fallback pool atau concurrent mutation
pada shared handle. Reader/writer tetap persistence melalui ORM.

PostgreSQL ORM TryLock memakai pg_try_advisory_xact_lock pada handle transaction;
lock dilepas saat commit/rollback, acquired=false berarti catalog RESOURCE_BUSY,
bukan dependency failure. Protocol tetap idem -> last-admin/policy -> account/
role-code -> user -> role -> application -> session; canonical sorted IDs,
discovered authority restart mengikuti implementation existing. Actor/target
recheck, uniqueness, expected entity version dan protected last-admin/system
roles tetap. Alternative memindahkan lifecycle/lock ke repository ditolak karena
mengubah ownership dan menyembunyikan failure ordering.

Test per outcome mengassert domain row/version, exact audit/event/result count,
handle/context identity, urutan call, begin/write/commit error, deadline dan panic
cleanup. Commit error selalu menahan success; rollback hanya dinyatakan terbukti
bila boundary/DB evidence mendukungnya. Untuk acknowledgement loss sesudah commit,
controlled wrapper boleh commit underlying tx lalu mengembalikan error, tanpa
retry mutation otomatis; hasil DB boleh atomik committed sementara RPC UNAVAILABLE.
Retry management memakai scoped idempotency existing; flow non-replay tidak
diberi credential cache atau janji safe replay baru. Alternative mengasumsikan
setiap commit error membuktikan rollback ditolak.

### 4. Pertahankan fingerprint, replay dan exception lifecycle

Mandatory key: CreateUser/CreateRole, ChangeUserStatus/ChangeRoleStatus,
GrantUserRoles/RevokeUserRoles, ReplaceRolePermissions,
GrantApplicationAccess/RevokeApplicationAccess,
RevokeUserSession/RevokeAllUserSessions. Optional: UpdateUser/UpdateRole.
Metadata key maksimal 200 karakter; scope actor + full method + canonical target
+ key. Canonical targets account/{email}, role-code/{code}, user/{id}, role/{id},
user/{id}/application/{id}, user/{id}/session/{uuid}, user/{id}/sessions sesuai
consumer. Alias app code/ID dinormalisasi sebelum scope.

Fingerprint mempertahankan presence, expected version dan sorted-set semantics,
mengecualikan trace/request/deadline. CreateUser memakai versioned keyed digest
terpisah dari authentication hash. Safe result 24 jam tanpa credential. Replay
sesudah current authority, sebelum target stale-version check. Different
payload/presence/version -> conflict; different target -> scope berbeda;
expired replacement tetap exclusion dan tx yang sama. No-op keyed result
changed=false dan satu NO_OP baru; replay tidak menulis ulang. Test optional-key
absent dan self-service tidak diberi general idempotency baru.

Refresh strict single-use: busy bukan reuse. Proven consumed-token reuse menulis
revocation/audit/applicable event, commit, lalu denial. Write/commit failure tetap
dependency error; tidak menjanjikan committed revocation atau rollback pasti
bila finalization unknown. Access 15 menit dibatasi session absolute tujuh hari,
issuer ApplicationName, skew 30 detik dan ignored audience tetap; EdDSA-only,
restricted policy, rotation response lost -> login ulang, tanpa grace/sliding/
legacy fallback. Alternative unify lifecycle dengan management replay ditolak
karena credential/result dan outcome semantics berbeda.

### 5. Reuse kecil setelah semua consumer dipetakan

Baca AGENTS.md root dan instruksi terdekat setiap berpindah scope. Utamakan helper
yang sudah ada; hanya adjust/extract fungsi bila consumer nyata mempunyai
semantik sama dan manfaat jelas. Catat candidate, caller list, argument/return/
error contracts, ctx/deadline, tx/lock ownership dan before/after side effects.
Namespace-specific busy/conflict mapping tetap milik consumer.

Candidate yang perlu dibandingkan terbatas pada wrapper lock/error dan mapping
safe event/snapshot yang nyata dipakai banyak consumer. Kemiripan struktur pada
user/role/app command tidak menjadi alasan menyatukan flow business dengan
boolean mode, reflection, option bags atau framework. Regression semua caller
success/failure/cleanup wajib. Alternative mempertahankan fungsi terpisah tetap
valid bila reuse menambah complexity; tidak ada quota refactor.

### 6. Safe failure audit dan outbox tetap dua outcome berbeda

Gunakan typed SafeSnapshot/EventEnvelope/SafeCommandResult existing; event hanya
catalog/allowlist dan correlation/causation/trace yang aman. Sentinel password,
hash, access/refresh token dan header harus tidak muncul di audit/outbox/result,
transport error maupun observation. Event pending bukan delivered; published_at
tidak diisi tanpa confirm. Jangan membuat event untuk version gap.

Success audit/outbox atomik dengan mutation. Independent security attempt
sesudah rollback/read cleanup mempertahankan original outcome walau recorder
error/panic, memakai verified service actor dan bounded observation. Existing
Login denial dan refresh reuse tidak diduplikasi; unverified actor tidak
manufactured. Rate rejection tidak menambah DB transaction per attempt.
Limiter quota/cap/cleanup/trust tetap mengikuti
[spec limiter](../../../../docs/grpc-auth-mvp-spec.md#8-rate-limiter-local-mvp);
protected-read shared quota 120/minute, burst20, cap10000, idle15m/cleanup1m
menjadi regression, tanpa threshold atau bucket baru. External IP sanitization
berada pada DEP-01. Alternative best-effort required success persistence atau
unbounded per-rejection audit ditolak.

### 7. Reuse suites dan evidence pada deliverable yang sama

Controlled source/test routing: tests/features/auth_feature untuk register/login/
refresh/logout/password; tests/features/profile; tests/features/user management/
assignments; tests/features/role; tests/features/application; tests/features/session;
tests/repositories, tests/handler/interceptors dan tests/integration/auth_reads.
Existing mocks ketat dan testutils dipakai; semua tambahan tetap root tests/.

Dedicated PostgreSQL memakai existing postgres-tag harness, guarded test DSN,
owned test namespace, minimal dua physical backend IDs dan explicit barriers.
Pilih actual row/audit/outbox/result assertions pada lifecycle, user/role/
assignment/application dan concurrency files yang sudah tersedia. Tambah assertion
atau case untuk gap teridentifikasi; tidak membuat duplicate harness. DB tidak
tersedia/skipped -> gate BLOCKED. Unit/mock tidak menggantikan real atomicity/
locking evidence. Tidak ada Docker, application DB atau migration baseline.

Required final gates: gofmt pada perubahan Go; targeted lalu full default test,
vet, race dan make build ke output sementara; targeted dan full opt-in PostgreSQL
normal/race. Ulangi workspace dan GOWORK=off pada published pins. Descriptor/
method count, exact permissions, docs/links dan scope/diff harus cocok. Catat
source digests/version checksums, commands/mode/time/exit/count/skip dan
expected/actual. Jika source berubah, ulangi gates yang terdampak.

## Acceptance ledger

Status baseline proposal semua MAR berikut **NOT_STARTED**; current actual results berada pada evidence/progress.md. Prerequisite source tersedia menurut
inspeksi, tetapi actual gate result change ini belum dijalankan. Semua owner
backend adalah grpc-auth; business meanings AC/M/EXT tidak diganti.

| ID / scope | Prerequisite | AC/M/EXT asal | Test dan evidence wajib | Status |
| --- | --- | --- | --- | --- |
| MAR-01 / matrix completeness | Registered inventory dan source baseline | M-05/07/11; AC-28/31 | 21-row method/outcome-to-source/test coverage, 37/nine descriptor inventory, safe source manifest dan gap ledger | NOT_STARTED |
| MAR-02 / safe persistence | Typed domain dan event catalog | M-06/08/11; AC-28, MVP envelope subset AC-30 | Domain/repository/feature sentinel assertions, safe before/after/event/result/fingerprint, exact correlation/count | NOT_STARTED |
| MAR-03 / atomicity and replay | Feature tx/locks dan current authority | M-07/08/09; AC-05/24/25/26/28/35 | Controlled each-write/finalization failure, unknown commit outcome, real DB no-op/replay/conflict/expiry/response-loss/uniqueness/version | NOT_STARTED |
| MAR-04 / lifecycle exception | Login/session/refresh/password source | M-03/04/07/11; AC-14/15/16/17/18/33/34 | Lifecycle normal/race, busy/reuse commit-before-denial, single/global scope, credential/restricted compatibility | NOT_STARTED |
| MAR-05 / compound authority | Role/app/assignment + last-admin guard | M-02/05/09; AC-08/11/12/13/19/20/21/22/23/35 | Current authority and two-backend/barrier relevant races; protected system role/last admin, exact compound count/version | NOT_STARTED |
| MAR-06 / independent attempts | Cleanup and verified-service recorder | M-01/05/07/11; AC-31 | Registered handler/interceptor denied/dependency/rate/locale, recorder error/panic, duplicate attempt and redaction assertions | NOT_STARTED |
| MAR-07 / deliverable regression | MAR-01..06 plus published dependencies | M-05/06/07/08/11; EXT-01 regression | Workspace/standalone full gates, source/contract identity, immutable safe receipts and scenario mapping | NOT_STARTED |

M-09 tidak mengklaim actual target-environment schema/bootstrap/admin. MAR-05
bukan CLM resource-policy proof. EXT-01 memakai published pin existing; EXT-03
hanya dibuka jika actual ORM gap terbukti dan diotorisasi. Phase 5 metrics/full
hardening, Phase 6 DEP-01, broker delivery/consumer dan external REST/UI tidak
menjadi PASS dari ledger ini. Mandatory unresolved gap menahan backend closure.

## Risks / Trade-offs

- [Dirty tree dan graph collision] -> preserve source baseline dan unrelated
  hunks; caller inventory diverifikasi dari source, bukan graph saja.
- [Overgeneralized helper mengubah failure ordering/catalog] -> candidate/caller
  matrix dan strict regression; preserve separate functions bila semantik berbeda.
- [Historical green suite dianggap proof baru] -> rerun required gates pada source
  deliverable; distinguish inspection/baseline/new actual results.
- [Commit failure disamakan rollback] -> simulated pre-commit dan post-commit
  acknowledgement failure dibedakan; record actual DB state dan no automatic retry.
- [Evidence berubah setelah final gates] -> source digests dan receipts terkait;
  rerun affected gates, jangan menyalin status PASS tanpa identitas source.
- [DB/environment unavailable] -> mark BLOCKED dan pertahankan mandatory gate;
  fixture/local backend bukan deployment readiness.

## Migration Plan

Tidak ada migration/schema/dependency/caller cutover yang direncanakan. Apply
nanti dimulai dari source snapshot, coverage matrix dan scoped gap ledger;
lanjut minimum fix/test, required verification, lalu ledger closure. Contract
atau business ambiguity yang baru ditemukan harus mendapat scope/keputusan
terpisah sebelum dependent edit; jangan mengubah behavior untuk menutup gap.

Rollback perubahan source bila diperlukan hanya menyentuh hunks change ini dan
menjaga unrelated working tree. Tidak memulihkan revoked sessions/credentials,
auth_version lama atau legacy JWT. Deployment, actual key/caller rotation,
consumer TLS/storage/IP/RetryInfo/PENDING/login ulang dan production readiness
tetap Phase 6 DEP-01; tidak diotorisasi oleh proposal atau kelulusan backend.

## Implementation status

Backend reconciliation COMPLETE on 18 October 2026; all23 tasks and MAR-01..07
verified on the same source snapshot and published dependencies. No production
source/business flow changed; six test files add commit acknowledgement-loss
and producer correlation coverage. See [actual evidence](evidence/progress.md).
Phase 5 broader hardening/metrics and Phase 6 DEP-01 remain separate.

# Plan auth-mutation-audit-reconciliation

Tanggal baseline: 18 Oktober 2026 (Asia/Jakarta). Status baseline: **PLANNED**.
Owner: backend `grpc-auth`. Dokumen ini adalah plan kerja Phase 4; belum ada
OpenSpec change, implementasi, atau verification baru untuk reconciliation ini.

## 1. Tujuan dan sumber keputusan

Membuktikan bahwa seluruh mutation RPC mengikuti kontrak audit, transactional
outbox, version, idempotency, dan error secara konsisten setelah increment
backend selesai. Perbaikan dibatasi pada gap yang terbukti melalui perbandingan
kontrak, source, dan test. Behavior yang sudah benar dipertahankan; refactor
kecil untuk kebutuhan reuse mengikuti batas pada bagian 2.1.

Sumber normatif:

- [AGENTS.md repository](../AGENTS.md) dan `AGENTS.md` terdekat yang berlaku
  pada file yang dikerjakan.
- [Roadmap Phase 4](grpc-auth-mvp-spec.md#10-tahapan-dependencies-dan-syarat-selesai).
- [Matriks full RPC](grpc-auth-mvp-spec.md#42-matriks-full-rpc).
- [Transaction dan lock](grpc-auth-mvp-spec.md#51-protocol-transaction-dan-lock),
  [counter](grpc-auth-mvp-spec.md#52-counter), dan
  [persistent idempotency](grpc-auth-mvp-spec.md#53-persistent-idempotency).
- [Matriks mutation/audit/outbox](grpc-auth-mvp-spec.md#6-kontrak-mutasi-audit-dan-transactional-outbox).
- [Acceptance dan evidence](grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence).
- [Event envelope](user-management-spec.md#102-envelope).

Jika source atau evidence lama berbeda dari kontrak normatif, catat perbedaannya
sebagai temuan. Jangan mengubah aturan bisnis atau kontrak untuk menyesuaikan
source tanpa keputusan eksplisit.

## 2. Baseline dan batas scope

Baseline dependency pada `go.mod`: Go 1.26.2,
`api-contracts v0.0.0-20261018073250-b4ad807ad5fa`, dan
`orm v1.8.0-support-subquery`. Evidence audit-read terakhir mencatat 37 methods
pada sembilan services; angka ini harus diverifikasi kembali saat inventory.

Evidence yang dapat dipakai untuk routing dan pemetaan test:

| Increment | Evidence historis |
| --- | --- |
| Foundation transaksi | [Transaction foundation](../openspec/changes/archive/2026-10-10-auth-transaction-foundation/evidence.md) |
| Foundation identitas dan authority | [Authorization foundation](../openspec/changes/archive/2026-10-10-auth-session-authorization-foundation/evidence.md) |
| Register/Login/profile | [Login/session producer](../openspec/changes/archive/2026-10-10-auth-login-session-producer/evidence.md) |
| Refresh/password/logout/revocation | [Session lifecycle](../openspec/changes/archive/2026-10-11-auth-session-lifecycle/evidence.md) |
| User/status | [User management](../openspec/changes/archive/2026-10-11-auth-user-management/evidence.md) |
| Role/permission | [Role management](../openspec/changes/archive/2026-10-12-auth-role-management/evidence.md) |
| Role assignment | [Assignment archive](../openspec/changes/archive/2026-10-12-auth-user-role-assignment/tasks.md) |
| Application access | [Application access](../openspec/changes/archive/2026-10-13-auth-application-access/evidence/progress.md) |
| Effective permissions | [Effective reads](../openspec/changes/archive/2026-10-14-auth-effective-permission-reads/evidence/progress.md) |
| Access decision | [EvaluateAccess](../openspec/changes/archive/2026-10-18-auth-evaluate-access/evidence/progress.md) |
| Audit history | [ListAuditLogs](../openspec/changes/archive/2026-10-18-auth-audit-log-reads/evidence/progress.md) |

Hasil PASS historis bukan PASS reconciliation pada source baru. Working tree
berisi perubahan increment sebelumnya; capture commit, working-tree digest,
module version/checksum, dan test mode sebelum mulai. Jangan reset atau commit
perubahan tersebut sebagai efek samping.

Graph AST pada inspeksi menunjuk commit
`2462cb09d0f8e7eef3b17a70456451b8bd2dd6ec`. Hub yang relevan adalah domain
outbox/audit/idempotency dan feature common. Graph hanya routing awal karena
working tree lebih baru dan terdapat collision extractor yang belum diperbaiki;
verifikasi setiap kandidat terhadap source.

Scope implementasi berikutnya adalah inventory, pemetaan evidence, pengujian gap,
dan perbaikan minimum di `grpc-auth`. Source ownership protobuf/generated/error
catalog tetap `api-contracts`; perubahan sibling, dependency, atau migration
memerlukan scope dan otorisasi terpisah ketika kebutuhan terbukti.

Phase 5 metrics/hardening menyeluruh, Phase 6 DEP-01, REST/UI, actual deployment,
production retention, relay RabbitMQ, consumer, CLM, distributed limiter, patch
Graphify, RPC baru, dan refactor luas berada di luar pekerjaan ini. Persistensi
outbox pending tetap wajib; pengiriman event belum menjadi klaim deliverable.

### 2.1 Aturan kerja dan reuse fungsi

Pada setiap tahap, baca dan ikuti `AGENTS.md` root serta instruksi lokal yang
berlaku sebelum menyentuh file. Periksa kembali ketika scope berpindah
direktori. Pertahankan perubahan user, batas layer, ownership transaksi,
konvensi test, dan required verification yang ditetapkan instruksi tersebut.

Fungsi existing boleh disesuaikan atau diekstrak untuk reuse bila ada consumer
nyata dengan semantik yang sama dalam scope reconciliation. Perubahan harus
kecil, mudah dibaca, dan memiliki manfaat konkret; gunakan owner/layer existing
yang tepat serta interface kecil sesuai kebutuhan consumer.

Refactor wajib mempertahankan flow bisnis lama pada success dan failure:
validation/authorization, input presence, return value dan public contract,
error identity/cause/locale, context/deadline, transaction/lock boundary,
urutan dan jumlah side effect, version, idempotency, serta audit/outbox.
Exception seperti refresh reuse commit-before-denial tetap berlaku.

Petakan seluruh caller sebelum perubahan, lalu buktikan behavior tetap sama
melalui existing regression dan assertion yang relevan untuk setiap consumer.
Jika helper memerlukan flag mode atau percabangan untuk menyatukan flow dengan
aturan berbeda, evaluasi kembali kebutuhan reuse agar complexity tetap rendah.
Perubahan behavior bisnis atau public contract memerlukan keputusan eksplisit
terpisah; izin refactor ini tidak memperluas scope tersebut.

## 3. Inventory mutation yang harus direkonsiliasi

Tabel ini menyalin expected behavior matriks MVP, bukan hasil audit source.
R berarti outbox wajib saat perubahan efektif; N berarti success audit tanpa
menciptakan event baru. No-op dan replay mengikuti aturan bagian 4.

| Full RPC | Success audit / outcome | Outbox pada perubahan efektif |
| --- | --- | --- |
| `/user.v1.UserService/Register` | USER_REGISTERED; PENDING, tanpa credential/session | R: iam.user.created.v1 |
| `/user.v1.UserService/CreateUser` | USER_CREATED | R: iam.user.created.v1 |
| `/profile.v1.ProfileService/UpdateProfile` | USER_PROFILE_UPDATED | R: iam.user.profile_updated.v1 |
| `/user.v1.UserService/UpdateUser` | USER_PROFILE_UPDATED | R: iam.user.profile_updated.v1 |
| `/user.v1.UserService/Login` | SESSION_CREATED | N |
| `/auth.v1.AuthService/RefreshSession` | Rotation: SESSION_REFRESH_ROTATED. Reuse: REFRESH_REUSE_DETECTED dan SESSION_REVOKED jika transisi efektif | Rotation N; reuse R: iam.session.revoked.v1 jika perubahan |
| `/auth.v1.AuthService/Logout` | SESSION_REVOKED | R: iam.session.revoked.v1 |
| `/session.v1.SessionService/RevokeMySession` | SESSION_REVOKED | R: iam.session.revoked.v1 |
| `/session.v1.SessionService/RevokeUserSession` | SESSION_REVOKED | R: iam.session.revoked.v1 |
| `/auth.v1.AuthService/ChangePassword` | PASSWORD_CHANGED dan USER_SESSIONS_REVOKED | R: satu iam.user.sessions_revoked.v1, reason_code PASSWORD_CHANGED |
| `/session.v1.SessionService/RevokeAllMySessions` | USER_SESSIONS_REVOKED | R: satu iam.user.sessions_revoked.v1 |
| `/session.v1.SessionService/RevokeAllUserSessions` | USER_SESSIONS_REVOKED | R: satu iam.user.sessions_revoked.v1 |
| `/user.v1.UserService/ChangeUserStatus` | USER_STATUS_CHANGED | R: iam.user.status_changed.v1; sessions_revoked=true saat suspend/deactivate |
| `/role.v1.RoleService/CreateRole` | ROLE_CREATED | R: iam.role.created.v1 |
| `/role.v1.RoleService/UpdateRole` | ROLE_UPDATED | R: iam.role.updated.v1 |
| `/role.v1.RoleService/ChangeRoleStatus` | ROLE_UPDATED | R: iam.role.updated.v1 |
| `/role.v1.RoleService/ReplaceRolePermissions` | ROLE_PERMISSIONS_CHANGED | R: iam.role.permissions_changed.v1 |
| `/user.v1.UserService/GrantUserRoles` | USER_ROLES_GRANTED | R: satu iam.user.roles_granted.v1 per batch |
| `/user.v1.UserService/RevokeUserRoles` | USER_ROLES_REVOKED | R: satu iam.user.roles_revoked.v1 per batch |
| `/application.v1.ApplicationService/GrantApplicationAccess` | APPLICATION_ACCESS_GRANTED | R: iam.user.application_access_granted.v1 |
| `/application.v1.ApplicationService/RevokeApplicationAccess` | APPLICATION_ACCESS_REVOKED | R: iam.user.application_access_revoked.v1 |

Inventory awal berisi 21 mutation methods; RefreshSession mempunyai dua jalur
utama. Cocokkan dengan registered server dan descriptor saat tahap A. Sisa
method, termasuk TesRPC, menjadi regression boundary; read RPC tidak boleh
mendapat success mutation/audit/event karena reconciliation ini.

## 4. Invariant dan pengecualian yang wajib dibuktikan

| Area | Expected behavior dan assertion |
| --- | --- |
| Ownership transaksi | Feature memiliki Begin/Commit/Rollback/TryLock; semua write sukses memakai handle yang sama. Guard, authority, expected version, dan lock order mengikuti kontrak existing. |
| Atomicity | Domain write, success audit, required outbox, dan applicable idempotency result commit bersama. Failure sebelum commit tidak menyisakan sebagian efek. Injection mencakup begin, setiap write, result, dan finalization; cancellation/panic harus cleanup. |
| Commit failure | Tidak mengembalikan success atau menyimpan replay seolah sukses. Bedakan failure yang membuktikan rollback dari hasil commit yang tidak diketahui; jangan menjanjikan rollback pasti untuk semua error driver. |
| Version | Effective management mutation menambah entity version sekali; auth_version berubah hanya pada invalidasi global yang disepakati. Role/permission/app changes menjaga sesi. Session terminal memakai revisi konseptual 2 tanpa kolom version baru. |
| No-op management | Valid no-op baru yang menerima key menyimpan safe changed=false dan satu NO_OP audit; tidak menaikkan version atau membuat event. Jangan menerapkan aturan ini ke login/refresh/self revoke-all secara generik. |
| Replay | Current caller/session/permission diverifikasi sebelum replay; replay mendahului target expected-version check. Scope actor + full method + canonical target + key. Payload/presence berbeda menghasilkan conflict; expired handling tetap memakai lock dan tx. Replay tidak mengulang audit/event/invalidation. |
| Key dan credential | Mandatory/optional key mengikuti daftar MVP bagian 5.3; self-service tidak diberi key wajib baru. CreateUser memakai keyed password fingerprint yang aman. Safe result tidak menyimpan password/hash/token. |
| Refresh reuse | Busy bukan reuse. Reuse melakukan revoke session/family beserta audit dan required event atomik, commit dahulu lalu UNAUTHENTICATED. Finalization gagal menghasilkan dependency error; jangan rollback hanya karena outcome RPC adalah denial. |
| Session event | Single-session transition menghasilkan satu session event; global revoke-all/password menghasilkan satu user event tanpa event per-session ganda. Terminal session/family tidak dihidupkan kembali. |
| Compound command | Grant/revoke role maksimum 20 all-or-nothing; app command memakai changed_role_ids, satu event compound, tanpa event role tambahan pada user version yang sama. |
| Envelope | Nama event, schema_version, UTC occurred_at, aggregate ID/version, verified actor, correlation/causation/trace dan typed data sesuai catalog. Version gap tidak diisi event buatan. published_at tetap kosong sebelum relay confirm. |
| Redaksi | Audit snapshot mengikuti allowlist; event data hanya field aman yang disepakati. Raw credential, auth header, password/hash, token response, private error, dan arbitrary request body tidak boleh tersimpan atau terekspos. |
| Failed attempt | Verified-service attempt independen berjalan sesudah cleanup, mempertahankan original denial/error walau recorder gagal/panic. Login denial dan refresh reuse tidak diduplikasi oleh interceptor; unverified actor tidak dipalsukan. Rate rejection tetap bounded telemetry tanpa per-request DB audit. |
| Compatibility | Preserve seluruh 37 methods, protobuf tags/types, error identity/locale, Register PENDING/token-empty, TesRPC, EdDSA-only, TTL/restricted/refresh behavior dan pagination existing. Tidak mengubah business contract untuk meluluskan test. |

Jika expected outcome atau ambiguity commit belum ditetapkan contract/test,
catat sebagai keputusan yang diperlukan sebelum mengubah behavior terkait.

## 5. Urutan pekerjaan dan deliverable

Setiap tahap bergantung pada tahap sebelumnya. Checklist ini adalah rencana,
bukan bukti implementasi dan bukan tracked task OpenSpec.

### A. Bekukan baseline dan petakan coverage

- [ ] Baca `AGENTS.md` root/terdekat untuk scope setiap tahap dan pastikan
  perubahan serta verification mengikuti instruksi yang berlaku.
- [ ] Capture source identity, dirty-tree scope, dependency workspace/published,
  inventory full method, permission/ownership boundary, dan mutation table.
- [ ] Petakan setiap baris ke handler, feature, domain, repository, existing test,
  serta archived evidence. Verifikasi source sebelum menilai gap.
- [ ] Buat ledger temuan: ID, full method/outcome, expected vs actual, path/line,
  missing assertion atau reproduced failure, dampak, dan proposed minimum fix.
- [ ] Pisahkan test yang sudah cukup dari test yang belum membuktikan invariant.
  Klaim gap production hanya setelah source/repro mendukungnya.

Hasil: matrix coverage seluruh 21 mutation methods dan regression inventory 37
methods. Belum ada PASS global hanya dari inventory.

### B. Rekonsiliasi catalog, safe data, dan error boundary

- [ ] Cocokkan audit action/target/actor/before-after/reason serta envelope/event
  count dengan tabel bagian 3 dan allowlist domain existing.
- [ ] Periksa safe result dan fingerprint; gunakan sentinel credential di test
  untuk membuktikan tidak bocor ke persistence, response error, atau observation.
- [ ] Lengkapi contract/mapping/locale assertions hanya ketika coverage kurang.
  Reuse domain audit/outbox/idempotency dan event builders yang ada.

Routing awal: `internal/repositories/domain/{audit,outbox,idempotency}.go`,
`internal/features/common/`, `pkg/types/`, dan corresponding root `tests/`.

### C. Tutup gap atomicity, replay, dan version

- [ ] Per method/outcome, assert return value, dependency arguments/context,
  exact order/count, shared transaction, dan semua side effect.
- [ ] Lengkapi failure injection setiap persistence boundary yang relevan,
  termasuk audit/outbox/result failure, commit error, cancellation, dan panic.
- [ ] Cover no-op/replay/conflict/stale/expiry, presence/set normalization,
  response lost sesudah commit, dan replay sesudah actor kehilangan authority.
- [ ] Implementasikan hanya minimum fix yang terbukti; tidak membuat generic
  command framework, global registry, atau refactor lintas fitur tanpa kebutuhan.
- [ ] Evaluasi fungsi yang dapat direuse sesuai bagian 2.1; catat consumer,
  manfaat, dan invariant yang dipertahankan. Bila dilakukan, jalankan regression
  semua caller yang terdampak, termasuk failure dan cleanup ordering.

Routing: feature auth/profile/user/role/application/session dan root test
masing-masing; repository tetap memakai ORM, tanpa lifecycle transaksi mandiri.

### D. Buktikan exception dan boundary antarlayer

- [ ] Uji refresh busy/rotation/reuse dan commit-before-denial, termasuk failure
  audit/outbox/commit; token response tidak dicache untuk replay.
- [ ] Uji password, global/single revoke, suspend/deactivate dan reactivate:
  scope invalidation, revision/version, event count dan stale credential tepat.
- [ ] Uji security attempt setelah rollback/cleanup, original error/locale,
  audit recorder failure/panic, dan duplicate-attempt prevention.
- [ ] Jalankan through registered handler/interceptor dengan controlled
  dependencies untuk caller, permission, cross-owner, restricted dan error cases.

Routing: `internal/features/auth_feature/refresh_reuse.go`,
`internal/features/common/session_event.go`, dan
`internal/interceptors/security_attempt.go`; cocokkan dengan lifecycle suites.

### E. Verifikasi PostgreSQL terkontrol

- [ ] Reuse opt-in harness `tests/integration/postgres` dan database test khusus.
  Petakan existing cases dahulu, tambah kasus hanya untuk evidence yang kurang.
- [ ] Assert actual rows, versions, audit/outbox/result counts dan safe payload
  sesudah success, rollback, no-op, replay dan response-loss retry.
- [ ] Uji parallel same/different key, conflicting payload, stale-version race,
  last-admin target berbeda dan actor privilege change yang relevan terhadap gap.
- [ ] Interleaving memakai minimal dua physical backend IDs dan barrier/channel;
  sertakan cleanup/pool reuse setelah failure/cancel/panic. Hindari time.Sleep
  sebagai satu-satunya bukti ordering.

DB suite yang tidak tersedia atau skipped menghasilkan BLOCKED pada gate DB,
bukan PASS. Tidak memakai Docker, database aplikasi, atau baseline migration.

### F. Final regression dan closure

- [ ] Jalankan required gates bagian 7 pada source deliverable yang sama.
- [ ] Rekonsiliasi setiap temuan menjadi fixed-and-verified, coverage-verified,
  atau unresolved dengan alasan/owner; gap mandatory unresolved menahan closure.
- [ ] Lengkapi acceptance ledger, source manifest, module checksums, test receipts,
  matrix-to-test mapping dan ringkasan perubahan minimum.
- [ ] Laporkan scope Phase 4 yang terbukti selesai; pertahankan Phase 5 dan
  Phase 6 DEP-01 sebagai pekerjaan terpisah.

## 6. Acceptance ledger yang akan diisi

Seluruh baris berstatus **NOT_STARTED** untuk reconciliation ini. Evidence lama
boleh dirujuk sebagai baseline, tetapi penerimaan memerlukan source identity dan
hasil verification deliverable. ID lokal berikut tidak mengganti AC/M asal.

| ID | Owner/scope dan prerequisite | Kaitan acceptance asal | Evidence yang diperlukan |
| --- | --- | --- | --- |
| MAR-01 | grpc-auth; A dan registered contract tersedia | M-05/07/11, AC-28/31 | Inventory seluruh methods, matrix mutation-to-source/test, gap ledger lengkap |
| MAR-02 | grpc-auth; B dan safe catalog existing | M-06/08/11, AC-28, bagian envelope MVP dari AC-30 | Typed snapshot/envelope/fingerprint/result dan credential-redaction assertions |
| MAR-03 | grpc-auth; C dan tx/lock foundation | M-07/08/09, AC-05/24/25/26/28/35 | Failure matrix dan controlled + PostgreSQL atomicity/replay/no-op/version/count evidence |
| MAR-04 | grpc-auth; D dan session lifecycle | M-03/04/07/11, AC-14/15/16/17/18/33/34 | Reuse commit-before-denial, invalidation scope, safe error dan exact audit/event count |
| MAR-05 | grpc-auth; C/E dan permission/last-admin guard | M-02/05/09, AC-08/11/12/13/19/20/21/22/23/35 | Relevant role/app/assignment races, current authority, system-role dan last-admin assertions |
| MAR-06 | grpc-auth; D dan independent attempt recorder | M-01/05/07/11, AC-31 | Registered negative cases; cleanup-before-attempt, no duplicate audit, original error/redaction |
| MAR-07 | grpc-auth; A–F dan published dependencies | M-05/06/07/08/11, EXT-01 regression | Workspace/standalone full gates, descriptor inventory, source manifest dan immutable receipts |

MAR-05 hanya mengklaim subset backend yang diuji; bukan proof CLM resource policy.
M-09 dibatasi database test/locking evidence. Actual schema/bootstrap/admin/key/
caller readiness environment target tetap DEP-01. EXT-01 diverifikasi melalui
published dependency existing; perubahan contract baru bukan prerequisite yang
diasumsikan. EXT-03 hanya dibuka jika source dan test membuktikan gap ORM.

## 7. Required verification gates saat implementasi

1. `gofmt` pada Go yang berubah; targeted unit/integration tests untuk gap dan
   seluruh consumer yang terpengaruh. Semua test baru di root `tests/` dengan
   runner/scenario/mock/fixture convention existing.
2. `go test ./...`, `go vet ./...`, dan `go test -race ./...` setelah targeted
   pass, memakai workspace dan `GOWORK=off` pada pinned published modules.
3. Targeted PostgreSQL normal/race untuk invariant yang direkonsiliasi, kemudian
   full opt-in PostgreSQL normal/race dalam workspace dan standalone. Harness
   wajib explicit test opt-in/DSN guard; rahasia tidak masuk command output atau
   evidence. Reuse existing setup/cleanup yang terbatas pada test namespace.
4. `make build` dengan output sementara untuk final backend deliverable di kedua
   mode; jangan menjalankan application server untuk verification.
5. Descriptor/method/permission inventory, docs link/anchor, diff/scope review,
   serta source identity seluruh receipt cocok. Jika source berubah sesudah
   gate, ulangi gate yang terpengaruh sebelum menyatakan final PASS.

Gunakan RTK untuk shell commands. Catat exact command, mode, waktu UTC, safe
environment identifier, expected/actual result, exit code, count/fail/skip,
source digest dan dependency version/checksum. DB unavailable berarti BLOCKED;
receipt historical atau suite skipped tidak menggantikannya. Phase 5 limiter
suites tetap dijalankan sebagai regression tanpa memperluas metrics scope.

## 8. Kriteria selesai dan langkah berikutnya

Plan ini selesai ketika dokumen, source references, matrix, urutan kerja dan
batas scope telah diperiksa. Penyelesaian dokumen tidak memberi status PASS
pada MAR-01..07 atau mengotorisasi implementasi/dependency/deployment otomatis.

Implementasi reconciliation dapat ditutup ketika seluruh mutation/outcome
terpetakan, semua mandatory gap terselesaikan, required gates pada deliverable
PASS, dan ledger mempunyai evidence yang dapat ditelusuri. Jika tidak ada gap
production, deliverable dapat berupa coverage/evidence tanpa perubahan business
logic; tidak perlu membuat perubahan source agar pekerjaan terlihat ada.

Sesudah persetujuan arah plan, workflow implementasi dapat dibuat terpisah.
Phase 5 metrics/hardening dan Phase 6 consumer/deployment readiness tetap
mengikuti roadmap; hasil reconciliation tidak meluluskan DEP-01.

## 9. Hasil implementasi

Backend reconciliation selesai pada 18 Oktober 2026:23/23 tasks dan MAR-01..07
PASS pada deliverable yang sama. Enam file test menambah coverage; source
production/business flow tetap. Full default 3523 dan full PostgreSQL 586 pass
pada normal/race dalam workspace dan GOWORK=off; vet/build serta inventory 37
methods/sembilan services lulus. Detail actual result dan source identity ada di
[evidence reconciliation](../openspec/changes/archive/2026-10-18-auth-mutation-audit-reconciliation/evidence/progress.md).
Phase 5 metrics/hardening, Phase 6 DEP-01 dan external consumer/deployment tetap
terpisah. Sync/archive menunggu workflow tersendiri.

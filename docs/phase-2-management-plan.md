# Plan implementasi Phase 2 — User, Role, dan Application Access

Tanggal baseline: 11 Oktober 2026. Status: **IN_PROGRESS**; increment A telah
diimplementasikan, B–E belum dimulai. Owner backend: `grpc-auth`.
Evidence A: [auth-user-management](../openspec/changes/archive/2026-10-11-auth-user-management/evidence.md).

## 1. Sumber dan batas phase

Plan umum sudah ada pada [spec MVP bagian 10](grpc-auth-mvp-spec.md#10-tahapan-dependencies-dan-syarat-selesai)
dan [rencana asal](grpc-auth-mvp-spec-plan.md). Dokumen ini memecah Phase 2
menjadi increment yang dapat di-review, diuji, dan diarsipkan sendiri.
Aturan bisnis tetap mengikuti [spec user management](user-management-spec.md)
dan [matriks RPC MVP](grpc-auth-mvp-spec.md#42-matriks-full-rpc).

Phase 2 mencakup administrasi user/status, custom business roles/permission
mapping, assignment role, application access, serta pembacaan permission efektif.
Application management MVP berarti membaca catalog dan mengelola akses user;
CRUD aplikasi/permission belum termasuk MVP. Role management menyediakan
create/edit/status dan replace permission, tanpa DeleteRole.

REST/frontend tetap repository lain. Deployment, actual schema migration,
bootstrap admin dan consumer readiness tetap gate mandatory
[Phase 6 DEP-01](grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment).
Relay RabbitMQ, CLM resource policy, distributed limiter, delegation, wildcard,
role hierarchy dan direct user permission adalah POST_MVP.

## 2. Baseline yang tersedia

| Bagian | Evidence / kondisi | Pemakaian Phase 2 |
| --- | --- | --- |
| Transaction ownership, ordered TryLock, audit/outbox/idempotency persistence | [Foundation archive](../openspec/changes/archive/2026-10-10-auth-transaction-foundation/evidence.md) | Dipakai ulang; bukan bukti management RPC sudah tersedia |
| TLS/service token, verified principal, authoritative permission dan primary snapshot | [Authorization foundation archive](../openspec/changes/archive/2026-10-10-auth-session-authorization-foundation/evidence.md) | Gate backend sebelum admin RPC diaktifkan |
| Register PENDING, session-aware Login/Me/UpdateProfile | [Login producer archive](../openspec/changes/archive/2026-10-10-auth-login-session-producer/evidence.md) | Creation/status harus kompatibel dengan alur ini |
| Refresh, Logout, ChangePassword, enam session RPC dan dynamic ListUsers | [Lifecycle archive: 41/41 task](../openspec/changes/archive/2026-10-11-auth-session-lifecycle/evidence.md) | Jangan diimplementasikan ulang; regression tetap wajib |
| Contract published baseline | `api-contracts v0.0.0-20261011113651-729758974696` dalam go.mod | Empat RPC management additive, generation/catalog tests dan publication PASS |
| UserService pada checkout contract | Register, Login, Me, ListUsers, TesRPC serta GetUser/CreateUser/UpdateUser/ChangeUserStatus | Default server19 methods; detail evidence increment A |
| SQL fixture dan migration yang disiapkan | [SQL management](sql/user-management/README.md), [session generation](sql/session-lifecycle/01_session_generation.sql) | Periksa kebutuhan command terhadap metadata read-only; jangan otomatis menjalankan migration aplikasi |

Graph `graphify-out/graph.json` dibangun pada commit
`b89f37270d019c8d0d7823a3ff5d3526874d2232`, lebih lama dari working tree.
Graph dipakai untuk routing, lalu diverifikasi terhadap source. Hub domain
auth/profile/role/audit/idempotency sudah ada; hindari persistence atau framework
transaksi baru yang menduplikasi hub tersebut. Perubahan apply sebelumnya masih
berada di working tree; pertahankan semuanya, jangan reset atau commit otomatis.

## 3. Urutan increment

Nama change B–E berikut adalah rencana, belum dibuat. Increment A **IMPLEMENTED**;
B–E **NOT_STARTED**. Setiap increment wajib mendapat contract yang
sesuai, test backend, evidence dan validasi sebelum sync/archive.

| Urutan | Change | RPC baru | Prasyarat utama |
| --- | --- | --- | --- |
| A | [auth-user-management](../openspec/changes/archive/2026-10-11-auth-user-management/proposal.md) | GetUser, CreateUser, UpdateUser, ChangeUserStatus | Foundation yang diarsipkan + publication contract A |
| B | `auth-role-management` | ListRoles, GetRole, CreateRole, UpdateRole, ChangeRoleStatus, ReplaceRolePermissions; ListPermissions, ListApplications | A, catalog existing + publication contract B |
| C | `auth-user-role-assignment` | ListUserRoles, GrantUserRoles, RevokeUserRoles | A/B; last-admin guard bersama + publication contract C |
| D | `auth-application-access` | ListUserApplicationAccess, GrantApplicationAccess, RevokeApplicationAccess | A/B/C; compound mutation + publication contract D |
| E | `auth-effective-permission-reads` | GetMyEffectivePermissions, GetUserEffectivePermissions | B/C/D untuk acceptance end-to-end; resolver existing + publication contract E |

Total: **20 RPC baru** untuk Phase 2; ListUsers existing merupakan regression.
`EvaluateAccess` dan `ListAuditLogs` tetap MVP_REQUIRED dan dijadwalkan sebagai
increment pelengkap pada Phase 4. Keduanya harus punya proposal/contract/test
tersendiri; tidak dianggap selesai hanya karena resolver/audit writer tersedia.
Phase 4 juga merekonsiliasi seluruh mutation matrix. Phase 5 melengkapi metrics
dan hardening; numeric admission untuk RPC Phase 2 diterapkan saat RPC diaktifkan.

## 4. Increment A — User dan status

| Full RPC | Permission exact | Behavior |
| --- | --- | --- |
| `/user.v1.UserService/GetUser` | `iam.user.read` | Safe detail/version, actor authority dan target data satu snapshot primary |
| `/user.v1.UserService/CreateUser` | `iam.user.create` | Admin-created PENDING/ACTIVE, Argon2id, version/auth_version=1, must_change_password=true; tanpa credential response |
| `/user.v1.UserService/UpdateUser` | `iam.user.update` | Hanya name/address/phone, presence/field_mask dan expected_user_version; email/password/status/owner/role tidak editable |
| `/user.v1.UserService/ChangeUserStatus` | `iam.user.status.update` | Transisi valid, reason 3–500, expected_user_version; suspend/deactivate mencabut seluruh credential atomik |

Transisi: PENDING → ACTIVE/DEACTIVATED; ACTIVE → SUSPENDED/DEACTIVATED;
SUSPENDED → ACTIVE/DEACTIVATED; DEACTIVATED → ACTIVE. Tidak kembali ke PENDING.
Valid same-status command mengikuti no-op policy; version tetap diperiksa.
Self suspend/deactivate dan penghilangan admin aktif terakhir ditolak.
Reactivate tidak memulihkan sesi lama atau mencabut assignment historis.

CreateUser/ChangeUserStatus wajib idempotency-key; UpdateUser opsional, tetapi
jika ada wajib mengikuti scope/replay protocol yang sama. Password fingerprint
CreateUser memakai keyed digest/version terpisah dari login/account-limiter
secret. Replay tidak menyimpan atau mengembalikan password/hash/token.

Deliverable detail: [design](../openspec/changes/archive/2026-10-11-auth-user-management/design.md),
[delta specs](../openspec/changes/archive/2026-10-11-auth-user-management/specs/user-administration/spec.md),
[status spec](../openspec/changes/archive/2026-10-11-auth-user-management/specs/user-status-management/spec.md),
dan [tasks](../openspec/changes/archive/2026-10-11-auth-user-management/tasks.md).

Pembuatan contract merupakan bagian pekerjaan increment A, dengan urutan:

1. Inventory source/descriptor/tags/errors dan additive contract design di api-contracts.
2. Tambah RPC GetUser/CreateUser, reuse existing messages dan tambah safe fields.
3. Tambah RPC/messages UpdateUser/ChangeUserStatus, presence/mask/version/status/reason.
4. Lengkapi dictionary, gRPC mapping, locales dan documentation/test scenarios.
5. Jalankan `make proto`, targeted/full tests, `go vet ./...`, regeneration dan
   compatibility/generated/diff review pada repository contract.
6. Publication pada branch existing sesuai otorisasi Git; catat commit/module/
   checksum dan verifikasi published module dapat diambil standalone.
7. Update dependency grpc-auth, verifikasi workspace/GOWORK=off, lalu backend apply.

Task contract 1.1–1.6 **PASS**; publication 729758974696 telah diverifikasi.
Pekerjaan contract termasuk scope increment A, bukan hanya menunggu pihak
eksternal. Dependency dan backend wiring sudah diterapkan; hasil ada pada
[evidence A](../openspec/changes/archive/2026-10-11-auth-user-management/evidence.md). Publication
terbatas pada contract; grpc-auth belum di-commit/push atau di-deploy.

## 5. Increment B — Role dan catalog

- ListRoles/GetRole memerlukan `iam.role.read`; GetRole menyertakan version,
  permission dan affected_user_count. Catalog role menampilkan role sesuai
  query catalog, berbeda dari opsi filter user yang hanya relevan pada user.
- CreateRole memerlukan `iam.role.create`; custom role hanya untuk aplikasi
  bisnis dengan permission terdaftar dari aplikasi yang sama.
- UpdateRole/ChangeRoleStatus memerlukan `iam.role.update`; ReplaceRolePermissions
  memerlukan `iam.role.permission.update`. Code/application immutable,
  metadata update memakai presence, mutation existing memakai expected_role_version.
- IAM system/legacy roles tidak dapat diubah metadata/status/permission melalui
  API. Role numeric existing dan string code tetap kompatibel.
- ReplaceRolePermissions menerima final set; disable/enable tidak menghapus
  assignment. Evaluation berikut memakai state primary terbaru; sesi dan
  auth_version user tetap.
- ListPermissions/ListApplications memerlukan `iam.permission.read`, read-only.
  CreateRole/ChangeRoleStatus/ReplaceRolePermissions wajib key; UpdateRole opsional.
  Initial version=1, effective role update +1 sekali; audit/event sesuai catalog.

Acceptance: AC-21/22/23, M-05/06/07/08/10/11. Catalog, collection count dan
pagination dihitung pada snapshot yang sama dengan authority. Scope sort/filter
RPC baru harus ditetapkan pada proposal B; jangan otomatis menyalin seluruh sort
ListUsers ke catalog tanpa contract yang disetujui.

## 6. Increment C — Assignment role

- ListUserRoles: `iam.user.read`, assignment/status/source aman.
- GrantUserRoles: `iam.user.role.grant`; target ACTIVE, role/application ACTIVE,
  dan access ACTIVE untuk role bisnis. Batch set maksimum 20, all-or-nothing.
- RevokeUserRoles: `iam.user.role.revoke`; boleh pada target status apa pun.
  Satu pair user/role tetap satu record; regrant mengaktifkan record yang revoked.
- Grant active/revoke revoked merupakan no-op. Effective batch menaikkan user
  version sekali dan satu event batch berisi changed_role_ids; sesi/auth_version
  tetap. Key wajib, expected_user_version wajib.
- Self revoke platform-admin assignment ditolak. Last-admin guard yang sama
  dengan ChangeUserStatus mencegah nol admin pada lintas-command concurrency.

Acceptance: AC-04..07/09/10/19/20/24..26/35; union permission dengan overlapping
roles, rollback bila satu role invalid, concurrent grant tidak duplicate.

## 7. Increment D — Application access

- ListUserApplicationAccess: `iam.user.read`, metadata/status aman.
- GrantApplicationAccess: `iam.user.application.grant` **AND**
  `iam.user.role.grant`. Target ACTIVE; pilih minimal satu role ACTIVE aplikasi
  bisnis yang sama; access dan assignment berubah atomik.
- RevokeApplicationAccess: `iam.user.application.revoke` **AND**
  `iam.user.role.revoke`; revoke access dan seluruh assignment aplikasi tersebut.
- Regrant hanya memulihkan role yang dipilih. Aplikasi lain, session dan
  auth_version tetap. Key dan expected_user_version wajib; effective command
  menaikkan user version sekali, satu compound event, tanpa role event ganda.
- IAM self-service tidak memakai application access sebagai gerbang baru.

Acceptance: AC-08/11/12/13/24..26, AND-permission negative, atomic rollback,
compound changed_role_ids, permission state berubah tanpa mencabut sesi.

## 8. Increment E — Effective permission reads

GetMyEffectivePermissions memakai verified self; GetUserEffectivePermissions
memerlukan `iam.user.read`. Keduanya mengembalikan union permission serta
contributing roles dari satu primary snapshot. Hanya assignment/role aktif
yang berkontribusi, dengan business application/access gate. Restricted caller
ditolak. Numeric role-code, role name, JWT role snapshot dan cache bukan bypass.
Acceptance: AC-09/10/13/23, M-05/06; dependency failure tetap UNAVAILABLE.

## 9. Protocol bersama dan contract dependency

Semua full RPC wajib TLS + verified method/namespace-allowed service dan active
unrestricted principal. Get/read memakai consistent primary snapshot; mutation
recheck current actor/session/exact permission setelah ordered locks.

Urutan lock: idempotency → last-admin bila relevan → policy → account/role-code
→ user IDs ascending → role IDs ascending → application IDs ascending → session
UUIDs canonical. Discovery yang menambah predecessor membutuhkan rollback dan
restart; jangan memperoleh guard lebih awal setelah aggregate lock.
Begin/TryLock/Commit/Rollback tetap call site feature/features/common. Repository
read/write setelah lock memakai satu tx, tidak fallback ke pool atau concurrent
writes pada handle yang sama. Shared database pool tetap dibuat sekali.

Semua error precommit, termasuk guard/read/conditional row count/cancellation/
panic setelah write, membatalkan seluruh transaksi bisnis. Failure audit melalui
tx terpisah setelah cleanup, safe reason/correlation; audit failure mempertahankan
error asli dan telemetry bounded. Commit uncertainty bukan claimed rollback.
Refresh reuse tetap exception commit revocation sebelum denial milik lifecycle.

Outbox sesuai [catalog/matriks MVP](grpc-auth-mvp-spec.md#6-kontrak-mutasi-audit-dan-transactional-outbox).
Effective change: mutation + version + audit + required pending event + safe
result atomik. No-op: satu NO_OP audit dan safe result jika key diterima; tanpa
event/version increment. Replay: tanpa audit/event/effect tambahan, current
authority sebelum replay dan replay sebelum target expected-version check.
Retention 24 jam; actor + full method + canonical target + key. Key maks 200.

Owner contract `api-contracts` menyiapkan RPC/messages/additive fields, presence,
safe responses, dictionary/mapping/locales dan generated output. Existing
CreateUserResponse/GetUserResponse memakai `User` yang punya password field;
field itu wajib kosong pada management response, tidak boleh meng-echo request.
Jangan menghapus/renumber field lama. Untuk increment A, contract empat RPC,
generator/test/docs, publication dan dependency update sudah masuk task plan;
implementasi dimulai saat apply, dengan commit/push sesuai otorisasi Git yang
berlaku. Scope tidak melebar ke contract role/application atau code runtime
sibling. Workspace sukses belum cukup: GOWORK=off harus memakai versi published.

## 10. Filter user dan batas REST

Dynamic sort/cursor/filter/search/unique total ListUsers sudah selesai pada
increment lifecycle dan dipertahankan. Role pilihan OR, lintas kategori/search
AND; collection response tidak dipotong menjadi hanya role yang cocok filter.

Kebutuhan awal opsi filter role yang hanya menampilkan role milik user serta
pencarian opsi seperti IAM Editor **belum memiliki filter-options RPC**. Jangan
menyamakan ListRoles catalog atau role OR pada ListUsers dengan endpoint opsi
tersebut. Ini tetap tracked untuk proposal integrasi REST/user filter-options
berikutnya; perlu scope/query contract tersendiri bila backend harus menyediakan
opsi berdasarkan seluruh matching users. REST tidak boleh menyimpulkan opsi
global hanya dari satu page ListUsers. Pembuatan plan ini tidak menyelesaikan
fitur filter-options atau integrasi rest-orc.

## 11. Verification dan completion

| Gate | Syarat dan bukti | Owner |
| --- | --- | --- |
| Contract | Inventory exact methods/fields/errors, generator/test/static checks, published commit/module dan standalone resolution | api-contracts + grpc-auth |
| Behavior | Success/validation/permission/restricted/system-role/ownership/no-op/replay/version/dependency tests melalui registered gRPC paths | grpc-auth |
| Atomicity | Failure injection pada tiap write/guard/read, independent audit, cancellation/panic/commit uncertainty | grpc-auth |
| PostgreSQL | Dedicated opt-in test DB, dua physical connections/barriers; last-admin lintas status/revoke, uniqueness, response-loss/replay/expiry, pool cleanup | grpc-auth |
| Regression | gofmt, targeted/default/race/vet/make build, workspace dan GOWORK=off; existing 15 RPCs dan dynamic ListUsers dipertahankan | grpc-auth |
| Documentation | Requirement/scenario traceability, actual evidence per increment, strict OpenSpec validation, diff/status/link checks | grpc-auth |
| Deployment | Actual schema/admin/key/caller/consumer readiness dan rollback/cutover | Phase 6 DEP-01 |

Semua test baru di root tests/ dengan runner/test_scenarios/mocks/fixtures dan
testutils existing. Default suite tanpa real service/DB; opt-in PostgreSQL
memakai dedicated test DB eksplisit, tanpa Docker atau database aplikasi.
Evidence mencatat commit atau working-tree digest, published dependency,
command, environment aman, waktu, expected/actual dan status per AC.

Phase 2 selesai setelah A–E implemented dan seluruh gate backend terkait PASS;
proposal/contract publication sendiri bukan completion. Setelah tiap increment
selesai: validate → sync/archive → increment berikutnya. Completion Phase 2
tidak meluluskan otomatis Phase 4/5/6 atau keseluruhan MVP.

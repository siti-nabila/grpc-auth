# Design

## Context

Motivasi dan scope ada pada [proposal](proposal.md); behavior normative ada pada
[delta user-role-assignment](specs/user-role-assignment/spec.md). Planning dibuat
12 Oktober 2026. Baseline grpc-auth `2462cb0` bersih, 27 RPC, Go 1.26.2,
api-contracts `v0.0.0-20261012030550-fd1147049d52`, ORM v1.7.0 dan PostgreSQL.
Tidak ada test aplikasi/DB baru yang dijalankan saat propose; semua criterion C
masih NOT_STARTED, termasuk contract yang belum dipublikasikan.

### Discovery dan structural routing

Graph `graphify-out/graph.json` built_at_commit `bf6be877296b8f4191f2e2db5f744b23ab636ed7`
lebih lama dari HEAD. Degree hub tertinggi yang relevan: domain/auth.go236,
idempotency.go228, outbox.go222, user_role.go217, transaction.go214,
snapshot.go213. Endpoint source/target dipakai untuk routing, bukan alasan
menduplikasi hub. Link AST EXTRACTED mengarahkan configs/handler ke feature user,
domain dan features/common; link INFERRED divalidasi dengan source.

| Area source yang diperiksa | Fakta baseline dan konsekuensi C |
| --- | --- |
| internal/configs/foundation.go dan register.service.go | sessionUserServer merangkai GetUser dan tiga user mutations; tambahkan adapter assignment pada server yang sama, jangan menggandakan UserService registration |
| internal/features/user/management_service.go, management_transaction.go, change_status.go | Feature-owned provider/bind/owner, ordered discovery/restart, auth recheck, replay-before-version, audit/outbox/result tersedia; assignment memakai pola ini dengan dependency kecil sendiri |
| internal/repositories/reader/management_policy.go | AdminCount memverifikasi singleton lalu menghitung distinct nondeleted ACTIVE users dengan ACTIVE iam.platform_admin pair; global lock dipanggil oleh feature |
| internal/repositories/domain/user_role.go dan reader/user_role.reader.go | Legacy shape hanya IDs; reader memakai pool dan tanpa assignment status/provenance; tidak cukup untuk protected snapshot listing C |
| internal/repositories/writer/user_role.writer.go | Transaction-bound Create existing, tetapi tidak memiliki same-pair revoke/regrant/conditional mutation; preserve consumer legacy dan tambah writer khusus assignment |
| internal/repositories/domain/idempotency.go, snapshot.go, outbox.go | SafeCommandResult sudah mempunyai ChangedRoleIDs; safe snapshots dan event catalog mendukung batch user roles; reuse tanpa credential cache |
| internal/repositories/reader/authorization.go dan features/common/authorization.go | Resolver sudah memakai current assignment/role/app/access dan union sources; C membuktikan perubahan melalui resolver serta RPC existing tanpa membuat RPC E |
| internal/interceptors/session_interceptor.go, rate_interceptor.go, security_attempt.go | Explicit method routing dan shared admission perlu tiga method tambahan; independent failure audit existing dipakai setelah cleanup |
| tests/features/user, tests/integration/auth_reads, tests/integration/postgres | Runner/scenario/mocks/fixtures serta two-connection last-admin/outbox barriers tersedia; extend dengan kasus C, jangan membuat framework test baru |

Contract sibling diamati read-only: UserService belum memiliki tiga RPC C;
UserRoleAssignment existing hanya role_id/code/name/status/role_status (tags1–5).
Role/application dictionaries belum memiliki ROLE_DISABLED dan
APPLICATION_ACCESS_REQUIRED; user dictionary belum punya dedicated self-admin
assignment revoke error. Existing version/idempotency/busy/last-admin catalog
tersedia. Sibling feat/role-management memuat untracked proto/response.json;
pertahankan file user tersebut, jangan ikut generate/stage/commit sebagai C.

Archived B [schema comparison](../2026-10-12-auth-role-management/schema-comparison.json)
menyimpan baseline metadata application; fixture SQL mempunyai user_role unique
pair, status/provenance/revocation checks dan admin_guard. Ini bukan bukti schema
C saat apply atau izin migration. Dokumentasi SQL lama memuat istilah password
TEXT/admin_guard FOR UPDATE; keputusan normative VARCHAR(150) dan shared ORM
TryLock tetap berlaku. Jangan menyalin stale README menjadi kebutuhan migration C.

## Goals / Non-Goals

**Goals:** Tiga RPC aman dengan query lengkap, batch all-or-nothing, same-pair
lifecycle, safe replay, shared last-admin invariant dan bukti PostgreSQL nyata.
Flow handler → feature user → domain → reader/writer tetap sederhana dan ctx
request diteruskan; reusable pool/env.yaml/ORM/locale boundaries tetap dipakai.

**Non-Goals:** Command D/E, resource policy CLM, REST/UI/filter-options, migration
aplikasi otomatis, bootstrap, perubahan JWT/session, relay dan framework baru.
Tidak ada abstraction command generik atau refactor management A/B tanpa kebutuhan C.

## Decisions

### 1. Contract additive dan public query yang diputuskan

Tiga method berada pada user.v1.UserService; exact permissions mengikuti delta.
Contract sources tetap api-contracts/proto/user/v1; generator sibling satu-satunya
pembuat pb. Pilihan dedicated list item menghindari perubahan arti default
collections existing; mengubah UserRoleAssignment global akan memperluas semua
response Me/GetUser/ListUsers tanpa kebutuhan.

Wire plan semua message baru; tags existing tidak disentuh:

| New message | Fields dan tag |
| --- | --- |
| ListUserRolesRequest | uint64 user_id=1; PageQuery query=2; UserRoleAssignmentFilter filter=3 |
| UserRoleAssignmentFilter | string application_code=1; repeated string statuses=2 (assignment ACTIVE/REVOKED) |
| ListUserRolesResponse | repeated UserRoleAssignmentItem items=1; PageMeta meta=2; string next_cursor=3; uint64 user_id=4; uint64 version=5 |
| UserRoleAssignmentItem | uint64 id=1, role_id=2; string code=3; int32 role_code=4; string name=5, status=6, role_status=7; uint64 application_id=8; string application_code=9, application_status=10; bool is_system=11; optional string application_access_status=12; optional uint64 granted_by=13; Timestamp granted_at=14; optional uint64 revoked_by=15; Timestamp revoked_at=16; optional string revoke_reason=17 |
| GrantUserRolesRequest / RevokeUserRolesRequest | uint64 user_id=1; repeated uint64 role_ids=2; string reason=3; optional uint64 expected_user_version=4; key hanya metadata |
| GrantUserRolesResponse / RevokeUserRolesResponse | uint64 user_id=1; repeated uint64 changed_role_ids=2; uint64 version=3; bool changed=4 |

Timestamp memakai google.protobuf.Timestamp; legacy NULL tetap absent, output UTC.
ID input harus 1..MaxInt32 dan expected version 1..MaxInt64. Nil/zero/overflow
version berbeda dari present valid. Raw role_ids dibatasi 1–20 entries sebelum
set normalization; duplicates valid kemudian sorted/deduplicated. Alasan di-trim,
3–500 Unicode characters, valid UTF-8 tanpa NUL. Exactly one key metadata valid,
nonblank maksimal200; key mengikuti byte/string-preservation helper existing,
bukan diam-diam di-trim untuk scope. Unknown status strings ditolak.

Keputusan user: listing lengkap seperti B, default dua status assignment; revoke
registered missing pair no-op. Technical defaults: item id adalah primary key
user_role, sehingga ID tie-break/numeric last_id konsisten pola B. Target/version
response tetap ada walau projection sempit. Default search code/name; explicit
fields code/name/application_code, CONTAINS/PREFIX, OR antarfields, keyword trim
maks200. Filter categories AND, statuses OR; absent filter semua status, unknown
application_code memberi zero matches pada target yang ada. Tidak ada filter
role_status/application_status tambahan dalam increment ini.

Seluruh17 item scalar fields terdaftar untuk sort/projection, termasuk nullable
source IDs/timestamps/reason dan application_access_status. Strings mengikuti
collation database existing; tidak menjanjikan ordering locale baru. ASC/DESC,
nulls-last kedua arah, duplicate/unknown sort invalid, default id ASC, append id
ASC ketika belum dipilih. Projection selalu id/role_id; internal selected sort
values tidak ikut keluar kecuali diproyeksikan. Expression SQL tidak berasal dari
request. Cursor opaque versioned berisi method, target user_id, query digest dan
typed/null values; digest mengikat normalized filter/search/sort, bukan projection,
limit/page. Query berubah mulai cursor kosong. Numeric last_id hanya id-only.
Total unik sebelum keyset/limit; offset hanya page>1 tanpa cursor, empty total_pages0.
Reuse pola catalog B dan helper yang memang cocok; jangan menganggap Catalog enum
existing sudah mampu merepresentasikan target-bound assignment cursor.

### 2. Authentication, snapshot dan wiring

TLS dan verified opaque x-service-token method/namespace allowlist tetap berada
pada boundary; authorization Bearer user JWT terpisah. List re-authenticates
session/user dan exact iam.user.read dalam primary repeatable-read snapshot yang
sama dengan target/version/page/total/source reads. Target nondeleted semua status
boleh dibaca. Mutation memanggil current recheck dalam tx sesudah lock. Browser
user/actor/service IDs dan numeric role codes tidak menjadi authority. Restricted
allowlist tetap hanya Me minimum, ChangePassword dan Logout.

Tambahkan feature assignment kecil dalam internal/features/user, domain DTO dan
narrow reader/writer contracts; bind dependency read/write ke handle yang diinject.
Handler hanya mapping, validation, metadata/presence dan safe response. Foundation
membuat dependency sekali memakai pool/provider existing, dan sessionUserServer
mendelegasikan tiga method ke handler. Tambahkan exact method routing session,
service policy, localized mappings, protected-read120/min burst20 dan
admin-mutation30/min burst5 pada buckets existing dengan cap/cleanup/RetryInfo
existing. Applicable invalid requests tetap consume; denial tidak menulis bisnis.
Security attempt path tetap bounded, verified service actor; jangan menambah
independent failure audit kedua di feature.

### 3. Transaction lifecycle, lock set dan shared last-admin

Provider Begin(requestCtx), BeginSnapshot, owner Commit/Rollback/TryLock call sites
hanya feature/features/common. Reader/writer hanya menerima tx/query handle;
tidak lifecycle/lock/pool fallback, external call, atau concurrent writes pada tx.
ORM1.7.0 PostgreSQL pg_try_advisory_xact_lock retained hingga commit/rollback;
false → user RESOURCE_BUSY, error → dependency mapping. Preserve cause.

Gunakan assignment command wrapper khusus dengan dua operation kecil dan shared
existing locking/replay/owner utilities. Jangan memperbesar ManagementRepositories
A menjadi dependency bag untuk command yang tidak memakai account/password/session
revocation. ManagementService A tetap kompatibel. Urutan lock:

1. scope idempotency digest actor/method/user-target/key;
2. auth:guard:last-admin:global pada seluruh revoke (conservative sebelum role
   discovery), lalu auth:guard:policy:global pada grant/revoke;
3. canonical ascending unique actor/target user IDs;
4. requested roles dan actor contributing roles, numerik ascending;
5. corresponding applications actor/target, numerik ascending;
6. actor session UUID canonical; tidak mengambil/mencabut seluruh target sessions.

Discovery boleh membaca IDs tetapi authoritative values selalu dibaca ulang di
bawah lock pada handle tx yang sama. Missing predecessor menyebabkan rollback dan
restart dengan expanded canonical set, mengikuti bounded3 attempts baseline lalu
busy; tidak acquire earlier guard setelah aggregate lock. Tidak ada business write
sebelum set stabil dan authority/replay/version/semua batch guard selesai.

Reuse ManagementPolicyReader.AdminCount tanpa menggandakan query/guard. Invariant
menghitung distinct nondeleted ACTIVE users dan ACTIVE iam.platform_admin pair,
bukan permission-union count atau jumlah row join. Verifikasi singleton id1;
missing/failing count menjadi dependency error. Revoke self platform admin ditolak
juga jika nominal no-op. Last-admin check hanya saat effective removal mengurangi
eligible count; revoked/absent pairs pada inactive target tidak menghilangkan admin.
Shared key sama dengan ChangeUserStatus memastikan lintas target/status serial.
Global policy guard mengoordinasikan grant dengan role/app/actor privilege writes.
Conservative all-revoke guard mengurangi concurrency tetapi menjaga correctness
MVP; finer-grained acquisition memerlukan keputusan dan evidence baru.

### 4. Mutation lifecycle dan atomic results

Setelah current actor/session/permission: lookup/replay result; target existence dan
expected version; seluruh batch existence/eligibility/self/last-admin guards; hitung
changed IDs; write changed rows; conditional increment auth.version WHERE id dan
expected version, check exactly one affected row; audit; outbox bila changed;
safe idempotency result; commit. No-op masih memeriksa current target version.
Existing unconditional UserVersionWriter.SetVersion bukan conditional guard untuk C;
gunakan operation assignment version khusus dengan affected-row result.

| Operation | State/provenance | Required effective audit/event | Counter |
| --- | --- | --- | --- |
| Grant new pair | Insert ACTIVE actor/time; revoked metadata kosong | USER_ROLES_GRANTED / iam.user.roles_granted.v1, satu batch | user.version +1 sekali |
| Regrant REVOKED | Update row yang sama ACTIVE; grant source/time terbaru; clear revoked_by/at/reason | Sama, changed IDs hanya transition | user.version +1 sekali |
| Revoke ACTIVE | Update REVOKED actor/time/reason; preserve granted_by/at | USER_ROLES_REVOKED / iam.user.roles_revoked.v1, satu batch | user.version +1 sekali |
| Unchanged pair | ACTIVE grant, REVOKED/absent revoke tidak di-write | Hanya termasuk audit command jika batch berubah | Tidak dihitung per pair |
| Full no-op baru | Tidak insert absent revoke, tidak overwrite provenance | Satu NO_OP audit + safe changed=false result; tanpa event | Tetap |
| Replay | Exact saved result | Tanpa audit/event/write tambahan | Tetap |

Grant ACTIVE target/roles/apps dan business access guards tetap berlaku pada
already-active pair; IAM tidak memerlukan IAM app access. Revoke tidak perlu ACTIVE
target/role/app/access, tetapi semua roles harus terdaftar. Seluruh nondeleted
account statuses mengikuti behavior ini. Preserve assignments di luar request,
application access, password/auth_version/session/refresh dan role entity versions.

Audit safe snapshots memakai target user/version, sorted IDs/status outcome dan
reason bounded; hanya actual changed IDs pada event. Envelope memakai user aggregate
ID/version committed, satu batch, safe actor/correlation/causation/traceparent;
freeform reason tidak masuk event, pakai bounded catalog reason_code jika perlu.
SafeCommandResult.UserID/Version/ChangedRoleIDs/Changed sudah representatif; validasi
response mengunci changed=false iff IDs kosong dan sorted unique IDs <=20. Outbox
published_at tetap NULL, tanpa runtime broker prerequisite.

TransactionOwner defer menjaga rollback seluruh precommit operation/guard/read/
conditional/write/cancel/panic failure. Independent interceptor failure audit baru
sesudah cleanup memakai existing bounded context/counter; error audit tidak menutupi
error asli. Tidak mengubah commit-on-denial refresh reuse yang existing. Commit
uncertain dipetakan dependency, tidak menyatakan rollback pasti atau sukses; retry
mengikuti persisted idempotency outcome. Test response-lost memakai actual commit
kemudian transport failure, bukan sekadar mock commit success.

### 5. Replay, error catalog dan compatibility

Idempotency scope actor/full method/user/{id}/key, canonical sorted IDs + trimmed
reason + expected version/presence, excludes request ID/deadline. Safe result24 jam;
same-scope different payload ABORTED; same key different target/method independen.
Expired lookup/replace dalam scope lock dan tx yang sama, retained result tidak
boleh dibuang dini. Current actor authorization sebelum replay; replay sebelum
version/eligibility target terbaru. Tidak ada credential/password/hash dalam result.

| Condition | Owner catalog / gRPC |
| --- | --- |
| Invalid input/query/version presence/key | Existing field/common validation / INVALID_ARGUMENT |
| Missing/deleted target; unknown role | Existing auth not-found / role.ErrRoleNotFound / NOT_FOUND |
| Target non-ACTIVE grant | New user grant-target-inactive semantic / FAILED_PRECONDITION |
| Disabled role | New role ROLE_DISABLED semantic / FAILED_PRECONDITION |
| Inactive application | Existing application inactive / FAILED_PRECONDITION |
| Missing/revoked business access | New application APPLICATION_ACCESS_REQUIRED / FAILED_PRECONDITION |
| Self platform-admin revoke | New user self-admin-role-revoke-forbidden / PERMISSION_DENIED |
| Last admin | Existing user LAST_ADMIN_REQUIRED / FAILED_PRECONDITION |
| Version/idempotency/busy | Existing user feature errors / ABORTED |
| Dependency/cancel/deadline | Existing cause-preserving mappings / UNAVAILABLE/CANCELLED/DEADLINE_EXCEEDED |
| Rate rejection | Existing shared RATE_LIMITED + RetryInfo / RESOURCE_EXHAUSTED |

Numeric dictionary codes dipilih generator/catalog allocation sibling, tidak
hard-code wire codes di service atau remap legacy. Reuse semantic errors existing
bila tepat setelah inventory; names tabel adalah planned semantics, bukan simbol
Go yang dianggap sudah tersedia. Supported locales/generated mappings wajib sinkron.

C menambah tepat3 RPC menjadi30; generated descriptors dan fixture baseline27
harus mengunci tags/types/full method compatibility. Preserve UserRoleAssignment
existing tags1–5 dan field legacy User.password kosong; dedicated item C terpisah.
Register PENDING/token-empty intentional, consumer adjustment existing; hanya
Login/Refresh credential responses. EdDSA-only/exact issuer/ignored audience,
15m access bounded expiry, absolute7d session/skew30s/restricted/strict refresh
rotation/reuse/session terminal tetap regression. Tidak ada grace/dual algorithm.

## Risks / Trade-offs

- Global revoke/policy guard mengurangi write parallelism → reuse bounded busy
  outcome; bukti correctness dua physical connections didahulukan.
- Graph/schema evidence historis lebih lama dari baseline → source sudah diperiksa,
  C fixture/metadata parity diverifikasi saat apply tanpa menjalankan migration app.
- Seventeen sortable fields dan nullable source data → typed registry, all-field
  both-direction SQL/keyset tests, target-bound cursor dan hidden projection tests.
- user_role hanya satu row/pair, bukan event history penuh → current grant/revoke
  metadata dibaca dari row; audit menyimpan transisi, C tidak menjanjikan history RPC.
- Contract publication belum ada → required EXT gate; jangan register stubs/local
  protobuf atau menyatakan backend PASS karena workspace sibling berhasil build.
- List default historical lebih luas daripada GetUser → dedicated RPC/item menjaga
  defaults existing; test tidak memberi effective permission dari revoked/disabled rows.

## Migration Plan

Implementation order: contract generator/tests → authorized publication dan module
resolution → explicit dependency update → persistence/query/domain → feature →
handler/session/rate/wiring → registered integration/DB gates → docs/evidence.
Propose hanya membuat planning artifacts. Git branch/commit/push mengikuti otorisasi
repo/branch yang masih berlaku saat apply, termasuk izin user pada branch yang sama;
permintaan propose sendiri tidak menambah izin Git. Jangan membuat branch/PR/tag/
release atau mempublikasikan grpc-auth secara implisit.

Tidak merencanakan tabel baru atau migration runtime. Dedicated schema assertions
C memverifikasi unique pair, FK/provenance/status/revocation checks, nullable legacy
source, numeric IDs/version bounds, user/status indexes dan admin_guard singleton.
Archived baseline dibandingkan read-only dari file. Bila metadata DB aplikasi
diperlukan saat apply, gunakan scope metadata READ ONLY yang telah diizinkan,
tanpa row bisnis/migration; izin tersebut tidak mencakup write aplikasi.
DSN opt-in hanya test DB khusus melalui env-test.yaml yang ignored; tidak disalin
ke artefak. DB unavailable berarti gate BLOCKED, bukan skipped PASS.

Rollback code hanya jika contract consumer kompatibel dan deployment belum aktif;
setelah assignment committed, jangan hapus row/audit/outbox/idempotency atau
mengaktifkan credential lama. Actual target schema/admin/TLS/key/caller rotation,
consumer IP/RetryInfo/PENDING/storage/login ulang serta cutover/rollback validation
berada di Phase6 DEP-01, mandatory sebelum deployment, di luar checklist backend C.

## Acceptance Ledger

Semua criteria berikut scope MVP_REQUIRED grpc-auth kecuali URA-EXT-01 contract
api-contracts; prerequisite bersama A/B dan published C. Status awal pada proposal NOT_STARTED. Implementasi C sekarang **PASS** sesuai
[evidence aktual](evidence.md) dan required gates workspace/standalone/dedicated PG.
Test source/fixture sendiri bukan actual PASS. AC ID mengikuti
[spec utama](../../../../docs/user-management-spec.md#15-acceptance-criteria-dan-pengujian) dan
[spec MVP](../../../../docs/grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence).

| ID | Scope/criterion dan AC | Required test/evidence | Status |
| --- | --- | --- | --- |
| URA-01 | Exact methods/permissions, service/restricted/actor-target, AC-31; M-05 | Handler/feature/registered negative + locked actor change | [PASS](evidence.md) |
| URA-02 | Historical safe list, version/source/NULL, same snapshot; AC-04/10; M-05 | Query/handler matrix + two-backend RR barrier fresh denial | [PASS](evidence.md) |
| URA-03 | All17 scalars sort, filters/search/projection/unique total/target cursor | Both directions, mixed null ties, literal wildcard, injection/mismatch, terminal/offset pages | [PASS](evidence.md) |
| URA-04 | Eligible atomic grant/regrant/duplicates/missing access; AC-04..07/35 | All-or-nothing writes; raw21 invalid; pair identity/source and one version/event | [PASS](evidence.md) |
| URA-05 | Any-status revoke, mixed active/revoked/absent, unknown role rejection; AC-09/35 | Full matrix, no absent insert, metadata preservation, rollback | [PASS](evidence.md) |
| URA-06 | Self admin + shared last-admin; AC-19/20 | Unit guard + two-backend revoke/revoke and revoke/status barriers/recount | [PASS](evidence.md) |
| URA-07 | Auth-before-replay/version, retention/scope/response loss; AC-24..26 | Real commit/lost response + reordered set/conflict/expiry counts | [PASS](evidence.md) |
| URA-08 | Audit/outbox/result atomic, independent failure audit; M-07/08/11 | Every write/conditional/guard/read failure, panic/cancel/commit uncertainty + pool cleanup | [PASS](evidence.md) |
| URA-09 | Credentials preserved; overlap union current contribution; AC-09/10 | Original Login JWT + primary resolver before/after; byte-equal credential rows | [PASS](evidence.md) |
| URA-10 | Shared quotas, errors/locales, exact registered30; M-10 | Old/new group exhaustion RetryInfo, safe localized catalog mappings, production server inventory | [PASS](evidence.md) |
| URA-11 | Baseline27 regression + delivery; M-11 | Default/vet/race/build workspace and GOWORK=off; full dedicated PG normal/race | [PASS](evidence.md) |
| URA-EXT-01 | EXTERNAL_REPOSITORY api-contracts C generated/published prerequisite | Compatibility/generation/full/vet/locales, commit/module/sums, standalone download | [PASS](evidence.md) |

AC-09/10 C evidence covers assignment contribution behavior using current resolver
and existing registered Me/ListUsers paths; E permission-read RPCs/EvaluateAccess
and CLM resource actions remain outside C. Their absence does not turn those full
MVP end-to-end criteria globally PASS. D access gates tested with controlled
fixtures without implementing D. DEP-01 remains OPEN in Phase6, outside C tasks.

Verification plan: every new test in tests/ thin runner + test_scenarios/mocks/
fixtures with testutils.Run; deterministic default tests without real DB/network.
Opt-in PostgreSQL tests use owned schema, explicit dedicated test DB, distinct
backend PID assertions, channel barriers/timeouts, both arrival orders where
meaningful, lock release/rollback/reuse and pool InUse0. Concurrent same-pair
commands cover same/different key; after busy retry produces replay/no-op/stale
version according to key/version, never duplicate pair or event. Actor revocation
and disabled-role/application competition use shared protocol. Required failure
injection spans insert/reactivate/revoke/version/audit/event/result/deferred commit;
read failures and conditional zero-row results cannot leave tentative writes.
Evidence records actual commit/version/source digest, command, environment without
secrets, duration and expected/actual; NOT_STARTED/BLOCKED stay honest until run.

# Design

## Context

Motivasi: [proposal](proposal.md). Normative behavior ada pada
[role administration](specs/role-administration/spec.md) dan
[catalog reads](specs/iam-catalog-reads/spec.md). Baseline 11 Oktober 2026:
increment A sudah diarsipkan; B belum diimplementasikan.

| Inspected source | Fakta existing / implikasi |
| --- | --- |
| `internal/repositories/domain/role.go`, reader/writer role | Model legacy berisi numeric code, name/description; reader hanya GetDefaultRole dan writer Create legacy. Jangan mengubah default Member/registration saat menambah model management |
| `internal/repositories/reader/authorization.go`, `internal/features/common/authorization.go` | Permission contributions membaca role/mapping/application/access; resolver ini harus menjadi sumber evaluation sesudah mutation |
| `internal/features/user/management_service.go`, `management_transaction.go`, `detail.go` | Pattern provider, feature-owned lifecycle/locks, tx-bound repositories, authoritative recheck, safe replay dan read snapshot sudah tersedia |
| `internal/features/common/transaction.go`, `locking.go`, `idempotency.go`; `pkg/database/transaction.go` | Cleanup/lock ordering/fingerprint/replay/provider existing; reuse contract kecil, jangan membuat generic command framework atau lifecycle tersembunyi di repository |
| `internal/repositories/domain/outbox.go`, `pkg/types/events.go`, writer audit/outbox/idempotency | Role event types dan typed role data sudah ada; safe replay payload/validation perlu role outcome yang additive |
| `internal/configs/foundation.go`, `register.service.go`, `internal/interceptors/rate_interceptor.go` | Default server19 methods, satu reusable primary pool, verified boundary dan shared numeric admission; role/catalog registration/policy belum ada |
| `../api-contracts/proto/`, dictionary dan mapping | Belum ada role/permission/application service/payload/catalog. PageQuery/PageMeta dan FieldMask patterns tersedia; published baseline `v0.0.0-20261011113651-729758974696` |
| `docs/sql/user-management/01_schema_backfill.sql`, `tests/integration/postgres/fixtures/schema.sql` | Role id/numeric code INTEGER, string code100, name50, description100 nullable, version>=1, status ACTIVE/DISABLED; sequence `role_numeric_code_seq`, unique string/numeric code; applications/permissions/mapping tables tersedia sebagai schema/fixture reference |

Graph `graphify-out/graph.json` pada `b89f372` lebih lama dari working tree.
AST menunjukkan domain role sebagai hub degree51 dan dependency auth feature
ke domain/reader/writer role. Source di atas diverifikasi langsung; graph tidak
dianggap evidence implementation terbaru atau live schema. `graph-instruction.md`
tidak ada. Current dirty tree lifecycle/user management harus dipertahankan.

## Goals / Non-Goals

**Goals:** delapan method production dengan contract published, query terukur,
protected business-role mutation dan acceptance ledger yang dapat diuji.

**Non-Goals:** tidak menggeneralisasi ListUsers menjadi shared SQL builder,
tidak menambah DB pool/cache/framework, tidak mengubah env.yaml architecture,
tidak menyiapkan RPC kosong untuk increment C–E atau mengimplementasikan
filter-options REST. Deployment gate tetap Phase6 DEP-01.

## Decisions

### 1. Feature ownership dan composition

Tambahkan feature role (`internal/features/role/`) untuk mutation/read role dan
feature catalog (`internal/features/catalog/`) untuk permission/application read.
Domain contracts/model/query berada di `internal/repositories/domain/`;
tx-bound adapters di reader/writer; handler hanya protobuf mapping/validation.
`internal/configs/` merangkai constructor, primary provider, clock/event ID,
registered servers dan error registry. Tambahkan typed role status, permission,
audit action/target/reason constants pada `pkg/types` sesuai pattern existing.
Jangan memakai string literal berulang untuk business constants.

Pilihan ini menjaga flow handler → feature → repository existing. Alternatif
memperluas legacy RoleReader/RoleWriter atau generic management framework akan
mengikat registration ke kebutuhan admin; gunakan interface management kecil
yang terpisah dan reuse helper transaksi existing bila cocok secara semantik.

### 2. Exact authentication dan admission

| Full method | Exact permission | Group |
| --- | --- | --- |
| `/role.v1.RoleService/ListRoles` | iam.role.read | protected-read |
| `/role.v1.RoleService/GetRole` | iam.role.read | protected-read |
| `/role.v1.RoleService/CreateRole` | iam.role.create | admin-mutation |
| `/role.v1.RoleService/UpdateRole` | iam.role.update | admin-mutation |
| `/role.v1.RoleService/ChangeRoleStatus` | iam.role.update | admin-mutation |
| `/role.v1.RoleService/ReplaceRolePermissions` | iam.role.permission.update | admin-mutation |
| `/permission.v1.PermissionService/ListPermissions` | iam.permission.read | protected-read |
| `/application.v1.ApplicationService/ListApplications` | iam.permission.read | protected-read |

Service caller harus verified opaque token melalui TLS, full-method allowed
dan namespace IAM allowed. Request application_code adalah target catalog,
bukan namespace identitas caller atau pengganti endpoint IAM permission.
Service/user ID/role dari payload tidak dipercaya. Semua method restricted
ditolak; allowlist tetap Me minimum/ChangePassword/Logout. Tambahkan delapan
full methods ke session policy dan security-attempt audit boundary existing.
Authority read dalam snapshot; mutation sesudah lock dan sebelum replay.

Quota shared: protected-read120/min burst20, admin-mutation30/min burst5.
Gunakan store existing cap10.000, idle15menit, cleanup1menit, bounded RetryInfo;
invalid request tetap menghitung admission. Tidak ada quota per role-code baru.
Service authentication mendahului allocation. IP trust hanya rest-orc verified
dengan forwarding sanitization tracked di Phase6; tidak ada IP quota baru di B.
Backend registration memerlukan guard/persistence/test PASS, tanpa menunggu
actual consumer deployment. Alternatif mempercayai interceptor snapshot untuk
seluruh command ditolak karena actor privilege dapat berubah sebelum lock.

### 3. Contract additive di api-contracts

File baru yang direncanakan: `proto/role/v1/role.service.proto`,
`role.payload.proto`, permission/application equivalents, generated `pb/`,
feature dictionaries/mappings/locales serta descriptor/wire tests. Semua tag
berikut baru, belum dialokasikan ke file existing; freeze saat contract task.
Existing descriptors/tags/types tidak disentuh. IDs uint64 divalidasi1..MaxInt32;
versions optional uint64 divalidasi hadir1..MaxInt64; numeric role_code int32
positif berasal sequence DB, berbeda dari string code.

| Message | Field number/type yang direncanakan |
| --- | --- |
| RoleSummary | id1 uint64, code2 string, name3 string, description4 optional string, application_id5 uint64, application_code6 string, status7 string, is_system8 bool, version9 uint64, role_code10 int32, created_at11 Timestamp, updated_at12 Timestamp, affected_user_count13 uint64 |
| PermissionItem | id1 uint64, application_id2 uint64, application_code3 string, code4 string, description5 string |
| ApplicationItem | id1 uint64, code2 string, name3 string, status4 string, is_system5 bool |
| RoleFilter | application_code1 string, statuses2 repeated string |
| ApplicationFilter | statuses1 repeated string |
| ListRolesRequest | query1 PageQuery, filter2 RoleFilter |
| ListPermissionsRequest | query1 PageQuery, application_code2 string |
| ListApplicationsRequest | query1 PageQuery, filter2 ApplicationFilter |
| Each list response | items1 repeated relevant item, meta2 PageMeta, next_cursor3 string |
| GetRoleRequest / GetRoleResponse | role_id1 uint64 / role1 RoleSummary, permissions2 repeated PermissionItem |
| CreateRoleRequest | application_code1 string, code2 string, name3 string, description4 optional string, permission_codes5 repeated string |
| CreateRoleResponse | role1 RoleSummary, permissions2 repeated PermissionItem |
| UpdateRoleRequest | role_id1 uint64, expected_role_version2 optional uint64, name3 optional string, description4 optional string, field_mask5 FieldMask |
| UpdateRoleResponse | role1 RoleSummary, changed2 bool |
| ChangeRoleStatusRequest | role_id1 uint64, status2 optional string, reason3 string, expected_role_version4 optional uint64 |
| ChangeRoleStatusResponse | role_id1 uint64, status2 string, version3 uint64, changed4 bool |
| PermissionSet | codes1 repeated string |
| ReplaceRolePermissionsRequest | role_id1 uint64, permissions2 PermissionSet, reason3 string, expected_role_version4 optional uint64 |
| ReplaceRolePermissionsResponse | role1 RoleSummary, permissions2 repeated PermissionItem, changed3 bool |

Timestamp messages adalah scalar temporal pada sort registry; NULL tetap absent,
tidak direkayasa menjadi migration time. Wrapper PermissionSet diperlukan agar
absent intent gagal validation tetapi present empty berarti clear all; alternatif
repeated field langsung tidak membedakan kedua kondisi. CreateRole empty set
boleh default tanpa wrapper karena creation selalu explicit new state.

Update mask/presence harus cocok persis, tanpa empty/duplicate/unknown path.
Trim name/description/code/application/permission codes; code tidak di-lowercase
diam-diam: harus canonical grammar lower-case. Name1..50, description0..100,
role code<=100 dengan regex schema dua segmen, permission code<=150 dengan
grammar catalog; prefix dan application_id keduanya harus cocok. Description
selected empty menyimpan cleared NULL, omitted tidak berubah. Reason trim3..500.
Create fixed ACTIVE/is_system=false; tidak ada input role_code/is_system/status.
Limit20 role pada batch assignment bukan limit permission set di B; jangan
meminjam limit tersebut. Semua input tetap tunduk pada existing gRPC size cap.

Error semantic inventory contract task: ROLE_NOT_FOUND→NOT_FOUND,
ROLE_CODE_ALREADY_EXISTS→ALREADY_EXISTS, SYSTEM_ROLE_PROTECTED→FAILED_PRECONDITION,
INVALID_PERMISSION_SET→INVALID_ARGUMENT, missing application→NOT_FOUND,
inactive/system business-create target→FAILED_PRECONDITION, version/idempotency/
busy→ABORTED, malformed/presence→INVALID_ARGUMENT. Reuse existing common error
bila tepat; dictionary role/application menambah hanya semantic baru, locales
en/id dan fallback serta cause-safe boundary tanpa global legacy remapping.
Missing permission pada final set diperlakukan INVALID_PERMISSION_SET.

Task publication harus mencatat commit/module/checksum dan standalone download.
Gunakan normal commit/push branch existing jika telah diotorisasi user; tidak
tag/release/merge/force-push. Persiapkan diff/test konkret sebelum meminta izin
Git yang belum tersedia. Dependency update backend setelah module tersedia;
workspace build sendiri tidak membuktikan publication. Tidak ada Go/proto/SQL
atau sibling edits pada tahap proposal ini.

### 4. Catalog query registry dan snapshot

Gunakan read-only repeatable-read primary snapshot provider existing dengan
feature-owned BeginSnapshot/Commit/Rollback. AuthenticateSession, resolve exact
permission, page/total/collection/count semua memakai satu tx-bound handle.
Tidak pool fallback atau parallel query pada handle bersama.

RoleSummary seluruh13 fields sortable: id/code/name/description/application_id/
application_code/status/is_system/version/role_code/created_at/updated_at/
affected_user_count. PermissionItem seluruh5 fields sortable; ApplicationItem
seluruh5 fields sortable. Registry explicit public→SQL expression, type dan null
semantics; termasuk scalar turunan count, bukan arbitrary column dari request.
Default id ASC; honor mixed multi-sort, reject duplicates, append id ASC jika
belum ada. Collation text dibuat deterministic menggunakan PostgreSQL C pada
expression/order/cursor comparison yang sama; nulls last kedua arah. Alternatif
sort-in-memory merusak total/keyset dan biaya page; query memakai registry SQL.

Role affected count memakai correlated aggregate atau preaggregated subquery
COUNT(DISTINCT user_id) dari user_role ACTIVE dengan existing auth FK. Tidak
filter auth.status, role.status, app/access.status; semua status akun termasuk
soft-deleted existing row dengan ACTIVE assignment tetap dihitung. Tidak join
permission collection ke root page sehingga role tidak duplicate. List/get
count identik dan query sort count memakai expression yang sama. Permission
detail diurutkan code ASC/id ASC, bukan dijadikan sortable collection.

Filter Role application_code + statuses OR, Permission application_code,
Application statuses OR; antar kategori/search AND. Catalog default semua
status termasuk system/unassigned; tidak menyimpulkan options user dari page.
Application filter yang valid tetapi tidak ada menghasilkan empty page; GetRole
yang absent menghasilkan NOT_FOUND setelah authority. Permission catalog tidak
memiliki status field; jangan membuat status atau CRUD permission terselubung.

Search trim keyword<=200, empty berarti no search. CONTAINS default/UNSPECIFIED
atau PREFIX, case-insensitive; escape percent/underscore/backslash untuk literal
matching. Selected fields OR, default role/app code+name; permission code+
description. Allowlist role code/name/description/application_code; permission
code/description/application_code; application code/name. Unsupported modes
FULL_TEXT/TRIGRAM/combined dan unknown fields gagal INVALID_ARGUMENT.

PageQuery.fields optional public projection: absent all, selected public fields
plus id selalu dikembalikan; reject duplicate/unknown paths. Query tetap membaca
hidden sort columns agar cursor tidak bergantung projection. Total unique root
sebelum cursor/offset/limit; page1 default, limit20/default/max100, empty total0.
Tanpa cursor page>1 memakai deterministic offset; dengan cursor keyset saja,
page hanya ordinal metadata dan tidak ditambah offset. Ambil limit+1 untuk
has_next/next_cursor; metadata has_prev bila cursor atau page>1 sesuai pattern.

Cursor versioned base64url safe payload: method, normalized query digest,
ordered typed sort values/null markers dan id. Digest binds sort/filter/search,
bukan page/limit/projection yang tidak mengubah membership/order. SQL binder
parameterizes every value; cursor tidak menjadi authorization proof. Numeric
last_id hanya diterima ID-only; cross-method/query/type/shape invalid ditolak.
Gunakan lexicographic OR predicate untuk mixed directions dan null bucket,
bukan offset yang disamarkan cursor. Dataset dapat berubah antar request;
keyset tidak menjanjikan snapshot retained. Registrasi field baru wajib test
coverage, tidak auto-generate field→column dari protobuf reflection.

### 5. Mutation protocol dan lock set

Feature Begin(requestCtx), NewTransactionOwner, AcquireLocks dan defer Finish;
repository hanya query/write di tx. Actual PostgreSQL ORM1.7 TryLock memakai
pg_try_advisory_xact_lock pada sql.Tx sampai commit/rollback; acquired=false
RESOURCE_BUSY/ABORTED, DB failure UNAVAILABLE. Bounded restart mengikuti pattern
existing: discovery dari tx, jika menemukan predecessor setelah lock akhir,
rollback lalu begin enlarged canonical set; tidak memperoleh lock urutan awal
di tengah command. Exhaustion menghasilkan busy tanpa retry commit unknown.

Urutan: idempotency → last-admin bila relevant → policy global → role-code
creation → actor user IDs ascending → actor-contributing/target role IDs ascending
→ application IDs ascending → actor session UUID canonical. Semua role mutations
mengambil global policy guard. Recheck actor session/status/auth_version dan
permission contributions setelah stable locks; target/application/permitted set
dibaca ulang dari handle sama. Create application resolved dulu untuk IDs lalu
rechecked sesudah locks. Application/catalog writes di luar service hanya melalui
reviewed migration saat write dikoordinasikan; B tidak menyediakan CRUD.

System/IAM protection membuat role-management API tidak dapat menghilangkan
platform admin; guard last-admin tetap dimiliki user-status/revoke assignment.
Jangan mengambil last-admin global untuk semua custom role writes tanpa alasan;
test protected role attempts mempertahankan admin invariant. Future C/D harus
mengikuti global policy/lock protocol yang sama. Tidak lock semua assigned users
untuk disable/replace: user rows/sessions tidak ditulis; policy guard dan current
resolver menyediakan propagation. Alternatif mass revoke session melanggar AC-23.

Creation uniqueness memakai role-code lock dan unique DB constraint. Numeric
code dialokasikan sequence existing, tidak MAX+1. Rollback boleh meninggalkan
sequence gaps, tidak meninggalkan role/mappings. Conditional version update
harus affect exactly one row; check failure setelah mapping write tetap rollback.
PermissionSet lookup divalidasi seluruhnya sebelum delete bila memungkinkan;
delete+insert final set+version+audit/event/result tetap satu commit.

### 6. Command persistence matrix dan replay

| Command | Required key / target | Effective write | Audit / event / aggregate |
| --- | --- | --- | --- |
| CreateRole | wajib; role-code/{canonical_code} | role ACTIVE version1 + final mappings | ROLE_CREATED / iam.role.created.v1 / role:id version1 |
| UpdateRole | optional; role/{id} | selected metadata + role version sekali | ROLE_UPDATED / iam.role.updated.v1 / role:id committed version |
| ChangeRoleStatus | wajib; role/{id} | ACTIVE↔DISABLED + role version sekali | ROLE_UPDATED / iam.role.updated.v1 / role:id committed version |
| ReplaceRolePermissions | wajib; role/{id} | exact final mappings + role version sekali | ROLE_PERMISSIONS_CHANGED / iam.role.permissions_changed.v1 / role:id committed version |

Semua existing-target mutation memeriksa expected_role_version positif setelah
current authorization/replay, sebelum no-op. System protection tetap wajib
sebelum new no-op. Valid new no-op satu NO_OP audit (UpdateRole unkeyed juga),
keyed safe changed=false result, tanpa event/version. Replay tanpa audit/event/
effect tambahan. User versions/auth_version/session/refresh/assignments/access
tetap. Disabled target role/app tidak membuat metadata/status/mapping read/edit
forbidden; creation membutuhkan ACTIVE business app, effective resolver tetap
memerlukan seluruh ACTIVE gates.

Scope actor+full method+canonical target+key<=200, caller service dicatat tanpa
unique dimensi baru. Fingerprint normalized payload/presence/expected version/
sorted unique permission codes; no credential atau trace/deadline. Extend safe
result secara additive dengan allowlisted role outcomes yang cocok setiap
response, termasuk nullable metadata dan permissions saat diperlukan. Replay
mengembalikan original safe response/count, bukan reread target yang berubah.
Unexpired24h retained; expired record diganti atomik di scope exclusion. Key
same target different payload conflict; different target independent. Response
hilang setelah commit diselesaikan keyed replay, tidak simpan credential.

Audit safe actor/service/target_role/before/after/reason/correlation; free-form
reason hanya bounded audit. Event envelope existing schema_version1, role ID
string, committed aggregate_version, UTC/correlation/causation/traceparent;
allowlisted data role_id/application_id/application_code/status/final permission
codes sesuai event, controlled reason code. Jangan field free reason atau
user credentials. Event tetap pending published_at NULL tanpa RabbitMQ.

Every precommit guard/read/write/affected-row/cancellation/panic error rollback
seluruh tx; failure audit independent setelah cleanup melalui boundary existing,
dengan bounded context/correlation dan original error preserved jika audit gagal.
No double failure audit dari handler dan interceptor. Commit uncertainty memberi
safe error, cleanup best effort, tidak retry mutation atau claim pasti rollback.
Refresh reuse tetap lifecycle exception commit-before-denial existing; tidak
diterapkan pada role command.

### Acceptance ledger

Status backend B berikut **PASS** setelah required gates aktual pada
[evidence](evidence.md) dan [39 scenario mappings](scenario-traceability.md).
DEP-01 tetap OPEN di luar completion B.
Ledger apply mencatat source commit/digest, module/checksum, command, sanitized
environment/version, waktu, expected/actual dan hasil. AC/M IDs merujuk
[spec MVP](../../../../docs/grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence)
dan [spec utama](../../../../docs/user-management-spec.md); ID/makna tidak diganti.

| ID | Scope / repository | AC/M asal dan criterion | Prerequisite | Test dan evidence wajib | Status |
| --- | --- | --- | --- | --- | --- |
| RM-01 | MVP_REQUIRED grpc-auth | AC-21/31, M-05: exact authority, direct gRPC bypass denial, restricted/system protection | contract B + existing auth foundation | handler/feature/registered gRPC negatives, stale actor DB barrier; evidence apply | PASS |
| RM-02 | MVP_REQUIRED grpc-auth | AC-22, M-05/07: custom create, namespace/set/uniqueness/schema bounds | role/app catalog metadata | unit + SQL DryRun + PG concurrent create, mapping counts | PASS |
| RM-03 | MVP_REQUIRED grpc-auth | AC-21/24/26: mask/status/set/version/no-op | RM-01/02 | presence/stale/no-op/clear-set tests + PG expected version conflict | PASS |
| RM-04 | MVP_REQUIRED grpc-auth | AC-23, existing AC-13 auth subset: immediate current policy, credentials remain | resolver + RM-03 | PG disable/enable/replace/overlapping roles, primary snapshot evaluation, user/session row equality | PASS |
| RM-05 | MVP_REQUIRED grpc-auth | AC-24/25/26/28, M-07/08: atomic persistence/replay/failure audit/pending events | tx + audit/outbox/idempotency | write/read/guard/cancel/panic injection; PG response-loss/expiry/mixed-key/count assertions | PASS |
| RM-06 | MVP_REQUIRED grpc-auth/api-contracts | M-01/02/03/05/06/10/11: compatible19→27 registration/admission/errors | contract publication | descriptor/wire/locale tests; all27 service/session policy, shared quotas; regression suites retain existing AC-34 ChangePassword behavior | PASS |
| RM-07 | MVP_REQUIRED grpc-auth | M-05/06: read authority/snapshot, safe detail/catalog | snapshot provider + contract B | registered RPC denied/empty/dependency; controlled PG read/commit barrier | PASS |
| RM-08 | MVP_REQUIRED grpc-auth | AC-23 impact metadata; user decision count | fixtures all account/assignment states | unit/PG count distinct/no join inflation/no status or access gate | PASS |
| RM-09 | MVP_REQUIRED grpc-auth | M-06 pagination and user decision all scalar sort | scalar registry + cursor contract | every field/direction/multi-sort/null/projection/type/query mismatch/numeric cursor, SQL + PG | PASS |
| RM-10 | MVP_REQUIRED grpc-auth | M-05/06/10/11: filter/search/unique total/admission | catalog query contract | OR/AND/literal search/empty/default/paging tests, shared read quota | PASS |
| RM-EXT-01 | EXTERNAL_REPOSITORY api-contracts + grpc-auth dependency | EXT-01, M-05/08/09: eight additive RPCs published | explicit contract tasks/publication authority | generation twice stable, compatibility/catalog/locale/full/vet, module origin/checksum, workspace + standalone | PASS |
| DEP-01 | EXTERNAL readiness, Phase6 owners | Mandatory actual deployment gate, outside completion B | all backend increments | actual migration/admin/key/caller/consumer/cutover evidence at Phase6 | OPEN, outside B |

Test layout seluruh baru root `tests/` mirrors features role/catalog, domain,
reader/writer, handler, interceptors/configs/integration; thin runner/testutils
Scenario + imported test_scenarios/mocks/fixtures. Unit/default integration
deterministic, no real network/database/environment. Separate opt-in postgres
suite memakai dedicated test DB dan existing guard, dua physical connections
verified backend IDs/barriers, tanpa Docker/sleep-only proof atau DB aplikasi.
Test processes PostgreSQL workspace/standalone dijalankan sequential karena
advisory lock keys global per DB meskipun schema isolated.

## Risks / Trade-offs

- [Sorting affected counts/description costs aggregate/text work] → SQL grouping
  per root, existing indexes, bounded page and measured fixture query plans;
  optimasi index hanya jika apply evidence menunjukkan kebutuhan, tanpa schema
  migration otomatis.
- [Rows change between cursor pages] → document no cross-request snapshot;
  query-bound keyset plus deterministic null/collation/type tests.
- [Global policy guard serializes mutations] → retain local MVP correctness;
  busy returns ABORTED with no partial writes, do not introduce distributed lock.
- [Sequence schema or live metadata differs] → targeted read-only comparison
  role/application/permission/sequence/constraints at apply; mismatch BLOCKED
  before incompatible code, migration prerequisite explicitly planned/reviewed.
- [New error registries or services omitted from startup] → assert27 exact
  methods and localized role/application errors through actual registered path.
- [Dirty tree and publication mismatch] → preserve current work, scoped diff,
  source manifest and standalone published-module verification.

## Migration Plan

1. Apply prepares/test/reviews contract B, publishes under authorized Git scope,
   verifies module origin, then updates grpc-auth dependency. No placeholder RPC.
2. Implement all backend guards/persistence/admission and pass workspace,
   standalone plus dedicated PostgreSQL gates; record actual evidence.
3. Final backend change is eligible for later sync/archive after validation.
   Proposal and publication alone do not complete role management.
4. Actual schema/sequence migration, seed, service-token method allowlist,
   TLS/EdDSA material and rest-orc readiness remain Phase6 DEP-01, mandatory
   before deployment. Existing issuer/access15m/session7d/skew30s/audience ignored,
   Register token-empty and client login ulang decisions remain.
5. Rollback deployment uses a previously verified secure artifact with compatible
   additive schema/contract; stop new admin methods if needed, never reopen
   unauthenticated routes or HS256/sid-less fallback. Data/event history is not
   deleted and no down-migration runs as part of proposal/apply verification.

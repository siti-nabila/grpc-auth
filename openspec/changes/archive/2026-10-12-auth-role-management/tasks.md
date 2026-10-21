# Tasks

Status implementation COMPLETE: all35 tasks complete; required backend gates PASS. Contract B published; dependency, schema preflight, domain inputs and safe replay verified. See [evidence](evidence.md). [Design/acceptance ledger](design.md#acceptance-ledger)
mengikat setiap task ke RM-01..10/RM-EXT-01; catat hasil aktual sebelum memberi x.
Pekerjaan contract api-contracts termasuk scope implementasi berikutnya; tahap
proposal ini tidak mengubah sibling/code/dependency. Backend wiring bergantung
publication contract B. Jika dependency atau dedicated test DB belum tersedia,
task dependent BLOCKED; jangan membuat substitute atau memberi PASS karena skip.
Actual deployment readiness tetap Phase6 DEP-01, di luar checklist completion B.

## 1. Additive contract dan publication api-contracts

- [x] 1.1 Catat inventory source/descriptor baseline19 methods, public fields/tags/types, error semantics dan contract B matrix pada contract-inventory.md; verifikasi delapan method belum ada dan design field allocation tidak bertabrakan.
- [x] 1.2 Tambahkan role/permission/application proto service/payload untuk delapan RPC, PageQuery/PageMeta/next_cursor, nullable RoleSummary, version/mask/status presence dan PermissionSet wrapper; descriptor/wire tests membuktikan old19 unchanged dan absent versus explicit empty permission set berbeda.
- [x] 1.3 Tambahkan semantic role/application dictionary, gRPC mappings, en/id locale dan fallback yang diperlukan tanpa legacy remap; tests encode/decode wrapped errors membuktikan NOT_FOUND/ALREADY_EXISTS/FAILED_PRECONDITION/INVALID_ARGUMENT/ABORTED dan cause/redaction sesuai design.
- [x] 1.4 Dokumentasikan delapan method, safe response, sort registry fields, count meaning, filters/search/projection/cursor dan mutation presence/key/version; verifikasi dokumentasi terhadap descriptor serta contract scenario tests.
- [x] 1.5 Jalankan make proto, targeted/full contract tests dan go vet ./..., lalu regenerate kedua kali; review diff compatibility/generated scope dan git diff --check, catat command/timing/results serta stable generated hashes di inventory.
- [x] 1.6 Siapkan diff contract terverifikasi lalu normal commit/push branch existing sesuai otorisasi eksplisit yang berlaku, tanpa tag/release/merge/force-push; verifikasi GOWORK=off module download exact commit dan catat published version/origin/checksum, BLOCKED bila publication belum tersedia.

## 2. Backend dependency domain dan persistence contracts

- [x] 2.1 Update grpc-auth go.mod/go.sum ke contract B yang sudah published; verifikasi exact module origin/version melalui GOWORK=off resolution dan workspace/standalone compile, pertahankan unrelated dependency hunks.
- [x] 2.2 Catat targeted read-only metadata comparison role/app/permission/mapping/sequence/constraints terhadap SQL reference dan PG fixture; hasil/schema-comparison mengonfirmasi name50/description100/code100/INTEGER/version/timestamps, mismatch menjadi prerequisite yang dibahas sebelum incompatible writes, tanpa migration aplikasi.
- [x] 2.3 Tambahkan management/catalog domain inputs/models/interfaces dan typed role status/permission/audit constants, tanpa mengganti legacy RoleReader/default Member; domain scenario tests membuktikan schema bounds, canonical namespace, set dedup, mask presence, reason/ID/version/key validation dan system/IAM protection.
- [x] 2.4 Extend safe result/fingerprint allowlist secara additive untuk role responses, nullable metadata/count dan canonical permission set; unit tests membuktikan exact saved response, input-order equivalence, unknown/unsafe field rejection dan existing user/session replay tetap kompatibel.
- [x] 2.5 Tambahkan tx-bound role/catalog/application/permission readers dan role/mapping writers dengan sequence allocation serta conditional expected version; DryRun/sqlmock tests membuktikan expressions/args/handle identity/not-found/uniqueness/affected rows, tanpa lifecycle/locks/fallback pool di repository.

## 3. Catalog query dan safe read implementation

- [x] 3.1 Implement registry seluruh scalar publik RoleSummary13/PermissionItem5/ApplicationItem5 beserta tipe/null/collation, multi-sort order/id tie-breaker dan projection validation; domain/query tests cover setiap field kedua arah, duplicates, unsupported/internal/SQL names dan hidden cursor values.
- [x] 3.2 Implement typed versioned composite cursor bound method/sort/filter/search, numeric ID-only compatibility, mixed-direction lexicographic/nulls-last predicate; tests membuktikan equal/null values, malformed/type/query/cross-catalog mismatch, changed query tanpa cursor serta terminal next_cursor kosong.
- [x] 3.3 Implement filters/status OR + category/search AND, literal CONTAINS/PREFIX, unique pre-cursor totals/page metadata/limit+1 dan affected count DISTINCT ACTIVE-assigned users semua status; SQL/scenario tests cover joins, disabled/unassigned roles, counts0, empty results, wildcard escaping, projection dan page versus cursor semantics.
- [x] 3.4 Implement feature-owned single primary repeatable-read snapshot untuk ListRoles/GetRole/ListPermissions/ListApplications dengan exact permission/session recheck dan safe collection/detail; feature mocks membuktikan auth-before-target, one-handle reads, snapshot cleanup, missing target/dependency/cancellation/panic tanpa partial result.
- [x] 3.5 Tambahkan handler protobuf mapping/safe response untuk empat reads dan docs/role-management-api.md bagian catalog; targeted handler tests memverifikasi nil input, query defaults/errors, missing auth/permission, null timestamps/description, safe count/projection dan propagation locale/context.
- [x] 3.6 Tambahkan opt-in PG read scenarios pada dedicated test DB untuk seluruh scalar sort, mixed tie/null/cursor/projection, OR/AND/search/unique totals/count semua account/assignment states dan concurrent snapshot commit; runner terkontrol membuktikan hasil dan two-connection barrier tanpa sleep-only proof, catat evidence RM-07..10.

## 4. Feature-owned role mutation dan atomic outcomes

- [x] 4.1 Implement shared role command flow Begin→canonical TryLock→stable discovery/restart→current actor/session/permission→safe replay→version/guard→write→Commit; feature mocks cover predecessor restart, contention versus dependency, target/actor separation dan stale authority, periksa lifecycle call sites hanya features/features-common.
- [x] 4.2 Implement CreateRole ACTIVE/is_system=false/version1 dengan same-business-app permission set, unique code/DB numeric sequence, ROLE_CREATED/iam.role.created.v1 dan safe keyed result satu tx; tests cover empty/duplicate/unknown/IAM set, inactive app, bounds/uniqueness dan failure injection tiap required write tanpa assignment/credential effects.
- [x] 4.3 Implement UpdateRole name/description presence+mask, nullable clear, expected_role_version, ROLE_UPDATED/iam.role.updated.v1 dan optional replay; tests cover omitted/empty/forbidden/mismatch/system/legacy/stale/no-op serta unkeyed NO_OP audit dan single version increment.
- [x] 4.4 Implement ChangeRoleStatus ACTIVE↔DISABLED dengan reason/version/required key dan ROLE_UPDATED/iam.role.updated.v1; tests membuktikan system/IAM protection, same-status no-op/stale behavior, disabled-app manageability dan user/session/assignment/access records tidak ditulis.
- [x] 4.5 Implement ReplaceRolePermissions explicit final-set wrapper, exact same-app registered mappings, required reason/version/key, ROLE_PERMISSIONS_CHANGED/iam.role.permissions_changed.v1 dan safe response; tests cover absent/empty/reordered/dedup/unknown/cross-app set, delete/insert/conditional-row failure dan all-or-nothing final set.
- [x] 4.6 Integrasikan rollback seluruh precommit guard/read/conditional/write/cancellation/panic, independent failure audit setelah cleanup dengan original error/telemetry, commit uncertainty tanpa auto retry; strict mocks membuktikan order/one handle, audit terpisah satu kali dan rollback termasuk setelah tentative success persistence.
- [x] 4.7 Tambahkan handler empat mutation dan dokumentasi input/output/key/version/no-op/replay/event matrix pada docs/role-management-api.md; targeted handler/serialization tests membuktikan presence/metadata cardinality/bounds, exact safe saved response/context dan no credential leakage.
- [x] 4.8 Tambahkan opt-in PG atomicity/uniqueness/version/replay/expiry/response-loss/failure-audit/rollback-after-write scenarios untuk keempat commands; assert exact role/mapping/version/audit/outbox/result counts dan sequence gaps allowed, evidence mencatat actual outcome bukan mocks-only PASS.

## 5. Production registration authorization dan policy concurrency

- [x] 5.1 Wire constructor/role/catalog handlers dan tiga generated servers di configs memakai reusable primary provider/clock/event ID existing; configs/registered-path tests membuktikan tepat27 methods, fail-fast incomplete deps, cleanup dan prior19 signatures tetap tersedia.
- [x] 5.2 Tambahkan delapan exact session/service policy entries, error registries dan security-attempt classification; registered gRPC tests cover service token/TLS/method/namespace invalid, restricted user, absent exact permission dan locale/catalog mapping tanpa target-existence disclosure.
- [x] 5.3 Sambungkan shared protected-read120/burst20 dan admin-mutation30/burst5 tanpa bucket baru; controlled-clock/concurrent tests membuktikan quota shared user/session/role/catalog, invalid input counts, capacity/cleanup/RetryInfo dan no business writes saat rejection.
- [x] 5.4 Tambahkan registered-path opt-in PG disable/enable/replace evaluation memakai JWT/sessions existing dan overlapping role contributions; assert immediate next-snapshot permission changes serta user versions/auth_version/sessions/refresh/assignment/access byte-equivalent, termasuk revoked assignments tidak pulih.
- [x] 5.5 Tambahkan dua-physical-connection PG barriers untuk concurrent same-code creation, same/different scoped keys, expected-role-version races, actor invalidation versus policy command, discovery/restart dan snapshot reads; assert busy/conflict/no stale allow, released locks/pool reuse serta protected system/admin invariant.
- [x] 5.6 Dokumentasikan production27 method inventory, mutation/query behavior dan remaining C–E/Phase6 ownership; perbarui roadmap status hanya sesuai evidence dan verifikasi local links/anchors serta docs sesuai registered behavior.

## 6. Final integration verification dan evidence

- [x] 6.1 Jalankan gofmt pada changed Go, targeted tests yang belum tercakup dan full go test ./..., go vet ./..., go test -race ./..., make build pada workspace; catat actual command/environment/timing/result di evidence.md dan source-manifest digest, failure wajib diselesaikan atau BLOCKED.
- [x] 6.2 Ulangi full test/vet/race/build dengan GOWORK=off memakai published contract B; module origin/checksum dan all27 method compatibility diverifikasi, bukan workspace-only success.
- [x] 6.3 Jalankan full dedicated opt-in PostgreSQL normal/race workspace lalu standalone secara sequential; pertahankan lifecycle/refresh reuse/last-admin/user management regression dan assert locks/connection pool cleanup, catat commands/results tanpa credential/DSN secret.
- [x] 6.4 Rekonsiliasi RM-01..10/RM-EXT-01 ke requirement/scenario→test→actual evidence; run openspec validate auth-role-management --strict, link/anchor checks, git diff --check/status dan scope/secret/generated review, lalu nyatakan completion backend B hanya setelah required gates PASS; tidak otomatis sync/archive/deploy atau memberi PASS DEP-01.

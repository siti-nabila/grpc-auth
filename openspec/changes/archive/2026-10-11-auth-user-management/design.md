# Design

## Context

Motivasi dan scope ada di [proposal](proposal.md); urutan seluruh Phase 2 ada
di [roadmap](../../../../docs/phase-2-management-plan.md). Status implementasi
change ini IMPLEMENTED; hasil aktual ada di [evidence](evidence.md). Contract/prerequisite evidence ada di
[contract inventory](contract-inventory.md) dan [schema comparison](schema-comparison.json). Keputusan user: setiap error precommit membatalkan
seluruh business transaction, failure audit tetap dicoba independen; readiness
deployment tetap Phase 6 terakhir.

Inspeksi source 11 Oktober 2026 setelah graph routing menunjukkan:

| Area | Observed source | Implikasi |
| --- | --- | --- |
| Feature user | internal/features/user/impl.go dan user.go hanya SearchUsers | Tambah service kecil per command/read; jangan menumpuk semua behavior di impl.go |
| Registration | internal/features/auth_feature/register_pending.go | Reuse normalized account lock, hash policy, account/profile/default-member persistence dan versioned event; admin creation punya actor/restricted/status/key berbeda |
| Profile | internal/features/profile/update_session.go dan update_fields.go | Reuse whitelist/presence validation dan version/event semantics tanpa memanggil self RPC untuk target admin |
| Admin session command | internal/features/session/admin.go | Pattern current authority → safe replay → target version → writes → commit sudah tersedia |
| Common transaction | internal/features/common/impl.go, transaction.go dan locking.go | TransactionOwner never auto-commits; ordered TryLock PostgreSQL transaction-scoped, cleanup tetap feature-owned |
| Idempotency | internal/features/common/idempotency.go, domain/idempotency.go | Versioned PasswordFingerprint/ReplayPassword sudah ada; safe result belum mencakup semua field detail management |
| Permission/principal | internal/features/common/authorization.go, internal/sessions/principal.go | Resolver authoritative existing, restricted allowlist exact; bukan snapshot izin permanen |
| Contract | api-contracts/proto/user/v1/user.service.proto dan user.payload.proto | Lima UserService RPC; CreateUser/GetUser messages belum method, User punya password field; UpdateUser/ChangeUserStatus belum message |
| Error catalog | api-contracts/pkg/dictionary/user/errors.go | resource_busy, idempotency_conflict, version_conflict tersedia; status/last-admin errors perlu inventory semantic lebih lanjut pada contract deliverable |
| Schema fixture | tests/integration/postgres/fixtures/schema.sql | INTEGER IDs, status/counters, normalized unique email; profile name100/address250/phone20, password VARCHAR150 |
| Test pattern | tests/features/session/runner_test.go, tests/features/user/runner_test.go dan tests/integration/postgres/ | Gunakan tests/ dan existing scenario harness; PostgreSQL opt-in terpisah |

Graph built_at_commit b89f372 lebih lama dari source; hanya actual source yang
mendukung kesimpulan di atas. Tidak membaca secret env atau menjalankan DB.
Historical SQL README/config context dapat memuat baseline sebelum lifecycle;
generation yang dipakai adalah sessions.auth_version additive milik lifecycle,
bukan klaim sessions.version tersedia. Foundation archives adalah evidence
prerequisite, bukan PASS untuk command Phase 2.

## Goals / Non-Goals

**Goals:** empat admin RPC dengan secure caller/principal, primary snapshot atau
locked recheck, optimistic version, last-admin protection, safe replay dan
mutation/audit/pending event/result atomik. Flow tetap mudah dibaca melalui
fungsi command yang fokus dan dependency kecil.

**Non-Goals:** perubahan sibling di luar contract empat RPC, role/app command, permission
delegation, filter-options endpoint, audit read/EvaluateAccess, REST/frontend,
broker, actual migration/bootstrap/deployment, serta refactor legacy di luar
blast radius. Existing lifecycle dan ListUsers menjadi regression.

## Decisions

### 1. Contract additive sebelum source wiring

Deliverable UM-EXT-01 dikerjakan di api-contracts melalui task 1.1–1.6 dalam
scope plan ini, sebelum dependency update/backend wiring. Bukan sekadar menerima
contract yang dibuat pihak lain. api-contracts tetap owner keempat exact methods
dan dictionary/mapping/locales. Pertahankan CreateUser/GetUser existing tags dan
wrapper; field tambahan memenuhi safe status/version/profile/created_at/
must_change_password response. Legacy User.password wajib kosong. CreateUser
menambahkan status presence PENDING/ACTIVE dan field profil yang diperlukan;
status harus eksplisit agar server tidak menebak request yang omitted.
UpdateUser memakai user_id, expected_user_version dan explicit field_mask/
presence. ChangeUserStatus memakai user_id/status/reason/expected_user_version.
Key berasal dari metadata, bukan field actor/privilege browser.

Batas email/password 6–50, supplied name 3–100, address250 dan phone20 mengikuti
validator/fixture yang diinspeksi; contract examples menggunakan safe dummy data.
Proto tags baru ditetapkan dalam pekerjaan api-contracts, bukan dikarang di sini.
Error target-not-found, duplicate account, illegal transition, self-inactivation
dan LAST_ADMIN_REQUIRED menggunakan semantic catalog yang diverifikasi di sana;
reuse existing errors bila sesuai dan jangan remap legacy secara global.

Alternatif memakai message lokal atau menghapus password tag ditolak karena
source ownership/backward compatibility. Urutan kerja contract: inventory dan
additive design → implement GetUser/CreateUser → implement UpdateUser/
ChangeUserStatus → dictionary/mapping/locales → `make proto` → targeted/full
contract tests dan `go vet ./...` → generated/compatibility/diff review →
publication → dependency update `grpc-auth` → workspace dan GOWORK=off.
Regenerasi ulang harus tidak mengubah output; generated file tidak diedit manual.
Test contract berada di api-contracts/tests/ mengikuti runner/scenario existing.
Repository contract tidak mengimplementasikan business authorization atau query
database; tests backend tetap diperlukan untuk membuktikan behavior RPC.

Publication disiapkan setelah seluruh source/generated/test/docs review; eksekusi
commit/push normal mengikuti otorisasi Git yang berlaku dan branch existing,
tanpa tag/release/merge/force-push. Catat commit/module/checksum dan verifikasi
module published dapat diambil standalone sebelum task 1.7. Tidak ada fitur
runtime untuk pin contract; module version hanya dependency build existing.

### 2. Secure entry dan consistent primary reads

Semua method memerlukan TLS opaque service token, exact method + namespace IAM
allowlist, verified ACTIVE unrestricted principal dan exact permission pada
proposal. GetUser = iam.user.read; CreateUser = iam.user.create; UpdateUser =
iam.user.update; ChangeUserStatus = iam.user.status.update. Jangan percaya actor
dari payload atau numeric admin code. Handler mapping/validation tetap tipis;
business validation/current authority dimiliki feature.

GetUser memakai injected primary read transaction/snapshot seperti ListUsers/
session lists: verify actor/session/current permission lalu target/profile/
active collections pada snapshot yang sama, dengan request context sampai query.
Feature menutup snapshot pada semua paths. Target not-found setelah authz;
dependency failure UNAVAILABLE, bukan empty result. Alternatif pool reads
terpisah ditolak karena bisa menggabungkan authority dan response berbeda.

### 3. Command flow dan lock ownership

Begin/TryLock/Commit/Rollback tetap dipanggil di features atau features/common;
provider teknis tetap pkg/database/ORM existing, pool dipakai ulang. Extend
feature user dengan dependencies yang sempit; reader/writer menerima tx yang
sama. Tidak ada lifecycle/locking mandiri atau fallback pool setelah lock.

Flow: verify service/input/admission → Begin(ctx) → keyed idempotency lock bila
ada → last-admin guard bila status command → policy guard → normalized account
lock untuk creation → actor/target user ascending → contributing/default role
ascending → application ascending → actor/target session canonical → authoritative
recheck → replay → target expected version → business guard/no-op → writes →
Commit → response. Discovery IDs dibaca ulang di bawah lock; jika perlu lock
predecessor, rollback/restart sesuai helper existing. Tx tidak dipakai untuk
concurrent writes atau external network.

ChangeUserStatus mengambil global last-admin sebelum policy untuk seluruh
status commands agar lock set tidak bergantung pada stale discovery. Recount
khusus effective removal menghitung ACTIVE auth + ACTIVE iam.platform_admin
assignment, menghormati admin_guard baseline. Contention terbatas merupakan
trade-off local MVP. Semua jalur Phase 2 berikutnya yang dapat menghilangkan
admin wajib mengikuti guard yang sama. Alternatif count-before-lock atau
per-target guard tidak menjaga invariant lintas dua target.

CreateUser shares normalized account uniqueness dengan Register. Siapkan hash
di luar write transaction bila diperlukan, lalu account uniqueness/authority
dibaca ulang dalam tx; keyed replay tidak perlu mengulang hash computation.
Default nonadmin member dapat memakai creation helper existing tanpa membuka
privilege payload; status dan must_change_password dibentuk server.

### 4. Version dan mutation persistence matrix

| Command | Version/auth_version | Audit | Required pending outbox | Safe result |
| --- | --- | --- | --- | --- |
| CreateUser | initial 1/1; restricted=true | USER_CREATED | iam.user.created.v1, aggregate version1 | wajib, tanpa credential |
| UpdateUser effective | user+1/auth tetap | USER_PROFILE_UPDATED | iam.user.profile_updated.v1, committed user version | jika key diberikan |
| Status activate/reactivate | user+1/auth tetap | USER_STATUS_CHANGED | iam.user.status_changed.v1 | wajib |
| Status suspend/deactivate | user+1/auth+1, revoke sessions/families | USER_STATUS_CHANGED | satu iam.user.status_changed.v1, sessions_revoked=true | wajib |
| Valid no-op | counter tetap | satu NO_OP | tidak ada | jika key berlaku |
| Identical replay | tidak ada write domain | tidak ada audit baru | tidak ada | result pertama |
| GetUser | read-only | tidak ada mutation audit | tidak ada | bukan replay cache |

Expected user version diperiksa sebelum no-op, sesudah replay. Conditional
updates harus affect satu row untuk perubahan entity; zero rows bukan success.
Overflow counter menghasilkan safe error dan rollback. Status revoke semua
session/family termasuk family latest revoked; jangan menghasilkan event global
atau per-session tambahan pada status outcome. Historical assignment tetap.
Reactivate tidak mereset auth_version atau sesi; permission eligible dapat
efektif kembali sehingga detail memberi admin informasi akses sebelum perubahan.

Envelope existing correlation/causation/trace dan data aman dipertahankan.
Reason bebas terbatas berada di audit, bukan raw outbox reason. Event pending
tanpa RabbitMQ. Membuat session/token saat CreateUser ditolak karena restricted
Login adalah producer credential existing.

### 5. Persistent idempotency dan password digest

CreateUser/ChangeUserStatus wajib key, UpdateUser opsional. Scope actor+full
method+account/{trim-lowercase-email} atau user/{canonical-id}+key, retention24h.
Fingerprint memuat normalized status/profile/target/presence/version; mask
diproses sebagai canonical selected set tanpa kehilangan arti omitted/empty.
Trace/deadline dikecualikan. Lookup/replay setelah current authority, sebelum
target version; scope expired di-replace atomik di bawah idempotency lock.

Reuse Fingerprinter.PasswordFingerprint/ReplayPassword existing untuk creation.
Sediakan typed env.yaml config active digest version + retained keyring dari
secret terpisah minimal32byte; bukan JWT key atau AuthLimiter.AccountHMACSecret.
Startup fail fast untuk konfigurasi tidak valid; retained versions tidak dihapus
selama unexpired records bergantung kepadanya. Unknown version fail closed
UNAVAILABLE. Jangan salin actual key ke docs/examples/tests; fixture deterministik.

Extend typed SafeCommandResult allowlist hanya untuk safe fields response
management yang benar-benar diperlukan contract, misalnya normalized email,
created_at, restriction flag dan profil. Decode/validate tetap strict; bukan
arbitrary protobuf/JSON map. Safe replay harus mereproduksi response awal dan
tidak membaca ulang mutable detail untuk menambal saved result. Jangan menyimpan
password, hash login atau credential header dalam fingerprint/result/log/audit.
Alternatif hash plaintext tanpa secret rentan offline guessing; alternatif
credential response cache melanggar security boundary.

### 6. Rollback dan failure audit

TransactionOwner.Finish di-defer langsung oleh feature; Commit eksplisit hanya
setelah seluruh required writes. Semua guard/read/conditional update/error,
cancellation dan panic precommit mengakibatkan rollback, termasuk error setelah
write pertama. Cleanup lalu independent failure audit memakai security-attempt
pattern existing, bounded detached context/correlation dan safe reason.
Audit failure tidak menimpa error asli, telemetry bounded tidak membocorkan PII/
credential. Commit outcome unknown tetap dependency error, tidak auto-retry atau
claimed rollback. Existing refresh reuse commit-before-denial tetap regression.

### 7. Admission, wiring dan backward compatibility

GetUser memakai protected-read shared verified-user/group120/minute burst20;
CreateUser/UpdateUser/ChangeUserStatus memakai admin-mutation shared actor/group
30/minute burst5. Reuse limiter numeric/cap10000/idle15m/cleanup1m dan RetryInfo.
Invalid request masuk applicable verified admission; limiter bukan pengganti
permission. Wiring default server menambahkan empat RPC menjadi19 tanpa
mengubah service token requirement, locale/error chain atau membuka restricted
allowlist. Request context dan synchronized random/hash dependencies dipertahankan.

ListUsers seluruh sort/filter/search/cursor/total yang diarsipkan tetap sama.
Register PENDING/token field1 kosong tanpa session; EdDSA-only access15m bounded
session7d, JWTskew30s tidak memperpanjang absolute boundary, issuer ApplicationName,
audience ignored; strict refresh/no grace dan all-session invalidation tetap.
Consumer IP sanitization/storage/cutover tetap Phase6, bukan blocking backend task.

## Risks / Trade-offs

- [Contract belum tersedia] → kerjakan task pembuatan 1.1–1.6 yang kini masuk
  scope; dependency/wiring menunggu publication tervalidasi, jangan klaim message
  berarti RPC sudah ada.
- [Legacy User memiliki password field] → safe mapping allowlist dan registered
  response serialization test memastikan field selalu kosong.
- [Global guard meningkatkan contention] → correctness local MVP diutamakan,
  bounded busy response; optimasi membutuhkan proof baru.
- [Baseline SQL docs berbeda dari fixture/session-generation] → compare hanya
  metadata relevan secara read-only; migration gap menjadi prerequisite eksplisit,
  tidak menjalankan script aplikasi otomatis.
- [Concurrent actor invalidation/last-admin removal] → guard protocol, locked
  recheck dan dedicated PostgreSQL two-connection barriers.
- [Key rotation menghilangkan replay proof] → retained key version sampai window
  habis dan unknown-version dependency denial; secret tidak masuk artifact.
- [Existing changes belum committed] → preserve working tree; evidence mencatat
  source digest selain published module version, tanpa commit otomatis.

## Migration Plan

1. Kerjakan api-contracts deliverable melalui task 1.1–1.6: empat RPC/messages,
   safe fields, dictionary/mapping/locales, generator/test/static analysis,
   documentation/review dan publication sesuai otorisasi Git. Lanjut task 1.7
   dependency update grpc-auth dan standalone resolution. Publication telah dieksekusi;
   commit/module/checksum dan hasil standalone ada pada contract inventory.
2. Cocokkan account/profile/search/index/admin_guard/session-generation metadata
   read-only. Schema additions hanya jika gap terbukti dan scope eksplisit;
   prepared migration tidak dijalankan ke database aplikasi pada increment ini.
3. Implementasi backend + controlled/opt-in DB verification, record actual
   evidence, validate dan sync/archive. Backend boleh selesai tanpa DEP-01 PASS.
4. Deployment Phase6 mengecek actual schema/admin/key/caller/consumer version
   set, release/rollback; rollback tidak memulihkan revoked credential atau
   menurunkan auth_version. Tidak ada otomatis bootstrap, promotion user pertama,
   HS256 fallback atau deployment karena dokumen sudah lengkap.

## Acceptance ledger

UM-01..10 **PASS**, owner grpc-auth. Actual commands, source manifest dan
mapping test ada di [evidence](evidence.md). Kolom berikut mempertahankan
acceptance scope. UM-EXT-01 **PASS**: published contract task1.1–1.6 dan dependency
1.7 tersedia; [inventory](contract-inventory.md) mencatat commit/module/checksum.

| ID | Scope / AC asal | Prerequisite | Test/evidence wajib |
| --- | --- | --- | --- |
| UM-01 | Four exact methods, M-05/AC-31 | UM-EXT-01, auth foundation | Registered gRPC TLS/caller/method/namespace/session/restricted/exact permission positives/negatives |
| UM-02 | GetUser, M-05/06 | UM-01, primary snapshot | Detail/nonexistent/pending/deleted target, collections, concurrent policy snapshot, safe serialization/dependency failures |
| UM-03 | CreateUser, AC-33/M-07/08 | Contract, digest key, account schema | PENDING/ACTIVE, normalized duplicate, restricted login→ChangePassword→fresh Login, no privilege/credential response |
| UM-04 | UpdateUser, M-06/07/08/AC-26 | UM-01, presence/version contract | Omitted/empty/mask/forbidden fields, stale version, no-op, profile/search changes visible to ListUsers |
| UM-05 | Safe replay, AC-24/25/26/M-08 | Fingerprinter/keyring, idempotency persistence | Same/different password/presence/target/key, expiry24h, retained/unknown key version, lost response, current authority before replay |
| UM-06 | Atomic command persistence, AC-28/M-07/11 | Feature owner and writers | Failure each read/guard/conditional-write/mutation/audit/outbox/result, unchanged business rows and safe independent failure audit |
| UM-07 | Cleanup/error mapping, M-07/11 | UM-06 | Begin/commit ambiguity/cancel/panic/rollback/failure-audit failure, locale, bounded telemetry and pool release |
| UM-08 | Numeric admission, M-10 | Existing limiter | Shared read/admin group quotas, invalid-input admission, RetryInfo, rejected request has no business effects |
| UM-09 | Status/restricted, AC-02/14/15/33/34 | UM-01/05/06, immutable session generation | Every valid/invalid transition, self guard, active/reactivated login, revoked proof rejection, exact counters/audit/one status event |
| UM-10 | PostgreSQL/concurrency/regression, AC-19/20/24..26/M-09 | Dedicated test DB + all backend tasks | Two physical connections/barriers for last-admin, actor invalidation, duplicate creation/replay, Login/Refresh/status; 15 prior RPCs, dynamic list and full suites |

UM-EXT-01 owner api-contracts: four methods, safe additive messages, catalog/
mapping/locales, generated tests/static analysis and published version, dikerjakan
pada task 1.1–1.6 dalam plan ini. Completion dibuktikan contract inventory,
generated/compatibility/error/locale test report, static analysis, reviewed diff
dan published commit/module/checksum yang dapat diambil standalone. Consumer compatibility
EXT-02 remains Phase6 DEP-01. Phase2 C must add cross-command status-versus-admin
role-revoke races; completion UM-10 only covers commands available in A.

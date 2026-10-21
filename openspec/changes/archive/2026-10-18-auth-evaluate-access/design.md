# Design

## Context

Motivasi dan scope ada pada [proposal](proposal.md). Inspeksi dilakukan terhadap working tree, bukan hanya graph atau dokumentasi historis. Graph pada inspeksi mencatat `built_at_commit=2462cb09d0f8e7eef3b17a70456451b8bd2dd6ec`; degree tertinggi mengarah ke domain auth/outbox/user-role/transaction. Routing graph kemudian diverifikasi pada source berikut:

| Existing source | Fakta relevan / reuse |
| --- | --- |
| `internal/features/common/authorization.go` | `ResolvePermissions` menjaga union dan active business gates; `HasPermission` membandingkan exact code tanpa admin/JWT/cache shortcut |
| `internal/repositories/reader/authorization.go` | Kontribusi role/assignment/application/access berasal dari ORM dan mengecualikan `rp.deleted_at IS NOT NULL` |
| `internal/features/user/effective_permissions.go` | Model read snapshot dan cleanup tersedia; descriptive nonactive-target view bukan evaluator session user |
| `internal/features/auth_feature/session.go`, `internal/sessions/principal.go` | Reauthentication terhadap snapshot, current user/entity version, session binding/expiry/auth_version dan restricted policy |
| `internal/repositories/reader/role_management.go`, `application_access.go` | Lookup application by code serta user/application access pada injected handle sudah tersedia; not-found dan absent access dibedakan dari dependency error |
| `pkg/database/transaction.go`, `internal/features/common/transaction*.go` | Primary RepeatableRead READ ONLY snapshot dan leased cleanup; lifecycle tetap di feature/common owner |
| `internal/configs/foundation.go`, `register.service.go`, `roles.go` | Satu composition root berbagi provider/clock/pool dan meregistrasikan 35 methods pada tujuh service |
| `internal/interceptors/{session_interceptor,rate_interceptor,security_attempt}.go` | Explicit protected method lists, shared quota dan safe error audit; saat ini response `err=nil` tidak diaudit walaupun isinya mungkin deny |
| `tests/integration/auth_reads/test_scenarios/effective_permissions.go`, `tests/integration/postgres/test_scenarios/effective_permissions.go` | Registered TLS/controlled SQL harness, fresh-read regression serta two-backend snapshot/cleanup tests dapat diperluas |

Checkout sibling `api-contracts/proto` belum memiliki AuthorizationService/EvaluateAccess. Published pin grpc-auth adalah `v0.0.0-20261014032725-545701c82e22`; ORM sudah `v1.8.0-support-subquery`. Baseline config OpenSpec yang menyebut enam RPC/ORM lama bersifat historis; source dan evidence increment E/ORM menjadi routing kondisi saat ini. Tidak ada perubahan sibling atau implementation dalam proposal ini.

## Goals / Non-Goals

**Goals:** satu keputusan self-permission dari current primary snapshot, privacy service namespace sebelum lookup aplikasi, deny reasons deterministik, bounded admission/attempt audit serta evidence melalui registered production path.

**Non-Goals:** evaluasi cross-user atau service-only, resource authorization domain, cancellation pekerjaan yang telah authorized di service lain, permission cache/replay, mutation/last-admin locks, schema/index migration, ListAuditLogs, REST/UI/CLM implementation, relay dan deployment.

## Decisions

### 1. Kontrak additive milik api-contracts

Rencanakan `proto/authorization/v1/authorization.{service,payload}.proto` dan generated packages di repository contract melalui generator resminya. Full method tepat `/authorization.v1.AuthorizationService/EvaluateAccess`.

| Message | Proposed new field tags/types |
| --- | --- |
| `EvaluateAccessRequest` | `application_code=1 string`, `permission_code=2 string` |
| `EvaluateAccessResponse` | `allowed=1 bool`, `reason=2 AccessDecisionReason`, `user_id=3 uint64`, `sid=4 string`, `user_version=5 uint64`, `evaluated_at=6 google.protobuf.Timestamp` |
| `AccessDecisionReason` | `ACCESS_DECISION_REASON_UNSPECIFIED=0`, `..._ALLOWED=1`, `..._PERMISSION_MISSING=2`, `..._APPLICATION_INACTIVE=3`, `..._APPLICATION_ACCESS_REQUIRED=4` |

Zero enum bukan keputusan valid dan tidak dikeluarkan server. New messages tidak memakai field target/user identity input, expected version, permission list, pagination atau idempotency key. Sid pada response adalah session identifier milik verified user, bukan access/refresh credential. Tidak ada credential/hash/role union/profil pada response. Preserve seluruh prior field numbers/types dan RPC termasuk TesRPC.

Gunakan existing common/auth validation, forbidden, invalid-session, dependency dan limiter catalogs. Deny reasons adalah enum hasil evaluasi, bukan error catalog baru atau remapping global. Jika contract verification menemukan kebutuhan error baru yang nyata, pemiliknya namespace authorization di api-contracts dengan mapping/locale resmi; tidak membuat registry/alias lokal.

**Alternatif:** menambah method UserService mencampur identity read dengan keputusan lintas aplikasi dan tidak mengikuti full method MVP; string reason bebas tidak membatasi vocabulary. Keduanya tidak dipilih. Cross-repository code/generation/dependency/publication tetap pekerjaan eksplisit saat apply, dengan otorisasi Git terpisah; transport backend BLOCKED sampai contract published/pinned tersedia.

### 2. Required exact inputs dan service namespace sebelum application lookup

Trim kedua kode, tanpa auto-lowercase. Application memakai existing aturan lowercase `^[a-z][a-z0-9_]*$`, panjang 1–50. Permission mengikuti existing `^[a-z][a-z0-9_]*([.][a-z][a-z0-9_]*){2,}$`, panjang 1–150, sehingga resource multisegmen tetap diterima. Namespace segmen pertama harus sama dengan application code. Nil request, kosong, wildcard, uppercase, invalid segment atau mismatch menghasilkan INVALID_ARGUMENT melalui common/structured field errors yang tersedia.

Service interceptor tetap memverifikasi TLS/token/exact method. Feature memeriksa `Caller.AuthorizeNamespace(application_code)` sebelum snapshot/application/access/contribution lookup. Caller bisnis yang hanya berwenang `clm` boleh mengevaluasi `clm.*` tanpa namespace IAM atau `iam.user.read`; identitasnya tetap user aktif. Jangan menambahkan method ini ke kelompok yang memaksa IAM di SessionInterceptor.

**Alternatif:** optional application/all-namespace seperti increment E tidak cocok dengan single exact decision; mencari aplikasi sebelum namespace check mengungkap catalog kepada caller yang tidak berwenang.

### 3. Feature read eksplisit dengan provider existing

Tambahkan input/result dan typed decision reasons pada domain/types existing sesuai tanggung jawab. Tempatkan satu fungsi snapshot pada `internal/features/authorization/evaluate.go`. Gunakan parameter claims/input, `domain.ReadSnapshotProvider` dan controlled evaluation time sebagaimana pola snapshot reads; tidak membuat service locator, generic policy engine atau transaction framework.

Thin `internal/handler/authorization.go` menerima snapshot provider dan clock, memvalidasi constructor, mengambil verified principal claims, memanggil feature dan memetakan safe result ke protobuf. Composition pada file authorization di `internal/configs/` membentuk handler satu kali dan mendaftarkan generated AuthorizationService. Bind concrete session, application/access dan permission readers pada transaction snapshot yang sama. Reuse application lookup existing melalui small consumer-facing contract bila diperlukan; query/transport detail tidak masuk feature. Tidak mengubah semantik existing RoleManagementReader atau resolver.

Alur feature: validate/context → verified caller namespace → BeginSnapshot(request context) → current AuthenticateSession → AuthorizeMethod → lookup application → business access gate jika relevan → ResolvePermissions/HasPermission pada current user → bentuk result → commit snapshot → return. Ambil user ID/sid/user version/evaluation time dari principal hasil recheck, bukan principal lama, input, JWT role atau payload metadata. Evaluation time UTC adalah instant recheck dengan injected clock, bukan expiry atau jaminan valid hingga commit domain lain.

Feature/common owner memanggil BeginSnapshot/Commit dan deferred Finish/Rollback. Repository hanya read via injected query handle; tidak Begin/Commit/Rollback/TryLock, fallback pool, external call atau concurrent query pada shared tx. Tidak ada TryLock/advisory guard: read-only MVCC tidak memerlukan mutation lock, expected version atau idempotency. Lock order, uniqueness, last-admin dan refresh commit-on-denial tetap milik mutation existing dan tidak diubah.

**Alternatif:** memanggil RPC GetMyEffectivePermissions atau menyalin logic union menambah transport/semantic coupling. Memakai interceptor principal saja dapat menerima stale access; keduanya ditolak.

### 4. Gate dan deny precedence

User memilih unknown application → PERMISSION_MISSING pada 18 Oktober 2026. Setelah identity/restricted/namespace/validation berhasil, urutannya:

| Current snapshot state | Output |
| --- | --- |
| Application lookup menyatakan not found | false / PERMISSION_MISSING; tidak dianggap dependency atau NOT_FOUND RPC |
| Application terdaftar dengan status DISABLED | false / APPLICATION_INACTIVE |
| Business application ACTIVE, access absent atau REVOKED | false / APPLICATION_ACCESS_REQUIRED |
| Application/access memenuhi gate, exact effective permission tidak ada/unknown | false / PERMISSION_MISSING |
| Exact permission ada dalam authoritative union | true / ALLOWED |

IAM tidak memerlukan application access, tetapi exact IAM permission dan ACTIVE application tetap diperiksa. Inconsistent application/access row yang bukan domain absence/status valid merupakan UNAVAILABLE, bukan false result. Reader not-found disaring secara spesifik; dependency error tidak disamakan dengan missing application. Missing privilege tidak memakai `AuthorizeCurrentPermission` secara langsung karena helper tersebut menghasilkan gRPC forbidden; gunakan authentication/restricted check yang sama lalu resolver existing untuk hasil boolean.

Role/permission application IDs dan namespace harus konsisten; revoked assignment, disabled role, soft-deleted role-permission pair, wildcard atau forged admin code tidak berkontribusi. Overlap role tetap memberi izin selama satu source yang sah tersisa. No active source tidak berarti aplikasi/access hilang: alasan harus berasal dari gate lookup eksplisit, bukan menebak empty join.

### 5. Snapshot consistency dan failure boundary

Primary RR READ ONLY mengikat state actor/session/version, aplikasi/access dan permission rows. Snapshot baru yang established setelah revoke/status/mapping commit melihat state terbaru dengan JWT lama yang masih valid; snapshot in-flight boleh selesai dengan state sebelumnya yang konsisten. Tidak ada cross-database commit fence untuk CLM atau access ticket/cache TTL.

Begin/read/commit/rollback dependency failure → UNAVAILABLE, cancellation → CANCELED, deadline → DEADLINE_EXCEEDED; unexpected panic → redacted INTERNAL. Deferred cleanup menutup lease sebelum return/attempt audit. Outer result-suppression guard mengikuti pola E: setiap error atau panic membersihkan result agar handler tidak mengeluarkan partial/allowed response. Commit failure tidak mengembalikan hasil allow maupun deny. Test memastikan dependency cause/identity tidak hilang dan cleanup/pool reusable.

**Alternatif:** pool reads terpisah dapat menggabungkan session lama dan policy baru; snapshot cache dapat melewati revoke. Keduanya tidak memenuhi immediate-next-evaluation semantics.

### 6. Admission dan safe denial audit

Tambahkan exact method pada protected session switch dan `rateKeys` ProtectedRead group, memakai bucket verified user yang sama dengan reads existing: 120/menit, burst 20; TTL 15 menit, cleanup 1 menit dan cap 10.000 tetap. Invalid typed input setelah valid identity tetap mengonsumsi quota. Unverified user tidak mengalokasikan protected bucket. RESOURCE_EXHAUSTED/RATE_LIMITED dan RetryInfo/locale existing dipertahankan; quota tidak mengganti current feature recheck.

`NewSecurityAttemptInterceptor` saat ini hanya mencatat `err != nil`. Perluas secara sempit untuk exact EvaluateAccess method dan typed generated response `allowed=false` setelah next selesai tanpa error. Reuse safe recorder dan action SECURITY_ATTEMPT_FAILED dengan existing REQUEST_DENIED, verified actor_service dan RPC target, tanpa raw request/permission/sid/token atau reason bebas. Tidak membuat error untuk mengganti wire deny; error path tetap diaudit sekali melalui jalur existing. Response nil/inconsistent adalah internal failure, bukan fabricated decision. Writer transaksi independen tetap melalui `features/common.RecordSecurityAttempt`, setelah snapshot selesai; tidak ada write pada READ ONLY tx.

Audit write failure/panic hanya menaikkan existing bounded audit-failure observation dan mempertahankan false/error asli; tidak pernah mengubah denial menjadi allow. Allowed read tidak menulis audit sukses. Rate rejection tetap telemetry bounded tanpa audit DB per rejection; method-denied request sebelum attempt interceptor mempertahankan existing service boundary, tidak membuat actor palsu. Denial volume dibatasi admission existing; tidak menambah quota/telemetry infrastructure.

**Alternatif:** hanya mencatat transport errors akan kehilangan successful-transport authorization denials. Menulis audit dalam snapshot merusak read-only semantics. Mencatat ulang di handler dan interceptor menduplikasi audit.

### 7. Compatibility dan state-effect matrix

| Outcome | Business/session/refresh/version/auth_version | Audit | Outbox / idempotency |
| --- | --- | --- | --- |
| allowed=true | Tidak berubah | Tidak ada success audit | Tidak ada |
| allowed=false | Tidak berubah | Satu safe independent denied attempt; best effort | Tidak ada |
| gRPC error setelah verified service boundary | Tidak berubah | Existing independent safe attempt sekali, setelah cleanup | Tidak ada |
| Rate rejection | Tidak berubah | Existing bounded limiter telemetry | Tidak ada |
| Response loss/repeated request/incidental idempotency metadata | Evaluasi snapshot baru; tidak ada replay credential/decision | Mengikuti outcome tiap evaluasi baru | Tidak membuat/consume saved result |

Entity `user_version` respons bersifat deskriptif. Role metadata/mapping changes tidak selalu menaikkan user version: consumer tidak boleh menggunakannya sebagai policy revision, cache validator atau expected-version token. Role/app/permission mutations tidak mencabut session/auth_version; password/global revoke/suspend/deactivate tetap membatalkan credential lewat flow existing.

Tidak mengubah EdDSA-only verifier/issuer ApplicationName, ignored audience, access 15 menit dibatasi session expiry, absolute session 7 hari atau skew 30 detik. Restricted allowlist hanya Me/ChangePassword/Logout. Register PENDING/token-empty, Login credential-only response, refresh single-use/reuse commit-on-denial, TesRPC dan existing pagination/cursors dipertahankan.

### 8. Acceptance dan evidence ledger

Semua ID berikut adalah scope increment ini, bukan deklarasi full AC lintas sistem PASS. Evidence saat apply disimpan pada `evidence/progress.md` dengan HEAD/working-tree digests, exact published contract/ORM/checksum, command/mode, safe environment, waktu, expected/actual dan exit. Status proposal tidak berasal dari hasil test rename fixture sebelumnya.

| ID | Scope / owner | Prasyarat | Required test / evidence | Status proposal |
| --- | --- | --- | --- | --- |
| EVA-EXT-01 | EXTERNAL_REPOSITORY api-contracts; EXT-01 | Scope contract dan publication tersendiri | Descriptor/tag/enum compatibility, generator clean round-trip, catalog/locales, published standalone resolution | BLOCKED: contract belum tersedia |
| EVA-01 | MVP_REQUIRED grpc-auth; M-01/02/03/05/06, AC-31 | Contract dan service/session foundation | Nil/malformed/namespace/caller/JWT/restricted/identity override registered and feature tests; no IAM requirement for business caller | NOT_STARTED |
| EVA-02 | MVP_REQUIRED grpc-auth; AC-09/10/11/12/13/23 | Current application/access readers dan resolver | Decision table, IAM exception, unknown app choice, overlap, soft-delete/cross-app/no-bypass and safe response tests | NOT_STARTED |
| EVA-03 | MVP_REQUIRED grpc-auth; AC-13/23/27, M-02/07/09 | Primary provider/cleanup owner | Begin/read/finalization faults/cancel/panic tests plus two-physical-backend snapshot/next-evaluation evidence | NOT_STARTED |
| EVA-04 | MVP_REQUIRED grpc-auth; M-05/10 | Shared ProtectedRead limiter | Mixed old/new requests, invalid input counts, RetryInfo/locale, existing cap/TTL/cleanup regression | NOT_STARTED |
| EVA-05 | MVP_REQUIRED grpc-auth; M-07/11 | Existing safe independent audit | False-result audit once after cleanup, audit failure retains deny/error, no rate audit, business/outbox/idem counts unchanged | NOT_STARTED |
| EVA-06 | MVP_REQUIRED grpc-auth; AC-09/11/12/13/23/27 | B/C/D commands and dedicated opt-in test DB | Original JWT after real command revoke/disable/replace/soft-delete/regrant; pool cleanup and default registered path evidence | NOT_STARTED |
| EVA-07 | MVP_REQUIRED grpc-auth; regression/EXT-01 | Published contract and implemented evaluator | Exactly 36 methods/eight services, prior35 retained; workspace/standalone targeted/full/vet/race/build and dedicated PostgreSQL normal/race gates | NOT_STARTED |
| DEP-01 | EXTERNAL_REPOSITORY consumer/deployment, Phase 6 | Mandatory backend increments | Actual consuming-client/key/TLS/caller/schema/cutover evidence | Tetap terpisah; belum PASS |

Default test mengikuti root `tests/` runner/test_scenarios/mocks/fixtures dengan controlled SQL/TLS in-memory dan injected time; tanpa network/DB eksternal. Dedicated `-tags postgres` suite memakai harness existing dengan disposable test namespace, explicit test opt-in, dua physical backend IDs dan channel barriers. Tidak memakai Docker, database aplikasi atau migration baseline. Test DB unavailable membuat gate EVA-03/06/07 terkait BLOCKED; mocks atau skipped suite tidak menggantikannya. Full mutation reconciliation dan audit-list acceptance belum ditutup oleh evaluator ini.

## Risks / Trade-offs

- [Graph identity collisions/freshness dapat menghilangkan kandidat] → graph hanya routing; seluruh keputusan berdasarkan source yang dibaca.
- [Empty permission join tidak menjelaskan application/access gate] → lookup kedua gate secara eksplisit pada handle yang sama, reuse existing readers dan bedakan not-found dari dependency.
- [Deny response lolos audit error-only existing] → typed exact-method extension dengan count/order tests dan safe REQUEST_DENIED; tidak memperluas generic response reflection.
- [User version tampak seperti authorization revision] → dokumentasikan descriptive-only dan wajib evaluasi baru; role changes tidak menjadi cache invalidation promise.
- [Unknown app denial reason berbeda dari E list semantics] → E tetap mengembalikan empty descriptive union; evaluator mengikuti keputusan user PERMISSION_MISSING tanpa mengubah RPC E.
- [Perubahan allowlist runtime memberi caller akses tanpa scope] → example/config docs hanya exact new method; actual private caller changes dan rollout tetap DEP-01, tanpa wildcard atau edit secret lokal.
- [Dependency publication/test DB tidak tersedia] → tandai gate dependent BLOCKED, pertahankan pekerjaan yang tidak bergantung padanya; tidak membuat dummy contract atau mengurangi security gate.

## Migration Plan

Tidak ada schema migration atau runtime credential invalidation. Urutan delivery: explicit contract scope → generate/test → publication sesuai otorisasi → pin published module → implement/register evaluator dan exact allowlist example/docs → workspace/standalone backend gates dan evidence → review. Main specs hanya disinkronkan melalui workflow sync/archive berikutnya.

Deployment/cutover tetap Phase 6 DEP-01. Compatible rollback boleh menghilangkan method baru sambil mempertahankan guard mandatory serta state/version/session terbaru; consumer menerima unavailable/unimplemented dan menolak aksi terlindungi. Tidak membuka legacy JWT, namespace wildcard, stale-cache allow atau memulihkan revoked sessions sebagai fallback.

## Implementation status

Backend COMPLETE on 18 October 2026; all 24 tasks and required workspace/standalone contract, default/race/vet/build and dedicated PostgreSQL normal/race gates PASS. Planning baseline tables above are historical. See [implementation evidence](evidence/progress.md) for immutable dependency/source identity and actual results. Phase 6 DEP-01 remains separate; main specs were synced and this change was archived on 18 October 2026.

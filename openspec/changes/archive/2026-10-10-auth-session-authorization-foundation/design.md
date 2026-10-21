# Design

## Context

Motivasi dan scope terdapat di [proposal](proposal.md). Baseline source/AST adalah commit `b89f37270d019c8d0d7823a3ff5d3526874d2232`. Foundation transaction yang diarsipkan menyediakan ownership, TryLock, audit/outbox/idempotency dan harness PostgreSQL, bukan persistent authentication yang sudah aktif.

Pipeline `internal/configs/server.go` sekarang memasang Language/Error/Token interceptor tanpa inbound TLS. `internal/interceptors/auth_interceptor.go` memverifikasi HS256 dan menyimpan hanya UserId melalui `internal/sessions`. `pkg/jwt` menghasilkan token tanpa sid; Login belum membuat session. Register sekarang menghasilkan token; UpdateProfile sudah memakai transaction/lock. Me membaca beberapa repository paralel dengan shared error variable. Model Go auth/role/assignment belum membawa seluruh field target.

Production provider memakai request context, ORM v1.7.0 dan PostgreSQL ReadCommitted. Statement yang berbeda belum menjamin snapshot bersama. Graph structural hubs adalah domain auth/profile/role serta feature auth; perluas contract consumer di sana, jangan membuat service locator atau framework transaksi baru. Config tetap typed `env.yaml`; source generated/error catalog tetap milik api-contracts.

## Goals / Non-Goals

**Goals:** membangun trust boundary service/user yang dapat dipakai feature existing dan command berikutnya; authorization read dan data mempunyai snapshot bersama; mutation recheck setelah locks; actor audit berasal dari trust boundary yang benar.

**Non-Goals:** implementation issuer/refresh/password/logout/revoke lifecycle Phase 3, management RPC Phase 2, permission/application definition CRUD, broker relay, REST/UI, migration/bootstrap otomatis. Menulis proposal ini tidak memberi izin mengubah Go, SQL, proto, dependency, sibling, atau database.

## Decisions

### 1. Pisahkan implementasi foundation, producer prerequisite, dan activation

Dependency graph:

1. Foundation transaction existing → service auth, session contracts/verifier, RBAC snapshot dan bound consumers change ini.
2. Foundation tersebut → increment Phase 3 minimum: real Login membuat session/refresh + SESSION_CREATED audit dan issuer EdDSA, basic auth limiter, contract response terpublikasi.
3. Kedua increment + local key/caller/admin/schema readiness → seluruh registered pipeline dihubungkan dan diuji dengan real Login. Client dan actual deployment readiness tetap mandatory sebelum deployment, bukan blocker source implementation.
4. Phase 2 management dan full Phase 3 lifecycle memakai foundation aktif sesuai dependency masing-masing; tidak perlu menunggu seluruh Phase 2 untuk membangun producer Login.

Unit verifier boleh memakai fixture, tetapi evidence activation harus menggunakan production Login, server wiring, reader/writer dan verifier yang sama. Tidak ada synthetic session, test issuer, token-only Login, HS256 fallback, dual algorithm, atau runtime bypass sebagai pengganti gate. Independent foundation tasks dapat berjalan dulu; source wiring menunggu backend prerequisite, sedangkan deployment tetap BLOCKED sampai seluruh external prerequisite tersedia. Alternatif mengaktifkan guard dahulu ditolak karena semua token Login existing akan gagal tanpa jalur login yang kompatibel.

### 2. Authenticate caller sebelum user dan expensive authentication

Tambahkan inbound TLS server serta typed caller configuration: service ID, digest credential current/previous, expiry/not_after dan exact full-method/permission namespace allowlist. Credential opaque minimal 32 byte cryptographically random; simpan digest, bandingkan constant-time. Rotation previous hanya sampai overlap eksplisit; tidak ada secret default. Key/secret material berasal dari deployment config, tidak dicantumkan dalam artifact/log. Server startup gagal pada config TLS/caller/key yang wajib tetapi invalid; propagate constructor error dengan cleanup resource existing.

Pipeline konseptual: locale/error boundary → verified service caller → basic limiter prerequisite untuk public auth → JWT verification untuk protected method → feature snapshot/policy → command. Pertahankan locale, wrapping, deadline/cancellation. Metadata `x-service-token` terpisah dari `authorization: Bearer`; ambigu/duplikat credential ditolak. TLS client wajib memverifikasi CA/hostname, diuji dengan in-memory transport/config fixture, tanpa menjalankan deployment.

Caller tidak dikenal/expired/credential invalid menghasilkan UNAUTHENTICATED; caller valid di luar method atau namespace menghasilkan PERMISSION_DENIED. Unknown method tidak mendapat policy allow default; RPC yang tidak terdaftar tetap UNIMPLEMENTED sesuai gRPC. Namespace policy siap untuk future EvaluateAccess, bukan RPC placeholder baru. Alternatif shared bearer service/user atau mempercayai serviceName payload ditolak karena mencampur identitas.

### 3. Principal memuat identitas terverifikasi, bukan permission permanen

Ganti UserId-only context menjadi verified caller dan immutable verified user principal (user ID, sid, jti, auth_version, restricted state dan timestamp evaluasi). Parse claims ketat: EdDSA/Ed25519, exact issuer ApplicationName, decimal sub valid, UUID sid/jti, iat/exp/auth_version wajib; legacy user_id bila hadir harus sama dengan sub. Audience sengaja diabaikan. Access 15 menit dan exp dibatasi session; JWT skew 30 detik tidak memperpanjang expiry database. Session/refresh absolute maksimal 7 hari sejak Login, tidak sliding.

Feature menerima verified claims lalu meminta primary snapshot untuk membentuk authoritative principal: user ACTIVE, session ACTIVE, sid milik sub, expires_at belum lewat dan auth_version sama. Token roles tidak pernah menjadi grant. Missing/invalid credential/state → generic UNAUTHENTICATED; repository failure → UNAVAILABLE dengan cause, timeout/cancellation mengikuti shared mapper. Jangan mengubah error legacy secara global; EXT-01 menyediakan catalog/mapping baru sesuai fitur.

Persistent domain/reader/writer membawa session UUID/user/status/timestamps/revocation/device metadata serta auth status/auth_version/must_change_password/entity version. Writer hanya menerima handle tx dari feature; tidak melakukan Begin/Commit/TryLock. Tidak menambah session version column: terminal revocation tetap conceptual revision 2, global invalidation memakai user auth_version. Refresh hashing/rotation dikerjakan producer/lifecycle follow-up.

### 4. Authorization dan sensitive read dalam satu primary snapshot

Untuk Me/ListUsers, gunakan satu query snapshot terstruktur yang membaca actor+session+permission dan sensitive response data, termasuk hasil kosong. Query harus tetap membawa actor authorization row saat page kosong sehingga denial tidak berubah menjadi empty success. Jangan sekadar memanggil interceptor read lalu list query pada ReadCommitted. Mapping dan deterministic ordering mempertahankan `auth_id`/`last_id`, filters dan numeric role_codes existing. Me tidak lagi menjalankan shared-err parallel reads; semua data safe berasal dari snapshot tersebut.

Jika query ORM tidak bisa merepresentasikan snapshot yang diperlukan, tambah contract kecil read-snapshot provider RepeatableRead read-only yang lifecycle-nya tetap di feature; buktikan kebutuhan melalui SQL/test sebelum memilih alternatif ini. Default pilihan satu statement menghindari perubahan isolation global pada mutation provider. Seluruh query request memakai primary, context dan reusable pool; tidak ada cache allow/read replica.

Union permission hanya dari active user-role assignment + active role + role-permission mapping. Business role juga memerlukan application ACTIVE dan user application access ACTIVE. IAM self-service tidak membutuhkan IAM app access/admin role. Pertahankan contributing role IDs/codes; tidak ada role_code=2 bypass, wildcard, inheritance atau direct grant. Perubahan role/app yang committed sebelum evaluasi berikutnya terlihat tanpa membatalkan session/auth_version. Tidak menjanjikan cancel request yang sudah authorized sebelum commit perubahan.

Exact-method matrix pada activation (semuanya memerlukan service auth):

| Full method | User/policy | Response boundary |
| --- | --- | --- |
| /user.v1.UserService/Register | Tanpa access JWT; credential mode registrasi | PENDING, token field 1 kosong, tanpa session/refresh |
| /user.v1.UserService/Login | Tanpa access JWT; real producer prerequisite memeriksa credential + ACTIVE | Credential hanya sesudah session transaction commit |
| /user.v1.UserService/TesRPC | Tanpa access JWT | Contract existing dipertahankan |
| /user.v1.UserService/Me | Active session, owner dari principal | Restricted hanya identitas/profil minimum; tanpa roles/app/permissions admin |
| /profile.v1.ProfileService/UpdateProfile | Active session, self-owner, tidak restricted | Name/address/phone saja; legacy full replacement |
| /user.v1.UserService/ListUsers | Active session, tidak restricted, iam.user.read | Safe page/filter existing; tanpa credentials |

Restricted full allowlist target hanya Me, `/auth.v1.AuthService/ChangePassword`, `/auth.v1.AuthService/Logout`. Dua method terakhir tetap follow-up, tidak diregistrasikan di sini. RefreshSession ditolak PERMISSION_DENIED tanpa rotation bila restricted. Me tidak dapat dialihkan ke user ID payload. Prefix matching atau implicit allow bagi method baru ditolak; management follow-up wajib mempunyai exact policy dan negative test sendiri.

### 5. Feature mengulang actor authorization setelah locks

UpdateProfile menjalankan Begin(requestCtx), TryLock, recheck user/session/ownership/restricted dan seluruh write pada handle yang sama. Konsistensi dengan command pengubah privilege memakai protocol foundation: idempotency → global last-admin/policy → account/role-code → sorted users (actor/target) → roles → applications → sessions. Register mengunci default-member policy/account sebelum create; profile mengunci actor user/session dengan urutan tersebut. Future admin consumer wajib ikut policy/actor/target guard agar revocation tidak lolos lewat stale interceptor state.

Discovery lock predecessor yang belum dipegang menyebabkan rollback dan bounded restart sesuai foundation; jangan acquire terbalik. ORM TryLock PostgreSQL memakai `pg_try_advisory_xact_lock` pada tx yang sama dan retained sampai commit/rollback. acquired=false → ABORTED/RESOURCE_BUSY; SQL/context failure tetap dependency/cancellation. Setelah lock tidak boleh fallback pool read atau concurrent tx writes. Commit failure tidak mengeluarkan success; rollback/panic/deadline cleanup mempertahankan original cause. Tidak menjanjikan replay bagi response profile/register yang hilang.

### 6. Hubungkan persistence sejak scoped command aktif

| Command | Actor / mutation | Version | Success audit | Required pending event | No-op/replay |
| --- | --- | --- | --- | --- | --- |
| Register | Verified service; user actor null; PENDING auth/profile/default non-admin member | Initial user version; initial auth_version sesuai schema | USER_REGISTERED | iam.user.created.v1, initial aggregate version | Tidak diberi general idempotency; duplicate email business error, tanpa success effects |
| UpdateProfile | Verified service+user; self fields only | Effective change user version +1; auth_version tetap | USER_PROFILE_UPDATED | iam.user.profile_updated.v1, committed user version | Identical replacement: changed=false, satu NO_OP audit, tanpa event/version; tidak menjanjikan credential/general replay |
| Login producer prerequisite | Verified service + credential-proven ACTIVE user; session/refresh creation | User version/auth_version tetap | SESSION_CREATED | Tidak ada event creation dalam catalog | Tidak ada safe result credential/replay; response hilang memerlukan login ulang |
| Future Phase 2 management | Current verified actor/session/permission; exact target | Expected entity version, bukan auth_version | Action atau NO_OP menurut matrix MVP | Sesuai catalog; compound event tanpa duplikasi | Mandatory key sesuai matrix; authorization sebelum replay; satu safe result 24 jam |

Register mutation/audit/event satu commit; tidak generate credential dan tidak otomatis mengangkat user pertama sebagai admin. Pertahankan field number/type token field 1 dengan nilai kosong; registration status/id additive menunggu EXT-01. Profile tetap full replacement ketika mask tidak tersedia; field_mask additive hanya setelah contract dipublikasikan, tidak disimulasikan lokal. Forbidden email/password/status/role/owner tidak masuk writer. Audit/outbox gagal → rollback mutation, tanpa success response. Outbox envelope catalog schema_version=1, correlation/causation/trace dibatasi, actor_user_id null untuk Register; published_at kosong tanpa broker.

Idempotency foundation tidak dihubungkan ke Register/Login/UpdateProfile sebagai general replay. Untuk management consumer berikutnya, scope user actor + full method + canonical target + key <=200; caller service dicatat tetapi bukan unique dimension. Normalize payload/presence/set/expected version; password-bearing fingerprint keyed digest/version, tanpa raw password/hash/token. Auth ulang sebelum same-payload replay, replay sebelum target stale-version check; beda payload conflict ABORTED, beda target beda scope. Record aman 24 jam, exclusion atomik untuk expiry, no-op satu audit tanpa event/version, replay tanpa efek baru. Belum ada production management consumer → AC-24..26 tetap terbuka.

Failed authentication/command memakai independent security-attempt audit setelah rollback; bounded safe correlation dan caller identity hanya bila verified. Audit attempt gagal tidak mengubah denial menjadi allow; telemetry redacted/bounded mencatat failure. Jangan mengarang verified actor untuk unknown credential. Authentication DB failure tetap dependency error. Login invalid/password/user inactive tetap generic UNAUTHENTICATED, tanpa enumeration.

Full refresh lifecycle follow-up mempertahankan strict single-use user/session lock, busy bukan reuse, proven consumed reuse commit revocation/audit/required event dahulu lalu denial; unknown token tidak dapat revoke sid. Response rotation hilang → login ulang, tanpa grace/credential replay. Session expiry tetap absolute; rollback tidak mengaktifkan token/session lama. Contract ini tidak ditandai implemented oleh fixture foundation.

### 7. Limiter dan external prerequisites bukan scope tersembunyi

EXT-01 mencakup additive fields, safe response dan errors/mapping/locale terpublikasi; generated files/pin hanya berubah pada pekerjaan implementasi terotorisasi. Workspace build tidak membuktikan publish: integration wajib diuji juga GOWORK=off.

Basic limiter dimiliki prerequisite increment dan wajib tersedia sebelum Register/Login dianggap production-ready. Gunakan angka MVP section 8: Login IP 30/min burst10 + normalized account HMAC 5/min burst3; Register IP 5/min burst2; protected read user/group 120/min burst20. Future refresh IP 60/min burst10 + verified session 10/min burst2; admin mutation 30/min burst5; password 5/min burst2. Semua applicable buckets consume atomik, controlled clock, idle15m/cleanup1m/cap10.000; no active eviction, new key cap rejection. Missing trusted IP fallback verified service_id; x-client-ip hanya dari rest-orc terverifikasi dengan header sanitization external. RESOURCE_EXHAUSTED/RATE_LIMITED retry detail menunggu EXT-01. Full limiter/hardening evidence tetap follow-up M-10; tidak menyatakan mock quota membuktikan readiness production.

EXT-02 mencakup TLS CA/hostname, caller credential, Register token-empty, cookie/CSRF, sanitized IP dan single-flight refresh/login ulang. Tidak mengedit sibling melalui change ini. Permission/application management RPC Phase 2 bukan prerequisite untuk reader resolver fixtures; production schema/admin state wajib diverifikasi, bukan diasumsikan dari seed.

## Risks / Trade-offs

- [Login legacy tidak compatible dengan guard baru] → prerequisite real producer + coordinated activation; gate BLOCKED tanpa fallback.
- [ReadCommitted mencampur state] → satu statement snapshot untuk read; transaction locked recheck untuk mutation; controlled PostgreSQL interleavings.
- [Schema/model belum lengkap] → preflight kolom/type/index/constraints/session ownership/RBAC dan admin count; gap dilaporkan, migration terpisah eksplisit.
- [Service token bocor melalui config/log] → digest comparison, external secret, expiry/allowlist, safe bounded audit; credential tidak masuk event/result.
- [Guard lolos pada interceptor tetapi privilege berubah] → feature recheck dan shared policy/actor locks; tidak menjanjikan pembatalan request authorized in-flight.
- [Dependency gate dianggap PASS lewat fixtures] → acceptance ledger memisahkan component evidence, real Login integration, external readiness dan full AC follow-up.

## Migration Plan

1. Read-only schema/config/contract preflight; catat versi published dan actual indexes tanpa menjalankan migration/seed/reset. Gap SQL atau ORM menjadi scoped change terpisah yang diotorisasi.
2. Implement service/session/RBAC contracts dan feature consumers serta tests; hubungkan seluruh six methods sekaligus dalam source. Jangan deploy partial protected pipeline.
3. Sebelum source wiring, verify producer Login/EdDSA/basic limiter, EXT-01, local keys/TLS/caller, issuer/TTL/status/auth_version dan minimal satu ACTIVE platform admin. Sebelum deployment, wajib EXT-02: clients menangani PENDING/token kosong, login ulang, sanitized IP dan RetryInfo.
4. Coordinated Phase 6 deployment cutover menonaktifkan credential/session/family legacy sesuai readiness runbook, mengaktifkan EdDSA-only dan seluruh exact method guards bersama real Login. Ops deployment/cutover bukan aksi proposal ini.
5. Readiness gagal sebelum cutover → tahan activation. Sesudah cutover, rollback hanya ke build security-compatible atau stop serving; jangan membuka HS256/sid-less, mengembalikan auth_version atau mereaktivasi sesi revoked.

## Acceptance dan evidence ledger

Ledger berikut memisahkan component evidence, local readiness dan activation evidence. Status artifact OpenSpec lengkap berarti planning ready, bukan PASS behavior. Link [MVP acceptance](../../../../docs/grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence) tetap otoritatif; subset test di sini tidak menutup seluruh AC lintas phase.

| Local ID / AC terkait | Scope / owner | Prerequisite | Required test/evidence | Status / actual evidence |
| --- | --- | --- | --- | --- |
| SAF-01 / M-01, AC-31 subset | MVP_REQUIRED grpc-auth service boundary | TLS/caller config + EXT-01 errors | All six direct methods, wrong/expired/rotated token, method/namespace deny, TLS CA/hostname reject, locale/redaction | BACKEND PASS / complete registered-server proof in server-verification.json; deployment Phase 6 DEP-01 open |
| SAF-02 / M-02, M-03 subset | MVP_REQUIRED grpc-auth verifier/session | Verified service, schema/session model | Algorithm/issuer/sub/sid/jti/time/owner/version/status cases, audience ignored, DB failure, exact expiry with controlled clock | BACKEND PASS / complete registered-server proof in server-verification.json; deployment Phase 6 DEP-01 open |
| SAF-03 / AC-03,09,10,11,13,23; M-05 subset | MVP_REQUIRED grpc-auth authoritative reader | Primary RBAC snapshot | Union/source role, inactive assignment/role/app/access, unchanged session/auth_version, post-commit evaluation, empty page denial | COMPONENT PASS + fresh PostgreSQL snapshot/post-commit proof; management mutation AC follow-up |
| SAF-04 / M-03,05,06; AC-31 subset | MVP_REQUIRED grpc-auth existing RPC consumers | SAF-01..03, contract response | Me restricted minimum, ListUsers permission, self profile ownership, no forbidden fields, unchanged paginator/TesRPC; race regression | BACKEND PASS / complete production Me/ListUsers/Profile registration, primary snapshots and normal/restricted/denial RPC proof; deployment Phase 6 DEP-01 open |
| SAF-05 / AC-01,02 subset,28; M-07,09,11 subset | MVP_REQUIRED grpc-auth scoped commands | Verified actor + actual schema, EXT-01 fields | Register PENDING/no credential; profile version/no-op; all audit/outbox/begin/commit failures, cancellation/panic; pending row counts | BACKEND PASS / registered PENDING Register, locked Profile version/no-op/audit/outbox and independent attempts; management activation AC-02 remains follow-up |
| SAF-06 / M-02,07,09; AC-13 subset | MVP_REQUIRED grpc-auth DB correctness | Dedicated opt-in PostgreSQL | Physical two-connection snapshot/actor-revoke interleavings, lock retention/recheck, failures/pool reuse, exact committed row counts | PASS / fresh dedicated PostgreSQL backend/barrier/atomicity/pool proof in consumer-verification.json |
| SAF-07 / EXT-01 | EXTERNAL_REPOSITORY api-contracts | Explicit separate contract work + publication | Compatible additive fields/error mapping/locale, version/build workspace and standalone | PASS / published v0.0.0-20261010071509-42a9e6fa23ad, workspace/standalone verified |
| SAF-08 / M-02,03,07,10 subset | MVP_REQUIRED grpc-auth Phase 3 prerequisite increment | SAF-02 component ready + SAF-07 | Real Login transaction session/refresh/audit and EdDSA issuer + basic limiter; no token-only producer | BACKEND PASS / real producer archived, registered-RPC/limiter/lifecycle integration accepted; external deployment readiness required |
| SAF-09 / EXT-02; M-01,03 subset | EXTERNAL_REPOSITORY rest-orc/frontend + deployment owner | SAF-07..08, CA/keys/admin/schema readiness | Caller/TLS/client compatibility/login ulang/IP trust evidence dan coordinated cutover rehearsal | BLOCKED / external evidence belum tersedia |
| SAF-10 / M-01..03,05..07 subset; AC-03,31 subset | MVP_REQUIRED grpc-auth backend wiring | SAF-01..08 and local readiness; deployment SAF-09 separate | Actual registered RPC pipeline + real Login integration; old token reject, current protected success/deny, failure mapping and standalone build | BACKEND PASS / complete source, registered RPCs, startup/lifecycle cleanup, workspace/standalone/race/PostgreSQL verified in server-verification.json; deployment BLOCKED at Phase 6 DEP-01 |
| SAF-11 / AC-14..18,24..26,33,34; M-04,08,10 remainder | MVP_REQUIRED follow-up Phase 2/3/5 | Foundation active, contracts/change masing-masing | Full lifecycle/management/idempotency/limiter evidence menurut MVP; bukan fixture proof change ini | NOT_STARTED / di luar implementation increment ini |

Evidence implementation harus mencantumkan commit/module versions, command, waktu, safe environment/test DB, expected/actual assertions dan report location. Tests baru berada di root `tests/`, runner tipis testutils.Run/Scenario; unit/default integration deterministic tanpa network/database nyata. PostgreSQL hanya suite build-tag `postgres` dengan explicit opt-in dedicated DSN/database; minimal dua physical backend, barriers/channels, tanpa Docker/time.Sleep-based ordering. Tidak memakai database baseline atau rerun migration otomatis.

Final implementation checks: gofmt perubahan Go, targeted/default tests, vet, race untuk shared state, opt-in PostgreSQL suites, make build, workspace dan GOWORK=off build/tests setelah dependency published. DB/prerequisite unavailable → BLOCKED. Hasil historical foundation tidak menggantikan evidence baru. Completion increment ini memerlukan backend integration evidence sesuai scope; readiness actual deployment hanya milik Phase 6 DEP-01 dan tidak menahan completion backend Phase 1–5. Ini tidak mengklaim seluruh capability Phase 1/2/3 selesai.

## Consumer resume status — 10 Oktober 2026

Historical component status before user-approved gate separation below.
Independent consumers and dedicated PostgreSQL proof were implemented;
see [current evidence](evidence.md#consumer-components-resume--10-oktober-2026).
Task 7.1 producer prerequisite is delivered and integrated. Task 7.2/7.3 remains
BLOCKED on actual consuming-client/IP/deployment readiness. Local private runtime
key/HMAC/TLS/caller validation now passes; fresh read-only schema/admin/member
inspection and six-method caller proof are recorded in activation-readiness.json.
User has deferred rest-orc work; its evidence remains a mandatory prerequisite. This earlier pause is superseded by the approved gate separation below: task 7.4
may complete backend source wiring; actual deployment remains a separate gate. No Phase 1/2/3 overall
completion or foundation archive is claimed. The baseline description at the
top remains a historical source snapshot.

## Approved source wiring and deployment gates — 10 Oktober 2026

User approved the following refinement while applying this change. Tasks 7.2/7.3
verify grpc-auth backend and actual local configuration/schema readiness. Task
7.4 composes every existing registered method with the real producer and all
security guards in the default source path; task 8.4 closes backend source/test
evidence. No partial method switch, feature toggle, HS256/sid-less acceptance,
fixture issuer or synthetic session is allowed. Tests exercise the production
server construction/registration, failure cleanup and limiter worker lifecycle.

The global MVP Phase 6 DEP-01 remains unchecked until EXT-02 consuming-client/IP/RetryInfo/PENDING/login
ulang and actual deployment key/caller rotation/schema/admin readiness pass.
Source completion is not a deployment action or production-readiness claim;
no deployment, credential invalidation, schema mutation or migration occurs.
SAF-09 denotes that deployment gate; SAF-10 can reach BACKEND PASS with real
registered-RPC tests while DEPLOYMENT remains BLOCKED. Historical evidence is kept.

## Backend completion — 10 Oktober 2026

Task 7.2/7.3 local evidence and task 7.4 full source registration are PASS.
`PrepareFoundation` composes real Login, PENDING Register, Me/ListUsers snapshot
reads and locked Profile. The default server applies Language/Error/Service/
Attempt/Session/Rate, rejects incomplete assembly and legacy tokens, and owns
limiter cleanup cancellation/join. Startup validates security before resources,
returns pool-open failures through cleanup and drains RPCs before closing pools.

[server-verification.json](server-verification.json) records fresh source hashes,
module identity and nine successful verification commands, including workspace
and standalone tests/vet/build, full race tests, full opt-in PostgreSQL and
standalone PostgreSQL race. Registered production RPC tests use real Login over
in-memory TLS with controlled SQL and owned dedicated PostgreSQL schemas. The
local schema/admin/key/caller checks retain their explicit observation scope.
Task 8.4 backend evidence is complete: **36/36** foundation tasks. Deployment readiness is tracked outside this increment at Phase 6 DEP-01 and remains unchecked;
EXT-02 and actual deployment certification are BLOCKED. No deployment, archive,
sibling source change, migration or whole-Phase-1/3 completion is claimed.

## Final-phase deployment ownership — 10 Oktober 2026

The latest user decision moves the former task 9.1, without passing or dropping
its requirements, to [Phase 6 DEP-01](../../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment).
This global checklist owns actual consumer TLS/service credential, IP/proxy,
RetryInfo, PENDING/token-empty compatibility, credential storage/login ulang and
deployment key/caller rotation/schema/minimal ACTIVE admin plus release/rollback.
It remains mandatory and BLOCKED. Completed backend increments may close on
their own evidence while that final-phase gate remains open. Historical reports
showing 36/37 retain the original task allocation, not a deployment PASS.

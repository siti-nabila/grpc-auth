# Design

## Context

Motivasi ada di [proposal](proposal.md). Inspeksi read-only 10 Oktober 2026 menemukan:

| Area | Fakta source saat proposal awal (historis) | Reuse / gap spesifik |
| --- | --- | --- |
| Login | `internal/features/auth_feature/login_session.go`, `login_session_impl.go`, `credentials.go`: user/session lock, authoritative password recheck, session + initial refresh + audit commit, issuer injection | Reuse proof/entropy/issuer pattern; refresh digest SHA-256 dari 32 random bytes yang di-encode base64url. Belum ada lookup/consume/replace/revoke refresh production |
| Persistence | `domain/session.go`, `domain/login.go`, `reader/session.go`, `writer/session.go`, `writer/refresh.go` | Session state read dan create-only writers existing; tambah contract kecil untuk lookup digest, conditional consume, terminal/global revoke, safe list dan password update |
| Transaction | `features/common/transaction.go`, `domain/transaction.go`, `pkg/database/transaction.go` | Reuse provider, request-owned handle dan explicit Commit/Finish; repository tidak memiliki lifecycle/lock |
| Permission | `features/common/authorization.go`, `reader/authorization.go`, existing snapshot/recheck consumer | Reuse active-role union untuk iam.session.read/revoke, tanpa numeric role/JWT role bypass |
| Credential boundary | `interceptors/session_interceptor.go`, `sessions/principal.go` | Default whitelist hanya enam RPC existing; restricted allowlist sudah mencantumkan ChangePassword/Logout, tetapi belum ada handler/registration. Principal normal menolak REVOKED sehingga repeat Logout perlu verifier khusus yang sangat sempit |
| Wiring | `configs/foundation.go`, `register.service.go` | Satu primary pool, real Login, caller/session/rate/attempt pipeline; tambah AuthService/SessionService bersama enam RPC existing |
| Limiter | `interceptors/rate_interceptor.go`, `internal/ratelimit/store.go` | Store/admission/RetryInfo/cap/cleanup reusable; routing saat ini hanya Login/Register/Me/ListUsers |
| Tests | `tests/features/auth_feature`, `tests/interceptors`, `tests/configs`, `tests/integration/postgres/{runner_test.go,test_scenarios/login_race.go,session_foundation.go,registered_server.go}` | Strict mocks, controlled clocks, dedicated opt-in PostgreSQL harness tersedia; belum ada bukti production lifecycle RPC |
| Contract | sibling `api-contracts/proto/` berisi user/profile/paginator; `pkg/dictionary/auth/errors.go` belum menyediakan refresh-reuse | Tidak ada AuthService/SessionService proto atau generated target RPC; contract gap terverifikasi, bukan asumsi karena response Login sudah memiliki refresh_token |

Graph `graphify-out/graph.json` dibangun pada `b89f37270d019c8d0d7823a3ff5d3526874d2232`; hub domain auth (degree 63), auth implementation (47), transaction domain (42). AST EXTRACTED import links mengarah auth feature -> domain/reader/writer dan common transaction. Graph belum memuat seluruh working-tree increment; keputusan di atas diverifikasi dari source, bukan inferred links. Tidak ada graph-instruction.md/nested AGENTS.md pada area yang diperiksa.

Existing [foundation evidence](../2026-10-10-auth-session-authorization-foundation/evidence.md) adalah baseline increment terdahulu, bukan PASS lifecycle baru. Global AC status di dokumen awal adalah snapshot historis; ledger increment ini tetap eksplisit.

### Revision baseline — 11 Oktober 2026

Pada baseline planning (historis), source grpc-auth HEAD `869515b` masih memakai api-contracts `v0.0.0-20261010071509-42a9e6fa23ad`. Checkout sibling telah memiliki sembilan lifecycle RPC/generated definitions dan catalog refresh-reuse/version-conflict secara lokal; publication belum dilakukan dan UserListItem.created_at belum tersedia. Inventory 10 Oktober di atas dan lifecycle-preflight.json dipertahankan sebagai evidence historis, bukan status contract terbaru.

Graph yang sama dipakai sebagai routing awal, lalu source diverifikasi: `features/user/user.go` menerima sort auth_id saja; `handler/user.handler.go` belum meneruskan statuses/application_code; `reader/user.reader.go` memakai auth_id pagination dan role join; response mapper belum mengisi status/version/roles/application_access. `features/user/read_snapshot.go` sudah menyediakan primary read-only repeatable-read snapshot dengan current principal + iam.user.read, yang harus direuse. Graph hub listing: domain user degree24, handler25, reader20; source menjadi acuan saat graph stale.

## Goals / Non-Goals

**Goals:** ListUsers sort/filter/search/unique total dengan snapshot authoritative serta sembilan RPC lifecycle production dengan credential modes tepat, locked authoritative recheck, required persistence, numeric limiter admission, controlled integration dan real PostgreSQL concurrency evidence. Backend increment dapat selesai sebelum deployment readiness.

**Non-Goals:** perubahan arsitektur/env.yaml, framework transaksi baru, kolom sessions.version untuk entity revision, seluruh CRUD Phase 2, status transition command, ResetPassword, broker/relay, UI/rest-orc atau deployment. Tidak menambah tabel/constraint/migration secara otomatis; gap yang terbukti harus mempunyai scope perubahan tersendiri.

## Decisions

### External contract prerequisites

Owner `api-contracts`, scope EXTERNAL_REPOSITORY EXT-01. Prerequisite SLC-EXT-01..06 **PASS** setelah scoped publication commit `668cbd2cc925326b63d5e59f94ba18923cd5c160` ke existing feat/user-err. grpc-auth memakai module `v0.0.0-20261011040220-668cbd2cc925`; generator/compatibility tests, normal push dan fetch verified di [publication evidence](evidence.md#apply--contract-prerequisite-task11-complete-11-oktober-2026). Additive created_at tag10 dan sembilan RPC/DTO/errors tersedia. Tidak ada tag/release/merge/force-push atau perubahan sibling lanjutan.

| ID | Deliverable contract yang diperlukan | Acceptance sebelum grpc-auth wire |
| --- | --- | --- |
| SLC-EXT-01 | AuthService RefreshSession(refresh_token), Logout(credential mode), ChangePassword(current_password,new_password) | Exact full methods sesuai matriks di bawah; Logout membedakan access metadata vs refresh field secara eksplisit dan menolak kedua/kosong; tidak memakai payload user_id sebagai actor |
| SLC-EXT-02 | SessionService enam self/admin methods, request target IDs/expected version, safe changed/version/login_required responses, safe session DTO + PageQuery/PageMeta | uint64 positive user_id muat INTEGER, UUID session, presence/version validation, current flag dan deterministic paginator; token/hash/password tidak menjadi session DTO |
| SLC-EXT-03 | Refresh response credential/session/expiry compatible dengan Login, validations dan error registry/mappings/locales | Preserve UserTokenResponse.token=1 dan additive existing fields 4..8 bila direuse; source protobuf/generator menentukan tags baru, tidak ditebak di grpc-auth |
| SLC-EXT-04 | REFRESH_TOKEN_REUSED -> UNAUTHENTICATED; session-target, VERSION_CONFLICT, IDEMPOTENCY_CONFLICT dan validation semantics yang dibutuhkan | Audit catalog existing sebelum menambah; reuse common/auth errors bila semantik cocok, jangan remap ErrPasswordMismatch atau legacy service unavailable global. Wrong current password memakai existing password-mismatch mapping jika sesuai; exact symbols/wire codes dikunci dalam contract change |
| SLC-EXT-05 | Generated server/client code, dictionary/mapping tests, published commit | Published module memuat sembilan RPC/fields/errors dan additive created_at; verify fetch berdasarkan commit, workspace dan GOWORK=off. Tidak perlu tag/release dan tidak cukup stub lokal/generated manual |
| SLC-EXT-06 | UserListItem.created_at additive timestamp serta docs sort/filter/count/cursor dan Logout/session modes | Pertahankan tags1..9; gunakan tag baru10 untuk created_at, generator resmi. PageQuery.sort/last_id existing cukup untuk multi-sort/composite cursor tanpa mengganti tipe field; docs UserTokenResponse mencakup Login/Refresh |

Message baru additive; field number/type existing tidak berubah. Tidak membuat pb/proto/catalog lokal atau edit module cache. Contract binding published yang belum tersedia memblokir handler/wiring terkait; domain/test planning tetap konkret. Exact new protobuf tag/error code adalah tanggung jawab contract owner dengan semantic acceptance di atas, bukan open business decision.

### RPC security matrix

Setiap row memerlukan TLS, verified `x-service-token`, exact full-method caller allowlist serta namespace yang relevan. `U` berarti access JWT + current primary user/session, `R` berarti refresh proof, bukan payload sid. Metadata locale/request ID/trace/deadline/cancellation diteruskan.

| Exact full RPC | Credential / authority | Restricted | Input → safe response |
| --- | --- | --- | --- |
| /auth.v1.AuthService/RefreshSession | R; ACTIVE user/session; no required U | Denied | refresh_token → access/refresh/session/expiries |
| /auth.v1.AuthService/Logout | Exactly one U or R; own proven session | Allowed | access metadata OR refresh field → changed |
| /auth.v1.AuthService/ChangePassword | U + current-password proof; self | Allowed | current_password/new_password 6–50 → changed/login_required |
| /session.v1.SessionService/ListMySessions | U; self | Denied | PageQuery → safe session page/current flag |
| /session.v1.SessionService/RevokeMySession | U; target session owned by self | Denied | session_id → changed |
| /session.v1.SessionService/RevokeAllMySessions | U; self | Denied | Empty → changed/login_required |
| /session.v1.SessionService/ListUserSessions | U + exact iam.session.read | Denied | user_id/PageQuery → safe session page |
| /session.v1.SessionService/RevokeUserSession | U + exact iam.session.revoke | Denied | user_id/session_id + idempotency-key → changed |
| /session.v1.SessionService/RevokeAllUserSessions | U + exact iam.session.revoke | Denied | user_id/expected_user_version + idempotency-key → version/changed |

Admin session subset memakai resolver yang sudah tersedia, tidak menunggu role CRUD. System roles tidak dimutasi; auditor tidak otomatis mendapat session-read. Authorization denial sebelum disclosure target; missing/mismatched target memakai catalog sesuai contract tanpa membocorkan lintas owner. Normal protected requests tetap melalui strict PrincipalForState. Jangan melonggarkan verifier global untuk repeat Logout: route Logout ke credential-mode verifier yang memeriksa signature/issuer/claims/time, authoritative account auth_version, token/family binding, dan hanya menerima terminal session ownership untuk changed=false. Context proof ini bukan principal umum yang mengotorisasi method lain. Expired/unknown credential ditolak; restricted Logout boleh, Refresh tidak.

List session memakai one consistent primary snapshot untuk authority + data, seperti existing read consumer. PageQuery/PageMeta existing, default page1, limit20/max100; default created_at DESC, id DESC. Primary sort hanya created_at, last_seen_at atau expires_at ASC/DESC; append session ID tie-breaker arah yang sama. last_seen_at null ditempatkan terakhir pada kedua arah. Tolak field/sort shape di luar single primary + optional matching ID tie-breaker; cursor membawa typed primary value/null dan ID serta terikat owner/normalized query. Current flag membandingkan session ID terhadap actor terverifikasi. Field numbers existing tetap; ListUsers diperluas eksplisit sesuai keputusan berikut.

### ListUsers public sort and query contract

Exact `/user.v1.UserService/ListUsers` tetap memerlukan active nonrestricted principal dan iam.user.read. Registry field publik → expression SQL yang diizinkan berada pada boundary domain/reader sesuai pattern existing; gunakan constants/types bermakna, bukan SQL dari request atau reflection. Scalar publik id, name, email, address, phone, status, version, created_at mendukung ASC/DESC; auth_id alias compatibility untuk id. Public field baru wajib mendapat mapping eksplisit dan test; kolom internal dan expression SQL ditolak walaupun ada di DB.

Roles dan application_access menggunakan nama terkecil secara alfabetis dari seluruh collection yang ditampilkan, bukan jumlah item, ID atau subset join yang cocok filter. Default collections hanya assignment/access ACTIVE dengan role/application ACTIVE. Empty collection/null sort key terakhir pada ASC maupun DESC; scalar nullable juga null-last. Gunakan comparator/collation yang sama untuk aggregate nama, ORDER BY dan cursor comparison; tidak menambah normalisasi case/locale yang mengubah search existing. Isi status/version/roles/application_access/created_at selain field yang sudah dipetakan, tanpa credential atau field internal. Hanya response projection yang boleh mengikuti requested fields; sort/filter/count tetap memakai full normalized query dan displayed collection semantics.

Hormati urutan semua sort request, append ID ASC bila belum diminta sebagai tie-breaker, preserve arah ID jika diminta eksplisit. Default hanya ID ASC; canonicalize auth_id menjadi id dan tolak duplicate canonical field atau arah yang bertentangan. Query memakai keyset comparator lexicographic dengan arah per field dan null-last yang sama dengan ORDER BY; jangan memakai anchor auth_id tunggal untuk ordered multi-sort. Cursor opaque versioned membawa seluruh typed sort values/null flags dan ID serta fingerprint normalized ordered sort/filter/search. Numeric last_id existing tetap diterima untuk ID-only ASC/DESC; composite cursor untuk multi-sort dan non-ID sort. Malformed/type-invalid/scope-mismatched cursor ditolak dengan catalog validation existing. Client mengirim cursor kosong ketika sort/filter/search berubah; server tidak menggunakan cursor lama secara diam-diam. Cursor bukan permission proof; authority diperiksa ulang setiap request, bukan snapshot lintas halaman.

Total menghitung DISTINCT user IDs yang visible dan cocok seluruh filter/search sebelum cursor/limit. Tanpa filter/search, total seluruh user yang boleh dilihat caller. Role_codes numerik yang dipilih di-OR; kategori role, statuses, application_code, created range dan existing keyword search di-AND. Filter collection memakai active assignment/access + active role/application yang sama dengan default response. Wire statuses/application_code yang sudah ada di contract, pertahankan keyword search fields/validation/behavior existing. Target response tetap menampilkan seluruh active collections milik matched user, bukan hanya role/app yang dipilih filter.

Reuse primary repeatable-read read snapshot existing untuk authority, total, page IDs dan batch collection reads. Hindari join multiplication melalui EXISTS/separate collection aggregates atau paged unique IDs lalu batch load; jangan query total/page/collections melalui pool/replica berbeda. Total tetap ada ketika cursor melewati seluruh page. Test actor permission berubah, row/filter/collection berubah antarread dan user dengan banyak role sekaligus banyak app; satu snapshot harus memberi authority/data yang konsisten tanpa duplicate user. Perubahan live data antarrequest tidak menjanjikan snapshot pagination lintas request.

Scope ini tidak membuat role CRUD, application management atau endpoint filter-options tersendiri; pekerjaan user management lainnya tetap increment terpisah.

### Logout proof selection

Caller mengutamakan access token valid; setelah expiry caller dapat memakai refresh terbaru. Ini guidance caller, bukan server menebak access token yang tidak dikirim. Optional refresh absent berarti access metadata; refresh present nonempty berarti refresh mode tanpa authorization metadata. Missing/both/duplicate metadata/present-empty ditolak. Refresh proof harus latest, unconsumed, bound owner/family/session dan unexpired; consumed token ditolak tanpa menjalankan refresh-reuse mutation dari route Logout. Latest unconsumed token yang revoked dapat membuktikan repeat Logout changed=false selama account/ownership/auth_version/expiry valid; tidak mengotorisasi method lain. Revoked/noncurrent token tidak memberi izin mencabut sesi lain. Access terminal proof tetap melewati signature/issuer/claims/time/auth_version checks; normal protected principal tetap ACTIVE-only.

### Session generation prerequisite

User menyetujui revisi schema saat apply: sessions/refresh_tokens existing tidak menyimpan generation issuance, sehingga terminal refresh Logout tidak dapat membuktikan auth_version setelah sesi sudah revoked lalu global invalidation/password mengubah generation user. Tambahkan nullable sessions.auth_version BIGINT dengan positive check. Login baru wajib menyimpan account.auth_version sebagai immutable snapshot; refresh dan Logout membandingkan snapshot dengan current account, bukan menganggap current account version sebagai issuance version.

Siapkan prerequisite SQL terpisah `docs/sql/session-lifecycle/01_session_generation.sql`, tanpa menjalankan migration aplikasi. Dalam transaksi yang menghalangi writes auth/sessions, backfill hanya sesi ACTIVE yang belum revoked dari current auth_version. Sesi revoked legacy tetap NULL karena generation historis tidak dapat dipastikan; proof terminal/refresh dengan unknown/mismatched generation ditolak dan memerlukan Login baru. Jangan menebak dari revoke_reason, timestamp atau audit. Tidak ada default generation1. Fixture opt-in membuktikan migration dan invalidation; deployment owner tetap memverifikasi penerapan migration dalam DEP-01.

### Transaction and lock ownership

Handler parse/map → feature command → small domain reader/writer contracts → injected shared primary provider. Auth commands ditempatkan pada `internal/features/auth_feature`; session list/revoke dapat memakai feature `internal/features/session` tanpa generic command framework. Adapters bind ke tx yang diberikan. `Begin`, `TryLock`, `Commit`, `Rollback` hanya dipanggil feature atau features/common; technical provider tetap infrastructure, bukan lifecycle di repository.

Urutan canonical shared protocol: idem (jika wajib) -> policy guard (jika authority admin direcheck) -> sorted actor/target user IDs -> relevant sorted role/application guards bila diperlukan protocol -> sorted session UUIDs. Last-admin guard tidak diperlukan karena command ini tidak menghapus role/status admin. Tidak mengambil kategori awal setelah session lock. Discovery token/session/IDs dapat memakai read awal untuk menemukan lock set, tetapi identity/state/fingerprint/permission dibaca ulang lewat tx yang sama setelah lock. Jika set berubah, rollback lalu restart discovery secara terkontrol, tanpa hidden retry mutation/commit.

Single-session dan refresh memakai user lock lalu session guard untuk seluruh family. Global revoke/password mengambil user locks, menemukan semua sessions di bawah user lock lalu session UUID ascending; Login existing juga mengambil user lock sehingga tidak dapat menyisipkan sesi sebelum invalidasi yang sedang berlangsung. Admin actor/target disort dan deduplicate; policy guard mencegah stale permission write, recheck actor session di transaction yang sama. Role/permission/app mutasi tetap tidak membatalkan session/auth_version; future status suspend/deactivate wajib memakai shared global invalidation protocol pada increment terpisah.

ORM PostgreSQL TryLock menggunakan `pg_try_advisory_xact_lock` pada sql.Tx; lock dilepas commit/rollback. acquired=false -> auth RESOURCE_BUSY/ABORTED tanpa effects; DB error -> dependency, bukan busy. Semua read/write setelah lock memakai handle sama, no pool fallback/no concurrent writes/no external calls. Conditional refresh consume dan session transition memeriksa affected rows untuk mempertahankan single-use/terminal semantics. Unique token hash/index dan replaced_by/FKs harus diverifikasi terhadap schema yang digunakan; tidak menyatakan source/table existence sebagai bukti concurrency.

Hash/verify password mahal disiapkan sebelum lock dengan injected function seperti Login; credential/account/version/auth_version/status proof dibaca ulang di bawah lock sebelum write. Request context dipertahankan; setiap error sebelum commit pada transaksi yang sudah dimulai melakukan rollback melalui TransactionOwner, termasuk business guard setelah partial write, read, conditional affected-row failure, write, cancellation dan panic. Tidak hanya DB write error yang memicu rollback. Commit gagal tidak mengembalikan credential/success; unknown outcome tidak diklaim rollback pasti. Tidak melakukan network atau automatic retry setelah commit.

### Refresh execution and denial outcome

Digest lookup memakai representasi SHA-256 existing Login; successor entropy minimal32 random bytes, same family/session, expires_at tetap original boundary. Generate/sign credential sebelum commit agar signing failure rollback; response hanya sesudah Commit berhasil. Tidak memakai GenerateInitialCredentials untuk membuat sesi/family baru setiap refresh; reuse helper kecil yang sesuai, tidak refactor Login di luar kebutuhan.

Refresh route memerlukan proof refresh, bukan access principal. Preflight menemukan digest owner/session tanpa mempercayai request sid; authoritative locked reread menentukan rotation vs consumed reuse. Unexpired consumed proof dapat membuktikan reuse; revoked/expired/unknown credentials tidak menjadi alat mencabut target lain. Insert successor mendahului conditional predecessor update consumed_at/replaced_by karena immediate self FK; update mensyaratkan predecessor masih unconsumed/unrevoked dengan binding valid. Error atau affected rows !=1 membatalkan seluruh transaksi, termasuk successor. Successor/predecessor/SESSION_REFRESH_ROTATED commit sekali. On proven reuse, explicit owner.Commit security mutation + required audits/event dahulu, baru return catalog reuse denial. Defer cleanup tidak membatalkan commit karena return RPC error. Bila write gagal, rollback dan dependency error lebih tepat daripada mengklaim reuse revocation telah committed. Repeated terminal detection tidak mengulang terminal event.

Expired access tidak memblokir refresh. Session absolute7hari, access max15menit capped expires_at, skew30detik token saja, issuer ApplicationName, audience ignored, EdDSA-only tetap. Token response hilang/commit unknown -> login ulang; no grace, sliding expiry, dual algorithm, restore consumed state atau general credential cache. Consumer single-flight merupakan Phase6 prerequisite yang tidak dapat diluluskan backend fixture.

### Mutation audit and outbox matrix

Semua mutation row memakai same transaction, verified actor, safe snapshots dan request correlation. Envelope/schema_version/producer/aggregate mapping mengikuti domain/outbox existing; published_at tetap kosong. Setiap error bisnis/read/conditional/write/cancellation/panic sebelum commit rollback seluruh business transaction, termasuk safe admin result dan audit/outbox yang belum committed. Sesudah cleanup rollback, feature/boundary mencatat independent safe failed-attempt audit dalam transaksi baru, memakai verified actor/service serta bounded catalog/category reason, tanpa token/password/internal exception text. Reuse failure-audit boundary existing agar tidak double record. Bounded independent context (existing WithoutCancel preserving metadata + timeout2s) memungkinkan audit saat request canceled; audit persistence failure mempertahankan error asli dan tercatat bounded telemetry. Panic tetap mengikuti cleanup/recovery mapping existing tanpa mengekspos detail. Unknown caller tidak diberi fabricated actor; rate attack volume tetap bounded telemetry, bukan audit DB tak terbatas. DB unavailable tidak menjamin audit tersimpan. Proven consumed refresh reuse adalah exception terencana: commit required security mutation/audit/outbox sebelum auth denial; tidak rollback committed state hanya karena RPC denial. Commit error tidak menjanjikan outcome pasti.

| Command | Mutation / counters | Required audit | Required pending outbox | No-op / replay |
| --- | --- | --- | --- | --- |
| Refresh rotation | consumed_at/replaced_by + successor; user/auth unchanged | SESSION_REFRESH_ROTATED | None: catalog tidak punya creation/rotation event | No credential replay; lost response login ulang |
| Refresh reuse | terminal session/family revoke; user/auth unchanged | REFRESH_REUSE_DETECTED + SESSION_REVOKED saat effective | iam.session.revoked.v1, session aggregate_version2 once | No duplicate terminal event; deny after commit |
| Logout | revoke proven session/family only; user/auth unchanged | SESSION_REVOKED saat effective | iam.session.revoked.v1 version2 | Valid repeated terminal owner changed=false; no duplicate success/event, no general cache |
| RevokeMySession | own target session/family; user/auth unchanged | SESSION_REVOKED saat effective | iam.session.revoked.v1 version2 | Already revoked changed=false; no duplicate terminal event |
| RevokeUserSession | bound target user/session/family; user/auth unchanged | SESSION_REVOKED effective; NO_OP for new no-op key | iam.session.revoked.v1 version2 effective only | Safe admin result24h; exact replay adds nothing |
| RevokeAllMySessions | revoke all target sessions/families; user version/auth_version +1 once | USER_SESSIONS_REVOKED | One iam.user.sessions_revoked.v1 at user version | A valid self invocation performs global invalidation including current; old session cannot retry |
| RevokeAllUserSessions | global target invalidation; user version/auth_version +1 once | USER_SESSIONS_REVOKED | One iam.user.sessions_revoked.v1 at user version | New valid key invalidates credential generation even when no ACTIVE session remains; same key replay no increments |
| ChangePassword | new hash/password_changed_at, restriction false, user/auth +1; revoke all | PASSWORD_CHANGED + USER_SESSIONS_REVOKED | One iam.user.sessions_revoked.v1 reason_code PASSWORD_CHANGED at user version | No general key/cache or replacement credential; login again |
| ListMySessions / ListUserSessions | Read only | No mutation success audit requirement | None | No version increment |

Global event tidak diduplikasi per session. Session table tidak memerlukan version column; terminal lifecycle conceptual revision1→2 sudah dikunci existing outbox contract. Tambah constants audit/action/reason yang benar-benar dipakai ke pkg/types sesuai pattern existing, bukan string tersebar atau abstraction kosong. Credential/raw password/auth hash tidak masuk audit/outbox/idempotency/log/metric. Password hash hanya pada credential storage auth, refresh hash hanya refresh_tokens.

### Admin idempotency and optimistic version

Hanya RevokeUserSession/RevokeAllUserSessions memakai required key dalam increment ini. Metadata key nonempty <=200; actor + exact full method + canonical user/{id}/session/{uuid} atau user/{id}/sessions + key. Caller service dicatat tetapi bukan unique dimension. Fingerprint normalized payload/target/presence/expected_user_version; exclude requestID/trace/deadline. User version bukan auth_version; single-session request tidak diberi expected version baru yang tidak ada dalam matrix.

Urutan: idem lock -> guard/user/session locks -> current actor/session/permission recheck -> safe unexpired replay -> expected target version (global admin only) -> mutation/audit/outbox/result -> commit. Safe result24h tanpa credential; identical replay exact saved safe response, no counter/audit/event. Same scope/key different fingerprint -> ABORTED/IDEMPOTENCY_CONFLICT; same key different target independent; expired record handled replace under same lock/tx. Response lost admin dapat retry key sama; self/admin revocation terhadap actor current session tetap membuat proof protected berikutnya invalid sehingga replay tidak bypass current auth. TryLock busy bukan deduplication. New no-op admin single revoke menyimpan changed=false + NO_OP audit/result, no event/version.

### Limiter integration

Reuse store dan exact [numeric matrix MVP §8](../../../../docs/grpc-auth-mvp-spec.md#8-rate-limiter-local-mvp), tanpa runtime defaults secret baru. Refresh IP60/min burst10 + verified-session10/min burst2; password actor5/min burst2; session reads user/group120/min burst20; admin session mutations actor/group30/min burst5. Existing TTL15menit/cleanup1menit/cap10,000/atomic admission/RetryInfo dipertahankan. Logout/self revoke tidak mendapat angka baru yang tidak ada pada matrix.

Refresh membutuhkan admission preparation yang membuktikan digest→session sebelum membentuk session bucket. Untuk valid proof, kedua applicable buckets di-check/consume atomik sebelum rotation/reuse effects. Untuk malformed/unknown token, hanya applicable trusted-IP bucket, tanpa arbitrary claimed sid bucket. Proof lookup bounded/read-only dan tidak dianggap izin; reread locked tetap mandatory. Jangan melakukan IP consume dulu lalu session consume terpisah untuk valid proof karena penolakan session akan mengonsumsi IP secara salah. Flow preparation dapat ditempatkan di feature refresh sebelum begin, menggunakan injected admission dan small verified proof value; tidak menyelipkan payload business decision ke generic interceptor. Dependency lookup failure tetap UNAVAILABLE tanpa credential. Same verified service trust dan bounded labels tetap; actual rest-orc forwarding/retry readiness di DEP-01.

## Risks / Trade-offs

- [Contract lokal belum published dan created_at belum ada] → gate SLC-EXT-01..06 sebelum handler/wiring; tidak memakai stub/fake production RPC.
- [Refresh overlap dapat mencabut sesi setelah rotation pertama] → strict semantics diuji untuk busy dan reuse; consumer single-flight/login ulang pada Phase6, bukan grace tersembunyi.
- [Repeat Logout membutuhkan terminal ownership proof] → verifier route khusus, tidak mengubah active-only PrincipalForState; test proof tersebut gagal untuk method lain.
- [Policy/user locks menambah contention] → protocol MVP existing untuk correctness, ABORTED busy; tidak menambah distributed lock/framework.
- [Password hashing di luar tx dapat stale] → compare credential/state proof di bawah user lock sebelum update; race Login/refresh/global revoke diuji.
- [Source/harness schema tidak membuktikan target deployment] → opt-in dedicated test DB untuk behavior, actual deployment schema/admin tetap DEP-01; migration tidak dijalankan otomatis.
- [Admin revokes own actor session] → response pertama boleh sukses tetapi replay tetap membutuhkan current valid authority; tidak membuat exception auth untuk saved result.

- [Collection sort/count mengembang join] → aggregate/EXISTS/paged unique IDs, total sebelum cursor dan one primary snapshot; test multi-role × multi-app.
- [Failure audit DB unavailable] → pertahankan error asli, bounded telemetry dan no false audit-success claim.

## Migration Plan

1. Saat apply dilanjutkan, selaraskan docs contract, tambah UserListItem.created_at tag10 secara additive, jalankan generator/test/vet workspace + standalone dan review compatibility/secret diff. Commit scoped api-contracts changes lalu normal push ke existing feat/user-err/origin; tanpa tag/release/merge/force-push. Verifikasi published module dapat diambil berdasarkan commit. Task1.2 baru memperbarui grpc-auth/go.mod ke published commit; ini referensi dependency build, bukan fitur runtime pin. Propose sekarang tidak melakukan langkah publication/update.
2. Implementasikan adapters/features/tests bersama audit/outbox/idempotency, lalu daftarkan semua sembilan RPC dengan guards/limiter pada default composition. Implementasikan ListUsers extension dan six-RPC compatibility sesuai delta yang disetujui; default ID/numeric cursor tetap lulus.
3. Catat evidence backend: source/module versions, command/timestamp, controlled environment, expected/actual results dan cleanup. Tidak menandai seluruh phase/MVP selesai dari subset ini.
4. Deployment tetap menunggu mandatory [Phase 6 DEP-01](../../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment), termasuk actual caller method allowlists, key rotation, client single-flight/cookie/IP/RetryInfo/login ulang dan schema/admin. Tidak ada task DEP-01 dalam checklist backend ini.
5. Rollback tidak memulihkan consumed refresh, revoked sessions, password/auth_version lama atau HS256/sid-less token. Bila binary rollback belum dapat memakai safe state/contract baru, hentikan cutover lewat deployment owner; jangan menambah fallback.

## Acceptance Ledger

Scope default semua SLC adalah MVP_REQUIRED/grpc-auth. Seluruh SLC-01..10 **PASS** pada source deliverable yang dicatat di [final verification](evidence.md#final-verification--task8183-complete) dan [source manifest](source.sha256). Full default/standalone/race/vet/build dan dedicated PostgreSQL suite telah dijalankan. Setiap row mempertahankan scope subset AC; publication sendiri tidak membuktikan runtime PASS. DEP-01 tetap terbuka di Phase6, tanpa klaim seluruh Phase/MVP selesai.

| ID | AC / scope | Prerequisite | Required tests / evidence | Status |
| --- | --- | --- | --- | --- |
| SLC-01 | EXT-01; sembilan published RPC/DTO/errors + UserListItem.created_at | SLC-EXT-01..06, owner api-contracts; explicit dependency-update scope | [Publication + dependency baseline](evidence.md#apply--dependency-baseline-task12-complete), commit668cbd2; generator/contract tests and workspace + GOWORK=off baseline PASS | PASS |
| SLC-02 | AC-17; M-03/04; refresh success/expiry/restricted | SLC-01 + existing issuer/session foundation | Feature/handler success, malformed/unknown/expired/nonactive/restricted; claims TTL/counters; signing and each write failure | PASS |
| SLC-03 | AC-17/28; M-04/07/11; consumed reuse | SLC-02 + required audit/outbox | PostgreSQL two physical connections/barriers: busy overlap, postcommit reuse, one successor, terminal event count, successor denial, dependency failure commit-on-denial | PASS |
| SLC-04 | AC-16/18; M-04/06/07; logout/self sessions | SLC-01 + credential-mode boundary | Both Logout modes, ambiguous/missing/expired/unknown proof, restricted logout, terminal no-op only Logout; self-list/IDOR/single/global revoke exact effects | PASS |
| SLC-05 | AC-34 + restricted subset AC-33; M-03/04/07 | SLC-01 + password proof + global invalidation | Wrong password/malformed/cross-owner/dependency; PostgreSQL password/hash/counters/restriction/audit/event atomicity; old credential rejection and fresh Login | PASS |
| SLC-06 | AC-16/24/25/26/31; M-05/07/08; admin session subset | SLC-01 + authoritative permission + idem | Read/revoke permission negatives, actor policy change before lock, missing key/stale version/conflict/different target/expiry; exact replay/no-op and response-loss counts | PASS |
| SLC-07 | M-06 + AC-18/31; safe read consistency | SLC-01 + existing read snapshot | Session dynamic primary sort/default/tie/null/cursor owner binding/current flag/redaction; authorized empty page; permission revoked before next snapshot and DB down not empty-success | PASS |
| SLC-08 | M-01..04/07/10/11, six-RPC compatibility | SLC-01 + implemented commands | Registered in-memory TLS server for all fifteen RPCs, allowlist/namespace/mode/restricted/locale/error mapping; quota layers/RetryInfo/cap/cleanup/privacy; TesRPC/Register/Login/Me/Profile/ListUsers regressions | PASS |
| SLC-09 | AC-16/17/18/24..26/28/34; required DB correctness | Implemented lifecycle + explicit isolated PostgreSQL test DSN/guard | Two physical connections verified via backend IDs, channel barriers, Login/refresh/password/global-revoke races, audit/outbox/result failure + deferred commit failure, business/read/conditional/write failure rollback + separate safe failure audit, cancellation/panic pool reuse; full default/vet/race/build workspace + standalone | PASS |
| SLC-10 | User-approved ListUsers extension; local criterion SLC-10 (bukan AC status lifecycle) | SLC-01 + existing authoritative read snapshot | Every scalar/collection sort, ordered multi-sort/ties/null/empty, malformed/query-mismatched/numeric cursors; DISTINCT total before cursor/limit, role OR/categories+search AND, statuses/app filters, response fields and multi-collection snapshot/permission consistency | PASS |

Full AC-14/15/status lifecycle, all AC-33/CreateUser, role/app/last-admin and whole-MVP gates stay open in their own increments. Existing transaction/last-admin harness regression must remain green; this plan does not claim implementing those new business commands. DEP-01 remains **BLOCKED** outside backend ledger completion.

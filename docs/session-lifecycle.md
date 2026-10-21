# Session lifecycle

Implementasi mengikuti [change auth-session-lifecycle](../openspec/changes/archive/2026-10-11-auth-session-lifecycle/proposal.md).
Contract published: `api-contracts v0.0.0-20261011040220-668cbd2cc925`.
Dokumen ini mencatat persistence dan feature yang tersedia. Registrasi server
serta keseluruhan lifecycle tetap mengikuti task/evidence change.

## Persistence binding

| Contract | Adapter | Schema dan invariant |
| --- | --- | --- |
| RefreshReader | reader.NewRefreshReader(primary/tx) | refresh_tokens; digest SHA-256 32 byte sesuai Login, consumed_at/replaced_by/revoked_at dibaca tanpa raw token |
| RefreshRotationWriter | writer.NewRefreshRotationWriter(tx) | INSERT successor sebelum conditional UPDATE predecessor; immediate replaced_by FK; nol affected rows wajib membatalkan successor |
| SessionLifecycleReader | reader.NewSessionLifecycleReader(primary/tx) | sessions; safe projection dan owner-bound list/count, tanpa credential/hash |
| SessionRevocationWriter | writer.NewSessionRevocationWriter(tx) | user_id + optional session ID, ACTIVE → REVOKED dengan revoked_at/by/reason; exact affected rows |
| RefreshRevocationWriter | writer.NewRefreshRevocationWriter(tx) | family terikat session, atau seluruh refresh milik user; hanya revoked_at yang ditulis |
| CredentialInvalidationWriter | writer.NewCredentialInvalidationWriter(tx) | auth; conditional entity version dan auth_version, naik satu; password update juga hash/changed_at/restriction clear |

Reader menerima handle primary atau transaksi milik feature; tidak mencari
fallback pool/replica. Writer hanya menerima transaksi dan tidak memulai,
mengakhiri, maupun mengunci transaksi. Domain memakai contract/context dan
model SQL existing, tanpa protobuf atau concrete database driver baru.

## Session listing persistence

Field sort diizinkan: `created_at`, `last_seen_at`, `expires_at`, ASC/DESC.
Feature menetapkan default `created_at DESC, id DESC`. Reader meneruskan arah
sort ke UUID ID tie-breaker dan menempatkan null terakhir untuk kedua arah.
Cursor internal membawa nilai timestamp/null dan ID. Validasi serta encoding
cursor publik yang terikat owner/query menjadi tanggung jawab feature.
Count dihitung sebelum cursor/limit; page mengambil limit+1 untuk has_next.
ListService kini membaca authority, count, dan page melalui satu primary
read-only repeatable-read snapshot. ListMySessions mengambil owner dari principal;
ListUserSessions memerlukan exact iam.session.read, termasuk saat target/page kosong.
Restricted user ditolak; permission loss pada snapshot berikutnya langsung berlaku.
Default page1/limit20/max100. Handler menolak fields/search/multi-primary sort yang
tidak didukung. Cursor canonical base64url JSON membawa version, owner, primary
sort/direction, limit, timestamp/null dan UUID; owner/sort/limit yang berubah
memerlukan cursor kosong. Page number adalah metadata dan tidak menggeser keyset.
Response memuat current flag dari verified session ID serta safe nullable metadata;
credential/generation/hash tidak diproyeksikan. Default registration memakai composition lifecycle pada configs.PrepareFoundation.

Schema existing menetapkan last_seen_at NOT NULL. SQL/domain tetap menangani null
secara defensif; pengujian real null boundary hanya melonggarkan constraint dalam
owned disposable fixture, tanpa perubahan/migration schema aplikasi.

## Session generation prerequisite

Login menyimpan account.auth_version pada sessions.auth_version sebagai generation
issuance yang tidak diubah oleh rotasi/revocation. Refresh dan Logout harus
membandingkannya dengan generation account saat ini, termasuk terminal proof.
Ini mencegah latest revoked refresh dari Logout lama diterima sesudah revoke-all
atau password change menaikkan generation user.

[Migration terpisah](sql/session-lifecycle/01_session_generation.sql) disiapkan
untuk schema public existing. Script transactional menghalangi account/session
writes dan membackfill hanya ACTIVE nonrevoked sessions. Tidak ada default1;
Login baru wajib mengirim generation. Revoked legacy rows tetap NULL karena
generation historis tidak diketahui, sehingga tidak dapat membuktikan terminal
Logout dan memerlukan Login baru. Script belum dijalankan pada database aplikasi;
pengujian hanya meremap target ke owned disposable schema pada dedicated test DB.
Penerapan dan verifikasi migration sebelum binary cutover tetap DEP-01.

## Transaction failure and audit

Feature memiliki Begin → canonical TryLock → reads/writes → explicit Commit.
Defer TransactionOwner.Finish menangani rollback untuk setiap error sebelum
commit, termasuk guard bisnis setelah write, read/conditional/write failure,
cancellation dan panic. Provider mempertahankan connection lease sampai driver
cleanup selesai; ini juga menunggu automatic rollback database/sql saat context
dibatalkan. Tidak ada automatic commit atau retry mutation.

SecurityAttemptInterceptor berada di dalam service authentication dan di luar
command/session handling. Sesudah cleanup, independent audit memakai verified
service actor, catalog identity atau kategori alasan aman, dan context terpisah
dengan batas dua detik. Error/panic audit menambah bounded telemetry tanpa
mengganti error asli. Payload/error text mentah tidak disimpan. Unknown caller
tidak diberi actor buatan; rate rejection memakai limiter telemetry.

Login credential denial memiliki recorder sendiri. Refresh reuse yang berhasil
commit security revocation/audit/outbox sebelum denial juga tidak boleh direkam
dua kali oleh interceptor. Pengecualian commit-before-denial tersebut tidak
berlaku untuk error bisnis lainnya.

## RefreshSession

RefreshService membaca digest SHA-256 dari opaque token dalam encoding Login.
Identity user/session berasal dari primary token/session lookup. Setelah
admission dengan persisted session ID, feature mengambil user lock lalu session
lock dan membaca ulang token/session/account melalui transaksi yang sama.
Unknown/expired/inactive/deleted proof ditolak generik; restricted account tidak
dapat merotasi token. Admission callback terhubung ke limiter pada default composition.

Rotasi membuat dua UUID token baru dan opaque successor dari 32 byte entropy;
tidak membuat session/family baru. Successor mewarisi family, session dan expiry
predecessor. INSERT successor mendahului conditional predecessor consume/link,
diikuti SESSION_REFRESH_ROTATED audit dan signing EdDSA sebelum commit.
Response diberikan setelah commit; user/auth counters tidak berubah dan rotasi
tidak membuat outbox event atau credential replay record. Access maksimum 15
menit, dibatasi absolute session expiry tujuh hari sejak Login; clock skew JWT
tidak memperpanjang session.

Consumed proof mencabut session dan family, menulis REFRESH_REUSE_DETECTED dan,
jika efektif, SESSION_REVOKED serta satu pending iam.session.revoked.v1 dengan
terminal aggregate version2. Semua required writes commit sebelum catalog
REFRESH_TOKEN_REUSED/UNAUTHENTICATED dikembalikan. Repeated terminal detection
tidak mengulang terminal event. Error write/commit mengembalikan dependency
failure dan tidak mengklaim revocation berhasil. Session lain tetap aktif.

Caller harus melakukan single-flight refresh per session. Overlap bisa mendapat
RESOURCE_BUSY saat lock dipegang, atau reuse denial setelah rotasi pertama
commit. Setelah response hilang atau commit outcome tidak diketahui, login
ulang; tidak ada grace, restore predecessor, credential cache, sliding expiry,
atau automatic retry. Backend verification tidak membuktikan client menerapkan
guidance tersebut; readiness client tetap DEP-01.

RefreshHandler memetakan published RefreshSessionRequest ke feature dan
UserTokenResponse beserta timestamps. Input credential diteruskan persis;
malformed proof ditolak feature tanpa trim/identity inference. Error catalog,
locale, cancellation dan context mengikuti error boundary existing. Default
AuthService registration dan credential routing diuji melalui registered TLS
server. Actual caller readiness tetap DEP-01.

## Logout

Caller mengutamakan access valid, lalu latest refresh setelah access expired.
Optional refresh absent memilih tepat satu authorization Bearer metadata;
present nonempty memilih refresh tanpa authorization metadata. Present empty,
both, duplicate, missing atau malformed proof ditolak. Signature/issuer/time
access tetap melalui verifier existing, kemudian owner/session/account/generation
dibaca ulang di bawah user/session locks. Restricted user boleh Logout.

Refresh harus latest unconsumed tanpa replaced_by, unexpired dan bound ke owner
session/family. Consumed proof ditolak tanpa menjalankan reuse mutation. Latest
revoked refresh boleh membuktikan already revoked owner dengan changed=false
selama generation dan expiry masih valid. Proof ini tidak memasang Principal dan
tidak membuka Me/ListUsers atau operasi lain; normal principal tetap ACTIVE-only.

Transisi efektif mencabut satu session dan seluruh refresh di session tersebut,
menulis SESSION_REVOKED dan satu terminal pending event version2 dalam satu
transaksi. Session lain dan counter user tidak berubah. Already revoked proof
mengembalikan changed=false tanpa audit/event terminal baru; nol conditional rows
saat authoritative state masih ACTIVE adalah error dan rollback. Required write,
guard, cancellation, panic dan commit failure mengikuti failure audit boundary.

LogoutHandler memetakan optional presence/metadata ke opaque LogoutCredential,
lalu hanya mengembalikan changed. Default server sudah meregistrasikan AuthService.

## Self session revocation

SelfService menerima actor dari verified active Principal dan mengulangi
account/session check di dalam transaksi setelah canonical user/session locks.
RevokeMySession hanya menerima canonical UUID; target harus milik actor.
Lock mencakup current dan target session dalam urutan UUID. Mencabut current
session diperbolehkan, tetapi credential tersebut tidak dapat mengotorisasi
retry. Target lain yang sudah REVOKED mengembalikan changed=false tanpa
terminal audit/event tambahan.

Transisi efektif mencabut seluruh refresh pada target, menulis SESSION_REVOKED
reason SELF_SESSION_REVOKED dan satu pending terminal event version2. Counter
user/auth_version dan device lain tidak berubah. Conditional zero setelah
locked ACTIVE read adalah error dan membatalkan seluruh transaksi.

RevokeAllMySessions mengambil user lock sebelum membaca seluruh session IDs,
lalu mengunci canonical complete set dan memeriksa ulang principal. Login
menggunakan user lock yang sama sehingga issuance baru tidak dapat lolos di
antara discovery dan invalidation. Command menaikkan entity version dan
auth_version sekali, mencabut seluruh sesi/refresh, menulis USER_SESSIONS_REVOKED
reason SELF_SESSIONS_REVOKED serta satu iam.user.sessions_revoked.v1 di committed
user version dengan sessions_revoked=true. Tidak ada per-session events tambahan.
Current session wajib ikut dicabut; zero transition setelah counter write adalah
business error yang rollback counter dan seluruh side effects sebelum safe
failure audit terpisah.

Response sukses hanya changed=true dan login_required=true, tanpa credentials.
Old principal/refresh ditolak setelah commit; Login baru untuk eligible account
menyimpan generation terbaru. Restricted user tidak boleh memakai kedua operasi
self revocation. SelfSessionHandler memetakan context/ID dan safe response;
registrasi default SessionService sudah terhubung.

## ChangePassword

ChangePassword memerlukan verified active self access/session dan current password.
Restricted user boleh menjalankannya tanpa role admin. Current/new password
mengikuti bounds existing 6–50 byte; nilai tidak di-trim. Wrong-current-password
memakai catalog ErrPasswordMismatch existing tanpa mengubah legacy mapping.
Field validation, dependency, cancellation dan unknown errors tetap lewat
boundary locale/redaction existing.

Feature membaca primary account, memverifikasi current password dengan existing
Argon2id policy dan menyiapkan hash baru sebelum mengambil locks. Setelah user
lock, feature mengunci seluruh session UUID secara canonical dan membaca ulang
principal/account. Identity, status, credential hash, entity/auth version dan
restriction harus cocok dengan proof preparation. Perubahan password/global
invalidation yang mendahului locked recheck menggagalkan stale proof.

Satu transaksi menyimpan hash baru/password_changed_at, membersihkan restriction,
menaikkan user/auth counters sekali dan mencabut seluruh session/refresh.
Required PASSWORD_CHANGED serta USER_SESSIONS_REVOKED audits hanya menyimpan
safe IDs/version/reason. Satu pending iam.user.sessions_revoked.v1 memakai
reason_code PASSWORD_CHANGED dan committed user version; tidak ada duplicate
per-session events, password/hash dalam audit/event, credential cache atau token
pengganti. Zero conditional/password/session transition, business/read/write
error, cancellation dan panic membatalkan seluruh transaksi. Safe failure audit
terpisah baru dicatat sesudah rollback selesai.

Response setelah commit berisi changed=true/login_required=true. Login ulang
memerlukan password baru dan menyimpan generation terbaru; seluruh old access/
refresh ditolak. Setelah response hilang, credential lama tidak dapat me-replay
command. Ini tidak menambahkan admin/reset/profile password editing dan tidak
menutup seluruh AC-33/CreateUser. Default RPC registration sudah terhubung.

## Administrative session revocation

ListUserSessions memakai exact iam.session.read; RevokeUserSession dan
RevokeAllUserSessions memakai exact iam.session.revoke. Permission berasal dari
current active assignment/role/application pada primary transaction/snapshot,
tanpa numeric role bypass atau JWT role cache. Restricted/invalid actor ditolak.
RevokeUserSession wajib mengikat canonical session UUID ke positive user_id.
RevokeAllUserSessions wajib membawa positive expected_user_version (entity
version, berbeda dari auth_version).

Kedua mutasi memerlukan satu metadata idempotency-key nonempty maksimal200
karakter. Scope mencakup actor + exact method + user/{id}/session/{uuid} atau
user/{id}/sessions + key. Fingerprint hanya normalized business fields/expected
version/presence; request ID, credential dan deadline bukan payload fingerprint.
Lock order: idem → policy guard → sorted actor/target users → sorted relevant
roles/applications → sorted actor/target sessions. Global discovery berada di
bawah user lock yang juga dipakai Login. Actor/session/permission dibaca ulang
sesudah locks dan wajib valid sebelum lookup/replay. Last-admin guard tidak
diperlukan karena operasi ini tidak mengubah assignment/status admin.

Contoh single payload: user_id2, session_id canonical milik user2; global payload:
user_id2, expected_user_version1. Key dikirim sebagai metadata, bukan field
payload. Replay same-scope/key/payload selama24jam mengembalikan saved safe result
sebelum membandingkan expected target version, tanpa audit/event/counter baru.
Payload berbeda pada scope/key sama menghasilkan IDEMPOTENCY_CONFLICT; scope
target berbeda independen. Expired record diganti atomik sebagai command baru.
Actor yang kehilangan permission atau mencabut current session-nya sendiri tidak
dapat memakai saved result untuk melewati current authentication.

Single effective revoke menulis SESSION_REVOKED serta satu terminal event version2;
new-key already-revoked target menulis NO_OP dan changed=false safe result tanpa
event/counter baru. Global command menaikkan target entity/auth versions sekali,
mencabut seluruh sesi/refresh, menulis USER_SESSIONS_REVOKED dan satu global event
pada committed user version. New authorized key tetap melakukan generation
invalidation bila tidak ada ACTIVE session; stale expected version ditolak.
Account status dipertahankan. Actor pada audit/event adalah admin sebenarnya,
terpisah dari target user. Safe target reader tidak memilih password/hash.

Mutation, audit, outbox dan safe idempotency result memakai satu transaksi;
required failure/conditional/cancel/panic melakukan cleanup sebelum independent
safe failure audit. Normal response hanya setelah commit. Admin dapat mengulang
key yang sama setelah response hilang selama current actor authority masih valid.
Default SessionService registration dan admission sudah terhubung.

## Verification scope

Hasil aktual tersedia di [apply evidence](../openspec/changes/archive/2026-10-11-auth-session-lifecycle/evidence.md).
Publication dan adapter tests tidak menyatakan sembilan RPC lifecycle selesai.
Readiness deployment tetap mandatory pada
[Phase 6 DEP-01](grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment).

## ListUsers

Public sort fields are `id`, `name`, `email`, `address`, `phone`, `status`,
`version`, `created_at`, `roles`, and `application_access`. `auth_id` remains an
alias for `id`. Ordered multi-sort preserves each direction and appends `id ASC`
when ID is absent. The default is `id ASC`. Internal names, expressions, unknown
fields and duplicate sort fields (including ID plus its alias) are rejected.

Role/application ordering uses the alphabetically smallest name from the full
eligible collection. Assignments/access and their role/application must be
ACTIVE. Null scalar values and empty collections sort last in both directions;
ID resolves ties. A profile's absent text maps to an empty response string while
its cursor retains the distinct null ordering value.

Numeric `last_id` remains supported for ID-only ASC/DESC. Other orders use a
versioned base64url cursor with typed nullable values, ID and a digest binding
normalized sort, filters and backend-owned `keyword` search. Changing these
requires restarting with an empty cursor. Projection does not change predicates
or cursor values. Cursor contents are public ordering data, not credentials.

`role_codes` uses OR. Status alternatives use OR; role, status, application_code,
created range and keyword categories use AND. Existing full-text/trigram
keyword semantics remain backend-owned. Active role/application filters use
EXISTS and do not trim the returned collections. Total counts unique matching
users before cursor/limit, including an empty later page. Without filters/search
all auth users are listed, including users without profile/search-index rows.

Authorization, unique total, scalar page, aggregate keys and two independent
batch collection reads use the same primary repeatable-read transaction. A read
failure rolls it back and returns an error with no partial response. The
collection queries cannot multiply page rows. Supported response projection
selects only requested public fields after the full cursor is prepared.

Session lists accept one primary sort plus an optional explicit `id` tie-breaker
with matching direction; a different secondary field/direction is invalid.

## Default server dan admission

PrepareFoundation meregistrasikan15 RPC melalui satu primary pool, satu issuer/
verifier dan satu limiter. Setiap request melewati locale/error boundary, TLS
service authentication/exact caller method allowlist, independent failure-audit
boundary, credential mode dan admission. Lifecycle methods memakai namespace
IAM. Refresh tidak menerima bearer; Logout memakai verifier khusus sehingga
terminal ownership tidak menjadi normal principal.

Refresh IP60/min burst10 dan proven session10/min burst2 diperiksa/di-consume
bersama sebelum rotation/reuse effects. Feature membuktikan persisted session ID;
unknown/malformed proof hanya memakai IP bucket. Rejection tidak menghabiskan
bucket lain. Failure lookup tetap UNAVAILABLE. ChangePassword actor5/min burst2;
session reads berbagi read group120/min burst20 dengan Me/ListUsers; admin
mutations actor30/min burst5. Logout/self revoke tidak diberi quota baru. Existing
trusted rest-orc IP/fallback, capacity10k, TTL15m, cleanup1m dan localized RetryInfo
tetap berlaku. Refresh admission state hanya hidup selama satu request.

Satu entropy reader tersinkronisasi digunakan concurrent credential/event
producers. Cleanup worker dibatalkan dan di-join saat stop/failure; composition
membuka nol pool/client per request. Required audit/outbox/idempotency writers
menggunakan transaksi feature yang sama; independent failure audit dijalankan
setelah cleanup. Registered TLS tests memakai bufconn dengan controlled clock/
entropy; dedicated PostgreSQL test memakai owned schema dan actual adapters.

Handoff deployment tetap [Phase6 DEP-01](grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment):
aplikasikan dan verifikasi session generation migration melalui deployment
owner, konfigurasi exact method allowlists caller, TLS/issuer keys/rotation,
trusted client IP dan storage cookie/token, single-flight refresh, RetryInfo,
login ulang setelah password/global invalidation. Backend tests tidak membuktikan
actual rest-orc client atau kesiapan deployment. Tidak ada migration aplikasi,
cutover, broker, REST/frontend atau role/application CRUD pada increment ini.

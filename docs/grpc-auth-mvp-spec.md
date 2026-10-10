# Spesifikasi MVP Microservice grpc-auth

Versi: 1.0\
Tanggal: 9 Oktober 2026\
Status: acuan implementasi; bukan bukti implementasi atau hasil pengujian aplikasi\
Acuan bisnis: [user-management-spec.md](user-management-spec.md)\
Asal rencana: [grpc-auth-mvp-spec-plan.md](grpc-auth-mvp-spec-plan.md)\
Aturan repository: [AGENTS.md](../AGENTS.md)

## 1. Batas service dan sumber keputusan

`grpc-auth` merupakan satu microservice Go dengan interface unary gRPC,
dipanggil oleh `rest-orc`. Service ini memiliki database auth dan bertanggung
jawab atas identitas, credential, sesi, user, role, permission, serta keputusan
akses. REST dan UI diimplementasikan di repository lain.

Alur: UI → `rest-orc` → `grpc-auth` → PostgreSQL. `rest-orc` tidak membaca atau
menulis database auth. Pemanggilan langsung gRPC tetap melewati autentikasi
service dan authorization user. Auth, user, role, application access, dan sesi
tetap satu deployment; penambahan package protobuf tidak membuat service runtime
baru.

Stack existing dipertahankan: Go, gRPC, protobuf `api-contracts`, PostgreSQL,
ORM, Argon2id, JWT, loader `env.yaml`, dan logger existing. Alur production tetap
handler → feature → repository. Feature memiliki transaction boundary; reader
dan writer menggunakan reusable database pool. Jangan membawa aturan konfigurasi
environment-only milik `rest-orc` ke service ini atau mengganti loader/framework.

Aturan bisnis mengikuti bagian [3](user-management-spec.md#3-konsep-akses),
[4](user-management-spec.md#4-status-dan-aturan-bisnis),
[5](user-management-spec.md#5-katalog-permission),
[6](user-management-spec.md#6-model-data-fisik-dan-mapping-domain),
[7](user-management-spec.md#7-token-sesi-dan-otorisasi),
[8.5](user-management-spec.md#85-idempotency-dan-concurrency),
[9](user-management-spec.md#9-kontrak-grpc-internal), dan
[10](user-management-spec.md#10-event-driven-dan-rabbitmq) spec utama. Dokumen
ini menetapkan subset repository, kontrak implementasi, serta keputusan pengguna
yang telah diselaraskan pada spec utama. Bila ketiganya berubah tidak serentak,
catat konflik sebelum implementasi terkait; jangan menganggap referensi otomatis
menyinkronkan dokumen.

### 1.1 Scope

| Klasifikasi | Pekerjaan |
| --- | --- |
| MVP_REQUIRED / grpc-auth | Register PENDING dan aktivasi admin, login, sesi persisten, refresh/logout/password, profil sendiri, administrasi user, custom business role, mapping permission, banyak role, app access, authoritative authorization, session management, audit, outbox persistence, idempotency, locking, limiter lokal |
| EXTERNAL_REPOSITORY / api-contracts | Tambahan message/RPC, field presence, catalog error per fitur dan gRPC mapping, generated contract serta version compatibility |
| EXTERNAL_REPOSITORY / rest-orc dan frontend | REST mapping, cookie/CSRF, IP sanitization, service credential client, single-flight refresh, perubahan response register/login, login ulang saat cutover |
| EXTERNAL_REPOSITORY / ORM | Perbaikan capability hanya bila source atau pengujian menunjukkan gap; tidak mengganti ORM atau mengubah sibling dalam pekerjaan dokumentasi |
| POST_MVP | Relay RabbitMQ, consumer/processed_events, resource policy CLM, UI admin, distributed limiter, eksperimen multi-replica/load, Elasticsearch/APM deployment |
| POST_MVP | DeleteRole, reset password admin/publik, invitation, CRUD definisi permission/aplikasi, MFA, SSO, multi-tenant, delegation IAM, wildcard/direct permission, role hierarchy, generic policy engine |

Catalog aplikasi/permission dan system role dikelola melalui migration/seed.
Read catalog, edit/status custom role, serta ReplaceRolePermissions termasuk
MVP. Menyiapkan catalog CLM tidak membuktikan integrasi CLM telah berjalan.
Audit dan required outbox ditulis sejak command pertama; relay bukan prerequisite
commit business mutation. Jangan menambah Redis, service discovery/mesh, CQRS,
event sourcing, framework, maupun placeholder RPC.

## 2. Baseline yang telah dicocokkan dengan source

Inspeksi source dilakukan pada 9 Oktober 2026, bukan menjalankan aplikasi.
HEAD `grpc-auth` dan graph AST: `46dc4debb87281d8357a1c671365407ac8c374ca`.
HEAD sibling `api-contracts`: `2473ca37d93f4d86bd7197adefdce91c9908e9d2`;
parent `go.work` memakai sibling tersebut. `go.mod` service masih menunjuk
`api-contracts v0.0.0-20260803064555-df4684bcd60d` dan `orm v1.7.0`.
Source ORM yang diperiksa adalah module v1.7.0 yang benar-benar dipakai, bukan
asumsi bahwa checkout sibling ORM memiliki versi sama.

| Bagian/source | Existing yang terlihat | Target/gap MVP |
| --- | --- | --- |
| [Register/Login/Me](../internal/features/auth_feature/auth.feat.go), [handler](../internal/handler/auth.handler.go) | Register membuat auth/profile/default role lalu token; login verifikasi Argon2id lalu token; Me menggabungkan auth/profile/role | Register PENDING tanpa token; status/session/permission authoritative; lifecycle session; audit/outbox dan lock |
| [JWT](../pkg/jwt/jwt.go), [constructor auth](../internal/features/auth_feature/impl.go) | HS256, user_id, issuer dari ApplicationName, lifetime satu jam | EdDSA saat cutover, claims session, expiry 15 menit, issuer verification |
| [TokenInterceptor](../internal/interceptors/auth_interceptor.go), [session context](../internal/sessions/user_session.go) | Bearer JWT; UserSession hanya UserId; public Register/Login/TesRPC; tidak membaca sessions/status/permission | Service identity terpisah; persistent session binding dan permission check per RPC |
| [Server](../internal/configs/server.go), [registration](../internal/configs/register.service.go), [config](../internal/configs/env.go) | UserService/ProfileService; Language → Error → Token; server belum dikonfigurasi TLS; outbound credentials insecure | TLS, service-token verification, additional mapping/RPC, configuration validation; locale/error tetap membungkus auth failures |
| [Profile feature](../internal/features/profile/profile.feat.go), [writer](../internal/repositories/writer/profile.writer.go) | Session menentukan owner; update dalam transaction; return writer.Update langsung sehingga defer tidak menerima operation error terbaru | Lock user, baca ulang melalui tx, propagation update/commit error, rollback benar, audit/outbox, response owner benar |
| [Register/Me](../internal/features/auth_feature/auth.feat.go), [transaction helper](../internal/features/common/impl.go) | errgroup menulis shared err; panic recovery register terjadi setelah pemanggilan helper transaksi | Jangan berbagi err antar-goroutine; write satu tx berurutan; panic rollback sebelum return; commit failure tidak hilang |
| [Domain auth](../internal/repositories/domain/auth.go), [role](../internal/repositories/domain/role.go), [user-role](../internal/repositories/domain/user_role.go) | Model belum memuat status/version/session/access baru | Model/reader/writer harus mengikuti schema authoritative hasil preflight; tabel/seed tidak dianggap behavior |
| [ListUsers handler](../internal/handler/user.handler.go), [domain](../internal/repositories/domain/user.go), [reader](../internal/repositories/reader/user.reader.go) | Cursor last_id/auth_id; sort auth_id; search keyword backend-owned; dates/numeric role filter; proto statuses belum dipetakan | iam.user.read pada gRPC; status/application filter; permission tidak berasal dari numeric role code; cursor compatibility tetap |
| api-contracts proto/user/v1 dan proto/profile/v1 | Enam RPC existing; AuthRequest email/password; UserTokenResponse hanya token; ProfileRequest scalar name/address/phone | Additive contract dalam sibling, tanpa rename/type/field-number reuse; shared User message memiliki password dan tidak boleh dipakai sebagai output management yang bocor |
| api-contracts pkg/grpcerror/mapping | PasswordMismatch → INVALID_ARGUMENT; ServiceUnavailable existing → UNIMPLEMENTED | Generic login error → UNAUTHENTICATED; dependency failure → UNAVAILABLE melalui error yang semantiknya tepat; jangan remap catalog legacy secara global tanpa analisis |
| [SQL](sql/user-management/01_schema_backfill.sql) | Artefak memakai password VARCHAR(150), sessions/refresh/audit/outbox/idempotency/admin_guard; belum diverifikasi pada DB live | User melaporkan migration/seed; cocokkan schema/index/constraint/admin aktual; tidak rerun otomatis |

Test yang tersedia: [handler runner](../tests/handler/runner_test.go) memuat
mapping/filter ListUsers; [interceptor runner](../tests/interceptors/runner_test.go)
memuat locale, error encoding dan redaction; [reader runner](../tests/repositories/reader/user/runner_test.go)
memuat SQL dry-run user search. Helper [testutils](../tests/shared/testutils/scenario.go)
dipertahankan. Test user feature/paginator legacy collocated bukan precedent
test baru. Keberadaan runner bukan bukti target auth/session/IAM telah lulus;
tidak ada test aplikasi yang dijalankan untuk finalisasi dokumentasi ini.

README SQL masih menyebut password TEXT dan script belum dijalankan; source
01_schema_backfill.sql sudah memakai VARCHAR(150) dan user melaporkan eksekusi.
Gunakan keputusan VARCHAR(150) dan preflight live sebagai acuan, bukan README
lama atau asumsi script sudah cocok dengan database. Artefak SQL/README tersebut
tidak diubah dalam finalisasi ini.

## 3. Identitas caller, JWT, dan authorization

### 3.1 Service token opaque melalui TLS

Setiap caller memiliki service token acak minimal 32 byte dari cryptographic
random source, terpisah dari signing key JWT. Credential client disuplai melalui
konfigurasi server `rest-orc`; bukan header browser. Server auth menyimpan digest
token beserta service_id, batas waktu berlaku, allowed full RPC methods, dan
allowed permission namespaces. Digest dibandingkan secara constant-time.
Caller awal `rest-orc` boleh memanggil RPC MVP pada matriks bagian 4 dan namespace
IAM/CLM catalog yang diperlukan; ini tidak memberikan permission user otomatis.
Caller lain tidak diterima sebelum didaftarkan eksplisit.

- Channel TLS mengenkripsi RPC dan client memverifikasi CA serta hostname/SAN
  server; tidak memakai InsecureSkipVerify atau plaintext fallback.
- Client menggunakan gRPC call credentials untuk `x-service-token`, wajib secure
  transport; JWT user tetap `authorization: Bearer <token>`.
- Server menolak metadata credential kosong/ambigu/multiple. Identitas service
  diambil dari credential yang cocok, bukan x-service-name/user_id dari payload.
- Service authentication berlaku juga pada Register, Login, RefreshSession, dan
  TesRPC. TesRPC tetap ada, response existing dipertahankan, tidak memerlukan JWT
  user dan tidak dipakai sebagai bypass auth/health service baru.
- Rotasi mendukung current dan previous credential per caller. Previous hanya
  diterima bila masa overlap dan not_after dikonfigurasi eksplisit; default tanpa
  overlap. Credential yang dicabut tidak diterima lagi. Restart untuk reload
  diperbolehkan pada MVP; jangan membuat token issuance service tambahan.
- Secret tidak masuk repository/example/log/error/audit/outbox. Konfigurasi TLS,
  caller digest/allowlist, key JWT dan TTL di-load sekali melalui config existing,
  tervalidasi saat startup, lalu dipakai ulang. Spec tidak berisi secret nyata.

Language/Error interceptor tetap membungkus error service/user authentication.
Service identity divalidasi sebelum memakai forwarded IP/metadata caller;
limiter public auth berjalan sebelum hashing dan DB lookup yang mahal. User
authentication/session/authorization menyusul menurut RPC. Unknown method tidak
otomatis mendapat privilege karena memakai prefix service yang sama.

### 3.2 Kontrak token dan sesi yang disepakati

| Parameter | Keputusan MVP |
| --- | --- |
| Algoritma | HS256 hanya baseline sebelum cutover; Phase 3 signing/verifying EdDSA dengan Ed25519; sesudah cutover HS256 ditolak |
| Issuer | Exact ApplicationName dari env.yaml; konfigurasi kosong gagal startup; issuer request harus sama |
| Audience | Tidak wajib dan tidak divalidasi, sesuai keputusan pengguna; verifier utama tetap grpc-auth |
| Claims | sub sebagai decimal user ID, sid UUID session, jti UUID token, iss, iat, exp, auth_version; user_id legacy boleh additive untuk compatibility tetapi bila ada harus sama dengan sub |
| Access TTL | 15 menit; exp tidak melewati expires_at session |
| Refresh/session TTL | Maksimal 7 hari sejak session/login dibuat; expires_at successor refresh tetap dibatasi expiry session yang sama |
| Clock skew | 30 detik untuk validasi waktu JWT; tidak memperpanjang batas absolute session database |
| Refresh sliding expiry | Tidak ada; refresh tidak memperpanjang batas 7 hari |
| Source permission | Primary PostgreSQL, bukan JWT role snapshot, event, Redis, atau read replica |

Private/public key Ed25519 berasal dari konfigurasi/key file yang dikelola di
luar repository; server memverifikasi pasangan key dan allowlist algoritma.
Jangan mencoba algoritma/key lain sampai suatu signature diterima. Source key
berversi untuk rotasi internal; overlap key hanya dalam algoritma aktif dan
retention token yang ditetapkan, tidak membuka kembali HS256 saat rollback.

Setiap protected RPC memvalidasi signature/algorithm/issuer/expiry, lalu membaca
user + session + role/permission/app access dengan snapshot konsisten. sid harus
memiliki user_id yang sama dengan sub; user ACTIVE, session ACTIVE dan belum
expired, auth_version token sama dengan user. Missing/invalid session atau
credential menghasilkan UNAUTHENTICATED. Dependency gagal menghasilkan
UNAVAILABLE, bukan allowed=false/PERMISSION_DENIED. Permission union hanya dari
assignment dan role aktif, dengan gerbang aplikasi bisnis aktif; IAM self-service
tidak membutuhkan role admin atau app access IAM.

Mutasi memeriksa ulang actor/session/permission di dalam transaction setelah
lock. Read authorization dan data sensitif memakai snapshot yang sama; jangan
otorisasi dengan beberapa query berbeda snapshot yang menghasilkan kombinasi
state yang tidak pernah berlaku bersama. Evaluasi yang dimulai setelah revoke
commit memakai state baru. Request yang sebelumnya sudah lolos mungkin selesai;
tidak ada janji pembatalan pekerjaan in-flight atau transaksi lintas CLM.

### 3.3 Restricted session

must_change_password dibaca dari user authoritative, bukan hanya marker JWT.
Allowlist penuh hanya `/user.v1.UserService/Me`,
`/auth.v1.AuthService/ChangePassword`, dan `/auth.v1.AuthService/Logout`.
Me restricted mengembalikan identitas/profil minimum tanpa role/app/permission
admin. UpdateProfile, admin/business operations, list/revoke sesi dan
RefreshSession tidak masuk allowlist restricted. Refresh yang credential-nya
valid tetapi user wajib ganti password ditolak dengan PERMISSION_DENIED;
token rotation tidak terjadi. Setelah access token expired, user login lagi
untuk memperoleh restricted access dan mengganti password.

ChangePassword memakai current_password dan new_password melalui RPC khusus,
bukan edit profil. Sukses clear must_change_password, ubah password_changed_at,
naikkan version/auth_version dan revoke semua session/family secara atomik;
tidak mengembalikan token baru, user login ulang.

### 3.4 Refresh, logout, dan global revocation

Login sukses membuat session + refresh family/token dalam satu transaction,
audit SESSION_CREATED, lalu response credential sesudah commit. Refresh token
opaque hanya disimpan sebagai hash. Device metadata dibatasi ukuran schema dan
dianggap informasi tampilan; forwarded IP hanya dari service tepercaya.

RefreshSession menerima refresh_token, tidak memerlukan access JWT valid.
Service credential, user ACTIVE, session ACTIVE dan absolute expiry tetap wajib.
Di bawah user/session lock, baca ulang token hash/family; token belum consumed
dikonsumsi dan successor dibuat dengan consumed_at/replaced_by secara atomik.
Tidak memakai general idempotency cache untuk response credential.

- Lock belum diperoleh → ABORTED/RESOURCE_BUSY; tidak otomatis menjadi reuse.
- Token terbukti consumed sesudah rotation committed → revoke seluruh family
  dan session, tulis audit + iam.session.revoked.v1 jika perubahan efektif,
  **commit revocation dahulu**, lalu return UNAUTHENTICATED/REFRESH_TOKEN_REUSED.
- Fake/unknown token tidak memberi hak merevoke sid dari input yang belum
  diverifikasi. Return generic authentication failure.
- Client single-flight per session. Dua refresh overlap dapat menghasilkan busy,
  atau satu rotation lalu reuse/revocation setelah commit pertama; kedua
  interleaving diuji. Tidak menjanjikan kedua request menjaga session aktif.
- Response token hilang sesudah commit atau commit outcome tidak diketahui:
  client login ulang; token consumed tidak diaktifkan kembali. Jangan retry
  refresh otomatis atau menambahkan grace/replay credential tersembunyi.
- Logout memakai access JWT valid atau refresh credential terverifikasi, tepat
  satu mode credential per request. Expired JWT saja bukan bukti identitas.
  Ownership/family diverifikasi; logout ulang dengan credential yang masih dapat
  membuktikan owner revoked session menghasilkan changed=false, bukan allow
  untuk operasi lain. Unknown/expired credential tetap UNAUTHENTICATED.

Revoke satu sesi/logout/reuse tidak mengubah auth_version user atau sesi lain.
Revoke-all, ChangePassword, suspend/deactivate menaikkan auth_version dan revoke
seluruh session/refresh family atomik. Reactivate tidak memulihkan sesi lama.
Grant/revoke role/app dan perubahan role-permission tidak membatalkan JWT/sesi;
permission terbaru berlaku pada evaluasi berikutnya.

## 4. Kontrak RPC, message, dan compatibility

### 4.1 Lokasi dan aturan contract

Enam full methods existing pada matriks bertanda E telah dibaca pada protobuf
sibling. T menandai **target contract yang belum tersedia**, bukan RPC dummy.
Target tambahan memakai package auth/user/role/permission/application/session/
authorization/audit versi v1 di `api-contracts/proto/` sesuai convention existing;
generator berjalan di sibling dan service memakai generated pb dari module itu.
Tidak membuat proto/pb lokal. Pemisahan ini hanya organisasi kontrak; jangan
menambah deployment maupun framework registrasi generik.

Register/Login/Me/ListUsers/TesRPC tetap UserService; UpdateProfile tetap
ProfileService. GetMe/UpdateMyProfile pada spec utama adalah nama capability yang
dipetakan ke Me/UpdateProfile existing. Field number/type existing tidak diubah
atau dipakai ulang; tambahan request/response bersifat additive. Boundary
message RPC T serta field baru RPC E harus tersedia sebelum implementation
change yang memakai contract tersebut dapat PASS.

### 4.2 Matriks full RPC

Semua baris memerlukan service credential melalui TLS. U = JWT/session user
valid; R = refresh credential; S = status/session checks tanpa permission admin.
Permission ditulis lengkap; `AND` bukan union alternatif. Error kategori A/V/B/D
mengacu ke bagian 7: authentication/authorization, validation, business/conflict,
dependency/internal. AC asal mengacu ke spec utama, M-* ke bagian 9.

| Status | Full RPC method | Credential/permission atau ownership | Request → response utama | Error | AC |
| --- | --- | --- | --- | --- | --- |
| E | /user.v1.UserService/Register | Tanpa U; registrasi PENDING | email/password → id/status PENDING; token kosong, tanpa refresh/session | A,V,B,D | AC-01,02; M-01 |
| E | /user.v1.UserService/Login | Tanpa U; credential valid + user ACTIVE | email/password + device metadata → access/refresh/session/expiry/restricted | A,V,D | AC-14,33; M-02,03 |
| E | /user.v1.UserService/Me | U + S; diri sendiri; restricted minimum | Empty → UserData safe, status/version/roles/app access; restricted minimum | A,D | M-02,03,06 |
| E | /profile.v1.ProfileService/UpdateProfile | U + S; diri sendiri; tidak restricted | name/address/phone, optional field_mask → ProfileResponse + version/changed | A,V,B,D | M-06,07 |
| E | /user.v1.UserService/ListUsers | U + iam.user.read | PageQuery/UserFilter → ListUsersResponse existing + field safe additive | A,V,D | AC-03; M-05,06 |
| E | /user.v1.UserService/TesRPC | Tanpa U; service-authenticated | Empty → TestRPC existing | A,D | M-01 |
| T | /auth.v1.AuthService/RefreshSession | R + user/session valid; tidak restricted | refresh_token → access/refresh/session/expiry | A,V,B,D | AC-17; M-03,04 |
| T | /auth.v1.AuthService/Logout | U atau R; current session owner | credential mode → changed; tanpa token baru | A,V,B,D | AC-16; M-04 |
| T | /auth.v1.AuthService/ChangePassword | U + current_password; restricted boleh | current_password/new_password → changed/login_required | A,V,B,D | AC-33,34; M-03,04,07 |
| T | /user.v1.UserService/GetUser | U + iam.user.read | user_id → safe user detail/version | A,V,B,D | M-05,06 |
| T | /user.v1.UserService/CreateUser | U + iam.user.create | email/password/profile/status PENDING atau ACTIVE → safe user/version; must_change_password=true | A,V,B,D | AC-33; M-05,07,08 |
| T | /user.v1.UserService/UpdateUser | U + iam.user.update | user_id/profile fields/field_mask/expected_user_version → safe user/version/changed | A,V,B,D | M-05,06,07,08 |
| T | /user.v1.UserService/ChangeUserStatus | U + iam.user.status.update | user_id/status/reason/expected_user_version → status/version/changed | A,V,B,D | AC-14,15,19,20; M-07,08,09 |
| T | /user.v1.UserService/ListUserRoles | U + iam.user.read | user_id/PageQuery → role assignments/status/source | A,V,B,D | AC-04,10; M-05 |
| T | /user.v1.UserService/GrantUserRoles | U + iam.user.role.grant | user_id/role_ids/reason/expected_user_version → changed_role_ids/version/changed | A,V,B,D | AC-04–07,24–26,35 |
| T | /user.v1.UserService/RevokeUserRoles | U + iam.user.role.revoke | Sama seperti GrantUserRoles → revoked role IDs/version/changed | A,V,B,D | AC-09,10,19,20,24–26,35 |
| T | /user.v1.UserService/GetMyEffectivePermissions | U + S; diri sendiri | application_code → effective permission codes + contributing roles | A,V,B,D | AC-10,13,23; M-05 |
| T | /user.v1.UserService/GetUserEffectivePermissions | U + iam.user.read | user_id/application_code → effective permission codes + contributing roles | A,V,B,D | AC-10,13,23; M-05 |
| T | /user.v1.UserService/ListUserApplicationAccess | U + iam.user.read | user_id/PageQuery → application access/status | A,V,B,D | AC-11,12; M-05 |
| T | /role.v1.RoleService/ListRoles | U + iam.role.read | PageQuery/application/status → roles safe | A,V,D | AC-21,23; M-05 |
| T | /role.v1.RoleService/GetRole | U + iam.role.read | role_id → metadata/status/version/permissions/affected_user_count | A,V,B,D | AC-21,23; M-05 |
| T | /role.v1.RoleService/CreateRole | U + iam.role.create | application_code/code/name/description/permission_codes → role/version | A,V,B,D | AC-22; M-05,07,08 |
| T | /role.v1.RoleService/UpdateRole | U + iam.role.update | role_id/name/description/field_mask/expected_role_version → role/version/changed | A,V,B,D | AC-21; M-05,07,08 |
| T | /role.v1.RoleService/ChangeRoleStatus | U + iam.role.update | role_id/status/reason/expected_role_version → role/version/changed | A,V,B,D | AC-21,23; M-05,07,08 |
| T | /role.v1.RoleService/ReplaceRolePermissions | U + iam.role.permission.update | role_id/final permission_codes/reason/expected_role_version → role/version/changed | A,V,B,D | AC-21,22,23; M-05,07,08 |
| T | /permission.v1.PermissionService/ListPermissions | U + iam.permission.read | PageQuery/application_code → read-only permission catalog | A,V,D | M-05,06 |
| T | /application.v1.ApplicationService/ListApplications | U + iam.permission.read | PageQuery → read-only application catalog | A,V,D | M-05 |
| T | /application.v1.ApplicationService/GrantApplicationAccess | U + iam.user.application.grant AND iam.user.role.grant | user_id/application_code/role_ids/reason/expected_user_version → app/changed_role_ids/version/changed | A,V,B,D | AC-08,12,24–26 |
| T | /application.v1.ApplicationService/RevokeApplicationAccess | U + iam.user.application.revoke AND iam.user.role.revoke | user_id/application_code/reason/expected_user_version → app/revoked role IDs/version/changed | A,V,B,D | AC-11,24–26 |
| T | /session.v1.SessionService/ListMySessions | U + S; diri sendiri | PageQuery → safe sessions/current flag | A,V,D | AC-16,18; M-06 |
| T | /session.v1.SessionService/RevokeMySession | U + S; target session owner | session_id → changed | A,V,B,D | AC-16,18; M-04,07 |
| T | /session.v1.SessionService/RevokeAllMySessions | U + S; diri sendiri | Empty → changed/login_required | A,V,B,D | AC-16,34; M-04,07 |
| T | /session.v1.SessionService/ListUserSessions | U + iam.session.read | user_id/PageQuery → safe sessions | A,V,B,D | M-05,06 |
| T | /session.v1.SessionService/RevokeUserSession | U + iam.session.revoke | user_id/session_id → changed | A,V,B,D | AC-16,18,24,25; M-05,07,08 |
| T | /session.v1.SessionService/RevokeAllUserSessions | U + iam.session.revoke | user_id/expected_user_version → version/changed | A,V,B,D | AC-16,24–26; M-05,07,08 |
| T | /authorization.v1.AuthorizationService/EvaluateAccess | U + caller namespace allowlist; exact requested permission | application_code/permission_code → allowed/reason/user_id/sid/user_version/evaluated_at | A,V,D | AC-13,23,27; M-02,05 |
| T | /audit.v1.AuditService/ListAuditLogs | U + iam.audit.read | PageQuery/actor/action/target/time → safe audit entries | A,V,D | M-05,06,07 |

EvaluateAccess tidak mengambil user_id dari payload. Jika identitas valid,
permission tidak ada/unknown menghasilkan allowed=false dengan reason terbatas
PERMISSION_MISSING, APPLICATION_INACTIVE atau APPLICATION_ACCESS_REQUIRED.
Caller service yang tidak diizinkan namespace ditolak PERMISSION_DENIED sebelum
target lookup. Token/session invalid dan dependency error tetap status gRPC,
bukan deny result. Response bukan tiket izin yang dapat di-cache/reuse.

### 4.3 Message, update presence, dan validation

- Register memakai AuthRequest existing. UserTokenResponse.token field 1 tidak
  dihapus/type-changed; Register target mengembalikan token kosong dengan
  registration id/status additive, tidak membuat access/refresh/session. Ini
  perubahan semantik eksplisit yang harus diikuti rest-orc/UI; jangan membuat
  dummy token agar client lama tampak berhasil login. Login memakai token field
  yang sama sebagai access token dan menambah refresh_token, session_id,
  access_expires_at, session_expires_at, must_change_password. Nilai credential
  hanya boleh keluar dari Login/RefreshSession, tidak dari response management.
- Preserve ID uint64 user existing dan int32 list item; new numeric IDs harus
  positive dan muat INTEGER PostgreSQL, UUID session/token/event valid. ID user
  target berbeda dari authenticated actor. role_codes numerik compatibility
  tetap; string role.code ditambahkan, tidak mengganti semantik field numerik.
- Edit profil biasa mengizinkan name/address/phone milik session user. Email,
  password, status, role, user_id/profile ownership bukan editable profile.
  ChangePassword terpisah; admin UpdateUser MVP juga hanya field profil tersebut.
- UpdateProfile existing tanpa field_mask mempertahankan full replacement tiga
  scalar field. Tambahan field_mask memungkinkan partial update: omitted tidak
  diubah, selected empty mengosongkan field yang boleh kosong, selected name
  mengikuti validation nama. Field di luar allowlist ditolak; jangan menafsirkan
  proto3 empty scalar sebagai omitted. New UpdateUser/UpdateRole wajib memakai
  presence/field_mask, tidak menerima arbitrary update map.
- Normalisasi email trim + lowercase sebelum validation/lookup/hash scope.
  Batas input email/password existing 6–50 dipertahankan; name 3–100 mengikuti
  validator existing saat user mengedit profil. Profil awal Register boleh kosong
  sesuai flow existing. Batas address/phone dan metadata mengikuti schema yang
  diverifikasi, bukan default baru yang mengubah format bisnis diam-diam.
- Register tidak menerima privilege/status/app access dari payload. Default
  iam.member tanpa permission admin boleh dibuat atomik sebagai assignment awal
  untuk compatibility; ini bagian creation, bukan bypass GrantUserRoles yang
  hanya menerima target ACTIVE. Tidak ada user pertama otomatis menjadi admin.
- Admin CreateUser hanya status PENDING/ACTIVE; password awal diberikan aktor
  di request dan disampaikan di luar aplikasi, tidak dipulangkan. Status change
  mengikuti transisi spec utama, reason 3–500 karakter, self suspend/deactivate
  dilarang, self revoke platform-admin assignment dilarang.
- Role code/application immutable; custom role hanya aplikasi bisnis; permission
  set terdaftar pada aplikasi sama. System/legacy IAM roles tidak dapat diedit
  code/status/permission melalui API. Banyak role maksimum 20 per command,
  positive IDs, deduplicate sebagai set; satu invalid menggagalkan seluruh batch.
- Grant app memilih minimal satu role aktif aplikasi sama, mengaktifkan access
  dan assignment atomik; regrant tidak memulihkan role lama yang tidak dipilih.
  Revoke app mencabut access serta seluruh assignment aplikasi itu; aplikasi lain
  dan session tetap berlaku. Grant role aktif/revoke already revoked adalah no-op.
- ListUsers mempertahankan last_id/auth_id, arah sort, next_cursor dan metadata
  existing. page default 1, limit default 20/max 100, keyword max 200; empty list
  total/total_pages 0. Proto statuses yang sekarang diabaikan harus tervalidasi
  dan dipakai; application filter additive. Search keyword backend-owned dan
  sort allowlist tetap, tidak membawa raw SQL dari request. List baru memakai
  PageQuery/PageMeta existing, sort deterministik dengan ID tie-breaker.
- List/detail/session/audit tidak mengandung password, token atau hash. Me
  biasa boleh mengembalikan roles/app access; effective-permission endpoint
  mengembalikan source roles. Auditor tidak memperoleh session-read admin.

## 5. Transaction ownership, TryLock, version, dan idempotency

### 5.1 Protocol transaction dan lock

ORM v1.7.0 SqlTransactionAdapter.TryLock melakukan QueryRowContext pada sql.Tx;
PostgreSQL dialect memakai pg_try_advisory_xact_lock. ReleaseLockQuery pada
PostgreSQL tidak melakukan manual unlock: lock dilepas oleh commit/rollback.
Ini fakta source; correctness dua koneksi/cancellation/pool tetap perlu DB test.
Advisory lock hanya mengoordinasikan command yang mengikuti protocol sama.

Pemanggilan `Begin`, `Commit`, `Rollback`, dan `TryLock` wajib berada di
`internal/features/`, termasuk helper lifecycle di `internal/features/common`
bila diperlukan. Feature menentukan transaction boundary, canonical lock keys,
urutan acquisition, serta outcome; repository tidak boleh memulai/menutup
transaksi atau mengambil lock secara mandiri dalam operasi read/write.

Feature memanggil `Begin(requestCtx)` melalui transaction provider/factory kecil
yang diinjeksi, lalu memanggil `TryLock`, `Commit`, atau `Rollback` pada handle
request tersebut. Implementasi teknis provider tetap di infrastructure dengan
`Db.BeginTx(requestCtx)` dan adapter ORM existing; tidak memindahkan driver/SQL
locking ke feature atau menambah framework transaksi baru. Baseline writer
memakai `Db.Begin` dan masih mengekspos `Begin`; pada implementasi, call site
lifecycle dipindahkan dari writer ke feature melalui provider tersebut.

Gunakan ctx request dan satu handle untuk seluruh
reader/check/writer/audit/outbox/idempotency terkait. Repository menerima handle
tersebut dan hanya melakukan read/write; jangan melakukan check mutasi melalui
reader pool di luar tx sesudah lock. Begin/read/write wajib mematuhi deadline/
cancellation. Jangan membagikan tx/shared mutable adapter lintas request.

Canonical key sesuai normalisasi/hash ORM:

| Urutan | Key / lingkup |
| --- | --- |
| 1 | auth:idem:{digest(actor,method,target,key)} bila key wajib |
| 2 | auth:guard:last-admin:global bila berpotensi menghilangkan admin, lalu auth:guard:policy:global untuk mutasi/check policy |
| 3 | auth:account:{digest(normalized_email)} atau auth:role-code:{digest(code)} untuk uniqueness creation |
| 4 | auth:user:{id}; actor/target ID numerik ascending, tanpa duplikasi |
| 5 | auth:role:{id}, numerik ascending |
| 6 | auth:application:{id}, numerik ascending |
| 7 | auth:session:{uuid}, UUID canonical ascending; semua family dalam session memakai guard sesi yang sama |

Discovery boleh membaca IDs dahulu, tetapi semua state harus dibaca ulang di
bawah lock. Bila lock set berubah, rollback dan restart; jangan mengambil guard
kategori awal setelah aggregate lock. Policy guard menyinkronkan perubahan
role/status/permission dengan recheck actor pada mutasi IAM; user locks menahan
grant/revoke/status/password/session terhadap user tersebut. Authorization read
tetap memakai snapshot terbaru, tanpa serialisasi seluruh protected read.
Global policy guard pada write dipilih untuk correctness local MVP; optimasi
granularity membutuhkan bukti invariant, bukan abstraction/cache baru.

Last-admin invariant: minimal satu user ACTIVE dengan assignment ACTIVE ke
iam.platform_admin. Guard global plus recheck aggregate digunakan pada revoke
assignment admin dan suspend/deactivate. Count dihitung ulang sesudah lock.
admin_guard singleton existing tetap diverifikasi; TryLock global menjadi
protocol bersama command service, bukan mengganti constraint database. Jalur
bootstrap/migration yang menyentuh invariant harus dikoordinasikan dengan
protocol yang sama atau dijalankan saat write dihentikan secara eksplisit.

Urutan command: validate service/input → limiter → begin → idempotency lock/lookup
→ invariant/actor/target locks → recheck authoritative actor/session/permission
→ replay jika cocok → expected entity version → business validation/no-op
→ mutation/version/audit/outbox/result → commit → response. Tidak ada external
network call saat tx/lock aktif; hash mahal boleh disiapkan sebelumnya, lalu
credential/version dibaca ulang di transaction sebelum menulis.

acquired=false menghasilkan ABORTED/RESOURCE_BUSY, bukan dependency failure.
Lock DB error dipetakan sesuai cause. Semua path setelah begin berakhir
commit/rollback; panic rollback dahulu. Operation error disimpan pada variable
yang dibaca defer; commit failure tidak hilang karena unnamed return. Jangan
berbagi err lintas goroutine atau menjalankan beberapa write concurrent pada
adapter/transaction yang sama. Reuse-revocation adalah commit-on-denial exception
eksplisit bagian 3.4; jangan memakai auto-rollback berdasarkan RPC error saja.
Commit outcome unknown tidak diklaim rollback pasti.

### 5.2 Counter

User/role creation memakai version=1 dan user auth_version=1. Effective profile,
status, assignment/app-access/global revocation user menaikkan user version sekali
per command; effective role metadata/status/permission menaikkan role version
sekali. Batch/compound tidak menghitung per role. expected_user_version dan
expected_role_version membandingkan entity version; auth_version hanya untuk
credential/global invalidation bagian 3.4. No-op dan replay tidak menaikkan counter.

User/role event aggregate_version memakai version entity committed; gap boleh
terjadi, bukan sequence semua event yang pasti contiguous. Schema sessions yang
dibaca tidak memiliki version/generation. MVP memakai consumed_at/replaced_by
untuk refresh single-use dan lifecycle session terminal ACTIVE → REVOKED:
revision lifecycle konseptual 1 → 2; single-session revocation event memakai
aggregate_version=2, satu kali per session. Session tidak diaktifkan ulang dan
rotation tidak mengubah revision lifecycle. Ini mapping implementasi yang
memakai schema existing, bukan klaim kolom baru telah dimigrasikan. Contract
test mengunci semantik ini; lifecycle lebih luas memerlukan contract/migration
tersendiri. Global session revocation memakai version user.

### 5.3 Persistent idempotency

Metadata canonical `idempotency-key`; wajib pada CreateUser/CreateRole,
ChangeUserStatus/ChangeRoleStatus, Grant/RevokeUserRoles, ReplaceRolePermissions,
Grant/RevokeApplicationAccess, RevokeUserSession/RevokeAllUserSessions. UpdateUser
dan UpdateRole menerima key opsional; bila diberikan mengikuti protocol sama.
Register/Login/RefreshSession/self-service tidak diwajibkan general key.

Scope tersimpan actor ID terverifikasi + full RPC method + canonical target + key.
Caller service terverifikasi tetap dicatat, tidak menjadi dimensi unique baru.
Target: user/{id}, role/{id}, user/{id}/application/{application_id},
user/{id}/session/{uuid} atau user/{id}/sessions. CreateUser memakai
account/{normalized_email}; CreateRole memakai role-code/{canonical_code}, karena
ID baru belum ada. rest-orc memetakan route/method ke full RPC dan target yang sama;
alias application code/ID dinormalisasi dahulu. Maksimum key 200 karakter sesuai
schema, nonempty; fingerprint mencakup normalized payload, target, expected
version dan presence. role_ids/permission_codes disort sebagai set, trace/request
ID/deadline dikecualikan. Password input CreateUser dibandingkan lewat keyed
digest dengan secret/version terpisah; jangan simpan password maupun encoded
authentication hash dalam record/fingerprint.

Simpan safe_response JSONB dan request_hash bersama domain commit, unique PK
(actor_id, method, route_scope, key), expiry 24 jam. Key sama pada target berbeda
merupakan scope berbeda. Scope/key sama + payload berbeda → ABORTED/
IDEMPOTENCY_CONFLICT. Scope/key/payload sama → exact saved management result,
setelah service/session/current endpoint permission diperiksa, sebelum current
target expected_version; jangan mengubah replay menjadi VERSION_CONFLICT.
Request paralel busy → RESOURCE_BUSY; rollback tidak meninggalkan success
record. Valid no-op menyimpan changed=false dan satu NO_OP audit management;
replay tidak menambah audit/event. Expired record bukan janji deduplication tanpa
batas; expiry handling/replace menggunakan lock scope dan tx yang sama.

Session-state logout/revoke sendiri memakai identity/ownership + state, bukan
cache token response. Revoke-all self yang masih memiliki session valid selalu
melakukan global invalidation; setelah current session dicabut, caller tersebut
tidak bisa melakukan request protected lagi. Admin replay key sama tidak
mengulang invalidation, walaupun expected version payload lama.

## 6. Kontrak mutasi, audit, dan transactional outbox

Semua effective mutation wajib success audit pada tx yang sama. R = required
outbox, N = audit saja karena tidak ada event creation/rotation dalam catalog.
Semua permission dan request field mengikuti matriks RPC bagian 4. Key/expected
version bagian 5 berlaku; read RPC tidak menambah version/domain event.

| Full RPC atau group full RPC | Lock tambahan selain actor/idempotency | Version | Audit action | Outbox |
| --- | --- | --- | --- | --- |
| /user.v1.UserService/Register | account/default-member policy | initial user | USER_REGISTERED | R iam.user.created.v1 |
| /user.v1.UserService/CreateUser | account/policy | initial user | USER_CREATED | R iam.user.created.v1 |
| /profile.v1.ProfileService/UpdateProfile; /user.v1.UserService/UpdateUser | user | user +1 | USER_PROFILE_UPDATED | R iam.user.profile_updated.v1 |
| /user.v1.UserService/Login | user/session creation | user tetap | SESSION_CREATED | N |
| /auth.v1.AuthService/RefreshSession rotation | user/session | consumed/replaced_by; user tetap | SESSION_REFRESH_ROTATED | N |
| /auth.v1.AuthService/RefreshSession reuse | user/session | session terminal revision 2 | REFRESH_REUSE_DETECTED + SESSION_REVOKED | R iam.session.revoked.v1 jika perubahan |
| /auth.v1.AuthService/Logout; /session.v1.SessionService/RevokeMySession; /session.v1.SessionService/RevokeUserSession | target user/session | session terminal revision 2; user tetap | SESSION_REVOKED | R iam.session.revoked.v1 |
| /auth.v1.AuthService/ChangePassword | user/all sessions | user/auth +1 | PASSWORD_CHANGED + USER_SESSIONS_REVOKED | R iam.user.sessions_revoked.v1, reason_code PASSWORD_CHANGED |
| /session.v1.SessionService/RevokeAllMySessions; /session.v1.SessionService/RevokeAllUserSessions | user/all sessions | user/auth +1 | USER_SESSIONS_REVOKED | R iam.user.sessions_revoked.v1 |
| /user.v1.UserService/ChangeUserStatus | user/all sessions; last-admin bila perlu | user +1; auth +1 hanya suspend/deactivate | USER_STATUS_CHANGED | R iam.user.status_changed.v1; sessions_revoked=true saat nonaktif |
| /role.v1.RoleService/CreateRole | code/policy/application | initial role | ROLE_CREATED | R iam.role.created.v1 |
| /role.v1.RoleService/UpdateRole; /role.v1.RoleService/ChangeRoleStatus | policy/role/application | role +1; user auth tetap | ROLE_UPDATED | R iam.role.updated.v1 |
| /role.v1.RoleService/ReplaceRolePermissions | policy/role/application | role +1; user auth tetap | ROLE_PERMISSIONS_CHANGED | R iam.role.permissions_changed.v1 |
| /user.v1.UserService/GrantUserRoles; /user.v1.UserService/RevokeUserRoles | policy/user/roles/apps; last-admin bila perlu | user +1; auth tetap | USER_ROLES_GRANTED / USER_ROLES_REVOKED | R iam.user.roles_granted.v1 / iam.user.roles_revoked.v1 |
| /application.v1.ApplicationService/GrantApplicationAccess; /application.v1.ApplicationService/RevokeApplicationAccess | policy/user/roles/application | user +1; auth tetap | APPLICATION_ACCESS_GRANTED / APPLICATION_ACCESS_REVOKED | R iam.user.application_access_granted.v1 / iam.user.application_access_revoked.v1 |

Mutasi/audit/required outbox/idempotency result gagal satu write → rollback semua.
No-op baru management mencatat satu NO_OP audit jika key diterima; tidak ada
state-change event. Replay tidak menulis ulang audit. Login/rotation success
audit juga wajib; tidak membuat event baru untuk mengisi version gap.

Single-session revoke/reuse menghasilkan satu session event; global revoke-all/
ChangePassword satu user.sessions_revoked event, tanpa per-session duplikasi.
Suspend/deactivate satu status event dengan sessions_revoked=true. Grant/revoke
roles memakai satu event batch maksimum 20; app command satu event dengan
changed_role_ids, tanpa role event ganda pada user version sama.

Envelope tetap [spec utama 10.2](user-management-spec.md#102-envelope): event_id,
event_type, schema_version=1, occurred_at UTC, producer, aggregate_type,
aggregate_id string, aggregate_version, actor_user_id bila ada, correlation_id,
causation_id, traceparent bila tersedia, data. Request ID menjadi correlation_id.
Payload outbox JSONB berisi envelope tersebut; tidak mengganti nama menjadi
request_id/payload pada wire event. data hanya ID/status/changed IDs/reason code
terkontrol; alasan bebas tetap audit terbatas. Register publik mempunyai
actor_service terverifikasi dan actor_user_id null, bukan aktor palsu.

Denied login/auth/command mencatat security audit independen setelah rollback;
tidak memakai success mutation transaction yang gagal. Jika audit attempt gagal,
return denial tetap, tulis structured redacted failure dan metrik bounded;
jangan memberikan akses atau token. DB auth yang gagal menghasilkan dependency
error. Reuse-revocation audit wajib atomik dengan revocation, bukan best effort.
Audit action/outcome NO_OP/failure disimpan lewat action/safe snapshot terkontrol
pada schema existing, bukan mengasumsikan kolom outcome yang belum tersedia.

Audit API hanya read, metadata/safe_before/safe_after allowlist; tidak mengirim
credential, hash, metadata auth, raw request body, atau alasan bebas pada event.
Audit/idempotency/outbox tidak mengandung token response. Local MVP tidak
menghapus audit atau unpublished outbox otomatis; idempotency window 24 jam.
Retention/archival production harus disepakati sebelum production deployment.
Outbox pending belum delivered; published_at tidak diisi tanpa relay/confirm.
Publication failure setelah commit tidak membatalkan domain transaction.

## 7. Error boundary dan catalog per fitur

Error baru yang semantiknya benar-benar baru ditambahkan pada namespace fitur
`api-contracts/pkg/dictionary/<feature>`; tidak membuat catalog/alias lokal.
Existing common/validation error yang semantiknya tepat tetap di-reuse. Logical
names pada tabel adalah kebutuhan catalog, bukan klaim wire code/generated Go
symbol telah tersedia. Generator/catalog workflow sibling menentukan code unik
serta localized messages tanpa mengubah code legacy. gRPC mapping baru wajib
di `api-contracts/pkg/grpcerror/mapping/` dan composition server mendaftarkannya.

| Kategori | Catalog pemilik / logical error | gRPC |
| --- | --- | --- |
| A: service/token/session/refresh invalid; login gagal/nonaktif | auth: SERVICE_AUTHENTICATION_FAILED, INVALID_SESSION, LOGIN_FAILED, REFRESH_TOKEN_REUSED sesuai semantik | UNAUTHENTICATED |
| A: credential valid, izin/ownership/restricted operation ditolak | Reuse forbidden existing atau error fitur spesifik tanpa membocorkan target | PERMISSION_DENIED |
| V: malformed request/ID/field mask/reason/presence | dictionary.FieldErrors; existing validation atau feature validation baru | INVALID_ARGUMENT |
| B: user target tidak ada/email duplicate/status transition/self-action/admin terakhir | user: existing semantic reuse atau USER_NOT_FOUND, INVALID_STATUS_TRANSITION, LAST_ADMIN_REQUIRED | NOT_FOUND / ALREADY_EXISTS / FAILED_PRECONDITION |
| B: custom/system role, wrong app, disabled role, unknown mapping | role: ROLE_DISABLED, SYSTEM_ROLE_PROTECTED, INVALID_PERMISSION_SET sesuai kebutuhan | FAILED_PRECONDITION / INVALID_ARGUMENT / NOT_FOUND |
| B: app access required/inactive/wrong assignment | application: APPLICATION_ACCESS_REQUIRED dan semantic errors | FAILED_PRECONDITION |
| B: entity version/idempotency/lock conflict | Namespace fitur command: VERSION_CONFLICT, IDEMPOTENCY_CONFLICT, RESOURCE_BUSY; reuse common bila tepat | ABORTED |
| V: limiter exhaustion/capacity | Shared limiter error atau catalog feature sesuai boundary, retry detail | RESOURCE_EXHAUSTED |
| D: dependency/deadline/cancellation/internal | Existing common jika mapping tepat; error transport baru jika belum ada | UNAVAILABLE / DEADLINE_EXCEEDED / CANCELLED / INTERNAL |

Generic LOGIN_FAILED menyamarkan email tidak ditemukan, password mismatch, dan
akun nonaktif; jangan mengubah ErrPasswordMismatch global menjadi error login
baru karena legacy tests/consumer masih memakai mapping INVALID_ARGUMENT.
Demikian pula ErrServiceUnavailable existing mapped UNIMPLEMENTED tidak boleh
dipakai untuk dependency down target tanpa mapping/semantic yang tepat.
Authorization endpoint mendahului lookup target. Unknown error menjadi internal
generik, cause/identity tetap untuk mapping/log aman. HTTP codes pada spec utama
diterapkan oleh rest-orc, bukan handler gRPC. Error/locale x-language, request ID,
trace metadata, deadline dan cancellation dipropagasikan sampai repository.

## 8. Rate limiter local MVP

Token bucket process-local, concurrency-safe. Angka ini konfigurasi awal local
MVP, bukan benchmark/limit produksi; restart boleh reset state dan reload config.
Setiap bucket independen berikut wajib lolos; service trust diverifikasi dulu.

| Group | Key | Refill | Burst |
| --- | --- | --- | --- |
| Login | Trusted client IP | 30/menit | 10 |
| Login | HMAC normalized account | 5/menit | 3 |
| Register | Trusted client IP | 5/menit | 2 |
| RefreshSession | Trusted client IP | 60/menit | 10 |
| RefreshSession | Verified session ID | 10/menit | 2 |
| Protected read | Verified user + RPC group | 120/menit | 20 |
| Admin mutation | Verified actor + RPC group | 30/menit | 5 |
| ChangePassword | Verified actor + RPC group | 5/menit | 2 |

Count request invalid juga, tanpa response yang membocorkan keberadaan account.
Layered check/consume atomik dan deterministik: bila satu bucket menolak, bucket
lain tidak dikonsumsi untuk business request tersebut. Jangan memakai reserve/
cancel yang race. Reject RESOURCE_EXHAUSTED/RATE_LIMITED dengan retry guidance
terstruktur sesuai contract sibling; no business mutation.

Idle TTL 15 menit, cleanup tiap 1 menit, maximum 10.000 entries/process.
Idle timestamp diperbarui saat check; expired bucket tidak menghapus kuota aktif.
Cap penuh: existing bucket tetap dipakai, new key ditolak dengan capacity reason;
jangan evict exhausted active bucket agar kuota reset. IP gRPC peer biasanya
rest-orc; metadata x-client-ip hanya diterima dari caller rest-orc terverifikasi
yang menyaring forwarding header browser. Missing trusted IP memakai fallback
service_id terverifikasi; tidak membaca IP spoofed dari caller sembarang.

Label metrik hanya group/method/reason bounded, bukan user/email/IP/token/digest.
Audit/log attack volume dibatasi. Metrics lokal minimum: auth failure/deny/
dependency, rate rejection/capacity, duration/error, audit persistence failure,
outbox backlog/oldest age, database pool usage; tidak mewajibkan infra monitoring
baru. Limiter bukan lockout akun permanen; distributed quota dan multi-instance
hardening POST_MVP, tanpa memperlemah invariant database MVP.

## 9. Acceptance criteria dan evidence

Semua AC utama dipetakan berikut; AC-* tetap ID asal, bukan penomoran lokal baru.
Semua status implementation **NOT_STARTED**, kecuali pekerjaan lanjutan DEFERRED.
Baseline source inspection bagian 2 bukan PASS functional behavior. Tidak ada
test aplikasi/live DB dijalankan pada finalisasi ini.

Repository default setiap bagian MVP adalah grpc-auth; dependency contract
EXT-01 wajib untuk setiap RPC/field/error tambahan. Semua AC MVP memerlukan
baseline Phase 0, transaction/auth/permission foundation Phase 1 yang relevan,
dan test contract/feature sesuai baris. AC DB concurrency/atomicity memerlukan
suite 9.1 dan PostgreSQL test khusus; EXT-03 hanya bila gap ORM terbukti.
EXT-02 menjadi prerequisite flow REST/browser serta cutover, bukan bukti yang
dapat diganti test auth lokal. Evidence actual seluruh AC saat finalisasi ini
**belum tersedia**, status per baris mengikuti NOT_STARTED/DEFERRED di atas;
implementation change harus mencatat prerequisite spesifik dan actual evidence
per AC, bukan menandai tabel ini PASS sekaligus.

| AC asal | Scope / repository | RPC atau capability dan expected behavior | Bukti wajib |
| --- | --- | --- | --- |
| AC-01 | MVP_REQUIRED; external REST input validation | Register: privilege payload ditolak tanpa assignment privilege | Handler/contract negative, REST adapter external |
| AC-02 | MVP_REQUIRED | Register PENDING; admin ChangeUserStatus activate sebelum login | Register/status/login scenario + DB atomicity |
| AC-03 | MVP_REQUIRED | ListUsers direct gRPC memerlukan iam.user.read | Service/user/permission negative |
| AC-04 | MVP_REQUIRED | GrantUserRoles batch; union efektif | Functional/DB assertions |
| AC-05 | MVP_REQUIRED | Satu role invalid → whole batch rollback | Failure injection; rows/audit/outbox/result counts |
| AC-06 | MVP_REQUIRED | GrantUserRoles disabled ditolak | ROLE_DISABLED gRPC mapping |
| AC-07 | MVP_REQUIRED | Role bisnis tanpa app access ditolak | APPLICATION_ACCESS_REQUIRED |
| AC-08 | MVP_REQUIRED | GrantApplicationAccess + initial roles atomik; dua permission wajib | AND-permission negative + DB |
| AC-09 | MVP_REQUIRED auth; CLM resource use external | RevokeUserRoles menghapus kontribusi reviewer, author tetap; sesi valid | EffectivePermission/EvaluateAccess DB snapshot |
| AC-10 | MVP_REQUIRED | Union + sources tetap ada dari role lain | User/self permission mapping |
| AC-11 | MVP_REQUIRED | RevokeApplicationAccess mencabut seluruh role aplikasi; sesi/aplikasi lain tetap | Compound tx/event + EvaluateAccess |
| AC-12 | MVP_REQUIRED | Regrant hanya role dipilih | DB state/changed_role_ids |
| AC-13 | MVP_REQUIRED | JWT lama sesudah revoke role/app masih mengidentifikasi user; permission terbaru | Primary DB authorization |
| AC-14 | MVP_REQUIRED | Suspend/deactivate invalid access/refresh; revoke semua | DB/session/claims tests |
| AC-15 | MVP_REQUIRED | Reactivate tidak memulihkan session lama | Status/new login tests |
| AC-16 | MVP_REQUIRED | Logout/single/global revoke sesuai scope | Ownership/session/family/outbox DB |
| AC-17 | MVP_REQUIRED; client single-flight external | Refresh reuse revoke family/session; audit/event commit meskipun RPC error | PostgreSQL controlled interleavings |
| AC-18 | MVP_REQUIRED | RevokeMySession cross-owner ditolak | IDOR negative; unchanged rows |
| AC-19 | MVP_REQUIRED | Status/revoke admin terakhir ditolak | LAST_ADMIN_REQUIRED + guard DB |
| AC-20 | MVP_REQUIRED | Dua admin target berbeda paralel tidak menyisakan nol | PostgreSQL dua koneksi/barrier |
| AC-21 | MVP_REQUIRED | System role metadata/status/permission protected | Role RPC negative |
| AC-22 | MVP_REQUIRED | Custom business role tidak memuat permission IAM | CreateRole/ReplaceRolePermissions |
| AC-23 | MVP_REQUIRED | DisableRole langsung tidak efektif tanpa revoke session | Role state + authoritative evaluation |
| AC-24 | MVP_REQUIRED | Same scoped key/payload replay satu efek/audit/event/result | Idempotency DB + response-loss |
| AC-25 | MVP_REQUIRED | Same scoped key/different payload ABORTED; different target scope berbeda | Contract + DB conflict |
| AC-26 | MVP_REQUIRED | expected entity version stale ABORTED tanpa mutation | Version race/no-op order |
| AC-27 | MVP_REQUIRED UNAVAILABLE behavior; CLM caller external | EvaluateAccess dependency down bukan allowed=false | Fault injection; CLM integration POST_MVP |
| AC-28 | MVP_REQUIRED pending outbox; delivery recovery POST_MVP | Mutation tidak membutuhkan broker; pending committed | Atomic DB assertions; relay test DEFERRED |
| AC-29 | POST_MVP / relay-consumer | Crash publish/mark duplicate-safe | DEFERRED e2e broker |
| AC-30 | POST_MVP / consumer; envelope compatibility MVP_REQUIRED | Version gap/out-of-order reconcile | Catalog test sekarang; projection e2e DEFERRED |
| AC-31 | MVP_REQUIRED direct gRPC checks; REST/UI EXTERNAL_REPOSITORY | Manual bypass tetap ditolak | Direct gRPC; browser tests external |
| AC-32 | POST_MVP / CLM | Reviewer resource policy | DEFERRED CLM resource test |
| AC-33 | MVP_REQUIRED; initial-password delivery external | CreateUser restricted sampai ChangePassword | Restricted full-method negative |
| AC-34 | MVP_REQUIRED | ChangePassword revoke semua lalu login ulang | Password/session/auth_version atomic DB |
| AC-35 | MVP_REQUIRED | Concurrent same assignment tidak duplicate/effect ganda | Dua koneksi/unique/replay/no-op |

AC tambahan tanpa ID asal:

| ID | Scope | Skenario minimum dan evidence |
| --- | --- | --- |
| M-01 | MVP_REQUIRED | Service token valid/invalid/rotated, RPC/namespace allowlist, TLS certificate/hostname rejection; TesRPC tidak dihapus |
| M-02 | MVP_REQUIRED | JWT wrong algorithm/issuer/signature/expiry/sid/sub/auth_version; audience ignored; permission primary snapshot; dependency negative |
| M-03 | MVP_REQUIRED | TTL 15 menit/absolute 7 hari/skew 30 detik terkontrol; restricted full allowlist, refresh restricted ditolak, cutover EdDSA-only |
| M-04 | MVP_REQUIRED | Refresh busy/rotation/consumed reuse, response-lost login ulang, commit-on-denial, logout credential mode, invalid token tak bisa revoke target |
| M-05 | MVP_REQUIRED | Setiap full RPC mempunyai positive/negative caller+permission/ownership check; UpdateUser/CreateRole/catalog/read audit sesuai matrix |
| M-06 | MVP_REQUIRED | Profile name/address/phone editable; email/password/status/owner forbidden; field mask compatibility; filter/pagination; output credential redacted |
| M-07 | MVP_REQUIRED | Failure setiap mutation/audit/outbox/result/begin/commit, panic/cancellation cleanup, profile existing return error, errgroup shared-err regression, exact ordering/count |
| M-08 | MVP_REQUIRED | Persistent fingerprint/presence/version/replay/expiry; CreateUser credential tidak di-cache; new feature catalog/mapping/locale |
| M-09 | MVP_REQUIRED | Actual schema/constraints/defaults/index/bootstrap admin; no automatic rerun/delete; context propagation; TryLock lifecycle/pool dua koneksi |
| M-10 | MVP_REQUIRED | Limiter burst/refill/layered rejection/concurrency/TTL/cleanup/cap/IP trust/retry detail; controlled clock |
| M-11 | MVP_REQUIRED | Failed attempt independent audit, audit failure redaction/denial tetap; no-op/replay auditing; pending event bukan delivery |
| EXT-01 | EXTERNAL_REPOSITORY prerequisite | api-contracts RPC/messages/error catalog/mapping/generator/version compatible; GOWORK=off setelah version tersedia |
| EXT-02 | EXTERNAL_REPOSITORY prerequisite | rest-orc service token/TLS/cookie/CSRF/IP forwarding/error mapping/register response/login ulang dan frontend single-flight |
| EXT-03 | EXTERNAL_REPOSITORY kondisional | ORM gap repair hanya jika source/test membuktikan kebutuhan; module version dan regression DB evidence |

### 9.1 Aturan pengujian dan pengecualian PostgreSQL

Pengguna pada 9 Oktober 2026 menyetujui **suite PostgreSQL terpisah, opt-in, hanya
database test khusus**. Ini pengecualian eksplisit terhadap larangan real-DB
integration test pada AGENTS.md; bukan izin memakai DB user/dev/production.
AGENTS.md tidak diubah dalam pekerjaan dokumentasi. Unit dan integration default
tetap mock/fake, tanpa external network/database/environment yang tidak dikontrol.

Suite nyata ditargetkan di tests/integration/postgres dengan build tag postgres
dan explicit test DSN/opt-in guard; tidak terpilih oleh go test ./... default.
Runner *_test.go hanya TestXxx via testutils.Run; scenario/mocks/fixtures .go di
root tests/ sesuai layer. Tidak membuat root test baru atau test-only production
behavior. Setup/cleanup hanya database test disposable yang dipilih eksplisit,
tidak menjalankan migration/seed pada database baseline. DB tidak tersedia →
gate BLOCKED; bukan PASS karena suite di-skip atau mock hijau.

Scenario concurrency memakai minimal dua koneksi fisik, verifikasi backend ID,
barrier/channel; bukan hanya time.Sleep. Uji same/different key, commit/rollback/
error/cancellation/panic, pool reuse, last-admin target berbeda, actor policy
revocation, optimistic race, exact audit/event/result count, response loss dan
refresh interleavings. Default mock/DryRun membuktikan SQL/argument/error path;
suite nyata membuktikan locking/invariant. Docker tidak dijalankan; PostgreSQL
test disediakan terpisah melalui workflow yang diotorisasi.

## 10. Tahapan, dependencies, dan syarat selesai

Phase numbering mempertahankan plan. Phase 2/3 dipecah menjadi increment yang
memiliki secure prerequisites; tidak membuka admin command sebelum auth/session/
permission checks tersedia. Contract/session authorization foundation dapat
disiapkan dahulu; permission RPC tanpa guard bukan deliverable yang berfungsi.

| Phase | Prasyarat | Deliverable | Gate |
| --- | --- | --- | --- |
| 0 | Source/docs/contract tersedia | Inventaris bagian 2, full-method/message/error mapping, actual schema/bootstrap plan, scoped OpenSpec change | Tidak ada ambiguous business decision atau dependency tersembunyi pada change |
| 1 | Scope dependency contract relevan disetujui | Transaction/shared reader-writer, TryLock protocol, audit/outbox/idempotency repositories, session persistence/auth foundation, opt-in DB harness | Mock dan PostgreSQL lifecycle/atomicity tests PASS; missing schema hanya perubahan eksplisit setelah preflight |
| 2 | Phase 1; service/user/session/permission guard siap | User/status, custom roles/mapping, assignment/app access; seluruh command dengan expected version/key/audit/event | Permission/ownership/system-role/last-admin/union/no-op/replay concurrent PASS |
| 3 | Persistence/auth guard + key/caller/client readiness | Register/login/password/restricted/refresh/logout/revoke, EdDSA cutover dan login ulang | Algorithm/issuer/TTL/sid/auth_version/TLS/service credential tests dan refresh PostgreSQL PASS |
| 4 | Implemented mutation RPC tersedia | Reconcile matrix/contract/AC, negative cases, security audit, event trigger/password/reuse compatibility | Semua mutation sesuai R/N, credential redacted, error mapping benar |
| 5 | Trusted key/IP dan error contract | Limiter/hardening/metrics local | M-10/M-11 PASS; auth basic limiter sudah tersedia sebelum public auth dinyatakan selesai |
| 6 | Semua mandatory increments dan dependencies | Full regression, opt-in DB, compatibility/version evidence | Semua MVP_REQUIRED PASS; tidak ada critical blocker |

Gate cutover Phase 3: pasangan key siap dan sign/verify PASS, session/status/
auth_version/issuer/TTL checks aktif, service token/TLS semua caller siap,
rest-orc dapat menangani response baru dan login ulang, minimal satu platform
admin ACTIVE terverifikasi. Saat cutover revoke semua session/family lama,
reject JWT lama tanpa claims/HS256, mulai EdDSA; tidak ada dual-algorithm window.
Readiness gagal → jangan cutover; setelah cutover rollback tidak membuka kembali
credential/session/auth_version lama. Tidak menjalankan migration/bootstrap
ulang hanya karena script tersedia.

Implementation verification: gofmt pada Go yang berubah, targeted test, default
go test ./..., go vet bila relevan, race untuk concurrency/shared state,
opt-in PostgreSQL suite dan race targeted suite bila terkait, make build untuk
wiring/startup/contract, workspace lalu GOWORK=off setelah dependency published,
diff/status review. Jalankan command shell melalui RTK. Tidak menjalankan server
sebagai unit test, real service/deployment, atau Docker sebagai efek samping.

Status AC: NOT_STARTED, IN_PROGRESS, BLOCKED, PASS, FAIL; DEFERRED hanya POST_MVP.
PASS memerlukan commit/module version, test command, DB/environment, waktu,
expected/actual result dan report aman. External prerequisite belum tersedia
membuat AC terkait BLOCKED, bukan menurunkannya menjadi POST_MVP. UI/broker
behavior belum terbukti oleh test auth lokal. Dokumentasi selesai tidak berarti
MVP implementation siap; membuat context OpenSpec tidak menciptakan change/tasks.

## 11. Decision log dan verifikasi dokumentasi

| ID | Keputusan / alasan | Status |
| --- | --- | --- |
| D-01 | Satu grpc-auth; IAM lengkap, relay/consumer/CLM ditunda | Disepakati plan |
| D-02 | Opaque service token per caller + TLS; service identity terpisah dari JWT user | Disepakati pengguna |
| D-03 | Register PENDING tanpa credential; activation admin; TesRPC dipertahankan | Disepakati pengguna |
| D-04 | EdDSA pada Phase 3; issuer ApplicationName; audience ignored | Disepakati pengguna/plan terbaru |
| D-05 | Access 15 menit, refresh/session absolute 7 hari, skew 30 detik, non-sliding | Disepakati pengguna |
| D-06 | Strict refresh rotation, single-flight client, locked server, response-lost login ulang | Disepakati pengguna/plan; tanpa credential replay cache |
| D-07 | Profile name/address/phone editable sendiri, email/password melalui profil dilarang; status/role/owner tetap protected | Disepakati pengguna; selaras source ProfileRequest |
| D-08 | Error baru menurut fitur; shared registry dan mapping di api-contracts | Disepakati pengguna |
| D-09 | TryLock ORM juga pada existing mutation, entity version terpisah dari auth_version; role/app changes menjaga sesi | Disepakati pengguna/plan |
| D-10 | Actor+method+target idempotency 24 jam, existing event names/envelope/session outbox dipertahankan | Disepakati plan terbaru |
| D-11 | Limiter angka local dan policy audit/outbox menurut bagian 6/8, production retention belum diputuskan | Baseline implementasi local; production di luar scope |
| D-12 | Suite PostgreSQL nyata opt-in khusus test DB; pengecualian AGENTS.md, tidak memakai DB baseline | Disetujui pengguna saat finalisasi |
| D-13 | Target full methods/message additive, field-mask compatibility, session terminal revision 2 tanpa counter schema baru | Keputusan desain berdasarkan source; contract dan DB evidence masih prerequisite |
| D-14 | DeleteRole/reset admin-publik/CRUD permission, cache allow dan distributed limiter bukan MVP | Disepakati plan terbaru |
| D-15 | Feature memanggil Begin/Commit/Rollback/TryLock; repository memakai tx yang diberikan, provider teknis tetap infrastructure/ORM existing | Disepakati pengguna |

Prasyarat teknis yang masih harus dibuktikan pada implementation change: schema
live dan batas kolom aktual, module contract baru/published version, TLS/key
deployment config, caller/client cutover, actual locking/deadline cleanup dan
database test evidence. Ini bukan pilihan bisnis diam-diam atau tanda feature
telah selesai. Production retention/archive dan tambahan catalog/RPC berada di
luar MVP lokal; perlu keputusan terpisah sebelum scope tersebut dikerjakan.

Finalisasi ini hanya membuat dokumen ini, menyelaraskan spec utama dan context/
rules OpenSpec. Verifikasi dokumentasi mencakup link/anchor, full-method inventory,
semua 35 AC asal, mutation-event coverage, YAML/context, serta scope/diff.
Tidak mengubah source Go, proto/generated contract, SQL, dependency, AGENTS.md,
database maupun sibling. Tidak membuat change, menjalankan migration, atau
mengklaim test aplikasi telah PASS.

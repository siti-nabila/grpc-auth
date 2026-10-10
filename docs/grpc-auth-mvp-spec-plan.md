# Rencana Spec MVP Microservice grpc-auth

Tanggal revisi: 9 Oktober 2026\
Status: rencana penyusunan spec; bukan bukti implementasi aplikasi\
Acuan bisnis: [user-management-spec.md](user-management-spec.md)\
Aturan repository: [AGENTS.md](../AGENTS.md)

Revisi ini menggabungkan Mutation, Audit & Transactional Outbox Contract, kontrak command, rate limiting lokal, dan acceptance criteria beserta tahapan implementasi. Penyelarasan berikutnya mempertahankan sesi pada perubahan role/akses aplikasi, memakai entity version untuk concurrency, memulihkan kontrak idempotency/error/event spec utama, dan membatasi scope fitur tambahan.

Spec utama `user-management-spec.md` versi 1.0 tanggal 8 Oktober 2026 telah dibaca untuk penyelarasan ini. Isi repository, AGENTS.md, dan schema aktual belum diverifikasi. Tautan relatif di atas ditujukan untuk penempatan dokumen di direktori `docs/`; validitas path dan anchor harus diperiksa ketika dokumen dipasang ke repository. Nama RPC pada matriks adalah nama capability yang harus dipetakan ke full RPC method existing, bukan klaim bahwa protobuf tersebut sudah tersedia.

## 1. Tujuan dan keputusan scope

Buat `docs/grpc-auth-mvp-spec.md` sebagai acuan implementasi khusus repository `grpc-auth`, terhubung ke spec utama sebagai sumber aturan bisnis keseluruhan sistem.

Keputusan MVP yang telah disepakati:

- IAM lengkap, termasuk custom role, permission mapping, dan banyak role per user.
- Audit mutasi dan transactional outbox yang diwajibkan kontrak ditulis atomik bersama mutasi.
- Relay RabbitMQ, consumer, dan integrasi CLM ditunda.
- Autentikasi service pemanggil menggunakan service token melalui TLS; JWT user tetap terpisah.
- Rate limiter menggunakan state in-memory untuk satu instance lokal.
- Locking memanfaatkan `TryLock` dan transaction adapter ORM existing.
- Last-admin guard, optimistic versioning, dan persistent idempotency merupakan behavior wajib MVP.
- Correctness locking, refresh rotation, dan idempotency paralel harus dibuktikan pada PostgreSQL nyata.
- Grant/revoke role, role-permission update, serta grant/revoke akses aplikasi tidak menaikkan `auth_version` dan tidak mematikan sesi. Authorization memakai data akses terbaru.
- Concurrency command user memakai `expected_user_version`; command role memakai `expected_role_version`.
- JWT audience diabaikan sesuai keputusan pengguna. Source EdDSA-only diterapkan bersama session-aware Login pada increment Phase 1/3; readiness deployment/cutover target dipindah ke Phase 6 DEP-01.
- DeleteRole, reset password administratif/publik, dan CRUD permission ditunda sampai aturan bisnis/permission-nya ditetapkan.

**Pekerjaan sekarang adalah revisi rencana dokumentasi.** Pembuatan spec repository, perubahan OpenSpec, implementasi Go/protobuf, pelaksanaan migration, dan pengujian aplikasi merupakan pekerjaan berikutnya. Penyimpanan rencana ini tidak berarti pekerjaan tersebut telah dilakukan.

Bedakan dua syarat selesai: **rencana/spec selesai** pada Section 7.6 dan **implementasi MVP selesai** pada Section 7.5. Jangan memakai selesainya dokumentasi sebagai bukti aplikasi siap digunakan.

## 2. Batas microservice dan tech stack

Spec MVP harus dibuka dengan pernyataan berikut:

> `grpc-auth` merupakan satu microservice Go dengan interface unary gRPC, dipanggil oleh `rest-orc`. Service ini memiliki database auth dan bertanggung jawab atas identitas, credential, sesi, user, role, permission, serta keputusan akses. REST dan UI diimplementasikan di repository lain.

Alur utama: UI → `rest-orc` → `grpc-auth` → PostgreSQL.

| Komponen | Tanggung jawab |
| --- | --- |
| `grpc-auth` | Business rule IAM, autentikasi/otorisasi gRPC, persistence, audit, dan outbox |
| `rest-orc` | HTTP API, parsing request browser, pemanggilan gRPC, response/error mapping, dan cookie pada boundary HTTP |
| `api-contracts` | Source protobuf dan generated contract sesuai workflow repository |
| ORM existing | Transaction adapter dan advisory lock; perbaikan library dikerjakan di repository ORM |
| Frontend | Login, profil, administrasi, dan tampilan hasil API |
| CLM | Data kontrak, workflow, dan resource authorization pada tahap lanjutan |

`rest-orc` tidak mengakses database auth. `grpc-auth` tetap memverifikasi identitas dan permission pada RPC yang dilindungi, termasuk pemanggilan langsung. Modul auth, user, role, application access, dan session tetap berada dalam satu deployment `grpc-auth`.

Cantumkan stack existing: Go, gRPC, Protocol Buffers dari `api-contracts`, PostgreSQL, ORM existing, Argon2id, JWT, konfigurasi, dan logger existing. RabbitMQ merupakan tahap lanjutan, bukan dependency runtime MVP pertama.

Pertahankan pola handler → feature → repository dan transaction ownership existing. Jangan mengganti library atau melakukan migrasi arsitektur tanpa kebutuhan konkret.

## 3. Kebutuhan MVP dan kontrak

| Capability | Cakupan MVP |
| --- | --- |
| Authentication | Register PENDING, login, refresh rotation, logout, change password, restricted session saat wajib mengganti password |
| User management | Me, update profil, list/search, detail, create/update oleh admin, suspend/reactivate/deactivate |
| RBAC | Catalog, custom role, permission mapping, grant/revoke banyak role, permission efektif, dan sumber role |
| Application access | List, grant/revoke akses aplikasi, perubahan assignment sesuai spec utama |
| Session | List/revoke sesi sendiri dan administratif, revoke seluruh sesi |
| Authorization | Evaluasi akses authoritative melalui gRPC |
| Audit dan outbox | Audit read, audit mutasi atomik, security audit, outbox persistence untuk event yang ditetapkan |
| Rate limiting | Token bucket in-memory, limit numerik, memory cap, rejection dan retry guidance |

### 3.1 Hubungan dengan spec utama

Gunakan referensi ke aturan bisnis spec utama, bukan salinan seluruh aturan. Referensi minimum:

- [Konsep akses](user-management-spec.md#3-konsep-akses).
- [Token, sesi, dan otorisasi](user-management-spec.md#7-token-sesi-dan-otorisasi).
- [Kontrak gRPC](user-management-spec.md#9-kontrak-grpc-internal).
- [Acceptance criteria dan pengujian](user-management-spec.md#15-acceptance-criteria-dan-pengujian).

Spec utama menentukan aturan bisnis. Spec MVP menentukan subset fitur dan batas implementasi `grpc-auth`. Jika ditemukan konflik, catat keputusan eksplisit sebelum implementasi; jangan diam-diam melemahkan aturan keamanan. Referensi antardokumen tidak membuat keduanya otomatis tersinkronisasi.

Status, permission union, system role, admin terakhir, revocation, idempotency, dan concurrency mengikuti spec utama. Jangan menciptakan status atau mengubah makna grant/revoke hanya dari nama RPC contoh di rencana ini.

**Ditunda (POST_MVP/DEFERRED): DeleteRole, reset password admin maupun publik, dan CRUD permission.** Jangan memasukkannya ke matriks mutasi MVP, membuat permission code/RPC/event baru, atau menjadikannya dependency create-user. Admin-created user menerima password awal melalui mekanisme existing dan wajib menggantinya lewat ChangePassword; password tidak dikirim kembali oleh API. Permission/application catalog dikelola lewat migration/seed, API catalog read-only. Edit/status custom role dan ReplaceRolePermissions tetap MVP; itu berbeda dari CRUD definisi permission.

#### 3.1.1 Compatibility register

| Area | Kontrak yang dipertahankan / keputusan eksplisit |
| --- | --- |
| Sesi dan akses | Spec utama Sections 3.2, 6.1, 7.2: role/app change hanya mengubah keputusan izin, bukan validitas JWT/sesi |
| Concurrency | Sections 8.4–8.5: `expected_user_version` dan `expected_role_version`, bukan auth_version |
| Idempotency | Section 8.5: actor + method + canonical route/target; response aman 24 jam |
| Error | Section 8.6: VERSION_CONFLICT dan IDEMPOTENCY_CONFLICT memakai ABORTED, HTTP 409 pada rest-orc |
| Event dan session outbox | Section 10.1: nama event/catalog serta compound-event policy dipertahankan; revoke sesi tetap mempunyai outbox |
| Audience JWT | Override eksplisit keputusan pengguna: audience tidak divalidasi; berbeda dari usulan audience di spec utama Section 7.1 |
| Algoritma JWT | HS256 baseline historis; EdDSA source bersama real Login/session-aware guards pada increment Phase 1/3; readiness deployment/cutover target di Phase 6 |
| Scope runtime | TryLock ORM, limiter lokal, dan penundaan relay/consumer adalah keputusan MVP repository ini; tidak mengganti seluruh scope sistem dalam spec utama |

Perbedaan yang disengaja harus dicatat saat memasang plan ke repository agar spec utama/spec MVP tidak memberi dua instruksi berbeda. Revisi ini mengubah file plan saja; tidak mengubah spec utama secara otomatis.

### 3.2 RPC dan komunikasi antarservice

- Pertahankan RPC existing; dokumentasikan tambahan berdasarkan capability. Tidak wajib membuat service protobuf terpisah untuk setiap modul.
- Source protobuf tetap di `api-contracts`; jangan membuat proto atau generated code duplikat di `grpc-auth`.
- Buat matriks full RPC method → identitas caller → permission → gRPC error → AC ID. HTTP route/status merupakan konteks integrasi `rest-orc`.
- JWT user menggunakan metadata `authorization: Bearer <token>`. Credential service menggunakan metadata terpisah `x-service-token` melalui TLS.
- Credential `rest-orc` diverifikasi terhadap konfigurasi server. Nama service atau `user_id` dalam payload bukan bukti identitas.
- Login/Register tidak membutuhkan JWT user, tetapi tetap memerlukan autentikasi service pemanggil. RefreshSession dikecualikan dari access-token authentication dan wajib memvalidasi refresh credential serta status session.
- Logout dapat mengidentifikasi sesi melalui access token yang valid atau refresh credential yang valid sesuai kontrak final; token expired tidak cukup sebagai bukti identitas.
- Jangan meneruskan credential service dari input browser atau memasukkannya ke contoh/log.
- Idempotency key dari REST diteruskan melalui metadata gRPC `idempotency-key`. Gunakan satu lokasi canonical; jangan membuat nilai request field dan metadata yang dapat berbeda.
- Propagasikan context, deadline, cancellation, request ID, dan locale sesuai contract existing. Jangan menambah retry mutation tanpa kontrak idempotency.
- Session-aware token mencakup cutover login ulang sesuai spec utama; jelaskan compatibility dengan token existing.

### 3.3 Database, transaksi, locking, dan idempotency

#### 3.3.1 Database compatibility

- Email tetap dinormalisasi pada kolom yang sama.
- Password tetap `VARCHAR(150)`; verifikasi encoded Argon2id hash dengan parameter final muat di kolom tanpa truncation.
- Seluruh ID existing dipertahankan.
- Migration/seed yang dilaporkan selesai harus diverifikasi terhadap schema/data aktual, bukan dijalankan ulang otomatis.
- Perubahan schema harus backward-compatible apabila memungkinkan, disertai rencana cutover jika tidak memungkinkan.
- Unique constraint, foreign key, dan constraint bisnis yang diperlukan tetap wajib; advisory lock bukan penggantinya.

#### 3.3.2 Kontrak ORM TryLock

Seluruh write mutasi memakai transaction adapter ORM existing dan transaction handle yang sama. Gunakan `TryLock` untuk concurrency-sensitive command; jangan membuat implementasi advisory lock baru di Auth Service jika ORM memenuhi kebutuhan.

Pemanggilan `Begin`, `Commit`, `Rollback`, dan `TryLock` wajib berada di layer
`internal/features/`, termasuk helper `features/common` bila diperlukan.
Feature menentukan boundary, canonical keys/order dan outcome; repository hanya
read/write dengan handle transaksi yang diberikan, tanpa lifecycle atau lock
mandiri. Feature menerima transaction provider/factory yang diinjeksi; provider
teknis memakai request-context `Db.BeginTx` dan adapter ORM existing.

Kontrak wajib:

1. Mulai transaksi sebelum memperoleh lock. Lock harus efektif sampai seluruh write dan commit/rollback selesai.
2. Bedakan hasil `acquired=false` dari error database. Busy → `ABORTED/RESOURCE_BUSY`; dependency failure → error dependency sesuai catalog, bukan busy atau permission denied.
3. Cleanup aman pada commit, rollback, error, cancellation, dan panic. Pemulihan panic harus tetap mempertahankan rollback dan lock cleanup.
4. Dokumentasikan urutan nyata commit/rollback/unlock dan kepemilikan koneksi adapter. Lock tidak boleh dilepas sebelum commit menjadi terlihat.
5. Verifikasi apakah dialect memakai transaction-scoped atau session-scoped lock. Jangan menganggap session-scoped advisory lock otomatis dilepas oleh COMMIT.
6. Untuk session-scoped lock, acquire dan unlock wajib pada koneksi fisik pemilik yang sama, tanpa melepas lock terlalu dini atau mengembalikan koneksi dengan lock tertinggal ke pool. Jika adapter tidak dapat menjamin ini, tandai blocker ORM; jangan membuat workaround SQL locking tersembunyi di service.
7. Semua command yang melindungi invariant yang sama harus mengikuti protocol lock yang sama.
8. Tidak ada external network call saat transaksi/lock aktif. Hash password yang mahal dapat disiapkan sebelum transaksi; state credential/version tetap diperiksa ulang di dalam transaksi.

Canonical lock key yang harus dipetakan ke normalisasi/hash ORM existing:

| Jenis | Key |
| --- | --- |
| Idempotency | `auth:idem:{digest(scope,key)}` |
| Invariant admin | `auth:guard:last-admin:{scope_id}` |
| Role/access policy | `auth:guard:policy:{scope_id}` |
| Unique account creation | `auth:account:{digest(normalized_email)}` |
| User | `auth:user:{user_id}` |
| Role | `auth:role:{role_id}` |
| Application | `auth:application:{application_id}` |
| Session/family | `auth:session:{session_id}`; gunakan family ID jika invariant rotation ada pada family |

`scope_id` mengikuti lingkup invariant spec utama: global atau aplikasi, bukan penambahan multi-tenant. Digest identifier/key tidak boleh dicatat sebagai metric label dengan cardinality tak terbatas.

Urutan lock: **idempotency → invariant guard (last-admin lalu policy) → account → user → role → application → session/family**. Urutkan ID secara canonical di dalam tiap kategori, hilangkan key duplikat, dan jangan memperoleh guard lebih awal kategorinya setelah memegang aggregate lock. Jika target perlu ditemukan terlebih dahulu, baca untuk discovery, lalu validasi ulang di bawah lock; restart transaksi bila lock set berubah.

Last-admin guard harus digunakan juga ketika mutasi terhadap user/role berbeda memengaruhi invariant yang sama: suspend/deactivate atau revoke assignment admin. System-role metadata/status/permission tidak dapat diubah lewat API; DeleteRole tetap ditunda. Lock user masing-masing saja tidak cukup. State actor yang dipakai untuk authorization harus ikut dilindungi atau diverifikasi dengan revision check yang konsisten; lock target saja tidak mencegah privilege actor dicabut bersamaan.

#### 3.3.3 Arti version dan perubahan efektif

Gunakan semantik spec utama Sections 6.1, 7.4, 8.4 dan 10.1–10.3:

| Counter | Kontrak |
| --- | --- |
| `version` entity user | Optimistic concurrency; effective profile/status/role assignment/application assignment bertambah satu per command, termasuk batch/compound |
| `version` entity role | Optimistic concurrency; metadata/status/permission set role bertambah satu per effective command |
| `auth_version` user | Invalidasi seluruh token setelah ChangePassword, RevokeAllSessions, suspend/deactivate; bukan revision permission |
| `aggregate_version` envelope | Version committed dari canonical aggregate sesuai event contract utama; jangan diam-diam menggantinya dengan counter event baru |
| Session generation/revision | Rotation atau revocation sesi; mapping ke field existing dan aggregate_version session ditetapkan di Phase 0–1 tanpa menjadikannya expected user version |

GrantUserRoles/RevokeUserRoles dan GrantApplicationAccess/RevokeApplicationAccess menaikkan user `version` satu kali ketika efektif, **tanpa menaikkan auth_version atau mencabut sesi**. ReplaceRolePermissions/ChangeRoleStatus menaikkan role version, tanpa mass-update auth_version user. Update profil hanya field yang diizinkan; email login tetap immutable.

ChangePassword dan RevokeAllSessions menaikkan auth_version serta mencabut seluruh session/refresh family atomik. User version juga bertambah satu untuk perubahan user aggregate tersebut. Suspend/deactivate melakukan hal yang sama bersama perubahan status. Activate/reactivate menaikkan user version, tidak perlu menaikkan auth_version lagi, dan **tidak mengaktifkan kembali session/token lama**.

No-op/replay tidak menaikkan counter. Creation memiliki initial version eksplisit. Grant/revoke batch maksimum 20 role bersifat all-or-nothing, menaikkan user version sekali, dan memakai satu event batch. Compound app access memakai satu event aplikasi dengan changed_role_ids; jangan membuat beberapa event dengan user version sama tanpa kontrak baru.

Version gap pada event user dapat terjadi ketika sebagian mutation tidak mengeluarkan event atau consumer hanya subscribe sebagian catalog. Ikuti reconciliation spec utama; jangan menafsirkan aggregate_version sebagai urutan event yang selalu contiguous atau menambah event hanya untuk mengisi gap.

Authorization membaca primary database dengan snapshot konsisten sebagaimana spec utama Section 7.2. JWT lama yang auth_version/sid-nya masih valid tetap dapat mengidentifikasi user setelah role/app change; permission selalu di-resolve ulang. Tidak memakai Redis/event/TTL sebagai sumber keputusan allow pada MVP. Revision/cache policy tambahan bukan dependency MVP.

**Expected version:** request command user yang memerlukan optimistic concurrency membawa `expected_user_version`, dibandingkan dengan user `version`; command role membawa `expected_role_version`, dibandingkan dengan role `version`. Authentication tetap memeriksa token auth_version vs user auth_version secara terpisah. Jangan menambahkan expected_auth_version atau field revision assignment sebagai pengganti field kontrak utama.

#### 3.3.4 Persistent idempotency

`TryLock` mengontrol concurrent execution; persistent record membuktikan idempotency.

- Unique scope mengikuti spec utama: `(actor_id, method, canonical_route_scope/target, idempotency_key)`. `caller_service_id` tetap diverifikasi dan dicatat sebagai caller identity, tetapi tidak ditambahkan sebagai dimensi unique baru. Mapping method/route REST ke command gRPC harus deterministik dan didokumentasikan; actor berasal dari authenticated identity.
- Canonical target merupakan bagian scope dan fingerprint. Key sama pada target berbeda berada dalam scope berbeda dan dapat menjadi command baru. Key sama + scope sama + payload berbeda harus konflik. Untuk batch role, target scope adalah user ID; role_ids termasuk payload set canonical, bukan pembeda scope. Untuk app access, target scope adalah user ID + application canonical ID; alias code/ID harus dinormalisasi.
- Key wajib mengikuti daftar mutasi admin spec utama, bukan otomatis diterapkan ke Register tanpa actor atau self-service. Jika idempotency Register ingin ditambahkan, tentukan sebagai perubahan kontrak terpisah; jangan menebak actor dari payload atau menambahkan scope publik yang belum disepakati.
- Fingerprint mencakup target canonical, field mutasi yang dinormalisasi, expected version, dan presence field/field mask yang bermakna; sorting hanya untuk field set yang urutannya tidak bermakna. Exclude trace/deadline.
- Untuk input password, jangan menyimpan plaintext atau hash password autentikasi dalam fingerprint. Gunakan keyed digest untuk perbandingan request bila diperlukan, dengan key/version terpisah; jangan gunakan digest tanpa key yang membuka dictionary attack.
- Retention response aman: 24 jam sesuai spec utama. Record unexpired tidak boleh dibuang untuk menghemat storage; perubahan retention memerlukan perubahan kontrak eksplisit.
- Setelah retention lewat, key dapat dianggap request baru; constraint dan expected version tetap melindungi state. Jangan menjanjikan deduplication tanpa batas.

| Kondisi | Hasil |
| --- | --- |
| Key baru | Jalankan setelah authorization dan lock/check |
| Scope/key sama + fingerprint sama + hasil committed | Replay hasil tersimpan tanpa mutation/audit/outbox baru |
| Scope/key sama + fingerprint berbeda | `ABORTED/IDEMPOTENCY_CONFLICT`; HTTP 409 di rest-orc |
| Key masih berjalan | `ABORTED/RESOURCE_BUSY`; retry dengan key dan payload yang sama |
| Transaksi rollback | Tidak ada success result committed; retry dapat mengeksekusi lagi |
| Version stale pada request baru | `ABORTED/VERSION_CONFLICT` |
| No-op valid | Sukses `changed=false`, result disimpan bila key wajib |

Autentikasi dan authorization caller yang masih berlaku harus diperiksa sebelum mengembalikan replay. Namun replay yang cocok dikenali sebelum memeriksa expected version target saat ini: version pasti mungkin berubah akibat command pertama. Replay tidak boleh berubah menjadi VERSION_CONFLICT hanya karena command asli sudah berhasil.

Error validasi/authorization tidak dicache sebagai sukses. Jika transaksi dibatalkan, failure auditing mengikuti Section 4.4. Batas timeout/deadline juga berlaku untuk unique insert/idempotency lookup; jangan membiarkan database wait tanpa batas.

Credential response tidak boleh masuk general idempotency cache. Login/RefreshSession menggunakan session contract di Section 3.4. CreateUser tidak mengembalikan password awal; jangan menciptakan response bootstrap secret. Jangan menerapkan cache response generik pada seluruh RPC.

#### 3.3.5 Urutan transaksi per command

Urutan konseptual yang harus diturunkan menjadi call/SQL sequence aktual:

1. Validasi bentuk request, caller service, credential user/refresh sesuai method, dan rate limit.
2. Normalisasi target/payload; siapkan fingerprint serta lock set.
3. Begin transaction melalui adapter existing.
4. Acquire idempotency lock bila wajib; lookup/claim persistent record secara atomik dengan unique constraint.
5. Acquire invariant/actor/target lock dalam urutan deterministik.
6. Baca ulang state authoritative: actor/session, permission, target, status, invariant, dan version.
7. Replay yang cocok: verifikasi caller masih berhak, kembalikan stored result tanpa cek version target sebagai command baru. Key conflict: tolak.
8. Request baru: periksa expected version terlebih dahulu, lalu tentukan effective mutation atau no-op.
9. Terapkan mutation dan relevant version increments; tulis mandatory mutation audit, outbox, dan idempotency result menggunakan transaction handle yang sama.
10. Commit. Baru setelah commit return success/credential. Event publication merupakan pekerjaan relay setelah commit.

Untuk no-op, hanya result dan no-op audit yang diwajibkan policy yang ditulis; tidak ada domain state-change event. Replay tidak menulis ulang mutation/no-op audit. Request ID dari retry boleh muncul pada operational log, tanpa dianggap mutasi baru.

Mutation, mandatory audit, mandatory outbox, dan idempotency result harus commit atau rollback bersama. Repository audit/outbox tidak boleh membuka transaksi lain untuk write atomik tersebut. Commit outcome yang tidak diketahui akibat connection failure tidak boleh diklaim rollback pasti; management client retry memakai key yang sama. Refresh memakai kebijakan respons hilang di bawah.

### 3.4 JWT, credential, dan session contract

Spec final harus memutuskan, bukan meninggalkan placeholder berikut:

| Keputusan | Isi yang wajib ditetapkan |
| --- | --- |
| JWT algorithm | EdDSA source bersama session-aware Login; deployment cutover digate di Phase 6. HS256 hanya baseline historis; key/algorithm binding ditentukan server |
| Issuer | Nilai exact per environment; issuer tetap divalidasi, secret/key tidak disimpan di spec |
| Audience | Diabaikan sesuai keputusan pengguna; aud tidak diwajibkan dan tidak menjadi dasar allow/deny atau trust service |
| Claims | `sub`, `sid`, `iss`, `iat`, `exp`, `jti`, `auth_version`; aud bukan claim wajib. Token type/restricted marker sesuai existing contract; status restricted tetap authoritative |
| TTL | Access/refresh TTL, clock skew, absolute session lifetime, dan apakah refresh memperpanjang idle lifetime |
| Restricted session | Full RPC method allowlist; akses admin/aplikasi ditolak sampai password change selesai |
| Cutover | Nasib JWT lama tanpa `sid/auth_version`, login ulang, dan compatibility protobuf |
| Refresh credential | Opaque token hanya disimpan sebagai hash, sesuai spec utama Section 7.4; session store authoritative |

Usulan awal bila baseline tidak menetapkan: access 15 menit, refresh/session absolute 7 hari, clock skew 30 detik. Nilai ini merupakan **proposal konfigurasi**, bukan klaim sudah disepakati atau teruji. Cocokkan dengan spec utama sebelum finalisasi.

Allowlist restricted session mengikuti spec utama Sections 4.1/7.2: ChangePassword, profil minimum/GetMe, dan Logout; petakan ke full RPC existing. Jangan menambahkan RefreshSession atau list/revoke sesi ke allowlist tanpa keputusan kontrak terpisah. Refresh tidak boleh menghilangkan must_change_password.

#### 3.4.1 Waktu migrasi HS256 → EdDSA

Keputusan user 10 Oktober 2026 memindahkan readiness deployment ke **Phase 6**,
phase terakhir. Phase 1/3 mengimplementasikan source EdDSA/session-aware bersama
real Login dan seluruh consumer setelah backend prerequisites/tests lulus.
Kesiapan actual rest-orc/deployment tidak menahan completion increment backend.
Tidak ada HS256 fallback, dual-algorithm window atau runtime bypass.

Deployment cutover/login ulang menunggu [Phase 6 DEP-01 pada spec MVP](grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment): actual key/TLS/caller rotation/schema/admin,
consumer credential/storage, PENDING response, trusted IP/proxy, RetryInfo dan
login ulang. Gate tetap mandatory dan belum PASS; backend fixtures tidak
menggantikan evidence actual environment. Readiness gagal menahan deployment.
Rollback hanya ke build security-compatible atau stop serving, tanpa memulihkan
revoked sessions/auth_version/legacy credentials. Deployment/migration tidak
dijalankan otomatis oleh revisi rencana ini.

#### 3.4.2 Refresh dan revocation

**Rekomendasi refresh MVP: strict single-use rotation tanpa credential replay cache.** Jadikan keputusan final setelah mengecek spec utama:

- Refresh tidak membutuhkan access token, tetapi membutuhkan service credential, refresh credential valid, session aktif, dan user yang boleh login.
- Rotation, consumed-token state, session generation, dan success audit commit atomik. Response token pengganti baru boleh dikirim sesudah commit.
- Request bersamaan saat lock masih dipegang → `ABORTED/RESOURCE_BUSY`, tanpa menandai reuse hanya karena busy. Client harus single-flight refresh; automatic retry dibatasi karena token bisa sudah consumed saat retry.
- Token yang terbukti consumed dipakai setelah rotation committed → reuse policy: revoke session/family terkait, simpan audit revocation atomik, commit, lalu return `UNAUTHENTICATED/REFRESH_TOKEN_REUSED`.
- Revocation akibat reuse adalah **mutasi yang harus commit**, walaupun RPC mengembalikan error autentikasi. Jangan menjalankannya dalam transaksi yang otomatis rollback karena handler mengembalikan error.
- Token palsu/tidak dikenal belum cukup untuk merevoke sesi berdasarkan session ID dari input tak tepercaya.
- Token pengganti gagal diterima client atau hasil commit tidak diketahui → client login ulang; token lama tidak dipulihkan. Ini konsekuensi strict policy yang harus didokumentasikan pada `rest-orc`.
- Dua refresh overlapping dapat berakhir satu rotation lalu reuse/revocation jika request kedua baru memeriksa token setelah commit pertama. Jangan menjanjikan dua request tersebut pasti menghasilkan sesi aktif; test harus mencakup kedua interleaving.
- Bila produk kelak mengharuskan retry transparan, buat kontrak replay terbatas dengan caller binding, retention, encryption, dan security review desain; jangan memasukkannya diam-diam ke idempotency cache generik.

Logout/revoke satu sesi memeriksa ownership/permission dan authoritative session state, tanpa menaikkan auth_version user. Revoke-all dan ChangePassword wajib menaikkan auth_version serta mencabut seluruh session/refresh family; suspend/deactivate juga demikian. Semua effective session revocation menulis audit dan required outbox sesuai Section 4.2, termasuk revocation akibat refresh reuse.

## 4. Mutation, Audit & Transactional Outbox Contract

### 4.1 Tujuan dan field wajib

Tetapkan kebijakan mutation → audit → outbox yang deterministik untuk setiap state-changing RPC. Kontrak meliputi registration, profile/account, password, login/session creation, refresh, logout, administrasi user, role/permission, dan application access.

Setiap baris kontrak final wajib memuat:

- Full RPC method, mutation target, canonical aggregate ID.
- Actor service/user dan target identity; actor berasal dari credential yang telah diverifikasi.
- Lock key lengkap, lock ordering, transaction boundary.
- Required permission, expected version, serta counter yang berubah.
- Audit action, outcome, dan kebijakan audit no-op/failure.
- Outbox policy: REQUIRED atau NONE, event type, schema version, payload allowlist, aggregate identity/order, consumer atau alasan publication policy.
- Idempotency requirement, fingerprint, replay, no-op, dan daftar error.
- AC ID dan evidence test.

Kontrak ini menerapkan edited Mutation, Audit & Transactional Outbox Contract: tidak semua audit menghasilkan event, tidak ada credential di payload, replay tidak menggandakan write, dan seluruh write wajib harus atomik.

### 4.2 Matriks mutasi, audit, dan outbox

Nama capability/RPC mengacu pada spec utama Section 8–9; full method existing tetap harus dipetakan. Audit action berikut merupakan label rencana, dipetakan ke audit catalog existing tanpa rename sembarangan. Event type mempertahankan catalog Section 10.1 persis; schema_version=1.

Policy **R** = outbox REQUIRED berdasarkan publication policy spec utama meskipun relay/consumer ditunda. Policy **N** = tidak ada event domain dalam catalog untuk mutasi tersebut; audit wajib. Status pending bukan published.

| Capability/RPC | Target canonical | Audit action | Outbox MVP | Counter pada effective change |
| --- | --- | --- | --- | --- |
| Register PENDING | User baru; email normalized untuk uniqueness | USER_REGISTERED | R: `iam.user.created.v1` | Initial user version/auth_version |
| CreateUser admin | User baru | USER_CREATED | R: `iam.user.created.v1` | Initial user version/auth_version; must_change_password sesuai policy |
| UpdateMyProfile / UpdateUser | User ID | USER_PROFILE_UPDATED | R: `iam.user.profile_updated.v1` untuk field profil; field lain harus cocok dengan catalog utama | User version +1; auth_version tetap; email login immutable |
| ChangePassword | User ID + semua session/family | PASSWORD_CHANGED, USER_SESSIONS_REVOKED | R: `iam.user.sessions_revoked.v1` untuk global revocation, reason code PASSWORD_CHANGED | User version +1, auth_version +1; semua sesi/family revoked; login ulang |
| Login / session creation | User ID + session baru | SESSION_CREATED | N: audit saja; tidak menciptakan event session-created | Session initial state; user counters tetap |
| RefreshSession rotation | Session/family ID | SESSION_REFRESH_ROTATED | N: audit saja; tidak menciptakan event refresh-rotated | Session generation +1; user counters tetap |
| Refresh consumed-token reuse | Session/family terverifikasi | REFRESH_REUSE_DETECTED, SESSION_REVOKED bila berubah | R: `iam.session.revoked.v1` bila sesi berubah menjadi revoked | Session revision berubah; auth_version user tetap |
| Logout / RevokeMySession / RevokeUserSession | Session ID | SESSION_REVOKED | R: `iam.session.revoked.v1` | Session revision berubah; user counters tetap |
| RevokeAllMySessions / RevokeAllUserSessions | User ID + semua session/family | USER_SESSIONS_REVOKED | R: `iam.user.sessions_revoked.v1` | User version +1 dan auth_version +1 pada global invalidation efektif |
| ChangeUserStatus suspend/deactivate | User ID + semua session/family | USER_STATUS_CHANGED | R: `iam.user.status_changed.v1`, sessions_revoked=true | User version +1, auth_version +1; semua sesi/family revoked |
| ChangeUserStatus activate/reactivate | User ID | USER_STATUS_CHANGED | R: `iam.user.status_changed.v1` | User version +1; auth_version tetap; sesi lama tetap revoked |
| CreateRole | Role baru | ROLE_CREATED | R: `iam.role.created.v1` | Initial role version |
| UpdateRole / ChangeRoleStatus custom role | Role ID | ROLE_UPDATED | R: `iam.role.updated.v1` | Role version +1; user auth_version/sesi tetap |
| GrantUserRoles / RevokeUserRoles | User ID; role_ids adalah payload batch | USER_ROLES_GRANTED / USER_ROLES_REVOKED | R: `iam.user.roles_granted.v1` / `iam.user.roles_revoked.v1` | User version +1 satu kali per batch; auth_version/sesi tetap |
| ReplaceRolePermissions | Role ID | ROLE_PERMISSIONS_CHANGED | R: `iam.role.permissions_changed.v1` | Role version +1; user auth_version/sesi tetap |
| GrantApplicationAccess | User ID + application ID | APPLICATION_ACCESS_GRANTED | R: `iam.user.application_access_granted.v1` dengan changed_role_ids | User version +1 satu kali; auth_version/sesi tetap |
| RevokeApplicationAccess | User ID + application ID | APPLICATION_ACCESS_REVOKED | R: `iam.user.application_access_revoked.v1` dengan changed_role_ids | User version +1 satu kali; auth_version/sesi tetap; aplikasi lain tidak berubah |

ChangePassword memakai event global-session-revocation yang sudah ada; tanpa membuat tipe event security baru. Reason code aman dan trigger password-change harus didokumentasikan dalam contract event tersebut. Mutation profil/account di luar field yang sudah disepakati tidak diperluas lewat event generic baru.

Compound grant/revoke app access memakai satu event aplikasi; role_ids yang berubah berada di changed_role_ids. Suspend/deactivate memakai event status dengan sessions_revoked=true; tidak wajib event per-session tambahan. Revoke-all/ChangePassword memakai event user.sessions_revoked, bukan event per-session ganda. Independent single-session revoke/reuse memakai event session.revoked. Event no-op/replay tidak ditulis ulang.

Login/rotation hanya audit karena catalog utama belum menetapkan event untuk keduanya. **Ketiadaan consumer runtime tidak boleh menghapus required event revocation yang sudah ada di spec utama.**

Read RPC tidak membuat event/version increment. ListPermissions/ListApplications read-only; DeleteRole, password reset, dan CRUD permission tidak berada di matriks mutation MVP.

### 4.3 Matriks command dan no-op

Request baru memeriksa expected entity version sebelum menentukan no-op; replay committed mengikuti Section 3.3.4. Authorization tetap wajib. Revoke assignment mengubah status REVOKED, bukan hard-delete; grant ulang mengaktifkan record existing.

| Command group | Lock minimum selain idempotency | Key | Expected version | No-op |
| --- | --- | --- | --- | --- |
| Register | Account uniqueness; user/default role bila relevan | Tidak wajib menurut kontrak utama | Tidak untuk creation | Duplicate email → ALREADY_EXISTS, bukan no-op |
| CreateUser / CreateRole | Account/role uniqueness; guard/policy sesuai invariant | Wajib | Tidak untuk creation | Duplicate email/code → ALREADY_EXISTS |
| UpdateMyProfile / UpdateUser | Actor/target user; field allowlist | UpdateUser menerima key sesuai main spec; jangan memperluas kewajiban self-service tanpa kontrak | User version bila optimistic check digunakan; bukan auth_version | Field profil sama → changed=false |
| ChangePassword | User + seluruh sesi/family | Kontrak credential; tanpa token response cache/retry otomatis | Credential current-password/session check; jika expected field ditambahkan gunakan expected_user_version | Tidak menebak no-op dari plaintext; efek credential/global invalidation mengikuti password policy |
| ChangeUserStatus | Last-admin guard bila relevan; actor/target user, sesi/family | Wajib | expected_user_version vs user version | Status sama tanpa efek lain → changed=false |
| UpdateRole / ChangeRoleStatus | Policy/role; system role ditolak | Status change wajib; metadata update menerima key sesuai main spec | expected_role_version vs role version | Metadata/status sama → changed=false |
| GrantUserRoles / RevokeUserRoles | Last-admin guard bila relevan; actor/target user, role batch, aplikasi terkait | Wajib | expected_user_version vs user version | Semua assignment sudah sesuai → changed=false; mixed batch mengubah yang perlu atomik |
| ReplaceRolePermissions | Policy dan role; protected/system role ditolak | Wajib | expected_role_version vs role version | Final permission set sama → changed=false |
| Grant/RevokeApplicationAccess | Actor/target user, aplikasi, seluruh assignment role yang relevan | Wajib | expected_user_version vs user version | Gerbang dan assignment yang diminta sudah sesuai → changed=false |
| Login | User lalu session creation | Tidak memakai general key | Current credential/status | Setiap login sukses membuat sesi sesuai policy |
| RefreshSession | User/session/family dengan state terbaru | Tidak memakai general key | Current generation + session/authentication state; bukan expected_auth_version command | Busy/rotation/reuse, bukan silent no-op |
| Logout / RevokeMySession / RevokeUserSession | Actor/target user dan session/family | Mutasi admin revoke-session wajib; self logout mengikuti session-state idempotency | Tidak menambah expected_session_version tanpa kontrak main spec | Already revoked → changed=false setelah identity/ownership valid |
| RevokeAllMySessions / RevokeAllUserSessions | User + semua session/family | Mutasi admin wajib | Expected user version bila diwajibkan endpoint final; tidak memakai auth_version sebagai optimistic field | Bedakan request baru global invalidation dari replay; jangan increment hanya karena mengulang key |

Grant/revoke maksimum 20 role, satu role invalid → seluruh batch rollback. Grant app memerlukan iam.user.application.grant AND iam.user.role.grant; revoke app memerlukan iam.user.application.revoke AND iam.user.role.revoke. System role code/status/permission tidak dapat diedit lewat API.

Request baru RevokeAll yang memang melakukan global invalidation menaikkan auth_version, meskipun sesi aktif bisa kosong; replay key yang sama tidak mengulang invalidation. Jika endpoint menawarkan no-op saat tidak ada session, policy-nya harus cocok dengan aturan global invalidation, bukan menaikkan version lalu melaporkan changed=false.

Spec final mengembangkan group menjadi full RPC existing/baru yang benar-benar berada di MVP. Permission CRUD, DeleteRole, dan reset admin/publik tetap POST_MVP, tanpa RPC/permission/event placeholder.

### 4.4 Audit success, no-op, dan security failure

- Audit success ditulis satu kali bersama effective mutation; failure menyimpan audit tersebut menggagalkan mutasi.
- No-op memakai outcome `NO_OP` jika attempt audit diwajibkan. Default MVP: management command valid mencatat satu NO_OP audit; replay tidak menambah audit tersebut.
- Failed login, invalid credential, unauthorized command, dan suspicious activity tanpa committed mutation memakai independent security-audit persistence setelah rollback atau melalui mekanisme logging yang andal.
- Usulan policy MVP: persist security audit independen; bila gagal, tulis redacted structured error dan increment metrik failure. Tidak ada fallback yang memberikan akses/token; error dependency vs authentication rejection ditetapkan per RPC dan diuji.
- Refresh reuse yang menyebabkan revocation memakai audit atomik bersama revocation. Attempt tanpa perubahan dapat memakai security audit independen. Jangan mencatat dua successful revocation untuk kejadian yang sama.
- Data audit minimum: audit ID, waktu UTC, caller service, verified actor ID bila tersedia, target ID, action/outcome/reason code, request/trace ID, relevant version, dan field berubah yang diizinkan.
- Jangan menyimpan plaintext password, password hash, access/refresh token, service secret, raw authorization metadata, atau seluruh request body. Bila email/IP dibutuhkan untuk investigasi, tentukan minimization, access control, dan retention; bukan label metrik.
- Security logging saat audit failure harus tetap dibatasi volumenya agar serangan tidak memenuhi disk/memory.

### 4.5 Event catalog, atomic persistence, dan publication

Envelope required event mempertahankan spec utama Section 10.2: `event_id`, `event_type`, `schema_version`, `occurred_at`, `producer`, `aggregate_type`, `aggregate_id`, `aggregate_version`, `actor_user_id`, `correlation_id`, `causation_id`, `traceparent` bila tersedia, dan `data`. Request ID dipetakan ke correlation_id, bukan mengganti nama envelope menjadi request_id/payload. `event_id` stabil untuk delivery retry; jangan menambahkan uniqueness aggregate sequence yang menolak event sah tanpa kontrak baru.

Data event memakai ID/status/field minimum sesuai schema utama. Alasan bebas tetap di audit terbatas, bukan payload event; reason code terkontrol diperbolehkan bila contract menetapkannya. Jangan mengirim credential/hash atau seluruh user/profile. Nama event, schema v1, aggregate identity, dan compound effects tidak boleh diubah tanpa compatibility decision serta consumer impact yang eksplisit.

- Outbox insert, required audit, mutation, dan idempotency result menggunakan satu transaction handle.
- Mandatory audit/outbox/result failure menyebabkan rollback. Failure inject test harus membuktikan tidak ada partial mutation.
- Outbox hanya memuat event sesuai policy R, dengan status pending sesudah commit.
- Tidak ada synchronous publishing ke broker saat transaksi aktif.
- Failure publication setelah commit tidak membatalkan business transaction.
- Relay tahap lanjutan memakai delivery at-least-once, retry/backoff, dan handling pending backlog. Consumer melakukan deduplikasi berdasarkan `(consumer_name, event_id)`; ordering per aggregate harus ditegakkan jika contract membutuhkannya.
- Outbox persistence MVP **bukan** bukti event sudah terkirim. Tanpa relay/consumer, test event retry dan duplicate delivery berstatus POST_MVP/DEFERRED, bukan PASS atau dihapus dari contract.
- Dokumentasikan retention audit/idempotency/outbox, akses baca audit, dan backlog pending; jangan membersihkan unpublished event seolah telah delivered.

## 5. Rate limiting terukur untuk local MVP

### 5.1 Kontrak dan usulan konfigurasi awal

Gunakan in-memory token bucket pada gRPC interceptor atau boundary yang dapat mengakses key tervalidasi. Reuse mekanisme existing; `golang.org/x/time/rate` atau setara boleh dipakai bila dibutuhkan. Tidak perlu Redis, gateway, Kubernetes, atau monitoring infrastructure baru untuk satu instance lokal.

**Angka berikut adalah proposal awal local-development, bukan hasil load test atau limit produksi.** Finalkan dalam spec/config setelah memeriksa pola client. `refill` merupakan token per waktu; `burst` kapasitas bucket, bukan batas fixed-window.

| RPC/group | Key bucket independen yang semuanya harus lolos | Refill | Burst |
| --- | --- | --- | --- |
| Login | Trusted client IP | 30/menit | 10 |
| Login | HMAC normalized account identifier | 5/menit | 3 |
| Register | Trusted client IP | 5/menit | 2 |
| RefreshSession | Trusted client IP | 60/menit | 10 |
| RefreshSession | Verified session ID; tanpa memakai token mentah | 10/menit | 2 |
| Protected read | Verified user ID + RPC group | 120/menit | 20 |
| Admin mutation | Verified actor ID + RPC group | 30/menit | 5 |
| ChangePassword | Verified actor ID + RPC group | 5/menit | 2 |

Jika semua trafik lewat `rest-orc`, peer IP gRPC adalah IP `rest-orc`, bukan IP browser. Client IP hanya boleh diteruskan dari boundary terpercaya setelah credential service diverifikasi, dengan aturan proxy/header sanitization di `rest-orc`. Jika client IP terpercaya tidak tersedia, gunakan service-level fallback yang terdokumentasi; jangan mempercayai metadata IP arbitrer. Scope account/session melengkapi fallback tersebut.

### 5.2 Behavior, memory bound, dan observability

- Semua requests yang sesuai group dihitung, termasuk credential invalid, tanpa membocorkan keberadaan account melalui response limiter.
- Per-key state concurrency-safe. Multi-bucket check/consume harus mempunyai perilaku deterministik saat salah satu menolak; jangan memakai pola reserve/cancel yang belum diuji concurrency.
- Rejected → `RESOURCE_EXHAUSTED/RATE_LIMITED` beserta retry guidance melalui error detail yang didukung `api-contracts`. Tidak ada business mutation. Retry bukan jaminan pasti lolos karena request lain dapat memakai kuota.
- Usulan idle TTL 15 menit, cleanup tiap 1 menit, total maksimum 10.000 entries per process. TTL harus lebih lama daripada waktu refill bucket penuh; menghapus exhausted bucket tidak boleh mereset kuota aktif.
- Ketika cap tercapai: bucket existing tetap berjalan; creation key baru ditolak dengan resource-capacity reason yang aman. Jangan membiarkan map tumbuh tanpa batas atau menghapus bucket aktif untuk membypass limit.
- Log memakai redaction; metrik allowed/rejected, entry count, cleanup, dan capacity rejection memakai label bounded seperti RPC/group/reason, tanpa email/token/IP lengkap/user ID.
- Rate limiter bukan pengganti account lockout, security audit, atau validasi credential. Lockout hanya diterapkan jika spec utama mengharuskannya, dengan policy tersendiri.
- Konfigurasi dapat berubah tanpa perubahan business logic; restart untuk reload diperbolehkan pada MVP. State reset saat restart dan kuota process-local harus dinyatakan eksplisit.
- Multi-instance deployment memerlukan evaluasi distributed rate limiting sebagai POST_MVP.

Test lokal menggunakan clock terkontrol bila memungkinkan: initial burst, exhaustion, exact refill, key isolation, layered rejection, concurrency, cleanup, cap behavior, trusted-IP spoof rejection, error detail, dan redaction. Hindari test yang hanya memeriksa bahwa config field tersedia.

## 6. OpenSpec, keputusan terbuka, dan batas kompleksitas

### 6.1 Deliverable penyusunan spec repository

Pekerjaan dokumentasi berikutnya dibatasi pada:

1. Membuat `docs/grpc-auth-mvp-spec.md` berdasarkan rencana ini.
2. Menambahkan tautan ke spec MVP pada spec utama.
3. Mengisi project context/rules pada `openspec/config.yaml` sesuai format existing.
4. Menyertakan RPC matrix, command contract, mutation/audit/outbox catalog, AC traceability, dan decision log; boleh sebagai section dokumen yang sama agar tidak menambah file tanpa kebutuhan.

Konteks OpenSpec mengarahkan AI membaca spec MVP, aturan bisnis yang dirujuk, dan AGENTS.md. Setiap perubahan harus memiliki capability, kontrak, test scenario, prerequisite, dan syarat selesai. Jangan menganggap spec utama sebagai izin mengimplementasikan seluruh repository sekaligus.

### 6.2 Keputusan yang wajib ditutup sebelum implementasi terkait

| ID | Keputusan | Gate |
| --- | --- | --- |
| DEC-01 | Full RPC method, permission code, status, dan mapping AC spec utama yang aktual | Sebelum spec final |
| DEC-02 | ORM TryLock dialect, connection ownership, cleanup, dan evidence PostgreSQL | Sebelum concurrency-sensitive implementation |
| DEC-03 | Audience ignored; source EdDSA bersama real Login/guard pada increment Phase 1/3; actual readiness deployment/cutover target pada Phase 6 DEP-01 | Backend prerequisites sebelum source; deployment readiness di phase terakhir |
| DEC-04 | Strict refresh rotation/reuse/response-lost policy atau alternatif terbatas yang terdokumentasi | Sebelum refresh implementation |
| DEC-05 | version untuk concurrency, auth_version untuk credential/global invalidation; finalkan mapping aggregate_version/session ke schema existing | Sebelum schema/command implementation |
| DEC-06 | Pertahankan actor+method+target dan retention 24 jam; finalkan canonical route/gRPC mapping serta fingerprint | Sebelum command implementation |
| DEC-07 | Definition/scope last-admin invariant dan seluruh mutation yang harus guard | Sebelum admin implementation |
| DEC-08 | Pertahankan event catalog utama dan session-revocation outbox; lengkapi schema trigger password/reuse, audit labels/failure policy dan retention | Sebelum audit/outbox implementation |
| DEC-09 | Angka limiter, trusted client IP, error detail, cap behavior | Sebelum rate limiter implementation |
| DEC-10 | Aturan AGENTS.md untuk tests/ dan cara menjalankan PostgreSQL integration test | Sebelum verification gate terkait |

Proposal di dokumen ini dapat menjadi default ketika kompatibel dengan baseline. Jangan mengklaim keputusan existing sudah diverifikasi jika source belum dibaca. Konflik dengan aturan bisnis dicatat bersama pilihan dan dampaknya.

### 6.3 Batas pekerjaan dan pencegahan overengineering

Tidak masuk implementasi MVP repository ini: REST/OpenAPI, frontend, consumer CLM, relay RabbitMQ, multi-tenant, MFA, SSO, policy engine generik, layanan baru, DeleteRole, reset password administratif/publik, serta CRUD definisi permission. System/catalog changes tetap melalui migration/seed.

- Reuse library, transaction helper, error catalog, mapper, logger, dan pola test existing.
- Tambahkan package/interface hanya jika memiliki consumer dan kebutuhan nyata.
- Jangan menambahkan Redis, service discovery/mesh, event sourcing, CQRS, framework baru, atau abstraction generik tanpa kebutuhan konkret.
- Jangan membuat scaffold, placeholder RPC, atau dummy yang terlihat berfungsi.
- Jangan melakukan refactor seluruh repository, deployment, atau perubahan repository sibling sebagai efek samping pekerjaan dokumentasi.
- Perubahan `api-contracts`, ORM, `rest-orc`, dan frontend dicatat sebagai external work dengan dependency serta evidence. Bila dependency wajib belum tersedia, AC Auth Service yang bergantung padanya tetap BLOCKED.

## 7. Acceptance criteria, tahapan pengerjaan, dan verification gates

### 7.1 Scope classification dan evidence

Setiap AC spec utama dipetakan satu per satu dengan ID asalnya, bukan hanya dirangkum. Gunakan:

- `MVP_REQUIRED`: wajib diimplementasikan dan diverifikasi untuk menyelesaikan Auth Service MVP.
- `EXTERNAL_REPOSITORY`: pekerjaan repository lain; dapat menjadi prerequisite/blocker AC wajib MVP.
- `POST_MVP`: tahap lanjutan yang tidak menjadi syarat MVP.

Field AC minimum: unique ID, source AC reference, related full RPC/capability, scope, target repository, prerequisite, expected behavior, test method, evidence, dan status. Satu AC boleh mempunyai beberapa external dependencies; klasifikasi dependency bukan alasan otomatis menurunkan mandatory behavior.

Status: `NOT_STARTED`, `IN_PROGRESS`, `BLOCKED`, `PASS`, `FAIL`, `DEFERRED`. DEFERRED hanya untuk POST_MVP atau perubahan scope eksplisit yang tercatat; critical MVP gap tetap BLOCKED/FAIL.

Evidence berisi commit/version yang diuji, command test, environment/DB version, waktu eksekusi, expected/actual result, dan report/log yang sudah direduksi credential-nya. Tidak ada evidence aplikasi dalam revisi rencana ini.

### 7.2 Matriks AC awal yang harus ditautkan ke AC spec utama

| ID lokal | Acceptance criterion | Scope / repository | Bukti minimum |
| --- | --- | --- | --- |
| AC-01 | Baseline schema, migration, seed, contract, dan ORM capability terverifikasi | MVP_REQUIRED / grpc-auth | Inventaris actual-vs-required; tidak rerun seed otomatis |
| AC-02 | Credential service dan JWT user terpisah; bypass REST tidak memberi akses tambahan | MVP_REQUIRED / grpc-auth | Direct gRPC positive/negative tests |
| AC-03 | User/role/custom role, permission union/source, dan application access mengikuti aturan bisnis | MVP_REQUIRED / grpc-auth | Functional dan permission matrix tests per RPC |
| AC-04 | Ownership session dan system-role restrictions berlaku | MVP_REQUIRED / grpc-auth | Cross-user dan protected-role negative tests |
| AC-05 | Role/app change mempertahankan JWT/sesi; keputusan akses berikutnya memakai state terbaru. Credential/global revocation membatalkan token sesuai policy | MVP_REQUIRED / grpc-auth | Spec utama AC-09–AC-16, AC-23, AC-34; authoritative DB tests |
| AC-06 | Last-admin invariant aman pada mutation terhadap target berbeda | MVP_REQUIRED / grpc-auth | PostgreSQL concurrent test dengan shared guard |
| AC-07 | TryLock eksklusif, key independen, cleanup dan pool tidak bocor | MVP_REQUIRED / grpc-auth + dependency ORM | PostgreSQL distinct physical connections, lifecycle tests |
| AC-08 | expected_user_version/expected_role_version stale → ABORTED/VERSION_CONFLICT; no-op tidak naik counter/event | MVP_REQUIRED / grpc-auth | Spec utama AC-26; entity-version/no-op tests |
| AC-09 | Idempotency actor+method+target; same-scope conflict → ABORTED; replay/paralel paling banyak satu effective mutation | MVP_REQUIRED / grpc-auth | Spec utama AC-24–AC-25, AC-35; same/different target tests |
| AC-10 | Mutation/audit/outbox/idempotency atomik; mandatory write failure rollback | MVP_REQUIRED / grpc-auth | Failure injection dan committed-state checks |
| AC-11 | Semua mutation RPC MVP mempunyai audit/outbox; nama/envelope catalog utama tetap; session revoke outbox mandatory | MVP_REQUIRED / grpc-auth | Method inventory dan event compatibility tests; deferred RPC tidak muncul |
| AC-12 | Audience ignored, issuer/session valid; Source EdDSA/session menolak HS256 lama; refresh/restricted policy berlaku; actual deployment cutover di Phase 6 DEP-01 | MVP_REQUIRED / grpc-auth | Claim/algorithm/readiness tests; spec utama AC-17/AC-33 dan PostgreSQL refresh tests |
| AC-13 | Failed login/unauthorized attempt tercatat tanpa bergantung pada transaksi rollback | MVP_REQUIRED / grpc-auth | Security-audit dan persistence-failure tests |
| AC-14 | Rate limiter numerik, concurrency-safe, bounded memory, identity source terpercaya | MVP_REQUIRED / grpc-auth | Clock/concurrent/cap/trust boundary tests |
| AC-15 | Dependency failure bukan izin akses atau permission denial palsu | MVP_REQUIRED / grpc-auth | Dependency fault/error catalog tests |
| AC-16 | Credential tidak muncul di log/audit/event; auth response hanya field credential yang memang ditetapkan | MVP_REQUIRED / grpc-auth | Payload/log assertions dan catalog review |
| AC-17 | Required event pending, tidak dinyatakan delivered tanpa relay | MVP_REQUIRED / grpc-auth | Outbox state dan status documentation |
| EXT-01 | Proto/generated contract tambahan tersedia dari api-contracts | EXTERNAL_REPOSITORY / api-contracts | Contract version/tag dan compatibility checks |
| EXT-02 | ORM bug/gap lock lifecycle diperbaiki bila ditemukan | EXTERNAL_REPOSITORY / ORM | Library version dan DB test; AC-07 BLOCKED selama gap |
| EXT-03 | HTTP/cookie/CSRF, IP sanitization, single-flight refresh, dan error mapping | EXTERNAL_REPOSITORY / rest-orc/frontend | Integrasi boundary; auth-local tests tidak membuktikan UI |
| POST-01 | Relay retry/backoff, duplicate delivery, ordering dan deduplication consumer | POST_MVP / relay + consumer | End-to-end delivery tests; DEFERRED selama belum dibuat |
| POST-02 | CLM authorization/integration dan distributed limiter | POST_MVP / CLM + infrastructure | Contract/e2e/load tests sesuai tahap |
| POST-03 | DeleteRole, reset password admin/publik, CRUD permission | POST_MVP / grpc-auth + api-contracts | DEFERRED sampai aturan bisnis/permission/AC ditetapkan; tanpa placeholder |

Ini bukan klaim bahwa AC spec utama sudah dipetakan seluruhnya. Lengkapi source AC reference dan pecah AC yang terlalu luas saat menyusun spec final.

### 7.3 Tahapan beserta prerequisite dan syarat selesai

Rate limiter dasar untuk public auth dapat disiapkan lebih awal; Phase 5 adalah hardening/completion. Audit/outbox/idempotency diterapkan bersama setiap command sejak awal, bukan ditambahkan setelah fitur berjalan.

| Phase | Prasyarat | Pekerjaan | Syarat selesai |
| --- | --- | --- | --- |
| 0 — Baseline dan contract finalization | Repository/spec utama/AGENTS.md tersedia | Verifikasi actual state, RPC inventory, AC mapping, decision log, external dependency | Baseline tercatat; keputusan prerequisite ditutup; blocker eksplisit |
| 1 — Persistence dan transaction foundation | Phase 0; ORM/proto dependency relevan | Schema yang benar-benar belum ada, transaction ownership, idempotency/audit/outbox repository, PostgreSQL test harness | Compatibility/migration sesuai baseline; rollback, locking dan pool tests PASS |
| 2 — User, role, permission, application access | Phase 1; permissions/status/guards final | Implement command beserta authorization, versions, idempotency, atomic audit/outbox | Functional/negative/no-op/replay/concurrent admin tests PASS |
| 3 — Authentication dan session security | Phase 1; backend key/verifier/caller valid; JWT/refresh contract final; role/status enforcement | Register/login/restricted/password/refresh, atomic revocation outbox, EdDSA source wiring | Backend algorithm/session/TLS/credential dan PostgreSQL refresh tests PASS; deployment readiness/cutover di Phase 6 |
| 4 — Contract completeness | Setiap implemented mutation tersedia | Reconcile method inventory vs matrices; failure auditing, event policy, write-failure/response-lost cases | Semua required mutation punya evidence; tidak ada audit/outbox gap |
| 5 — Rate limiting dan hardening | Key trust dan error contract final; base limiter jika sudah ada | Numeric config, memory cap/cleanup, concurrency, redaction, dependency faults | AC limiter/audit/security/error mapping PASS |
| 6 — Final verification dan readiness deployment | Seluruh mandatory backend behavior tersedia; dependency final | Complete suite, compatibility, lock/pool, AC evidence review dan actual consumer/deployment DEP-01 | Semua MVP_REQUIRED dan Phase 6 DEP-01 PASS; tidak ada critical blocker sebelum deployment |

Readiness deployment dipusatkan pada [Phase 6 DEP-01](grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment), bukan checklist completion increment backend Phase 1–5.

Integration tests dibuat dan dijalankan bertahap sejak Phase 1. Phase 6 merupakan regression/completion gate, bukan pertama kali menggunakan database nyata. Phase 2/3 boleh dibagi menjadi increment capability; prerequisite keamanan setiap increment tetap berlaku.

### 7.4 PostgreSQL integration requirements

Local PostgreSQL atau Docker PostgreSQL mencukupi. Pengujian tidak harus menunggu cloud/OpenShift. Gunakan database test terpisah yang boleh dibersihkan; jangan menghapus data baseline pengguna.

Unit test/mock/fake tetap mengikuti root `tests/` dan AGENTS.md setelah diverifikasi. **Mock bukan satu-satunya bukti concurrency database.** Jika aturan repository belum menyediakan real-DB suite, tetapkan cara yang kompatibel atau catat konflik/blocker secara eksplisit; jangan otomatis mengubah aturan repository atau mengklaim PASS.

| Scenario | Bukti yang diwajibkan |
| --- | --- |
| Key sama, dua transaksi | Hanya satu TryLock diperoleh; gunakan koneksi fisik berbeda dan verifikasi identitas koneksi |
| Key berbeda | Keduanya dapat memegang lock jika tidak berbagi invariant guard |
| Commit/rollback/error/cancellation/panic | Lock dapat diperoleh kembali; tidak ada uncommitted mutation |
| Session-scoped lock/pool | Unlock pada connection owner, tidak reentrant secara tidak sengaja, tidak ada leaked lock ketika koneksi dipakai ulang |
| Concurrent key claiming | Satu mutation, satu mutation audit/event set; duplicate busy/replay sesuai contract |
| Response lost setelah commit | Management replay exact result; refresh mengikuti strict login-again policy |
| Same scope/key, different payload | ABORTED/IDEMPOTENCY_CONFLICT tanpa mutation |
| Same actor/method/key, different canonical target | Scope berbeda sesuai spec utama; dedupe tidak dicampur antar-target |
| Optimistic concurrency | Dua expected_user_version/expected_role_version sama → maksimal satu effective update; auth_version bukan field concurrency |
| No-op dan replay | Counter/outbox/audit mutation tidak bertambah |
| Last-admin | Dua target user/role berbeda tetap tidak melanggar invariant |
| Refresh overlapping | Uji busy-before-commit dan consumed-after-commit; maksimal satu rotation, reuse/revocation sesuai contract |
| Reuse revocation + error response | Revocation dan audit tetap committed meskipun RPC mengembalikan UNAUTHENTICATED |
| Atomic audit/outbox/idempotency | Inject failure setiap required write, state seluruhnya rollback |
| Role/app change vs session | auth_version/sid tidak berubah; JWT lama tetap valid; izin baru mengikuti primary DB |
| Credential/global revocation | Password/revoke-all/suspend/deactivate membatalkan token lama; reactivate tidak memulihkan sesi lama |
| Policy/actor privilege change | Authorization menggunakan state konsisten sesuai protocol |
| Event compatibility | Register→created, profile→profile_updated, role batch plural, app grant/revoke compound, session revocation mandatory outbox |
| JWT cutover | Source session-aware memakai EdDSA-only dan menolak HS256/sid-less; actual deployment cutover/login ulang menunggu Phase 6 DEP-01; audience diabaikan |

Gunakan barrier/channel/test hook untuk mengatur interleaving, bukan hanya `time.Sleep`. Setidaknya dua koneksi fisik diperlukan untuk scenario contention; satu connection pool slot tidak membuktikan exclusivity antar-koneksi.

Jika PostgreSQL tidak tersedia atau dependency ORM belum memenuhi contract, gate terkait berstatus BLOCKED. Mock hijau tidak boleh menggantikan gate itu.

### 7.5 Definition of done implementasi MVP

MVP hanya selesai jika semua MVP_REQUIRED berstatus PASS, external dependency wajib tersedia, dan tidak ada critical blocker.

Setiap PASS memerlukan implementasi tersedia, test yang diwajibkan benar-benar dijalankan, actual result sesuai contract, evidence tercatat, dan versi yang diuji sesuai deliverable. Report membedakan feature existing, baru, external work, dan POST_MVP.

Tidak boleh menyatakan:

- migration/seed selesai hanya karena script ada;
- concurrency benar hanya karena sqlmock PASS;
- event integration selesai hanya karena outbox insert berhasil;
- flow browser selesai hanya karena gRPC method lulus;
- auth response aman tanpa membedakan credential yang memang menjadi output Login/Refresh dari kebocoran ke response management/log.

### 7.6 Definition of done penyusunan spec

Spec dokumentasi selesai ketika:

- Batas repository, MVP/external/POST_MVP, dan source AC references jelas.
- Semua mutation RPC memiliki command serta mutation/audit/outbox contract.
- JWT audience ignored/cutover Phase 3, restricted allowlist, refresh reuse/paralel/response-lost, TryLock, idempotency scope/error, versions, serta rate limiter memiliki keputusan yang dapat diuji.
- Compatibility register mempertahankan event/envelope/session-outbox utama; deferred DeleteRole/reset/permission CRUD tidak menjadi kewajiban MVP.
- Referensi path/anchor diverifikasi terhadap repository actual; perbedaan aturan bisnis diselesaikan dalam decision log.
- OpenSpec YAML yang benar-benar diubah telah divalidasi dengan workflow existing.
- Diff hanya mencakup dokumentasi/context yang diizinkan untuk tahap tersebut.
- Status test aplikasi dinyatakan belum dijalankan bila pekerjaan hanya dokumentasi.

Revisi rencana ini tidak membuat `docs/grpc-auth-mvp-spec.md` atau mengubah `openspec/config.yaml` secara otomatis, karena repository belum tersedia pada tugas ini.

## 8. Instruksi tindak lanjut untuk Codex di repository

Baca rencana ini, `docs/user-management-spec.md`, AGENTS.md, kontrak `api-contracts`, serta ORM version yang benar-benar digunakan. Terapkan keputusan pengguna yang dicatat di compatibility register (terutama audience ignored), dan pertahankan kontrak utama lain; jangan mengganti grant/revoke menjadi global token invalidation. Mulai dari Phase 0. Susun spec MVP dan OpenSpec context dengan verification-first; jangan mengimplementasikan Go/SQL/protobuf hanya karena dokumen rencana telah direvisi.

Laporkan actual baseline, keputusan tertutup/terbuka, daftar external work, AC traceability, dan diff dokumentasi. Jika kemudian diminta implementasi, kerjakan per increment dengan evidence gate. Jangan menyatakan behavior wajib selesai ketika dependency atau PostgreSQL verification masih BLOCKED.

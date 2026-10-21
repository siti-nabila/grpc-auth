# Design

## Context

Motivasi terdapat pada [proposal](proposal.md). Inspeksi 10 Oktober 2026 memakai graph AST sebagai routing, kemudian source working tree. Graph dibangun pada `b89f37270d019c8d0d7823a3ff5d3526874d2232`; perubahan foundation yang belum committed lebih baru daripada graph.

| EXISTING yang diverifikasi | Konsekuensi desain |
| --- | --- |
| `auth_feature.Login` membaca email, memverifikasi Argon2id, lalu mengembalikan `*string` HS256; handler hanya mengisi `token` | Tambahkan structured Login result dan production producer; jangan menyamarkan legacy path sebagai persistent session |
| Foundation 14/36 tasks menyediakan transaction provider, service identity, EdDSA verifier, session persistence dan read authorization snapshot | Reuse komponen tersebut; final consumer/handler activation tetap tugas foundation |
| Bound auth reader, session/audit writer dan shared canonical lock protocol tersedia | Tambahkan refresh persistence adapter pada tx yang sama; repository tidak memiliki lifecycle/lock |
| Belum ada initial refresh writer, access signer atau local limiter | Tiga capability baru diperlukan; generic transaction main specs tetap berlaku |
| `api-contracts v0.0.0-20261010071509-42a9e6fa23ad` menyediakan additive Login fields dan catalog error | Tidak perlu proto/generated/pin/sibling change. Build release tetap diuji dengan `GOWORK=off` |

Go 1.26.2, ORM v1.7.0, JWT/v5, Argon2id existing, konfigurasi `env.yaml`, alur handler → feature → repository dan root `tests/` dipertahankan. Historical [schema preflight](../../auth-session-authorization-foundation/schema-preflight.json) hanya hasil read-only baseline; bukan bukti Login baru atau izin memakai database `user` untuk test. Data refresh ada pada `refresh_tokens.family_id`, bukan tabel family terpisah; `sessions` tidak memiliki entity version.

## Goals / Non-Goals

**Goals:** production producer yang dapat diuji melalui adapter Login memakai dependency nyata yang sama dengan calon composition root; credential baru kompatibel dengan foundation; atomic persistence serta admission control yang terbatas dan deterministic. Component-ready dan activation-ready mempunyai bukti terpisah.

**Non-Goals:** menutup keseluruhan Phase 1/3, menambahkan RefreshSession/Logout/ChangePassword/management RPC, melakukan deployment atau migrasi, mengganti Register/Profile behavior yang dimiliki foundation, atau mengerjakan REST/frontend. Refresh credential awal adalah data siap dikonsumsi increment rotation berikutnya, bukan klaim endpoint refresh telah tersedia.

## Decisions

### 1. Producer mandiri, satu coordinated activation

Feature Login menerima dependencies kecil: credential reader, password verifier, transaction provider/binder, signer, clock, entropy dan safe failed-attempt recorder. Result domain berisi access/refresh credential, session ID, expiry dan restricted flag; generated protobuf berhenti di handler/mapper. Pertahankan context, locale, request ID/deadline/cancellation dari RPC sampai adapter.

Sediakan constructor/handler adapter dengan dependencies eksplisit dan production assembly yang dapat dipakai integration test. Jangan menambahkan flag mode, nil-dependency fallback, user identity dari payload, atau selector dua algoritma. Default registered server beralih hanya pada foundation task 7.4 bersama seluruh consumer. Constructor/component dapat selesai sebelum gate tersebut; tidak ada dependency balik yang mengharuskan producer menunggu activation foundation.

Alternatif mengaktifkan Login terlebih dahulu ditolak karena legacy protected RPC masih memerlukan HS256. Menyediakan token dummy atau membuat session secara manual dalam acceptance test tidak membuktikan producer. Integration harus menjalankan production Login feature, signer dan persistence adapters, lalu verifier/session checker foundation terhadap hasilnya.

### 2. Password proof di luar tx, authoritative recheck di dalam tx

Limiter memutus admission sebelum credential lookup/Argon2. Normalize email dengan aturan existing trim/lowercase dan pertahankan validation existing. Baca candidate dari primary DB dengan request context; password Argon2id diverifikasi sebelum membuka transaction agar durasi lock pendek. Unknown account, mismatch, deleted dan non-ACTIVE menghasilkan catalog `LOGIN_FAILED`/UNAUTHENTICATED yang sama, tanpa mengubah mapping legacy `ErrPasswordMismatch` secara global. Dependency dan cancellation tetap error tersendiri, bukan denial credential.

Setelah proof valid, feature membuka tx melalui injected provider dan memperoleh canonical user lock lalu session lock untuk UUID baru. Discovery user ID bukan authoritative read: bound reader harus membaca ulang email/ID, password hash, deleted/status, `auth_version`, entity version dan `must_change_password` dalam tx setelah lock. Bila credential/state berubah dari candidate, rollback dan generic denial; tidak ada hidden retry menggunakan proof lama. `TryLock=false` menghasilkan `RESOURCE_BUSY`/ABORTED, bukan password mismatch. Login tidak memerlukan policy/last-admin guard atau expected version karena tidak mengubah account/privilege; shared user lock menyerialisasi Login dengan password/status/global revoke commands yang mengikuti protocol.

State read dan write setelah lock hanya memakai handle tx yang sama, tanpa pool fallback atau concurrent writes. `Begin/Commit/Rollback/TryLock` tetap dipanggil feature/features-common; provider/ORM hanya infrastructure. Cleanup error/cancellation/panic mengikuti owner existing. Alternatif memegang tx sepanjang Argon2 ditolak karena menambah waktu lock; alternatif mempercayai candidate setelah lock ditolak karena race password/status.

### 3. Initial session dan refresh credential atomik

Generate session/token/family/jti UUID independen serta refresh secret dari `crypto/rand` melalui entropy abstraction yang dapat diuji. Refresh secret minimal 32 random bytes, opaque base64url tanpa padding. Keputusan encoding baru: digest SHA-256 dari representasi token yang dikembalikan, disimpan sebagai 32-byte `BYTEA`; tidak ada raw credential di database/log/audit/event/idempotency result. Future refresh parser harus memakai representasi yang sama dan strict parsing. Entropy failure menolak issuance tanpa credential.

Gunakan satu waktu UTC dengan precision detik untuk session created/expiry dan JWT timestamps. Session ACTIVE dan refresh expiry sama, absolute 7 hari sejak Login; initial refresh `consumed_at/revoked_at/replaced_by` NULL, `family_id` baru. Nullable metadata hanya dari input terpercaya; AuthRequest existing hanya email/password, jadi jangan mengarang device fields atau menyimpan fallback service ID sebagai IP.

Dalam tx: persist session, initial refresh, `SESSION_CREATED` audit dengan actor user yang sudah terbukti dan safe snapshot; sign access token sebelum commit untuk memastikan signing failure masih rollback. Setelah commit sukses, handler boleh mengembalikan credential. Tidak ada external network/broker di tx. Audit sukses wajib; gagal audit/refresh/session/signing membatalkan seluruh issuance. Commit failure mengembalikan dependency/cancellation error tanpa credential. Commit outcome ambigu dicatat sebagai ambigu, tanpa klaim rollback pasti menghapus commit yang mungkin sudah terjadi.

Alternatif separate session/refresh transactions ditolak karena orphan/partial issuance. Tidak menambah tabel family atau session version; UUID uniqueness/digest uniqueness dan foreign keys memakai schema existing, bukan asumsi SQL DryRun sebagai DB proof.

### 4. Issuer configuration sesuai verifier yang sudah tersedia

Tambahkan typed issuer configuration pada loader `env.yaml`: private-key file PKCS#8 Ed25519, public-key file PKIX Ed25519, key version dan issuer dari `ApplicationName`; limiter account-HMAC secret terpisah. Nama config baru dijelaskan pada example tanpa material secret. Load sekali saat startup, validasi tipe/pair/match/sign-verify dan nonempty issuer/version, copy key bytes; tidak ada weak default. Caller/service token bukan signing key atau HMAC secret. Startup resource cleanup tetap berurutan terbalik bila validation gagal.

JWT hanya EdDSA/Ed25519, `sub` canonical positive INTEGER user ID, UUID `sid/jti`, exact `iss`, `iat/exp`, positive `auth_version` dari locked snapshot. Access expiry minimum waktu issuance +15 menit dan absolute session expiry; response timestamp konsisten dengan claims/session. Audience sengaja tidak diwajibkan/divalidasi. Skew verifier 30 detik tidak memperpanjang absolute DB session expiry. Key version adalah identifikasi operasional; proposal tidak memperkenalkan keyring, JWKS atau masa penerimaan HS256. Uji issuer dengan verifier foundation, bukan verifier alternatif khusus test.

Alternatif membuat signer lokal memakai legacy secret ditolak karena melanggar single-algorithm cutover. Rotasi operasional hanya EdDSA dan harus menjaga key/verifier readiness terkoordinasi.

### 5. Failure audit dan mutation matrix

| Outcome Login | Session/refresh | Audit | Outbox / versions / replay |
| --- | --- | --- | --- |
| Valid, commit sukses | Satu session dan satu initial refresh/family | `SESSION_CREATED` satu dalam tx yang sama | Tidak ada Login event pada catalog; tidak menambah user entity/auth version; tidak menyimpan idempotency result |
| Unknown/mismatch/nonactive atau locked recheck gagal | Tidak ada issuance committed | Safe failed-attempt audit independen setelah rollback bila ada tx | Tidak ada outbox/result/counter mutation |
| Resource busy/dependency/cancel/panic/write/sign/commit error | Tidak mengembalikan credential; rollback bila masih applicable | Failure recorder tidak mengubah outcome menjadi sukses | Tidak ada replay credential; commit ambiguity dilaporkan aman |
| Rate rejected | Tidak ada business DB work | Bounded aggregate telemetry, bukan satu DB audit untuk setiap attack request | Tidak ada credential/session/event |
| Request valid diulang/response hilang sesudah commit | Login berikutnya membuat session berbeda | Audit tiap issuance sukses | Login tidak deduplicated; session awal tetap sampai expiry/revoke |

Failed-attempt audit tidak bergantung pada transaction issuance yang gagal. Gunakan verified service actor bila user belum terbukti; jangan memperlakukan account email sebagai verified actor. Target audit harus safe/nonempty (misalnya full method untuk unknown account), bukan fabricated user ID. Snapshot/reason/request IDs bounded, tanpa email/password/hash/token/raw forwarding header. Recorder memakai request context; cancellation dapat menggagalkan independent audit, lalu redacted bounded failure metric dicatat. Audit failure mempertahankan denial, bukan allow atau mengganti generic denial dengan detail internal. Admission control membatasi attack volume.

General idempotency TTL 24 jam dan version/no-op rules tetap berlaku pada commands pemiliknya; Login bukan mandatory idempotent command dan tidak memiliki no-op/replay credential. Full refresh reuse increment berikutnya wajib single-use rotation, session TryLock, busy bukan reuse, absolute expiry, consumed-token reuse commit revocation/audit/required outbox sebelum UNAUTHENTICATED, dan response-loss login ulang. Proposal ini hanya menyerahkan initial state, tidak mengimplementasikan exception commit-on-denial tersebut.

### 6. Local limiter dengan atomic layered admission

Gunakan satu process-local concurrency-safe token bucket store dan injected clock. Admission memeriksa semua bucket, capacity dan refill dalam satu critical section; consume/allocate hanya bila semuanya lolos. Check yang ditolak tetap memperbarui idle timestamp untuk existing entries, tanpa consume bucket lain. Tidak memakai reserve/cancel yang race. Quota awal mengikuti MVP:

| Existing RPC group | Key | Refill | Burst |
| --- | --- | --- | --- |
| Login | Trusted client IP / fallback verified service ID | 30/menit | 10 |
| Login | HMAC(normalized account) | 5/menit | 3 |
| Register | Trusted client IP / fallback verified service ID | 5/menit | 2 |
| Protected read: Me + ListUsers | Verified user + shared protected-read group | 120/menit | 20 |

Invalid request juga dihitung; nil request tetap menjalani IP dan canonical empty-account bucket sebelum handler menolak validation, tanpa dereference panic atau lookup account. HMAC key menggunakan secret terpisah; key digest tidak menjadi metric/log label. Semua entries process dibatasi 10.000, idle TTL 15 menit, cleanup tiap 1 menit. Existing key tetap usable pada capacity; key baru ditolak capacity, tanpa evict exhausted active key. Cleanup dan shutdown worker memakai lifecycle terkontrol dan deterministic test, bukan wall-clock sleeps.

`x-client-ip` hanya dipercaya dari verified rest-orc caller yang sanitization-nya dibuktikan EXT-02. Terima satu IP canonical valid; missing/malformed/multiple/untrusted metadata tidak menjadi identity key, gunakan verified service ID fallback. Peer gRPC IP biasanya rest-orc dan tidak menggantikan browser IP. Caller yang gagal service authentication tidak mengalokasikan limiter state.

Rejection adalah catalog `RATE_LIMITED`/RESOURCE_EXHAUSTED dengan standard `RetryInfo` dan bounded reason. Transport adapter membangun status memakai catalog encoder existing terlebih dahulu, lalu menambahkan details; outer error interceptor mempertahankan detailed status. Retry duration dibulatkan ke atas dari waktu tunggu bucket maksimum; capacity memberi guidance waktu cleanup, tanpa zero retry loop. Tidak menambah catalog error/protobuf lokal. Metrik hanya bounded group/method/reason.

Alternatif Redis ditunda POST_MVP. Restart dapat mereset local quota; limiter tidak menjanjikan distributed quota/lockout. Future RefreshSession/admin mutation/ChangePassword groups tetap follow-up, memakai angka MVP ketika RPC tersedia, tanpa placeholder registration.

### 7. Full-method pipeline dan ownership

Semua metode tetap melalui TLS dan verified service credential/allowlist terlebih dahulu.

| Full method | User mode / permission | Limiter atau handoff |
| --- | --- | --- |
| `/user.v1.UserService/Login` | Tanpa access JWT; credential proof + ACTIVE | Public layered limiter sebelum validation/Argon2; producer change ini |
| `/user.v1.UserService/Register` | Tanpa access JWT | Public IP limiter; PENDING/token-empty command milik foundation |
| `/user.v1.UserService/Me` | Verified user/session; self; restricted response minimum | Protected-read quota sesudah verified principal; snapshot response milik foundation |
| `/user.v1.UserService/ListUsers` | Verified user/session + `iam.user.read`; restricted ditolak | Shared protected-read quota; permission/snapshot/filter/pagination tetap foundation/existing contract |
| `/profile.v1.ProfileService/UpdateProfile` | Verified self + authoritative mutation recheck; restricted ditolak | Tidak menambah quota baru di increment ini; foundation command/guard |
| `/user.v1.UserService/TesRPC` | Service authenticated, tanpa user JWT | Tetap ada; tidak menambah bucket di luar matrix |

Protected-read admission memakai immutable principal dari session validation; quota tidak menggantikan authorization. Dependency down tetap UNAVAILABLE/fail-closed. Production switch dan regression seluruh registered RPC dimiliki foundation 7.4; producer menyerahkan ready assembly dan evidence, bukan mengaktifkan pipeline sebagian.

## Risks / Trade-offs

- [Hash/status berubah selama proof] → locked authoritative comparison menolak stale proof; uji two-connection ordering.
- [Credential response hilang atau commit ambigu] → tidak cache/replay credential; login ulang dapat menambah session, tanpa melaporkan rollback pasti.
- [Refresh credential belum dapat dipakai oleh endpoint] → response/schema siap untuk lifecycle increment; cutover readiness wajib menyatakan endpoint belum tersedia, tidak mengklaim full MVP.
- [Local quota reset/replika berbeda] → dokumentasikan satu-process guarantee; DB invariant tetap authoritative dan distributed hardening follow-up.
- [Trusted IP owner belum memberi evidence] → fallback verified service ID dapat diuji lokal; real forwarding/cutover EXT-02 tetap BLOCKED.
- [Dedicated PostgreSQL atau deployment keys tidak tersedia] → gate terkait BLOCKED; fake tests/config existence tidak diberi label PASS DB/activation.

## Migration Plan

1. Implement dan verifikasi komponen/injected assembly tanpa mengganti default registered Login/guards. Cocokkan schema test refresh/session/audit dan exact published contract; gap baru menjadi scope eksplisit, tanpa rerun migration/seed baseline.
2. Handoff producer/limiter evidence ke foundation 7.1/7.2. Foundation melengkapi Register PENDING, Me/ListUsers/Profile consumers dan kesiapan seluruh protected RPC.
3. Foundation 7.3 memverifikasi actual key pair/TLS/caller allowlist, minimum ACTIVE platform admin, sanitized forwarding, REST/frontend additive credential/Register token-empty handling, login ulang dan operational legacy session/family invalidation sesuai cutover checklist. Deployment tidak dilakukan change ini.
4. Foundation 7.4 mengalihkan real Login + issuer + seluruh consumer/guards/limiter bersama. Tolak HS256/sid-less; tidak menyediakan bypass/dual algorithm. Role/app changes tetap tidak menaikkan auth_version atau membatalkan session; password/status/global revoke menggunakan invariant lifecycle pemiliknya.
5. Bila activation gagal, hentikan traffic affected deployment atau rollback deployment terkoordinasi sesuai runbook yang telah diverifikasi. Jangan membuka fallback algoritma atau menonaktifkan session/permission check pada deployment aktif. Pastikan credential/session compatibility sebelum traffic dibuka kembali.

## Acceptance ledger

Semua baris berikut **MVP_REQUIRED / grpc-auth** kecuali external rows. Status menilai implementation evidence, bukan kelengkapan proposal. AC asal dipetakan hanya bagian yang tercakup; tidak mengubah arti atau menutup seluruh AC.

| ID lokal | AC asal / scope | Prerequisite | Test dan actual evidence yang wajib | Status saat proposal |
| --- | --- | --- | --- | --- |
| LSP-01 | AC-02/14/15 subset Login eligibility, M-07 | Foundation provider/lock + auth schema | Candidate/locked ACTIVE, deleted, hash/status/version race; generic denial/dependency/context args | NOT_STARTED; source legacy saja |
| LSP-02 | M-07/09 subset issuance atomicity | Bound session/refresh/audit + dedicated test DB | Failure tiap begin/lock/write/sign/commit, cancellation/panic, row counts, two backend IDs/barriers | NOT_STARTED; DB execution BLOCKED sampai DSN test tersedia |
| LSP-03 | M-02/03 issuer subset | Foundation verifier + issuer key config | Actual producer→verifier, claims/TTL/absolute expiry/issuer/audience, wrong key/algo, config failure | NOT_STARTED |
| LSP-04 | M-04 initial-state subset; AC-17 rotation tetap follow-up | Published response + schema digest/FK/UUID | Response mapping, raw-secret absence, family initial state, response-loss/repeated Login | NOT_STARTED |
| LSP-05 | M-11 failed-attempt subset | Service actor + independent audit writer | Rollback-before-failure-audit, unknown actor, failed audit denial retained, redacted telemetry | NOT_STARTED |
| LSP-06 | M-10 existing-group subset | Verified service/principal + injected clock | Numeric burst/refill, atomic layered/concurrent/cap/TTL/cleanup, invalid requests, RetryInfo decode | NOT_STARTED |
| LSP-07 | AC-33 + M-03 restricted producer subset | Locked must_change_password + foundation restricted checks | Restricted Login response; Me minimum allowed/ListUsers denied via actual producer integration | NOT_STARTED; full ChangePassword/RefreshSession follow-up |
| LSP-08 | EXT-01 published contract integration | Version 42a9e6fa23ad, workspace and release builds | Exact tags/six methods, catalog/locale/details, `GOWORK=off` test/build on deliverable | NOT_STARTED; module inspected, no producer tests |
| LSP-09 | EXT-02 / EXTERNAL_REPOSITORY; foundation SAF-09/10 activation | rest-orc/frontend/deployment evidence | Actual TLS/key/client/IP readiness and whole-server cutover/rollback, owned foundation | BLOCKED; no external evidence; not component-completion prerequisite |

Producer component-ready memerlukan LSP-01..08 relevan PASS, termasuk real DB atomicity; activation-ready memerlukan foundation gates dan LSP-09. Acceptance tidak dapat PASS dari schema inspection, manually-created session, skipped suite atau historical foundation test result. Record commit/working-tree hashes, module versions, command/time, safe environment, expected/actual dan limitations dalam `evidence.md` saat apply.

Default tests memakai fixed clock/entropy dan mock/fake SQL di root `tests/`, thin runner `testutils.Run` + scenario `.go`. Opt-in `postgres` suite memakai dedicated explicit test DSN, dua koneksi fisik dengan backend ID berbeda dan barrier, tanpa Docker atau development DB. Uji real locked credential/status change, contention, successful rows/digest/audit, injected write rollback dan response-loss; pool reuse/lock release mengikuti existing verification spec. No test run atau DB connection dibuat oleh proposal ini.

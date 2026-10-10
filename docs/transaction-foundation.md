# Transaction foundation

Implementasi increment `auth-transaction-foundation` memperkeras Register dan
UpdateProfile existing serta menyediakan persistence untuk consumer MVP
berikutnya. Ini belum security cutover atau seluruh Phase 1/MVP.

## Ownership dan terminal outcome

Feature memanggil `TransactionProvider.Begin(requestCtx)`, lalu membuat
`common.TransactionOwner`. Provider infrastructure memakai reusable pool,
`Db.BeginTx` Read Committed, dialect/config ORM v1.7.0, dan mematikan query debug
logging pada transaction yang dapat membawa credential.

Panggil `defer owner.Finish(&err)` langsung pada scope feature. Feature mengambil
lock melalui owner/helper, membagikan handle yang sama ke bound readers/writers,
lalu memanggil Commit eksplisit sebelum success response. Finish hanya recover
panic menjadi generic internal error dan rollback transaction yang belum terminal;
Finish tidak melakukan commit. Operation cause tetap tersedia jika cleanup gagal.
Commit failure menahan credential/success, melakukan best-effort cleanup, dan
tidak mengasumsikan network/connection failure berarti commit pasti gagal.
Jangan retry mutation secara otomatis jika commit outcome unknown.

Owner yang closed menolak lock/commit berikutnya. Rollback cleanup idempotent,
termasuk `sql.ErrTxDone` akibat automatic request cancellation. Request baru
selalu memakai transaction baru. Reader/writer bound menolak nil handle, tidak
menjalankan lifecycle/lock, dan tidak fallback pool. Writer role legacy yang
belum mempunyai consumer juga menerima immutable supplied handle; ini bukan
implementasi command CreateRole. Interface legacy `AuthWriterOld` dan
`ProfileWriterOld` tidak mempunyai runtime implementation/call site.

Register menormalisasi email sebelum validation, melakukan hash sebelum begin,
mengunci policy lalu canonical account, recheck duplicate/default member dalam
tx, dan menulis auth/profile/assignment secara berurutan. Token dibuat sebelum
commit tetapi hanya dikembalikan setelah commit berhasil. Login memakai
normalisasi email yang sama. Semantik JWT/status legacy tetap sampai change
security/PENDING diaktifkan. Profile mengunci session user sebelum owner read,
menolak owner berbeda, mengabaikan payload ID, mengganti tiga scalar secara
penuh, dan mengembalikan ID/owner committed ke handler.

## Lock contract

`CanonicalLockKeys` deduplicate dan mengurutkan idempotency, last-admin, policy,
account/role-code, user, role, application, lalu session. Numeric IDs diurutkan
numerik; account email/key di-hash sebelum menjadi lock key. Semua acquisition
memakai ORM TryLock existing, tanpa advisory SQL baru. Acquired=false menghasilkan
error busy fitur; DB failure, deadline dan cancellation tetap berbeda.

`RequiresLockRestart` mendeteksi predecessor yang baru ditemukan. Consumer harus
rollback, mulai transaction baru dengan deadline/retry bound, mengambil complete
ordered lock set, lalu membaca ulang state. Helper tidak melakukan retry mutation.
Proof discovery saat ini consumer fixture, belum invariant last-admin nyata.

## Audit dan pending outbox

`ActorForVerifiedUser`/`ActorForVerifiedService` hanya memvalidasi storage identity;
consumer wajib membuktikan authentication terlebih dahulu. Tidak mengisi
actor_service dari browser, producer, atau nilai hardcoded. AuditRequest dibatasi
schema action 100, target type 50, target ID 100, reason 500, request/trace 100
karakter, waktu nonzero UTC. Optional field kosong disimpan NULL.

SafeSnapshot hanya memuat version/status, optional name/address/phone
(100/250/20), role IDs, permission codes, application ID, UUID session,
controlled reason code dan no-op. Typed JSON menolak unknown credential fields
serta trailing input; jangan memasukkan request/response mentah atau credential
ke scalar aman. Tidak ada kolom outcome baru.

EventEnvelope mengikuti [catalog MVP](grpc-auth-mvp-spec.md#6-kontrak-mutasi-audit-dan-transactional-outbox):
event_id UUID, schema_version=1, occurred_at, producer, aggregate_type/id/version,
optional actor_user_id, correlation_id, causation_id, optional traceparent, data.
Data hanya IDs/status/changed role IDs/permission codes/application code/
controlled reason code/sessions_revoked; alasan bebas hanya pada bounded audit.
User/role version datang dari entity committed. Session terminal event memakai
UUID aggregate dan revision 2, tanpa membuat session store/version column.
Role grant/revoke batch maksimal 20; compound app event mempertahankan changed IDs.
Outbox awal published_at NULL, attempts 0, next_attempt_at=occurred_at. Writer
tidak memanggil broker dan pending belum berarti delivered.

Consumer memasok waktu dan UUID nonzero dari clock/secure UUID source production;
suite memasok fixed time/IDs. Factory/wiring consumer production belum diaktifkan.
Audit/outbox tidak membuat actor atau correlation metadata sendiri.

## Idempotency dan fingerprint

Scope persis actor user terverifikasi/full RPC/canonical target/key. Key nonempty
maksimal 200 karakter; method 100 dan target 300 sesuai schema. Target menerima
user/role IDs, user/application ID, user/session UUID, user/sessions,
account/canonical email, role-code/canonical code. Application alias wajib
di-resolve oleh consumer ke authoritative ID sebelum fingerprint. Caller service
bukan unique dimension; catat melalui verified audit actor/correlation consumer.

`FingerprintPayload` memuat normalized business fields, expected version,
presence dan canonical sorted/deduplicated sets. Trace/deadline tidak menjadi
payload. Consumer tetap memvalidasi transport/business contract dan field mask.
SHA256 biasa untuk payload tanpa password. Password diberikan terpisah ke
Fingerprinter dengan keyring injected, menggunakan HMAC-SHA256 format versioned;
tidak ada default key, environment read, password atau login hash tersimpan.
Key harus terpisah dari JWT/refresh secret. Consumer wajib mempertahankan setiap
key version yang diperlukan record unexpired 24 jam. Rotasi active key boleh
terjadi selama old key masih tersedia; missing retained key fail closed sebagai
dependency unavailable, bukan idempotency conflict.

Urutan consumer wajib: current identity/endpoint permission -> scope lock ->
bound Lookup -> Replay/fingerprint comparison -> expected target version untuk
request baru -> mutation/audit/required event/Save -> explicit Commit. Replay
identik mengembalikan SafeCommandResult pertama sebelum stale-version check.
Mismatch menggunakan user.idempotency_conflict ABORTED untuk consumer user;
command fitur lain wajib memetakan conflict ke catalog pemiliknya ketika dibuka.
Safe result typed tidak dapat mewakili token response dan menolak unknown fields.

Save tidak menimpa unexpired record, termasuk request identik. Expiry fixed
created_at+24h; tepat pada expires_at record dianggap expired. Replacement
memakai full composite PK plus expires_at condition pada transaction/lock sama.
Tidak ada eviction, cleanup audit, sliding expiry, pool fallback, atau independent
commit. Baru no-op menyimpan changed=false dengan satu NO_OP audit; replay tidak
menambah audit/event/version. General idempotency tidak dipasang ke Register atau
self-profile existing.

## Verification dan batas

Default scenarios menguji lifecycle/context/handle/order, canonical keys,
normalization/owner/response, error locale/redaction, safe serialization,
versioned keys dan SQL/arguments melalui driver mock terisolasi. Opt-in
[PostgreSQL suite](../tests/integration/postgres/README.md) memakai production
provider/owner/ORM/repositories serta backend berbeda untuk committed state,
lock lifecycle, Register/profile, atomic stores, replay/expiry/concurrency/no-op.

Security consumer fixture menulis status SUSPENDED/auth_version dan audit/event
secara atomik, commit, lalu mengembalikan denial; required write failure rollback.
Ini hanya lifecycle subset, bukan RefreshSession, session revocation store,
JWT enforcement, last-admin atau privileged actor command end-to-end.
Commit-failure fixture yang diinjeksi sebelum actual commit juga tidak membuktikan
hasil network failure yang unknown.

Follow-up B-04 tetap MVP_REQUIRED: verified service/user principal, opaque service
token melalui TLS, PENDING registration/admin activation, session/permission
authoritative enforcement, refresh/revocation, management RPC/version policy,
dan pemasangan mandatory audit/outbox/idempotency saat command tersebut dibuka.
Target access 15 menit, absolute refresh/session 7 hari dan skew 30 detik tetap
milik follow-up security; increment ini mempertahankan behavior JWT existing.

# Tasks

Scope hanya `grpc-auth`. Acceptance dan prerequisites mengikuti [design ledger LSP-01..09](design.md#acceptance-ledger); implementation evidence dibuat saat apply, bukan dari proposal ini. Setiap group menambahkan tests di root `tests/` dengan thin runner/testutils dan scenario `.go`. Component delivery tidak mengalihkan default registered server; coordinated activation tetap foundation 7.4. Dedicated DB unavailable berarti gate PostgreSQL BLOCKED, bukan PASS.

## 1. Deliverable baseline dan evidence

- [x] 1.1 Catat working-tree baseline foundation 14/36, constructor/interface yang direuse, exact six full methods, published api-contracts version dan test-schema prerequisites dalam evidence change; verifikasi source/config/contract tetap cocok dengan design dan tidak mengubah sibling/generated/pin atau perubahan user.
- [x] 1.2 Buat `evidence.md` dengan ledger LSP-01..09, component-versus-activation status, actual command/version/time/expected-actual slots serta batas dedicated-test-DB; verifikasi semua belum-dijalankan tetap NOT_STARTED/BLOCKED dan historical schema/test evidence tidak dihitung sebagai producer PASS.

## 2. Issuer, entropy dan configuration

- [x] 2.1 Tambahkan typed Ed25519 issuer/key-file/key-version dan separate limiter account-HMAC configuration melalui loader env.yaml existing; uji missing/malformed/wrong type/mismatched key pair/empty issuer/secret serta startup cleanup, tanpa default secret atau per-request config read.
- [x] 2.2 Implement reusable EdDSA signer yang menerima authoritative claims, clock dan validated key; uji issuer→foundation verifier, canonical sub/UUID sid-jti/auth_version, exact issuer, audience ignored, access 15 menit dibatasi session, dan penolakan wrong key/algorithm/claims tanpa HS256 fallback.
- [x] 2.3 Implement initial credential generation/digest dengan injected entropy: independent UUIDs, opaque base64url minimal 32 random bytes, SHA-256 encoded-token digest; uji entropy failure, deterministic vectors/uniqueness inputs, no raw persistence, serta UTC-second expiry 7 hari yang konsisten dengan JWT/response.
- [x] 2.4 Dokumentasikan config shape tanpa secret, key provisioning/rotation ownership, token/digest encoding dan TTL dalam example serta docs session/issuer; verifikasi YAML shape dan dokumentasi cocok dengan validated loader serta tidak menyatakan deployment keys telah tersedia.

## 3. Bound refresh dan authoritative credential adapters

- [x] 3.1 Tambahkan domain initial-refresh state dan tx-bound writer memakai schema UUID/family_id/BYTEA/expiry/nullable consumption-replacement existing; uji exact SQL/arguments/context/handle, validation dan write error, tanpa family table/version baru atau lifecycle/lock di repository.
- [x] 3.2 Sediakan candidate dan bound authoritative credential readers yang memuat hash/status/deleted/auth_version/entity version/restricted fields; uji normalized account lookup, missing/dependency/cancellation dan same-tx read tanpa fallback pool setelah lock.
- [x] 3.3 Dokumentasikan schema prerequisites/index/FK/uniqueness dan adapter ownership bersama group ini; verifikasi terhadap historical preflight sebagai routing saja, catat dedicated schema verification masih pending, dan laporkan gap tanpa migration/seed otomatis.

## 4. Production Login transaction dan failure audit

- [x] 4.1 Tambahkan structured domain Login result dan injected production constructor; uji input normalization/validation, ACTIVE-only proof, unknown/mismatch/deleted/nonactive generic LOGIN_FAILED, restricted flag dan dependency distinction, tanpa remap legacy catalog secara global atau trust payload identity.
- [x] 4.2 Implement feature-owned issuance Begin → canonical user/session TryLock → authoritative bound recheck → session/refresh/SESSION_CREATED write → sign → Commit; uji call ordering/count/handle/context, changed hash/status/version, busy, rollback tiap write/sign failure, cancellation/panic/commit ambiguity dan credential keluar hanya setelah commit.
- [x] 4.3 Implement independent safe failed-attempt recording setelah issuance rollback dengan verified service actor bila user belum terbukti; uji original denial tetap saat audit gagal/canceled, safe target/snapshot/redaction serta bounded observer tanpa detached credential work atau aborted-tx reuse.
- [x] 4.4 Uji repeated valid Login dan response-loss menghasilkan session berbeda, user entity/auth versions tidak berubah, no Login outbox/idempotency credential cache, serta initial family/NULL state dan sensitive output tidak masuk log/audit/event.
- [x] 4.5 Dokumentasikan Login mutation/audit/outbox/version/response-loss matrix dan error mapping bersama producer; verifikasi setiap branch memiliki scenario terkait LSP-01/02/04/05/07 dan full refresh rotation/reuse/Logout/ChangePassword tetap follow-up eksplisit.

## 5. Local limiter dan transport rejection

- [x] 5.1 Implement single-store atomic layered token bucket dengan injected clock dan angka Login IP 30/min burst 10 + account 5/min burst 3, Register IP 5/min burst 2, protected-read 120/min burst 20; uji burst/refill dan concurrent all-or-nothing admission tanpa reserve/cancel atau partial consumption.
- [x] 5.2 Implement HMAC normalized-account key, trusted rest-orc x-client-ip parsing dan verified service fallback; uji spoofed/missing/malformed/multiple metadata, nil/invalid requests tetap counted, unverified caller tanpa allocation dan bounded metric labels tanpa user/email/IP/token/digest.
- [x] 5.3 Implement max 10.000 entries, idle TTL 15 menit, cleanup 1 menit dan lifecycle stop; uji capacity existing-key admission/new-key rejection, no active exhausted eviction, idle update termasuk rejection, atomic allocation dan cleanup/shutdown dengan controlled clock, lalu jalankan targeted race suite.
- [x] 5.4 Tambahkan rate rejection transport adapter memakai catalog encoder + standard RetryInfo, rounded-up maximum wait dan bounded capacity reason/cleanup guidance; uji outer interceptor preservation, locale, RESOURCE_EXHAUSTED/RATE_LIMITED dan decode published client, tanpa proto/catalog lokal.
- [x] 5.5 Dokumentasikan exact group/full-method order, local restart/multi-process limitation, trust/EXT-02 readiness dan deferred future groups; verifikasi docs/example serta limiter tests memakai angka MVP section 8 dan tidak memperkenalkan quota baru.

## 6. Handler adapter dan composition preparation

- [x] 6.1 Tambahkan production Login response adapter dengan published token/refresh/session/expiry/restricted fields dan dependency injection eksplisit; uji nil/invalid input, exact response mapping, error/locale/context serta no credential saat failure, tanpa mengganti default registered Login sebelum gate foundation.
- [x] 6.2 Sediakan reusable producer/limiter assembly dan interceptor components untuk verified service → public admission → Login/ Register handler serta verified user/session → protected-read admission → authoritative consumer; uji no expensive work sesudah rejection, shared Me/ListUsers quota, restricted Me/ListUsers dan permission/dependency negatives tanpa partial default-server activation atau runtime mode/fallback.
- [x] 6.3 Dokumentasikan handoff constructors/config/error details dan ownership final switch pada foundation 7.4; verifikasi six-method table, TesRPC/list filter-cursor contract tetap, Register PENDING/profile command tetap foundation, dan tidak menambah unavailable RPC registration.

## 7. Dedicated PostgreSQL producer evidence

- [x] 7.1 Tambahkan opt-in `postgres` scenarios pada harness root tests existing dengan explicit dedicated test DSN/guard; uji actual Login session+refresh+audit row counts/digest/FK/uniqueness dan atomic write rollback memakai production adapters, tanpa development DB, Docker atau otomatis rerun migration; unavailable tetap BLOCKED.
- [x] 7.2 Jalankan controlled Login-versus-password/status change serta contention/cleanup/pool reuse dengan dua physical backend IDs berbeda dan barrier; verifikasi stale proof tidak issued, busy berbeda dari denial, context/panic/failure cleanup dan zero partial issuance, tanpa sleep sebagai satu-satunya concurrency proof.
- [x] 7.3 Catat command/tag/opt-in/safe DB identity, deliverable commit/hashes/module versions/time/expected-actual dan response-loss observation dalam evidence; verifikasi no secret/raw credential serta real DB status tidak berasal dari mock/skip/historical preflight. Full refresh reuse/idempotency/last-admin cases tetap change pemiliknya.

## 8. Integration verification dan foundation handoff

- [x] 8.1 Jalankan root integration melalui production Login→issuer→foundation verifier/session/read snapshots, termasuk normal/restricted success-deny, absolute expiry, generic denial, failure rollback, locale/retry details dan direct caller checks; verifikasi component LSP-01..08 pada dependency nyata yang sama, bukan manual session/token producer pengganti.
- [x] 8.2 Setelah targeted tests tiap group PASS, jalankan gofmt, `rtk go test ./...`, `rtk go vet ./...`, relevant race suites dan `rtk proxy make build`, lalu workspace serta `GOWORK=off` release test/build pada published dependency; catat actual hasil/limitations dan jalankan ulang hanya bila ada perubahan/failure relevan.
- [x] 8.3 Serahkan component evidence dan concrete assembly ke foundation 7.1/7.2, serta readiness checklist ke 7.3/7.4; verifikasi LSP-09 external activation tetap BLOCKED sampai actual owner evidence, dengan no partial producer/guard activation, no dual algorithm dan operational login-ulang/rollback prerequisites tertulis. Menyelesaikan handoff tidak menandai foundation gates PASS.
- [x] 8.4 Jalankan strict change validation, main specs validation, local links/AC/matrix review dan diff/status/whitespace review; finalisasi evidence hanya bila required producer component termasuk PostgreSQL PASS, laporkan remaining blockers/lifecycle follow-ups, tanpa archive/full Phase 1/3 completion claim atau sibling/generated/secret changes.

# Proposal

## Why

Login existing masih menghasilkan JWT HS256 tanpa persistent session. Foundation service/session/authorization sudah mencapai 14/36 task, tetapi pengalihan protected RPC akan menolak token tersebut; diperlukan producer Login yang kompatibel sebelum coordinated activation.

## What Changes

- Bangun production Login producer untuk `/user.v1.UserService/Login`: credential valid dan user ACTIVE/nondeleted, locked authoritative recheck, initial session + refresh token/family + `SESSION_CREATED` audit dalam satu transaction, credential response hanya setelah commit.
- Terbitkan access JWT EdDSA/Ed25519 dengan `sub/sid/jti/iss/iat/exp/auth_version`; access 15 menit, session/refresh absolute 7 hari, issuer exact ApplicationName, audience ignored, kompatibel dengan verifier foundation.
- Simpan initial refresh credential sebagai digest BYTEA, tanpa raw token atau credential replay. Refresh rotation/reuse dan RPC RefreshSession tetap increment lifecycle berikutnya yang wajib untuk full MVP.
- Sediakan basic process-local token bucket untuk Login, Register, dan existing protected reads, sesuai angka/TTL/cap/trust boundary MVP. Limiter berjalan sesudah service authentication dan sebelum expensive auth work; retry detail memakai contract published.
- Generic `LOGIN_FAILED` untuk unknown account/password mismatch/nonactive, safe independent failed-attempt audit dan bounded telemetry; dependency/cancellation tetap dibedakan.
- **BREAKING pada coordinated cutover:** Login menerbitkan EdDSA/session-bearing token, bukan HS256; client login ulang. Producer dan verifier/consumer harus diaktifkan bersama melalui gate foundation, tanpa dual algorithm atau runtime fallback.

## Capabilities

### New Capabilities

- `login-session-issuance`: authoritative Login, initial refresh persistence, atomic session/audit, credential response dan failure semantics.
- `eddsa-access-issuance`: validated key/configuration, EdDSA claims/TTL dan interoperabilitas producer dengan verifier foundation.
- `local-auth-rate-limiting`: layered quota Login/Register/protected reads, trusted identity keys, bounded state dan structured retry.

### Modified Capabilities

Tidak ada. Main specs `transaction-ownership`, `transaction-locking`, `command-persistence` dan `postgres-verification` tetap menjadi invariant dependency. Capability verifier `session-authentication` pada change foundation tidak diduplikasi oleh producer.

## Impact

**MVP_REQUIRED / grpc-auth:** auth feature/domain dan injected dependencies, bound refresh/auth readers/writers, session/audit persistence existing, JWT issuer/key configuration, limiter/interceptor, response mapper, composition preparation dan root `tests/`. Pertahankan Go 1.26.2, ORM v1.7.0, Argon2id existing, env.yaml, locale/error boundary, feature-owned transaction dan reusable transports/pool. Detail baseline dan acceptance terdapat di [design](design.md).

**Contract tersedia / api-contracts:** published `v0.0.0-20261010071509-42a9e6fa23ad` sudah menyediakan Login response additive fields, generic login error dan rate-limit error. Request email/password field 1/2, response token field 1 serta six RPC tetap; Login mengisi refresh_token 4, session_id 5, expiry 6/7 dan must_change_password 8. Tidak menambah RPC atau mengubah generated files/pin/sibling. Structured RetryInfo dapat dibangun oleh transport adapter memakai catalog/mapping existing; decoder published telah mendukungnya.

**Dependency foundation / grpc-auth:** `auth-session-authorization-foundation` tasks 2.1–4.2 adalah baseline component-ready di working tree, belum archived/released. Producer component evidence tidak menunggu foundation activation, full management Phase 2 atau seluruh lifecycle Phase 3. Pengalihan handler production serta protected server pipeline tetap satu coordinated gate milik foundation task 7.4 setelah seluruh mandatory consumers, DB dan readiness prerequisites siap; jangan deploy producer atau guard sendirian.

**EXTERNAL_REPOSITORY / rest-orc/frontend/deployment:** TLS/service credential, sanitization x-client-ip, cookie/CSRF/credential handling, key provisioning, login ulang dan readiness. Proposal tidak mengubah repository tersebut, melakukan deployment, migration/seed/reset atau membaca secret. Gap contract/schema baru harus menjadi pekerjaan eksplisit.

**Follow-up MVP_REQUIRED:** RefreshSession strict rotation/reuse, Logout, ChangePassword, session/global revocation, management dan limiter groups bagi RPC yang belum tersedia. Initial refresh issuance tidak mengklaim refresh endpoint atau full Phase 3 selesai. Register PENDING/token-empty, profile mutation dan authoritative RPC consumer wiring tetap dimiliki foundation; read limiter tidak menggantikan permission checks. **POST_MVP:** RabbitMQ relay, Redis/distributed quota, multi-replica hardening, CLM dan UI implementation.

Acuan: [MVP sections 3, 5–10](../../../../docs/grpc-auth-mvp-spec.md), [business/schema/AC](../../../../docs/user-management-spec.md), [foundation current evidence](../../auth-session-authorization-foundation/evidence.md), [producer handoff](../../../../docs/session-authentication.md). Proposal ini hanya planning; implementation dan runtime readiness belum PASS.

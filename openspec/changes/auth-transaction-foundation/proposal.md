# Proposal

## Why

Register dapat commit sebelum panic ditangani, membagi `err` lintas goroutine, dan menjalankan write paralel pada satu transaction adapter; UpdateProfile dapat kehilangan operation/commit error. Fondasi transaksi, locking, dan persistence harus benar sebelum command IAM, sesi persisten, dan cutover keamanan MVP memakai jalur tersebut.

## What Changes

- Tempatkan pemanggilan Begin/Commit/Rollback/TryLock di features atau features/common; feature menerima injected transaction provider, repository hanya read/write dengan handle yang diberikan, dan implementasi teknis provider/lock tetap infrastructure/ORM existing.
- Perbaiki ownership transaksi Register dan UpdateProfile: request context sampai Begin/read/write, satu transaction handle, write berurutan, error commit terlihat, serta rollback saat failure/cancellation/panic.
- Gunakan TryLock ORM v1.7.0 pada account creation dan profil sendiri; normalisasi email trim/lowercase konsisten pada Register/Login mengikuti keputusan MVP, tanpa menghapus collision legacy otomatis. Baca ulang state melalui transaction setelah lock.
- Kembalikan profil committed dengan ID/owner yang benar melalui response existing. Pertahankan full replacement name/address/phone, field numbers/types, JWT dan bentuk credential sebelum cutover.
- Implementasikan persistence audit, pending outbox, dan scoped idempotency dengan transaction adapter existing; buktikan atomicity bersama mutation melalui repository integration tests. Tidak membuat command bus, RPC placeholder, atau menempelkan general idempotency ke Register/self-service.
- Tambahkan suite PostgreSQL terpisah opt-in pada database test khusus untuk lock lifecycle/pool, race account/profile, serta atomic audit/outbox/idempotency. Default suite tetap deterministic tanpa real database.
- Catat schema/constraint actual dan external error-contract prerequisites. Tidak menjalankan ulang migration karena script tersedia atau mengubah sibling sebagai efek samping.

Ini **increment awal Phase 0–1**, bukan seluruh deliverable Phase 1 atau seluruh MVP. Tidak ada perubahan breaking protobuf yang direncanakan; error busy baru membutuhkan catalog/mapping additive dari api-contracts.

## Capabilities

### New Capabilities

Belum ada capability pada `openspec/specs/`; existing Go behavior berikut diperkenalkan sebagai requirement OpenSpec pertama:

- `transaction-ownership`: lifecycle transaksi yang benar dan response setelah commit pada Register/UpdateProfile, termasuk failure dan panic.
- `transaction-locking`: canonical TryLock, ordering, transaction-bound checks, dan pemisahan busy dari dependency error.
- `command-persistence`: audit/outbox/idempotency repositories pada satu transaksi, safe result dan canonical replay/conflict semantics.
- `postgres-verification`: suite opt-in terisolasi dan evidence PostgreSQL yang tidak digantikan mock.

### Modified Capabilities

Tidak ada; inventaris OpenSpec specs existing kosong.

## Impact

| Scope | Area / dependency |
| --- | --- |
| EXISTING | `/user.v1.UserService/Register`, `/user.v1.UserService/Login` untuk email normalization saja, `/profile.v1.ProfileService/UpdateProfile`; common transaction helper, readers/writers dan constructors terkait |
| MVP_REQUIRED dalam change ini | Lifecycle/locking existing write, persistence audit/outbox/idempotency, test harness, schema preflight dan gate evidence; stack handler → feature → repository tetap |
| EXTERNAL_REPOSITORY / api-contracts | Catalog RESOURCE_BUSY auth/user dan mapping ABORTED; semantik dependency/cancellation yang belum tersedia; version published dan GOWORK=off proof. Logical names bukan klaim symbol/code telah tersedia |
| EXTERNAL_REPOSITORY / ORM kondisional | Repair hanya bila distinct-connection tests membuktikan gap; PostgreSQL v1.7.0 memakai transaction-scoped lock, bukan session-scoped |
| Prerequisite lingkungan | PostgreSQL test khusus, explicit DSN/opt-in dan schema sesuai preflight; database baseline user tidak diakses oleh suite |

**Non-goals:** service-token/TLS deployment, JWT/session authentication dan EdDSA cutover, Register PENDING/token-empty switch, session/refresh repositories dan RPC, field-mask/version response additive, RPC admin/permission/status/last-admin business commands, serta limiter. Semua ini tetap MVP_REQUIRED pada change berikutnya; tidak dinyatakan DEFERRED atau selesai oleh fondasi ini. Login/Me/TesRPC/ListUsers tetap memakai contract existing; Me shared-error issue tidak direfactor sebagai side effect.

Audit/outbox/idempotency production wiring menjadi bagian wajib setiap command MVP berikutnya sejak command tersebut diaktifkan. Existing write masih baseline sebelum cutover; suite fondasi tidak membuktikan actor_service/caller terpercaya atau authoritative permission. Jangan menambah audit dengan identitas caller palsu untuk mengakali prerequisite service authentication. Register/profile MVP AC tetap belum PASS sampai command wiring dan security gates terkait selesai.

Relay/consumer RabbitMQ, CLM, UI, distributed limiter, DeleteRole/reset-password/permission-application CRUD merupakan POST_MVP sesuai [spec MVP](../../../docs/grpc-auth-mvp-spec.md). Scope, protocol dan gate mengikuti spec itu serta [spec utama](../../../docs/user-management-spec.md); proposal ini tidak mengubah keputusan TTL, audience, revocation, atau catalog event.

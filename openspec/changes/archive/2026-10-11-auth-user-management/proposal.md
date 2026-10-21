# Proposal

## Why

Register sudah membuat user PENDING dan lifecycle credential sudah tersedia,
tetapi belum ada RPC admin untuk membaca detail, membuat user, mengedit profil
target, atau mengaktifkan/suspend/deactivate akun. Increment ini membuka bagian
pertama Phase 2 dengan permission, last-admin protection dan persistence atomik.

## What Changes

- Kerjakan contract empat RPC di `api-contracts` sebagai bagian terjadwal change
  ini: proto/messages additive, dictionary/mapping/locales, generator, contract
  tests/static analysis dan publication sebelum dependency update `grpc-auth`.
- Tambahkan `/user.v1.UserService/GetUser`, `CreateUser`, `UpdateUser`, dan
  `ChangeUserStatus` mengikuti exact permission pada spec MVP.
- CreateUser menghasilkan PENDING/ACTIVE dengan password awal dari request,
  must_change_password=true, tanpa credential response atau privilege payload.
- UpdateUser hanya name/address/phone dengan presence/mask dan expected version.
- Status transition, self-inactivation denial, global credential invalidation
  pada suspend/deactivate, dan perlindungan admin aktif terakhir.
- Seluruh mutation menyertakan version/audit/pending outbox/idempotency sejak
  command pertama; error precommit rollback dan failure audit independen.
- Pertahankan 15 RPC existing, dynamic ListUsers, numeric role compatibility,
  Register token-empty/PENDING, restricted allowlist dan EdDSA-only lifecycle.
- Dokumentasikan urutan seluruh Phase 2 di [roadmap](../../../../docs/phase-2-management-plan.md).

## Capabilities

### New Capabilities

- `user-administration`: detail/create/update admin yang aman, current authority,
  expected version, safe replay, required audit/outbox dan regression compatibility.
- `user-status-management`: status transition, self/last-admin guard, invalidasi
  credential atomik serta aktivasi ulang tanpa memulihkan sesi lama.

### Modified Capabilities

Tidak ada. Capability authoritative-authorization, command-persistence,
transaction-locking, session-authentication dan local-auth-rate-limiting existing
menjadi prerequisite; invariant-nya diterapkan pada command baru tanpa diganti.

## Impact

- **MVP_REQUIRED / grpc-auth:** handler → features/user → domain → reader/writer,
  shared feature transaction helpers, primary snapshot, existing principal,
  configs/wiring/interceptors, typed permissions/status/audit/event constants,
  tests/ dan evidence. Reuse persistence dan fingerprint helpers existing.
- **EXTERNAL_REPOSITORY / api-contracts:** CreateUser/GetUser message sudah ada,
  tetapi keempat RPC belum ada. Tambahkan method/messages/additive safe fields,
  presence/version/status/reason, feature error mapping/locales, generate/test
  dan publication pada task 1.1–1.6 sebelum dependency update dan wiring.
  Repository ownership tetap api-contracts; pekerjaannya termasuk scope plan,
  bukan hanya menunggu deliverable dari luar. Existing User password field wajib
  kosong; tidak menghapus/renumber field lama.
- **Prasyarat backend:** foundation/lifecycle archives, published management
  contract, versioned password fingerprint key/config dan schema yang cocok.
  Missing prerequisite memblokir task dependent, tanpa dummy/generated lokal.
- **Di luar increment:** role/catalog/assignment/application/effective permission
  RPC pada increment Phase 2 berikutnya; EvaluateAccess/audit read pada Phase 4;
  filter-options RPC, REST/frontend dan actual deployment/migration/bootstrap.
  Deployment tetap mandatory Phase 6 DEP-01; broker/relay/CLM POST_MVP.
- Apply mencakup contract publication dan backend empat RPC; actual results ada
  pada [evidence](evidence.md). Tidak menjalankan migration aplikasi/deployment
  atau otomatis commit/push grpc-auth dan sync/archive change ini.

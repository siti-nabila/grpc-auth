# Proposal

## Why

Kode production existing `grpc-auth` memuat alur transaksi panjang, duplikasi discovery lock dan otorisasi, serta jalur auth/profile lama yang tidak dirangkai server saat ini. Refactor diperlukan sebelum capability berikutnya agar developer junior dapat mengikuti alur bisnis dan menjaga invariant keamanan tanpa menambah abstraction yang tidak diperlukan.

## What Changes

- Refactor production existing secara bertahap mengikuti `grpc-auth/AGENTS.md`, KISS, DRY dan YAGNI; fungsi/signature internal boleh diubah bersama seluruh pemanggilnya.
- Bersihkan jalur legacy yang terbukti tidak memiliki consumer production setelah memisahkan `TesRPC` dan mempertahankan komponen yang masih digunakan snapshot, transport atau persistence aktif. Pensiunkan hanya test khusus jalur yang dihapus; cakupan behavior aktif dipertahankan.
- Sederhanakan orchestration user-management, role-management dan user-role-assignment; satukan algoritma discovery lock yang identik di helper feature kecil tanpa membuat transaction framework.
- Pecah validation, proof/recheck, mutation dan persistence pada auth/session/profile menjadi fungsi dengan tanggung jawab jelas; pertahankan transaksi, urutan side effect dan refresh reuse commit-on-denial.
- Rapikan normalisasi catalog, query construction, adapter mapping dan composition foundation yang terdampak. Pertahankan typed allowlists, query semantics, reusable pool dan dependency injection existing.
- Tambahkan characterization/regression scenarios hanya untuk celah coverage bagian yang diubah; verifikasi tiap tahap sebelum melanjutkan.

Tidak ada breaking change pada public API atau perubahan behavior bisnis yang diusulkan. Tidak ada RPC, status, permission, schema, dependency, konfigurasi runtime atau feature baru.

## Capabilities

### New Capabilities

Tidak ada. `.openspec.yaml` menggunakan `skip_specs: true` karena perubahan ini murni refactor; tidak dibuat capability maintainability atau delta requirement untuk memenuhi validator.

### Modified Capabilities

Tidak ada requirement existing yang diubah. Spec berikut menjadi acuan regresi, bukan delta pada change ini:

- [transaction-ownership](../../../specs/transaction-ownership/spec.md), [transaction-locking](../../../specs/transaction-locking/spec.md), [authoritative-authorization](../../../specs/authoritative-authorization/spec.md), [command-persistence](../../../specs/command-persistence/spec.md).
- [user-administration](../../../specs/user-administration/spec.md), [user-status-management](../../../specs/user-status-management/spec.md), [role-administration](../../../specs/role-administration/spec.md), [user-role-assignment](../../../specs/user-role-assignment/spec.md).
- [session-authentication](../../../specs/session-authentication/spec.md), [login-session-issuance](../../../specs/login-session-issuance/spec.md), [refresh-token-rotation](../../../specs/refresh-token-rotation/spec.md), [password-change](../../../specs/password-change/spec.md), [session-management](../../../specs/session-management/spec.md).
- [iam-catalog-reads](../../../specs/iam-catalog-reads/spec.md), [user-list-sorting](../../../specs/user-list-sorting/spec.md), [service-authentication](../../../specs/service-authentication/spec.md), [eddsa-access-issuance](../../../specs/eddsa-access-issuance/spec.md), [local-auth-rate-limiting](../../../specs/local-auth-rate-limiting/spec.md), [postgres-verification](../../../specs/postgres-verification/spec.md).

## Impact

- **EXISTING, owner grpc-auth:** production auth/profile/user/role/catalog/session, shared feature locking/authorization helpers, handler, reader/writer, interceptor dan startup wiring yang terdampak. Scope mencakup seluruh production untuk penilaian; implementasi dipusatkan pada kandidat konkret di [design](design.md), bukan rewrite setiap file.
- **Public contract:** seluruh 30 full methods yang sudah terdaftar dipertahankan; daftar method/permission, credential mode, persistence invariants dan verification gates ditetapkan di design. Me/GetUser/ListUsers/TesRPC serta token-empty Register tetap kompatibel dengan baseline terbaru.
- **Tests:** root `tests/` dengan runner/scenario/mock existing; legacy-only tests dapat dihapus bersamaan dengan implementasi yang dipensiunkan setelah safety scenarios aktif dipertahankan.
- **MVP_REQUIRED:** keamanan/transaksi yang sudah implemented tetap mandatory. Change ini tidak mengimplementasikan sisa MVP seperti application-access commands, effective-permission RPC, EvaluateAccess atau audit read.
- **EXTERNAL_REPOSITORY:** `api-contracts`, ORM, `rest-orc`, dan consumer lain hanya referensi contract; tidak diedit dan tidak memerlukan dependency update.
- **POST_MVP:** broker/relay, cache otorisasi, distributed limiter, multi-instance hardening, UI dan redesign arsitektur tidak termasuk scope.
- Working tree sudah berisi assignment increment C dan arsipnya yang belum committed. Baseline apply harus mencakup perubahan tersebut, bukan hanya HEAD; tidak ada reset/stash/branch/commit/push otomatis.
- Planning ini hanya menambahkan artifact change. Implementation PASS membutuhkan hasil baru sesuai [tasks](tasks.md); evidence change lama bukan hasil refactor ini. Deployment readiness tetap [Phase 6 DEP-01](../../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment).

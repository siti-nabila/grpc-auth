# Spec Delta

## Purpose

Memisahkan pengujian PostgreSQL nyata yang diperlukan untuk correctness transaksi dari suite default, dengan database test eksplisit dan evidence yang dapat membuktikan concurrency lintas koneksi.

## ADDED Requirements

### Requirement: Real PostgreSQL verification requires explicit opt in

Suite PostgreSQL SHALL terpisah dari default tests dan hanya berjalan dengan opt-in serta DSN database test khusus yang dipilih eksplisit. Suite MUST tidak mengambil default DSN dari env.yaml aplikasi, membuka DB user/development/production, menjalankan Docker, atau memakai migration/seed baseline sebagai setup otomatis.

#### Scenario: Default tests run

- **WHEN** developer menjalankan default suite tanpa opt-in PostgreSQL
- **THEN** suite default tidak membuka koneksi real database dan tetap deterministic menggunakan mock/fake

#### Scenario: Explicit PostgreSQL run lacks required test configuration

- **WHEN** suite PostgreSQL dipilih tetapi DSN/identity database test/opt-in tidak tersedia atau tidak cocok
- **THEN** requested verification gagal dengan pesan redacted, tanpa fallback aplikasi atau destructive setup, dan gate dilaporkan BLOCKED bukan PASS karena skip

#### Scenario: Approved test database is used

- **WHEN** explicit test configuration cocok dengan dedicated database yang telah dipilih
- **THEN** setup/cleanup hanya memengaruhi fixture namespace test disposable tersebut dan seluruh koneksi/pool ditutup setelah suite

### Requirement: Database contention evidence uses distinct physical connections

Concurrency tests SHALL menggunakan setidaknya dua physical connections yang identitasnya dibuktikan, dengan interleaving terkontrol. Satu pool slot, sequential requests, atau wall-clock sleep saja MUST tidak menjadi proof exclusivity.

#### Scenario: Same scope is held on one connection

- **WHEN** koneksi A menahan transaksi dan koneksi B mencoba scope sama sebelum A commit
- **THEN** B belum memperoleh scope; setelah A commit/rollback, transaksi baru B dapat memperoleh scope dan melihat outcome yang benar

#### Scenario: Independent scopes are used

- **WHEN** koneksi berbeda memegang scope berbeda tanpa shared invariant
- **THEN** kedua scope dapat dimiliki bersamaan dan tidak diserialkan hanya karena pool/harness memakai satu koneksi

### Requirement: Foundation suite verifies lifecycle and atomic persistence

Suite SHALL memverifikasi commit/rollback/failure/cancellation/panic, pool reuse, concurrent canonical registration/profile, idempotency same/different target, expiry, response-loss, serta mutation/audit/outbox/result atomicity. Test fixture SHALL memakai actual production repository contracts; evidence MUST membedakan simulated command consumer dari real RPC business flow.

#### Scenario: A required persistence write fails

- **WHEN** failure terkontrol terjadi pada mutation/audit/outbox/idempotency write
- **THEN** database committed state membuktikan rollback seluruh write dan absence success record/event

#### Scenario: Two transactions claim the same idempotency scope

- **WHEN** dua consumer memakai scope/key/payload sama secara overlap
- **THEN** paling banyak satu melakukan effective mutation, contender mendapat busy atau replay sesuai interleaving, dan committed audit/event/result count tidak ganda

#### Scenario: A security-denial consumer fixture commits

- **WHEN** fixture menjalankan production persistence/transaction owner untuk commit security write lalu denial
- **THEN** database menunjukkan commit tetap ada; report tidak menyebutnya bukti real RefreshSession yang belum diimplementasikan

### Requirement: PASS requires actual reproducible evidence

Setiap foundation criterion SHALL mencatat source/commit/module version, test command, environment/PostgreSQL version, waktu, expected/actual result, dan report yang bebas credential. Mock atau keberadaan SQL/source MUST tidak menggantikan required PostgreSQL evidence. Semua target MVP di luar change ini SHALL tetap memiliki statusnya sendiri.

#### Scenario: PostgreSQL is unavailable

- **WHEN** database test tidak tersedia atau dependency yang diperlukan belum published
- **THEN** gate terkait BLOCKED dengan sebab eksplisit dan tidak ditandai PASS dari mock-only result

#### Scenario: Planning artifacts are validated

- **WHEN** OpenSpec proposal/specs/design/tasks lulus validator
- **THEN** status menunjukkan planning selesai saja; implementation tests dan AC fondasi tetap NOT_STARTED sampai benar-benar dijalankan

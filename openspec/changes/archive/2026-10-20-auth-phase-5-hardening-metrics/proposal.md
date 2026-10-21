# Proposal

## Why

`grpc-auth` sudah memiliki limiter dan audit failure counter, tetapi belum
memiliki metrics runtime terpadu; session denial juga dapat mencapai independent
audit sebelum limiter existing. Phase 5 melengkapi observasi lokal dan membatasi
admission verified service sebelum path tersebut tanpa mengganti audit atomik.

## What Changes

- **BREAKING (admission policy):** tambahkan quota aggregate **600 request/menit,
  burst 100 per verified service**, process-local, untuk seluruh 37 RPC existing,
  termasuk Logout dan self/admin revoke. Gate berada sesudah TLS/service/method
  authentication dan sebelum Attempt/Session. Rejection memakai catalog existing
  `RATE_LIMITED`, `RESOURCE_EXHAUSTED` dan `RetryInfo`, tanpa session lookup,
  transaksi audit atau mutation. Ini keputusan user 20 Oktober 2026.
- Pertahankan delapan quota downstream existing. Stage service terpisah:
  admission yang sudah terpakai tidak di-refund ketika downstream menolak.
  Atomic layered consumption tetap berlaku di masing-masing stage downstream.
- Tambahkan satu metrics store dan typed snapshot process-local: RPC
  completion/duration, auth failure/deny/dependency, rate rejection, audit
  persistence failure, outbox backlog/oldest age, pool usage dan sample health.
  Label berasal dari finite allowlist; credential/identitas tidak diekspor.
- Tambahkan aggregate outbox reader ORM dan sampler satu menit, query timeout
  dua detik, menggunakan primary pool existing serta freshness eksplisit.
- Ekspor ringkasan periodik lewat logger existing. Sesuai keputusan user
  20 Oktober 2026, shutdown boleh menunggu write sinkron selesai; collector
  terpisah dari cleanup limiter dan tidak menulis log pada request path.
- Rekonsiliasi coverage M-10/M-11 dan catat evidence P5-01..07 pada source final,
  tanpa menganggap historical PASS sebagai verification baru.

## Capabilities

### New Capabilities

- `local-runtime-metrics`: typed snapshot, bounded RPC/auth/audit metrics,
  aggregate outbox/pool gauges, periodic local logging, freshness dan lifecycle.

### Modified Capabilities

- `local-auth-rate-limiting`: service admission baru sebelum session/audit,
  exception aggregate yang disetujui untuk Logout/self revoke, dan batas
  atomicity antartahap tanpa mengubah delapan quota existing.

## Impact

Owner implementasi **MVP_REQUIRED: grpc-auth**. Source terdampak:
`internal/{configs,interceptors,ratelimit}`, consumer audit pada features/common
dan feature terkait, reader/domain untuk aggregate outbox, logger adapter serta
tests root. `internal/telemetry` hanya dibuat untuk consumer nyata tersebut.
Inventory tetap 37 methods/sembilan services; tidak ada protobuf/schema/error
catalog atau dependency baru. `api-contracts` tetap
`v0.0.0-20261018073250-b4ad807ad5fa`, ORM `v1.8.0-support-subquery`.

**EXTERNAL_REPOSITORY:** compatible published contract tetap prerequisite;
actual rest-orc IP sanitization/RetryInfo/TLS dan consumer readiness menjadi
Phase 6 DEP-01, bukan task pending completion backend ini.
**POST_MVP:** distributed quota, monitoring infrastructure, relay RabbitMQ,
consumer/CLM dan multi-replica. REST/UI, migration/bootstrap, release/deployment,
credential rotation dan update sibling berada di luar scope.

Sumber: [plan Phase 5](../../../../docs/phase-5-hardening-metrics-plan.md),
[MVP scope/AC](../../../../docs/grpc-auth-mvp-spec.md), dan
[hasil Phase 4](../2026-10-18-auth-mutation-audit-reconciliation/evidence/progress.md).
Keputusan user di atas menggantikan batas plan awal tentang tidak menambah
quota dan keharusan sink yang dapat dibatalkan. Tidak ada perubahan runtime
dalam workflow proposal ini; seluruh acceptance implementasi belum PASS.

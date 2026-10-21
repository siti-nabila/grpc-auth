# Proposal

## Why

Increment mutation, EvaluateAccess, dan ListAuditLogs sudah memiliki evidence
terpisah, tetapi belum ada rekonsiliasi lengkap seluruh mutation terhadap satu
matriks audit/outbox/version/idempotency pada deliverable yang sama. Phase 4
memerlukan pemetaan dan verification lintas increment untuk menemukan gap nyata
sebelum menyatakan konsistensi backend selesai.

## What Changes

- Rekonsiliasi 21 mutation methods, termasuk rotation/reuse RefreshSession,
  terhadap matriks MVP; pertahankan inventory 37 methods/sembilan services.
- Perluas skenario normatif `command-persistence` pada atomicity, event cardinality,
  no-op/replay, commit-before-denial, dan safe producer/error outcomes. Requirement
  statement dan seluruh skenario existing dipertahankan; skenario tambahan
  memperjelas penerapan kontrak yang sudah disepakati pada production consumers.
- Petakan actual source/test/evidence dan tutup gap yang terbukti melalui minimum
  fix serta assertion yang relevan. Scope tidak mengasumsikan adanya bug baru.
- Izinkan refactor kecil untuk consumer nyata dengan semantik sama sesuai
  [plan bagian 2.1](../../../../docs/auth-mutation-audit-reconciliation-plan.md#21-aturan-kerja-dan-reuse-fungsi),
  dengan flow bisnis success/failure dan seluruh caller regression tetap terjaga.
- Verifikasi shared transaction, ordered locks, exact side effects, version,
  safe snapshots/envelope/results, current authority sebelum replay, dan
  independent failed attempt setelah cleanup. Required PostgreSQL evidence
  terpisah dari default controlled suites.
- Catat acceptance MAR-01..07 beserta source identity, pinned published dependency,
  workspace/standalone results, dan unresolved mandatory gaps secara eksplisit.

## Capabilities

### New Capabilities

Tidak ada capability atau RPC baru.

### Modified Capabilities

- `command-persistence`: tambah skenario lintas production mutation pada lima
  requirement existing; preserve semua statement dan skenario sebelumnya,
  termasuk pengecualian security mutation yang commit sebelum denial.

## Impact

| Scope / owner | Baseline dan pekerjaan target |
| --- | --- |
| EXISTING / grpc-auth | Feature-owned transactions, shared audit/outbox/idempotency domain, event builders dan lifecycle/management suites tersedia; source inspection bukan bukti closure reconciliation |
| MVP_REQUIRED / grpc-auth | Handler/feature/repository/interceptor consumer yang terbukti terdampak; root tests/, targeted/full controlled dan dedicated PostgreSQL normal/race; evidence MAR-01..07 |
| EXTERNAL_REPOSITORY / api-contracts | Ownership source/generated/catalog tetap; pinned published contract existing dipakai untuk regression. Tidak ada planned API/dependency change; gap baru memerlukan scope/otorisasi terpisah |
| EXTERNAL_REPOSITORY / ORM | Pakai adapter TryLock dan primary handle existing. Perubahan sibling hanya bila gap terbukti dan diotorisasi terpisah |
| EXTERNAL_REPOSITORY / deployment/consumer | Phase 6 DEP-01 tetap mandatory sebelum deployment dan tidak menjadi task pending completion backend ini |
| POST_MVP / relay/consumer/CLM | Broker delivery, projection reconciliation, CLM resource policy dan distributed limiter tetap di luar increment |

Acuan: [plan](../../../../docs/auth-mutation-audit-reconciliation-plan.md),
[matriks mutation MVP](../../../../docs/grpc-auth-mvp-spec.md#6-kontrak-mutasi-audit-dan-transactional-outbox),
[acceptance](../../../../docs/grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence).
Keputusan user: selalu ikuti AGENTS.md root/terdekat; reuse diperbolehkan dengan
business flow lama terjaga. Planning ini hanya menghasilkan artifact; implementasi
menunggu apply eksplisit.

Compatibility: tidak ada breaking change atau migration yang direncanakan.
Register PENDING/token field 1 kosong, credential-only Login/Refresh, TesRPC,
existing tags/types/errors/locale/pagination, restricted allowlist, EdDSA-only,
issuer/TTL/session semantics, exact permission dan idempotency tetap berlaku.
Non-goals: RPC baru, contract/catalog baru tanpa scope, patch Graphify, broad
refactor, Phase 5 metrics overhaul, REST/UI, production retention, migration,
bootstrap, deployment/cutover, broker/relay/consumer atau CLM integration.

## Implementation status

Backend reconciliation COMPLETE on 18 October 2026; all23 tasks and MAR-01..07
verified on the same source snapshot and published dependencies. No production
source/business flow changed; six test files add commit acknowledgement-loss
and producer correlation coverage. See [actual evidence](evidence/progress.md).
Phase 5 broader hardening/metrics and Phase 6 DEP-01 remain separate.

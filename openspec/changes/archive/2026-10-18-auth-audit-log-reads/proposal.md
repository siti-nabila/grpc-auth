# Proposal

## Why

Audit mutation dan security attempt sudah disimpan, tetapi belum ada RPC untuk membacanya secara aman. Setelah EvaluateAccess selesai, ListAuditLogs melengkapi bagian audit-read Phase 4 agar pemegang `iam.audit.read` dapat memeriksa riwayat tanpa akses langsung ke database.

## What Changes

- Tambahkan `/audit.v1.AuditService/ListAuditLogs` dengan TLS, verified caller yang diizinkan exact method dan namespace IAM, serta ACTIVE unrestricted user/session dengan exact permission `iam.audit.read` dari primary snapshot yang sama dengan hasil dan total.
- Ikuti keputusan user 18 Oktober 2026: query minimal dengan filter actor/action/target/waktu, pagination, urutan tetap `occurred_at DESC, id DESC`, tanpa search atau field selection. Sort kustom juga tidak tersedia.
- Return safe audit metadata dan typed before/after allowlist; historical actor/target tidak harus masih aktif. Audit ID mengikuti BIGINT existing, terpisah dari batas INTEGER user ID.
- Gunakan shared protected-read quota existing dan independent failure-attempt audit existing. Successful list tidak menghasilkan success audit, event, version increment atau persistent replay.
- Rencanakan additive protobuf/generated contract di `api-contracts`, publication/pinning yang diotorisasi dan verification workspace/standalone. Inventory target menjadi 37 methods/sembilan services dengan prior 36 tetap kompatibel.

## Capabilities

### New Capabilities

- `audit-log-reads`: authorized administrative history, exact filters, fixed deterministic pagination, safe typed output, snapshot/error/admission behavior dan evidence.

### Modified Capabilities

Tidak ada requirement existing yang diganti. Change memakai [authoritative authorization](../../../specs/authoritative-authorization/spec.md), [command persistence](../../../specs/command-persistence/spec.md), [service authentication](../../../specs/service-authentication/spec.md) dan [local admission](../../../specs/local-auth-rate-limiting/spec.md). Dynamic query catalog existing tetap milik capability-nya; tidak diperluas ke audit.

## Impact

| Scope / owner | Existing → target | Prerequisite / status baseline |
| --- | --- | --- |
| MVP_REQUIRED / grpc-auth | AuditRecord/AuditWriter/SafeSnapshot, primary snapshot provider, resolver dan protected-read pipeline tersedia → domain query/result, audit reader, feature, handler dan registration baru | Source telah diperiksa; behavior RPC baru NOT_STARTED |
| EXTERNAL_REPOSITORY / api-contracts | PageQuery/PageMeta dan common error catalogs tersedia; audit proto/generated service belum ada → additive audit v1 messages/service dan compatibility tests | BLOCKED sampai scope sibling diotorisasi saat apply dan published version tersedia; otorisasi kontrak EvaluateAccess sebelumnya tidak mencakup change ini |
| MVP_REQUIRED / verification | Reuse root tests/, controlled registered gRPC dan opt-in PostgreSQL harness → filter/BIGINT/snapshot/redaction/cleanup evidence | NOT_STARTED; source/schema inspection bukan PASS aplikasi |
| EXTERNAL_REPOSITORY / REST/UI/deployment | Consumer integrasi dan retention/archival operasional | Phase 6 DEP-01; terpisah dari completion backend |

Production scope yang direncanakan: `internal/features/audit/`, `internal/handler/audit.go`, `internal/repositories/domain/` dan `reader/`, `internal/configs/foundation.go`, session/rate integration, permission constant, exact caller example di `env.example.yaml`, serta API docs. Reuse transaction provider dan error/localization; tidak menambah dependency/runtime infrastructure.

Non-goals: edit/delete audit, export/detail RPC, free-text search, dynamic sort/projection, audit retention job, schema migration/seed, session admin read, per-application CLM audit, full mutation/outbox reconciliation, Phase 5 metrics overhaul, broker/relay, REST/UI dan deployment. Audit read tidak mengubah Register PENDING/token-empty, TesRPC, pagination RPC lama, credential-only Login/Refresh, EdDSA-only atau session lifecycle.

Traceability: [MVP full RPC matrix](../../../../docs/grpc-auth-mvp-spec.md#42-matriks-full-rpc), [MVP criteria](../../../../docs/grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence), [audit business contract](../../../../docs/user-management-spec.md#11-audit-dan-observability), AC-31 dan M-01/02/03/05/06/07/09/10/11 serta EXT-01, dengan ledger ALR pada design. Proposal ini planning-only; tidak memberi izin implementasi sibling, Git publication, migration atau release.

## Implementation status

Backend COMPLETE on18 October2026; all22 tasks and required contract/default/race/vet/build/dedicated PostgreSQL normal/race gates pass in workspace and standalone modes. Planning baseline statuses above are historical. Actual ALR-01..06/ALR-EXT-01 results and immutable dependency/source identity are in [implementation evidence](evidence/progress.md). Phase6 DEP-01 remains separate; main-spec sync/archive has not been performed.

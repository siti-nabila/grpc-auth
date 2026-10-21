# Mutation audit reconciliation evidence

Date: 18 October 2026 (Asia/Jakarta). Status:**BACKEND_COMPLETE,23/23 tasks complete**. MAR-01..07 required backend gates PASS on the same Go source snapshot and published dependency pins. Phase 5 broader hardening/metrics and Phase 6 DEP-01 remain separate.

## Actual changes

MAR-G01 adds common ownership plus production UpdateUser/refresh-reuse acknowledgement-loss cases. The real underlying Commit succeeds before its acknowledgement is lost; dependency identity/cause is preserved, no success/credential escapes, and mutation is not automatically retried. DB state remains atomically committed. Keyed management retry with the original version replays once, while reuse retry never duplicates a terminal event or revives the successor. Existing role/assignment/app uncertain-commit cases were reused. The new production-consumer checks call the real service/repositories; existing handler/error-boundary suites additionally prove safe RPC mapping and credential suppression.

MAR-G02 adds six metadata variants across all three shared UserEvent/RoleEvent/SessionRevokedEvent builders. Correlation/causation/trace and argument-owned actor remain correct; duplicate/invalid metadata follows existing fallback and authentication sentinel values do not enter payload. Existing all 12 catalog and typed safe persistence/fingerprint/result checks remain unchanged and pass.

MAR-G03 [reuse review](reuse-review.md) and [caller inventory](reuse-callers.json) retain existing shared helpers and namespace-specific consumer wrappers. No production refactor/framework was needed. Success/failure ordering, context/transaction/locks, errors and existing business behavior remain unchanged.

Six Go test files plus the source plan completion note changed. Production source, external contract/dependencies, schema and runtime config were unchanged. [Baseline](baseline.json), [all-Go snapshot](code-snapshot.json) and [deliverable source](implementation-source.json) record dirty-tree/source identity and 776 preserved unrelated baseline files. No grpc-auth commit/push was performed.

## Verification

| Gate | Actual result | Status |
| --- | --- | --- |
| Fresh targeted controlled suites |3387 pass,0fail/skip | PASS |
| Focused ownership/correlation/safe persistence |192 pass,0fail/skip | PASS |
| Targeted production PostgreSQL normal/race |387 pass each,0fail/skip;68 physical-backend log records in normal run and distinct-connection/barrier assertions | PASS |
| Full default workspace normal/race |3523 pass each,0fail/skip | PASS |
| Full default standalone normal/race |3523 pass each,0fail/skip | PASS |
| Full PostgreSQL workspace normal/race |586 pass each,0fail/skip | PASS |
| Full PostgreSQL standalone normal/race |586 pass each,0fail/skip | PASS |
| Vet/build workspace and standalone |All four exit0; output binaries only in temporary paths | PASS |
| Wire/registration |All 20 descriptors identical workspace/standalone;37 methods/nine services unchanged | PASS |
| Shape/scope/docs |Metadata/example YAML, links/anchors, strict OpenSpec, gofmt and diff/scope checks | PASS |

[Backend receipts](backend-checks.json) and [PostgreSQL receipts](postgres-checks.json) retain exact commands/mode/UTC times/exit/counts/log hashes, code snapshot and per-suite pass counts. PG receipts retain physical-backend records. All required test runs use count=1. Raw logs remain temporary; durable evidence excludes DSN/password/token/credential values. The dedicated guarded DSN is supplied only to the opt-in child process. Default suites use controlled dependencies; real suites use only owned dedicated PostgreSQL fixture namespaces. No Docker, application DB, baseline migration/seed or application server startup was used.

## Acceptance

[21-method matrix](mutation-matrix.md), [34-scenario traceability](scenario-traceability.md), [machine-readable runners](scenario-traceability.json) and [closed gap ledger](gap-ledger.md) preserve expected/actual/source/test identity. All 13 original scenarios remain, with 21 additional scenarios covered. Actual criteria below preserve meanings from the [design ledger](../design.md#acceptance-ledger).

| Criterion | Actual backend evidence | Status |
| --- | --- | --- |
| MAR-01 |All 21 mutation methods/outcomes mapped to production source and current controlled/PG suites;37/nine inventory | PASS |
| MAR-02 |Typed safe snapshot/event/result/fingerprint, all 12 catalog events, correlation/actor and credential redaction | PASS |
| MAR-03 |Required-write rollback, no-op/replay/conflict/expiry/version/response-loss and unknown commit; exact DB state/counts | PASS |
| MAR-04 |Login/rotation audit, busy/reuse commit-before-denial, lost acknowledgement, single/global/password/status invalidation, restricted/credential compatibility | PASS |
| MAR-05 |Current exact authority, compound role/app/assignment counts, system-role/last-admin and two-physical-backend/barrier races | PASS |
| MAR-06 |Registered denials, safe locale/errors, cleanup before independent attempt, recorder failure/panic, no duplicate Login/reuse attempt and no per-rate-rejection DB audit | PASS |
| MAR-07 |Same source/modules, workspace/standalone full normal/race, vet/build, descriptors/checksums, manifests and safe receipts | PASS |

M-09 is dedicated test DB/locking evidence, not actual target schema/bootstrap/minimal ACTIVE admin verification. Target TLS/key/caller rotation and consumer readiness remain Phase 6 DEP-01. MAR-05 does not claim CLM resource-policy integration; pending outbox is not delivered. Full Phase 5 hardening/metrics, REST/UI, production retention and broker/consumer work remain separate. No mandatory reconciliation gap remains unresolved. Main-spec sync/archive is a subsequent workflow.

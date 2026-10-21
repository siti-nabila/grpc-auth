# Effective permission reads E — final evidence

Backend E COMPLETE,24/24 tasks; all required runtime gates and final artifact/scope checks PASS. Date: 14 Oktober 2026, Asia/Jakarta. Final artifact/scope checks accompany the task checklist.

## Deliverable identity

- grpc-auth HEAD baseline: 2462cb09d0f8e7eef3b17a70456451b8bd2dd6ec, with existing B/C/D/refactor user changes preserved. E remains in the working tree; no grpc-auth commit/push was performed.
- [Final source manifest](final-source.json): runtime/test SHA256 855468acaee0c4068a42d3b3a6a94fb1ae579e9778acbff337475d5c1c361e2f. The manifest defines the digest and per-file hashes, excluding docs/evidence/ignored binaries.
- api-contracts commit 545701c82e2234041df1bf4d2ce4fef7665dd75d, pushed to existing origin/feat/role-management after explicit user authorization. grpc-auth pins published module v0.0.0-20261014032725-545701c82e22.
- Module checksum h1:UR+dSQt+mzhQAMRcVeSzN3v8fdNkUpaMg6RU95XZ6r0=; go.mod checksum h1:Jb5bNxnX/aW6YWghT/pXorXlmIDRRxXltY8gL7Udzg4=. GOWORK=off download/list confirmed immutable version and origin commit.
- [Baseline](baseline.json) records initial HEAD/status/nonignored-file hashes, graph freshness and exact 33 pre-E methods. [Prepared patch](contract-e.patch) and [draft manifest](contract-draft-manifest.json) retain the reviewed 13-file contract change. Actual generated files matched that draft, regeneration was stable, and unrelated proto/response.json remains untracked/unchanged.

## Behavior and affected areas

GetMyEffectivePermissions and GetUserEffectivePermissions are registered in the existing UserService: exactly 35 methods across seven services. Self derives identity exclusively from verified claims. Admin requires current iam.user.read, including self-target, plus IAM service namespace. Both reject restricted callers. Optional filters enforce service namespace visibility independently of actor authority; malformed/disallowed inputs never become unfiltered queries.

Two explicit snapshot functions use provider/now parameters and the existing SessionReadHandler, primary reader and transaction owner. No read-service constructor, interface, binder, lock, connection or framework was added. Changed areas: domain input/result; feature functions; handler mappings; thin registration delegates; interceptor method admission; root tests/scenarios/fixtures; published contract dependency; API/allowlist documentation. See [API](../../../../../docs/effective-permission-api.md).

Unique sorted permission codes preserve all active role sources under existing application/access gates. Admin can inspect retained ACTIVE-role configuration on nondeleted PENDING/SUSPENDED/DEACTIVATED targets, with required account status. This view grants no access, restores no session and excludes REVOKED/DISABLED history. Reads change no entity/auth_version/session/refresh/success-audit/outbox/idempotency result. Failure audit stays independent and preserves the original error if persistence fails.

## Actual verification

[Command/mode results](verification.json) record timestamp semantics, safe environment, final source identity, expected/actual, exit codes and status. Timestamps are result-recording times for this apply session. Go 1.26.2 darwin/arm64; writable GOCACHE; workspace resolves actual sibling, standalone resolves the published module.

| Required gate | Workspace | GOWORK=off |
| --- | --- | --- |
| api-contracts go test ./... | PASS, 130 tests | PASS, 130 tests |
| api-contracts go vet ./... | PASS | PASS |
| grpc-auth go test ./... | PASS, 3234 tests | PASS, 3234 tests |
| grpc-auth go test -race ./... | PASS, 3234 tests, no race report | PASS, 3234 tests, no race report |
| grpc-auth go vet ./... | PASS | PASS |
| grpc-auth make build | PASS | PASS |
| Full PostgreSQL -tags postgres -count=1 | PASS, 527 tests | PASS, 527 tests |
| Full PostgreSQL with -race | PASS, 527 tests, no race report | PASS, 527 tests, no race report |

Targeted domain/feature/handler/interceptor/registered tests passed before broad gates. Targeted PostgreSQL E passed normal/race (14 tests), including four RR snapshot interleavings using distinct physical backend IDs and channel barriers. Full DB results also include prior last-admin, refresh/reuse, idempotency, atomicity and pool cleanup regressions. Private env-test.yaml selected an explicit opt-in dedicated database with identity checks; only fixture-owned schemas were created/cleaned. No Docker, application DB/migration/seed, runtime server startup or deployment was used.

## Requirement ledger

| ID | Actual proof | Status |
| --- | --- | --- |
| EPR-01 | Self/admin caller and current exact permission, self-target denial, restricted/session/auth_version failures; all 35 anonymous/method-allowlist boundaries; forged metadata cannot redirect self | PASS |
| EPR-02 | Sorted overlap sources; revoked/disabled/application/access/mapping gates; optional/unknown/disallowed filters; real B/C/D revoke/disable/enable/replace/app revoke/subset regrant visibility using original JWT | PASS |
| EPR-03 | Safe fields/status/version and ID/filter validation; missing/deleted/all nonactive targets; retained active sources; target credentials stay rejected after reactivate, without target principal | PASS |
| EPR-04 | Strict request ctx and begin/actor/target/contribution/commit ordering/handle; rollback/cancel/deadline/panic/commit suppression; real two-backend snapshots observe consistent prior state then current data/actor denial; pool reusable | PASS |
| EPR-05 | Exact 35/prior 33 inventory, shared invalid-input quota/RetryInfo, business-only self service, namespace isolation, localized/redacted failures and independent failed-audit error preservation; no read/retry business or success-audit/outbox/replay effects | PASS |
| EPR-06 | Targeted/full workspace and standalone test/vet/race/build plus all four full DB gates PASS on final runtime source; final formatting/YAML/link/scope/OpenSpec checks tracked in checklist | PASS |
| EPR-EXT-01 | Actual additive 13-file contract, frozen 33-method descriptors, new field/signature/wire/docs tests, generation/static checks, authorized publication and standalone immutable resolution | PASS |

AC-09/10/23 cover visibility after existing role mutations; AC-13 covers application-gate visibility only. EvaluateAccess and ListAuditLogs remain Phase 4; REST/UI/CLM remain separate. Actual schema/admin/key/TLS/caller/consumer deployment readiness remains mandatory Phase 6 DEP-01 and is not claimed PASS by backend fixtures.

## Corrected attempts and scope

Earlier cache-permission attempts passed after switching to a writable cache. Temporary contract tests caught old exact inventory counts; assertions were updated to 15 UserService/35 total before publication while prior descriptors stayed frozen. A narrowed temporary workspace needed full parent membership for offline resolution. Initial sandbox PostgreSQL connection failure was resolved by the authorized dedicated-test run outside that network restriction. These are corrected attempts, not remaining failing gates.

At apply completion, user hunks were preserved. No manual generated-code edit, secret/DSN output, unrelated refactor, new branch, grpc-auth commit/push, main-spec sync, archive or deployment had occurred. Contract application/publication was explicitly authorized. No backend E blocker remained; subsequent sync/archive is recorded below.

Final review: strict OpenSpec, diff whitespace, gofmt and YAML shape/exact35 allowlist PASS;53 local links/anchors valid. Runtime/test source hashes remained identical to the verified deliverable;20 changed baseline paths stayed in E scope and unrelated baseline files/contract user data were preserved. See [scope review](scope-review.json).

## Archive status — 14 October 2026

User selected sync then archive. All artifacts and 24/24 tasks were complete. The [main spec](../../../../specs/effective-permission-reads/spec.md) now contains all 8 requirements and scenarios from this delta, with Purpose and traceability preserved. The change was moved to `openspec/changes/archive/2026-10-14-auth-effective-permission-reads`, including `.openspec.yaml` and all existing evidence. Documentation links were updated for the archived location. Deployment readiness remains Phase 6 DEP-01.

Archive verification PASS: `openspec validate --specs --strict` passed 21/21 specs; the active change list is empty; all 8 requirement/scenario blocks match the delta; 63 local links/anchors resolve; all 12 original change files, including immutable evidence and `.openspec.yaml`, were preserved. Scope comparison found only the new main spec and expected Markdown navigation/status edits; every verified runtime/test source hash remains unchanged. `git diff --check` passed. No new runtime behavior was introduced by archive.

# Application access implementation evidence

Status: **COMPLETE,33/33 tasks verified**. All12 final gates PASS on the same source identity; ledger reconciliation complete. DEP-01 stays open separately. No application migration, application DB write, REST/UI change, Docker run or grpc-auth commit/push was performed.

## Baseline and publication

[Baseline manifest](baseline.json) captures both working trees including untracked C/refactor files. Baseline workspace/standalone default tests each passed2,690 test events. [Scoped review](scoped-review.json) confirms no baseline file deletion, unchanged archives/main specs, unchanged user api-contracts/proto/response.json, and only scoped D edits. D changed34 existing grpc-auth files and added24 focused files besides evidence. The earlier checkpoint is retained in [historical record](checkpoint-independent.md); its pending decision/incomplete inventory are superseded.

User approved APPLICATION_ACCESS_SYSTEM_PROTECTED specifically for D. The new key application.application_access_system_protected/AASP maps FAILED_PRECONDITION/HTTP409 with EN “System application access cannot be modified.” and ID “Akses aplikasi sistem tidak dapat diubah.” Existing SAP key/code/messages are preserved. [Contract tests](contract-error-final.json):125 PASS/no skips; [vet](contract-vet-final.json):PASS. Official make proto regeneration and committed-tree review passed. Immutable descriptor fixtures preserve all30 old methods/tags/types with exactly3 additions. Generated files were not manually edited.

[Publication](publication.json): commit21082d7a243da2b400579db7d1fd40ebdcbeb37f pushed on authorized feat/role-management. grpc-auth pins v0.0.0-20261013075041-21082d7a243d; standalone download and checksums are recorded. No unpublished module fallback or module-cache patch.

## Acceptance ledger

All AAP rows below are PASS for their backend scope; expected outcomes are actual scenario assertions. [Final gate ledger](final-gates.json) records all12 accepted runs.

| ID | Expected outcome / verified coverage | Source scenarios |
| --- | --- | --- |
| AAP-01 | Current exact AND permission before target/replay, stale/restricted denial, one contribution query/evaluation | [plural authority](../../../../../tests/features/auth_feature/test_scenarios/permission.go), [feature matrix](../../../../../tests/features/application/test_scenarios/access.go), [registered SQL](../../../../../tests/integration/auth_reads/test_scenarios/application.go), [PG privilege races](../../../../../tests/integration/postgres/test_scenarios/application_race.go) |
| AAP-02 | Stored ACTIVE/REVOKED, disabled/system apps, account states, missing/deleted/empty/null sources, one authority/version/count/page snapshot | feature matrix, registered SQL, [PG list/snapshot](../../../../../tests/integration/postgres/test_scenarios/application_list.go) |
| AAP-03 | All11 scalar directions, nulls/ties/hidden projection/literal search/filter/unique total/numeric/typed/offset/terminal cursors, old B/C encoding | [domain](../../../../../tests/repositories/domain/application/test_scenarios/access.go), [strict repository SQL](../../../../../tests/repositories/application/test_scenarios/persistence.go), PG list/snapshot and baseline catalog fixtures |
| AAP-04 | Additive grant, selected reactivation/pair identity/revoke clearing, active and unselected provenance unchanged, access-only, raw bounds/presence | domain, feature matrix, [handlers](../../../../../tests/handler/test_scenarios/application.go), [PG management](../../../../../tests/integration/postgres/test_scenarios/application_management.go) |
| AAP-05 | Complete revoke including29 roles, absent-access cleanup without insert, revoked provenance retained, disabled/inactive cleanup, other apps unchanged | feature matrix, repository SQL, PG management |
| AAP-06 | IAM/is_system denial on grant/revoke, wrong-app rejection, no platform-admin loss | feature matrix, PG management, full baseline C/status last-admin regressions |
| AAP-07 | One version/compound; fresh stale no-op conflict; optional status/access-only result; original replay before fresh version,24h expiry/conflict, independent user/app/method scopes | domain, feature matrix, handlers, registered SQL, PG management/terminal |
| AAP-08 | One compound audit/event/result, no role events, every write fault/conditional zero access-role-version/deferred commit rollback, independent failure audit | repository SQL, feature matrix, registered SQL, PG fault matrix/terminal |
| AAP-09 | Two physical backends for D-D/C/status/role both arrival orders; canonical predecessor rollback/restart, bounded instability, panic/cancel/uncertain commit and lock/pool reuse | feature matrix, [shared discovery](../../../../../tests/features/common/test_scenarios/discovery.go), PG race/terminal and baseline refresh committed-denial |
| AAP-10 | Original JWT primary contribution changes after revoke/subset regrant, other-app and credential state unchanged, concurrent RR visibility | PG management/list/snapshot, baseline C/role evaluation |
| AAP-11 | Exact33/seven services, old30 preserved, TLS/service allowlist/IAM namespace, shared read120/20 and mutation30/5 incl invalid D input, RetryInfo/cap/cleanup, startup provider/entropy | [registered inventory](../../../../../tests/integration/auth_reads/test_scenarios/lifecycle_registered.go), registered SQL, [mixed admission](../../../../../tests/interceptors/test_scenarios/role_rate.go), PG scopes/quota and configs/lifecycle regressions |
| AAP-12 | All workspace/standalone core and serial PG gates PASS on same identity; gofmt/scoped diff/strict validation/local links PASS | gate files below, scoped-review.json |
| AAP-EXT-01 | Additive contract/presence/tags/EN-ID/errors/generator/published standalone dependency | publication.json, contract-error-final.json, contract-vet-final.json |

Mappings retain design AC-08/11/12/13/19/20/24–26/31 subset and M-01..11. No completion claim includes E/effective-read RPCs, EvaluateAccess, REST/UI, Phase4 or deployment readiness.

## Final gates

Source identity: 8746bf2c3914799fb83934466574feb0bb83bd270888ea320db35585d910dc5f. Hashes cover Go/SQL/JSON/YAML under internal/pkg/cmd/tests and go.mod/go.sum/Makefile; docs/evidence edits do not change this identity. Each JSON records actual command/mode/start UTC/duration/exit/identity and test events. Counts include parent/subtests; packages without tests are not skipped scenarios. Build outputs remain under /tmp.

| Gate | Workspace | GOWORK=off |
| --- | --- | --- |
| Full go test | [3,096 PASS](final-workspace-test.json) | [3,096 PASS](final-standalone-test.json) |
| go vet | [PASS](final-workspace-vet.json) | [PASS](final-standalone-vet.json) |
| Full go test -race | [3,096 PASS](final-workspace-race.json) | [3,096 PASS](final-standalone-race.json) |
| make build BIN_DIR=/tmp/... | [PASS](final-workspace-build.json) | [PASS](final-standalone-build.json) |
| Full PostgreSQL -tags postgres -count=1 | [513 PASS](final-pg-workspace.json) | [513 PASS](final-pg-standalone.json) |
| Full PostgreSQL -race -tags postgres -count=1 | [513 PASS](final-pg-workspace-race.json) | [513 PASS](final-pg-standalone-race.json) |

PostgreSQL uses only private env-test.yaml via the existing private wrapper; credentials go directly to its subprocess and are not printed. Owned schemas have PID/counter identities and cleanup. Access fixture revocation check/index follows agreed DDL, and affected synthetic B/C statements now provide valid revoke metadata without weakening assertions. Archived B live metadata excludes access; no live access-schema parity is claimed.

Four initially parallel final PG runs collided on database-wide advisory lock keys despite separate schemas. Their RESOURCE_BUSY/downstream timeout failures are retained in parallel-final-pg-*.json and separate /tmp logs, not accepted as gates. Final suites run serially without changing business locks/retry policy/assertions/source identity. Earlier incomplete inventory30/33 and strict SQL expectation failures were corrected and their successful reruns are recorded; no waiver was accepted.

Readability review: six focused application feature files keep composition/list/transaction/grant/revoke/evidence visible. Existing discovery/catalog/assignment writers and current permission resolver are reused. The local D command descriptor shares only its own transaction/replay stages; no generic framework or nested C mutation call. Direct defer owner.Finish remains at each owner. Conditional-write and optional-status helpers carry error/presence semantics. No unused dependency, credential result, role-event duplication or source scope expansion found. Gofmt/git diff --check/strict OpenSpec/local links PASS. See [API documentation](../../../../../docs/application-access-api.md).

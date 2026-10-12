# Implementation evidence — contract B checkpoint

Date: 12 October 2026 (Asia/Jakarta). Scope completed: tasks1.1–1.6 and2.1 (7/35).
Backend role/catalog implementation is NOT_STARTED; RM-01..10 and RM-EXT-01
are not PASS. Contract publication and consumer dependency resolution PASS;
required PostgreSQL/backend behavior gates remain open.

## Environment and source

- api-contracts baseline commit `7297589746962a3d994cf850a623502562017db0`, existing branch `feat/user-err`, initially clean.
- Go1.26.2 darwin/arm64; protoc33.4; protoc-gen-go1.36.11; protoc-gen-go-grpc1.6.1.
- Parent workspace uses local api-contracts/error-package. Standalone contract verification uses published dependencies from api-contracts/go.mod with GOWORK=off.
- Go default cache write failed under sandbox on first targeted command. Recovered using `/tmp/auth-role-build-cache`; no dependency/code workaround or module-cache edits.
- Created and switched to `feat/role-management` from `feat/user-err` on the user's explicit request. Contract changes committed/pushed on explicit user authorization; grpc-auth dependency now pins published B, verification PASS. No database or deployment changes performed. rest-orc existing working tree preserved.

## Actual checks

All contract commands ran in `/Users/bleki/Desktop/mini-projects/api-contracts` through RTK. Source Go files formatted with gofmt. Timings below are reported process/package timings; async workspace full-process wall time was not captured.

| Command | Timing | Actual result |
| --- | --- | --- |
| `make proto` first run | 0.571s tool process wall | PASS |
| `GOCACHE=/tmp/auth-role-build-cache go test ./tests/protobuf/... ./tests/grpcerror/...` | final protobuf package2.237s; grpcerror cached (prior1.564s) | PASS |
| `GOCACHE=/tmp/auth-role-build-cache go test ./...` | dictionary0.496s, locale0.876s, protobuf/grpcerror cached | PASS |
| `GOCACHE=/tmp/auth-role-build-cache go vet ./...` | async; no diagnostic output | PASS, exit0 |
| `make proto` second run | 0.637s tool process wall | PASS, all22 generated hashes identical |
| `GOWORK=off GOCACHE=/tmp/auth-role-build-cache go test ./...` | 5.962s wall | PASS |
| `GOWORK=off GOCACHE=/tmp/auth-role-build-cache go vet ./...` | 0.556s wall | PASS |
| `git diff --check` | read-only check | PASS |
| Changed file whitespace/generated provenance/local documentation links | 29 files checked | PASS |
| `openspec validate auth-role-management --strict` in grpc-auth | read-only check | PASS |

Compatibility scenarios compare every entire baseline FileDescriptorProto,
including old messages/enums/services/options, and confirm19 old unary methods.
The eight new exact unary signatures and every new field tag/type/cardinality/
optional presence are frozen. Binary/JSON tests cover nil/zero/empty mutation
presence, absent versus explicit empty PermissionSet, nullable description/time,
count/version and pagination. Documentation method/scalar tables match descriptors.
New catalogs round-trip wrapped errors for en/id/id-ID/English fallback with
qualified identity, HTTP/application/gRPC codes, safe redaction and preserved
source identity/cause. Existing common/user mappings remain unchanged.

Contract tests verify transport shape and semantic documentation, not backend
SQL/authorization/transaction/concurrency implementation. No race suite required
for this contract-only change; no concurrent or shared mutable production state
changed. Backend race and real PostgreSQL gates remain pending.

## Source manifest

[contract-source-manifest.sha256](contract-source-manifest.sha256) records all29
changed api-contracts files (three modified existing files,26 new files).
Manifest SHA-256: `956e040921f8e7a8a59769a5bbf661ab112505acfbd5a16209fc5f37de81c30c`.
Generated file hashes are in [contract inventory](contract-inventory.md#verified-generated-hashes).
Old proto/generated files, module dependencies and legacy mappings have no diff.

## Publication — task1.6 PASS

User explicitly authorized commit/push on `feat/role-management`.

- Commit: `fd1147049d52b6115f9b59e802585f11be301b76` (`feat: add role management and catalog contracts`).
- Remote branch: `origin/feat/role-management`; push confirmed `7297589..fd11470`, clean contract worktree.
- `GOWORK=off go mod download -json github.com/siti-nabila/api-contracts@fd1147049d52b6115f9b59e802585f11be301b76` PASS.
- Published version: `v0.0.0-20261012030550-fd1147049d52`.
- Origin VCS: git; URL: `https://github.com/siti-nabila/api-contracts`; hash matches the commit above.
- Module sum: `h1:yO+em7TgK63ubgLTxKlseFGVp839cSPKiwaUzYt+w6s=`.
- GoMod sum: `h1:Jb5bNxnX/aW6YWghT/pXorXlmIDRRxXltY8gL7Udzg4=`.

Staged diff checking caught a final extra blank line in each new YAML catalog.
Both were removed before commit; targeted grpcerror/dictionary tests passed
(grpcerror1.384s, dictionary cached), and staged `git diff --cached --check` passed.
The source manifest was refreshed for these whitespace-only changes. Generated
hashes are unchanged. The first push created the remote branch at the baseline
while the whitespace fix was being completed; the subsequent push published the
verified contract commit. No force-push/tag/release/PR/merge was performed.

## Backend prerequisite — task2.2 pending

No `GRPC_AUTH_POSTGRES_TEST_ENABLED`, `GRPC_AUTH_POSTGRES_TEST_DSN` or
`GRPC_AUTH_POSTGRES_TEST_DATABASE` is configured in this execution environment.
The dedicated-test configuration file path has been requested from the user.
No application env.yaml/DSN is used as a substitute and no DB connection or
migration was attempted. SQL reference and fixture were inspected, but source
inspection is not current live metadata evidence. Task2.2 remains incomplete;
backend role/catalog implementation and PostgreSQL gates are not claimed PASS.

## Backend dependency — task2.1 PASS

Only go.mod contract pin and the two new go.sum entries changed. No unrelated
dependency update. `GOWORK=off go list -m -json github.com/siti-nabila/api-contracts`
confirms the published version and both checksums above, with no local replace.
Targeted configs/handler tests passed (1.503s/1.932s). Full checks ran in grpc-auth
with GOCACHE=/tmp/auth-role-build-cache; standalone sets GOWORK=off. Build output
uses /tmp/auth-role-build, avoiding a repository binary.

| Mode | Command | Wall duration | Result |
| --- | --- | --- | --- |
| workspace | `go test ./...` | 10.797s | PASS |
| workspace | `go vet ./...` | 1.035s | PASS |
| workspace | `make build BIN_DIR=/tmp/auth-role-build` | 2.156s | PASS |
| standalone | `go test ./...` | 12.266s | PASS |
| standalone | `go vet ./...` | 0.695s | PASS |
| standalone | `make build BIN_DIR=/tmp/auth-role-build` | 1.586s | PASS |

No backend role/catalog implementation or27-method registration is implied by
these existing-service regression/build checks. grpc-auth Go module edits remain
uncommitted; Git publication authorization covered api-contracts only.

## Schema preflight — task2.2 PASS

User supplied root env-test.yaml (Database.GRPC_AUTH_TEST). Guarded test DB
identity/connectivity verified, baseline test public tables empty as designed.
Automatic review initially rejected application metadata access; user then
explicitly authorized baseline READ ONLY via env.yaml. Retried only after that
authorization, reading no business rows and executing no application write/DDL.

[schema-comparison.json](schema-comparison.json) captures PostgreSQL16.13 metadata
for31 columns/26 constraints/one numeric role sequence, compared with SQL reference
and an owned disposable test fixture. role_name50/role_description100 nullable/
string code100, INTEGER IDs/numeric code, version BIGINT>=1, ACTIVE/DISABLED,
nullable timestamps, application/permission FKs and unique string/numeric codes
match. role_numeric_code_seq is INTEGER min1/max2147483647/increment1 and is the
role_code default. Permission code150 and registered two-or-more-dot grammar
match. Mapping PK is role_id+permission_id with both FKs.

Existing fixture generates role.id/user_role.id via identity BY DEFAULT while
baseline uses equivalent positive INTEGER sequence defaults; this is an existing
fixture mechanism, not an application schema change. No incompatible baseline
write mismatch found. Fixture permissions_code_ck was missing and is now copied
from the verified baseline/reference. No migration was run on the application.

`go test -tags=postgres ./tests/integration/postgres -run
'^TestPostgres(RoleSchema|FoundationHarness)$' -count=1` via a private YAML-to-env
wrapper PASS1.443s. All fixture lifecycle/cleanup stays in its owned schema.
Source/config secrets are never included in logs/reports.

## Domain inputs and safe role replay — tasks 2.3–2.4

PASS: `GOCACHE=/tmp/auth-role-build-cache go test ./tests/repositories/domain/role/... ./tests/features/common/... ./tests/features/user/... ./tests/repositories/transaction/...` (role 1.380s; common 1.624s; user 2.158s; transaction 2.745s). Covers canonical bounds and namespaces, mask/status/set presence, protected IAM/system resources, exact nullable saved response and affected count, canonical sets, unsafe/unknown result rejection, and existing user/session replay regression. Changed Go files formatted.

## Persistence and catalog query — tasks 2.5, 3.1–3.3

PASS: `GOCACHE=/tmp/auth-role-build-cache go test ./tests/repositories/role/... ./tests/repositories/domain/role/... ./tests/repositories/transaction/...` (1.068s / 1.506s / 1.753s). Strict one-handle SQL scenarios verify safe role/app/permission fields, not-found versus dependency errors, create omitting numeric role_code, conditional version/affected rows, mapping replacement and failure causes, filter/search binding, C-collated mixed/null cursor predicates and projection retaining hidden cursor values. Domain descriptor checks cover exactly 13/5/5 public fields, both directions, projection validation, typed/query/method cursor rejection and canonical sets.

PASS dedicated PostgreSQL: private wrapper loads ignored env-test.yaml, validates dedicated database, then invokes `go test -tags=postgres ./tests/integration/postgres -run ^TestPostgresRolePersistence$ -count=1`; latest run 1.164s. Owned fixture schemas cleaned. Real DB scenarios verify numeric sequence allocation/gaps, code uniqueness, expected-version zero-row result, nullable timestamps/description, mapping clear, FK failure after tentative writes and rollback. All 23 public scalar fields sort ASC/DESC with limit2 keyset continuation; two mixed-direction nullable sorts compare against independently ordered in-memory rows. Projection always returns ID and hides unselected metadata without losing cursor. Literal percent/underscore/backslash search, status OR and category/search AND, nonexistent app, deterministic offset and distinct ACTIVE assignment count across all account states pass. No registered authorization/snapshot-concurrency claim yet (tasks 3.4–3.6 remain).

The ORM has no DELETE builder. A narrow optional StatementWriter capability on the existing leased primary transaction executes parameterized mapping DELETE/INSERT on its native handle; repository owns no begin/commit/rollback/locks and cannot fall back to a pool. No ORM sibling change or dependency update.

## Feature read snapshots — task 3.4

PASS `GOCACHE=/tmp/auth-role-build-cache go test ./tests/features/catalog/... ./tests/features/role/...` (catalog1.168s, role1.616s). Strict injected repository scenarios verify BeginSnapshot→bind same handle→current session→exact current permission→target→Commit, denial before target, stale/restricted sessions, distinct exact catalog versus role permissions, dependency causes, cancellation, query errors, missing role, collection failure, panic and failed commit cleanup. Every failure returns zero/empty result. Reads use feature-owned TransactionOwner and existing primary repeatable-read provider; no lifecycle in repository.

## Catalog handlers — task 3.5

PASS `GOCACHE=/tmp/auth-role-build-cache go test ./tests/handler/...` (1.156s). New strict scenarios for all four reads cover nil/missing authentication, invalid query/ID, normalized defaults/sort/search/filter/projection, original request context and locale metadata, safe role count/numeric code, nullable versus explicit empty descriptions/timestamps, no credentials, and forbidden/dependency errors returning no partial response. Added docs/role-management-api.md catalog contract; production registration is still pending, explicitly documented.

## Dedicated catalog snapshots — task 3.6

PASS private dedicated-test wrapper: `go test -tags=postgres ./tests/integration/postgres -run ^TestPostgres(CatalogSnapshots|RolePersistence)$ -count=1` (1.780s). Four feature read scenarios use real Login-produced claims and primary state. RR connection A establishes a snapshot at a channel barrier; distinct backend B removes actor permission mappings while changing role/application/permission metadata and inserting matching roots, then commits. A returns entirely prior authority/total/items/detail/mappings; a fresh read is denied. Explicit backend PID inequality, channel coordination, worker completion, owned schema cleanup and pool InUse0 assert actual overlap and resource reuse. All scalar/mixed/null cursor/projection/filter/count scenarios from RolePersistence run in the same gate. No production registered RPC claim (Phase5 tasks pending).

## Role command unit protocol — tasks 4.1–4.5

Implemented four command methods with canonical global policy/role-code/actor/contributing-role/target-role/app/session locks, bounded stable discovery and fresh-handle predecessor restart. Current state/exact permission recheck precedes replay; replay precedes expected role version/new no-op guard. Create uses DB sequence/ACTIVE/version1/nonsystem and registered same-app set; update preserves presence and clears selected empty description to NULL; status and permission replacement preserve user/session/assignment/access rows. Each effective command increments role version once and requires one role audit/event/safe keyed result in the same transaction. New no-op audits once, has no version/event, including unkeyed update. Role-specific busy/conflict semantics preserve prior user errors.

PASS targeted feature matrix `GOCACHE=/tmp/auth-role-build-cache go test ./tests/features/role/...` (initial1.248s; follow-up includes lost permission, protected/stale no-op guards). Matrix injects error/panic/cancellation at discovery/auth/session/replay/role/mapping/conditional version/audit/event-ID/outbox/save stages; tests assert original causes, no partial result, rollback, uncertain commit without retry, one supplied handle, canonical lock order and predecessor restart with distinct transaction mocks. Replay preserves original safe nullable metadata/count and adds no effects. Cases include expired/conflicting keys, disabled app management, required guards, empty/unknown/cross-app sets, metadata clear, zero-row version conflict and overflow. PostgreSQL atomic/concurrency verification and independent failure audit still pending 4.6/4.8/5.x.

## Mutation handlers, failure cleanup and registered PostgreSQL outcomes

- Tasks4.6–4.8,5.1,5.4: workspace targeted role/interceptor/handler/registered-read tests PASS (1.665s/1.197s/1.106s/1.798s). Full27-method/seven-service inventory, all27 anonymous denials and new-method exact caller allowlist denials execute the production TLS pipeline without persistence.
- Role strict failure matrix now invokes the outer security-attempt interceptor: every injected error/panic/cancellation/commit uncertainty ends rollback before exactly one independent bounded audit. A failing failure-audit preserves the original error and increments telemetry once. No successful/no-op/replayed command records a failure audit.
- Dedicated opt-in PostgreSQL: `go run -tags=postgres /tmp/auth-role-postgres.go test test -tags=postgres ./tests/integration/postgres -run '^TestPostgresRole(Management|Evaluation)$' -count=1` PASS3.667s. Guarded credentials originate only from ignored env-test.yaml; no DSN/secret output or app writes.
- Actual registered four-command flow proves sequence allocation, uniqueness, conditional version, canonical/empty final sets, safe historical response replay/lost-response retry, target scope separation and expiry replacement. Exact9 new audits (including2 NO_OP),7 events and7 scoped results; failed calls do not count. Numeric sequence gaps are excluded from rollback equivalence.
- Real trigger injection for role/mapping DELETE+INSERT/audit/outbox/result and deferred commit, plus conditional zero-row, proves complete business/credential rollback and exactly one separate failure audit per RPC with released pool slots. An initial assertion incorrectly used uppercase ROLE instead of the typed contract value role; corrected test passes without production changes.
- Registered disable/enable/replace/empty mutations and Me using the original JWT are followed by primary RR permission evaluation. Overlapping contributions remain correct; disabled app excludes contributions while edits stay allowed; revoked assignments never contribute. All account/profile/user-version/auth_version/session/refresh/assignment/access rows remain byte-equivalent throughout.

## Production policy, shared quota and two-backend mutation barriers

- Tasks5.2/5.3/5.5/5.6: registered TLS namespace denial for every new method with safe independent audit; malformed/duplicate service credentials are unauthenticated without persistence. Registered missing exact permissions never disclose nonexistent targets. Restricted user and localized role catalog errors verified. Unit policy matrix adds all8 methods across success/restricted/missing/duplicate/expired/revoked/dependency/namespace.
- `go test ./tests/integration/auth_reads/... ./tests/configs/...` PASS1.153s/cached. Additional constructor omissions fail before transaction creation. `go test ./tests/features/role/... ./tests/interceptors/... ./tests/ratelimit/...` PASS1.528s/1.074s/1.971s. Concurrent80 mixed user/session/role/catalog requests admit exactly shared read20 or mutation5 burst with one actor entry; existing store capacity/idle cleanup/refill tests remain PASS. Registered invalid input fills shared mutation bucket; subsequent denial has RetryInfo2s and no business writes. Read20 shared across user/role/permission/application then session is denied.
- Dedicated PG `... test test -tags=postgres ./tests/integration/postgres -run '^TestPostgresRoleConcurrency$' -count=1` PASS2.373s. Each of4 commands holds the outbox barrier with physical backend A while backend B receives busy for same/different key. After release, same scope replays; new keys return uniqueness or stale-version conflict. Admin remains active and credential snapshots identical; all pool slots released.
- Pre-lock actor generation/session/permission invalidation on the distinct backend commits before saved replay; recheck denies without effects. Synthetic discovery switches contributing role before stability, forcing rollback then distinct tx restart with canonical locks.
- Uncertain-commit provider performs one actual commit then reports response loss: no internal retry (one Begin/Commit), saved result persists, next keyed call recovers it. Subsequent mutable metadata changes never alter replay. Every command also performs real tentative writes then injected panic/cancel; exact business rollback and successful subsequent lock/pool reuse confirmed.
- Initial concurrency assertions compared time.Location identity rather than serialized response and expected3 instead of actual2 restart transactions. Both corrected to assert contract JSON and the observed canonical predecessor restart; production unchanged. A direct tagged invocation without dedicated opt-in correctly failed closed; it is not counted as a PostgreSQL PASS.
- Documentation now records27 methods, all mutation/query/key/version/no-op/replay semantics, C–E and Phase6 ownership. Full verification remains pending explicitly; DEP-01 remains outside backend B completion.

## Final full workspace and standalone gates (tasks6.1–6.2)

All commands run in grpc-auth with `GOCACHE=/tmp/auth-role-build-cache`; default suite has no real DB/network. Standalone adds `GOWORK=off`. `rtk proxy env ...` forwards full command behavior. Build uses `make build BIN_DIR=/tmp/auth-role-build-{workspace,standalone}` to keep the compiled binary outside the repository. Raw command logs remain in private /tmp files, not committed artifacts.

| Environment | Command | Actual result / seconds |
| --- | --- | --- |
| workspace | go test -count=1 ./... | PASS11.027 |
| workspace | go vet ./... | PASS0.665 |
| workspace | go test -race -count=1 ./... | PASS25.627 |
| workspace | make build BIN_DIR=/tmp/auth-role-build-workspace | PASS2.057 |
| GOWORK=off | go test -count=1 ./... | PASS12.737 |
| GOWORK=off | go vet ./... | PASS0.866 |
| GOWORK=off | go test -race -count=1 ./... | PASS16.713 |
| GOWORK=off | make build BIN_DIR=/tmp/auth-role-build-standalone | PASS2.289 |

Final gofmt applied to all73 changed Go files. Only import grouping changed after workspace gates; standalone full gates cover that formatting. No subsequent production/default-test behavior changes after these gates. Final PostgreSQL assertions additionally inspect a disabled system role in a disabled application and authorized NOT_FOUND through the registered server; both final standalone PG full runs include them, with focused workspace normal/race verification recorded below. Standalone `go list -m -json github.com/siti-nabila/api-contracts` confirms published v0.0.0-20261012030550-fd1147049d52 at2026-10-12T03:05:50Z, module Sum h1:yO+em7TgK63ubgLTxKlseFGVp839cSPKiwaUzYt+w6s= and GoModSum h1:Jb5bNxnX/aW6YWghT/pXorXlmIDRRxXltY8gL7Udzg4=; no workspace replace in standalone. Exact27 inventory executes in both full suites. api-contracts branch feat/role-management remains clean and tracks origin without ahead/behind changes.

Preliminary strict validation PASS and43 local links/anchors in9 changed markdown files PASS. Final evidence reconciliation and full dedicated PostgreSQL race/standalone gates remain pending; no skipped test is counted PASS.

## Full dedicated PostgreSQL regression (task6.3)

Go1.26.2 darwin/arm64; PostgreSQL16.13; primary test pool max2. App baseline access was explicitly approved metadata-only READ ONLY; no app row reads/writes or migrations occur. All write scenarios use only dedicated env-test.yaml credentials and owned grpc_auth_test_ schemas with guarded creation/drop. Processes run sequentially because advisory locks are database-wide. No Docker, RabbitMQ or app DB writes.

Shared wrapper: `go run -tags=postgres /tmp/auth-role-postgres.go test test [race] -tags=postgres ./tests/integration/postgres -count=1`, with `GOCACHE=/tmp/auth-role-build-cache`; standalone adds `GOWORK=off`. Wrapper exports only guarded dedicated-test opt-in/DSN/database to the child, never prints credentials.

| Environment | Full dedicated suite | Actual result / package seconds |
| --- | --- | --- |
| workspace | normal | PASS33.473 |
| workspace | -race | PASS91.195 |
| GOWORK=off | normal | PASS22.560 |
| GOWORK=off | -race | PASS91.138 |

All existing foundation/Login/session/refresh/password/logout/last-admin/user-management tests remain in the full suite. New schema/persistence/query/catalog-snapshot/mutation/evaluation/concurrency tests execute, including real write failures, physical backend barrier identity and pool/lock reuse checks. No opt-in guard failure or skipped test counts as PASS.

## Acceptance reconciliation (task6.4)

[Scenario traceability](scenario-traceability.md) maps every one of39 delta scenarios across10 requirements to executable tests. The normative RM/AC/M meanings are unchanged.

| Ledger ID | Actual implemented and verified evidence | Status |
| --- | --- | --- |
| RM-01 | Exact service method/IAM namespace/session/restricted/permission guards; system/IAM nominal no-op protection; real actor invalidation before saved replay | PASS |
| RM-02 | Schema bounds/metadata comparison, business-app and registered same-app set, DB sequence/uniqueness, empty creation and concurrent same code | PASS |
| RM-03 | Presence/mask/NULL clear/status/final set, conditional version, stale no-op, unkeyed and keyed NO_OP audits/counts | PASS |
| RM-04 | Registered writes with original JWT Me and current RR resolver: disable/enable/replace/clear, overlap, disabled app and revoked assignment; byte-equivalent account/version/auth_version/session/refresh/assignment/access | PASS |
| RM-05 | Required role/mapping/version/audit/outbox/result atomicity; all failure stages, cancellation/panic after writes; separate audit after cleanup plus failure telemetry; exact response replay, scoped keys/expiry and actual commit response loss | PASS |
| RM-06 | Published additive contract baseline19 compatibility, production27 inventory, localized catalog mapping, all8 policies and shared user/session/role/catalog admission; default/PG old lifecycle regressions | PASS |
| RM-07 | One primary RR authority+data+total+detail handle with auth-before-target; four actual two-backend snapshot barriers and fresh denial; safe errors/no partial response | PASS |
| RM-08 | COUNT DISTINCT ACTIVE assignments for all account states including deleted, revoked exclusion, no join/status/access inflation, unused0 and readable disabled system role in disabled app | PASS |
| RM-09 | All13+5+5 scalar fields ASC/DESC, mixed keysets/null-last/C collation/ID tie, typed method/query-bound cursor, numeric compatibility/projection/terminal semantics in domain/SQL/PG tests | PASS |
| RM-10 | OR statuses / AND category+search, literal wildcard escaping, unique pre-cursor total, empty/default/page/cursor, shared protected-read quota/capacity/cleanup/RetryInfo | PASS |
| RM-EXT-01 | Published fd1147049d52, exact pseudo-version/checksums and clean pushed branch;29-file contract manifest, stable generation/compatibility/locale/full/vet and standalone module resolution | PASS |
| DEP-01 | Actual schema/admin/key/caller/consumer/deployment/cutover evidence remains Phase6 ownership, outside backend B | OPEN, outside B |

Base grpc-auth HEAD bf6be877296b8f4191f2e2db5f744b23ab636ed7. Deliverable remains an uncommitted working tree identified by source-manifest.sha256; only api-contracts was committed/pushed under explicit authorization. Manifest covers402 production/test/docs/build/module files, excluding env/credentials/binaries and planning evidence to avoid a recursive hash. Contract manifest verifies all29 captured files against the published checkout. Changed-path secret/env/binary scope and root-tests layout checks pass. New reader/writer lifecycle/lock isolation checks pass; transactions and locks remain feature-owned. Existing unrelated rest-orc working-tree changes remain untouched.

## Final closure

- Final additional registered PG assertions (disabled system role in disabled app with retained collection/count; authorized missing-role NOT_FOUND) PASS in both full standalone runs. Focused final workspace normal/race runs also PASS5.783s/14.874s wall time. No production/default-test behavior changed after full gates.
- All35 task checkboxes complete. Normative ledger RM-01..10/RM-EXT-01 reconciles to PASS and39 scenario mappings across10 requirements. DEP-01 remains OPEN, outside B. C–E remain NOT_STARTED.
- Strict OpenSpec validation PASS. Final local Markdown links/anchors, git diff --check, source/contract manifest integrity, scope and secret/env/binary/test-layout review PASS. Source manifest digest is recorded below after final documentation updates.
- Source changes include role/catalog domains and tx-bound adapters, feature-owned reads/mutations, four read/four mutation handlers, three generated-server registrations, additive errors/policies/quota integration, shared database native statement support, safe replay/audit/event allowlists, tests/fixtures and API/roadmap documentation. No generated contract file is manually edited; generated publication stays only in api-contracts.
- api-contracts fd1147049d52 is already normally committed/pushed on feat/role-management and clean. grpc-auth remains uncommitted; no PR/tag/release/merge/sync/archive/deploy or app migration is performed. Application DB access remains approved metadata-only READ ONLY.

Final source-manifest SHA256 `0ccf0a4c13a50ef66c40921dd9a7352e01cf3f90605ba552e5c2148757ee0b48` (402 files); source content verified against every entry. Final local link/anchor check PASS182.

## Archive follow-up — 12 October 2026

User selected main-spec synchronization followed by archive. Both new capabilities
were synchronized: iam-catalog-reads (4 requirements, 15 scenarios) and
role-administration (6 requirements, 24 scenarios). Purpose and all normative
requirement/scenario blocks match their deltas. OpenSpec spec validation passed
18/18, and strict change validation passed before the directory move. All 35
tasks and four planning artifacts were complete.

The full change, including .openspec.yaml and both original manifests, moved to
openspec/changes/archive/2026-10-12-auth-role-management. Relative Markdown links
in the archive and the two linked root documents were adjusted for this move.
source-manifest.sha256 remains the historical pre-archive implementation snapshot;
its two root documentation entries precede these link-only edits. Production,
test, module and build files are unchanged by synchronization/archive; no new
runtime or database verification is claimed for this documentation operation.

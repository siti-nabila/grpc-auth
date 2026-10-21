# Apply progress — 15 October 2026 (Asia/Jakarta)

29/29 tasks complete. All required implementation and delivery gates PASS.
The authorized ORM commit is published and pinned; workspace and standalone
verification use the same tested source. The application migration was later
applied under explicit user authorization, as recorded below. Service deployment
remains separate.

## Source and scope

- grpc-auth baseline HEAD: `2462cb09d0f8e7eef3b17a70456451b8bd2dd6ec` plus preserved user working-tree changes.
- ORM baseline HEAD: `90b9cdd8860578725f7402f12fbaa86af83cb4f1`, baseline branch `codex/update-model`; published branch `feat/composable-subqueries-insert-result`.
- [Baseline grpc-auth hashes](grpc-auth-baseline.json), [baseline ORM hashes](orm-baseline.json), and [verified runtime/test hashes](verified-source.json).
- Temporary workfile `/private/tmp/grpc-auth-orm-only/go.work` retains all four parent members and adds the sibling ORM. Parent go.work and ORM dependency files remain unchanged. grpc-auth go.mod/go.sum now pin the published ORM version; unrelated dependency changes are preserved.
- api-contracts remains pinned to `v0.0.0-20261014032725-545701c82e22`; server/contract/security/limiter registration sources are unchanged by this apply. Existing registered-path tests assert exactly 35 methods across seven services.
- [grpc-auth changed baseline paths](grpc-auth-changed-paths.json) and [ORM changed baseline paths](orm-changed-paths.json): no baseline file was removed. New implementation/test/schema/evidence paths supplement these lists.
- [ORM release patch](orm-release.patch) contains exactly 11 scoped files: four documents, four query files, one insert-result implementation, and two test files. Pre-existing Finder/graph artifacts are excluded.

All grpc-auth runtime/test source hashes remained unchanged across final checks.
The ORM test-only public Join shape assertion was changed to a keyed literal to
satisfy vet; ORM full tests/race/vet were rerun afterward. Production ORM source
remained identical. Final hashes include that test correction.

## Behavior

Repositories use GitHub ORM transaction chaining; the legacy pkg DBLogger is
unused. StatementWriter and native statement execution were removed. Features
retain transaction/lock/recheck/version/audit/outbox/idempotency ownership.

ORM children are snapshots, including bound slices; compilation uses parent
configuration/dialect and ordered placeholders, including UPDATE SET offsets.
EXISTS/NOT EXISTS/IN/derived joins, count/page reuse, invalid input, quoting and
PostgreSQL/MySQL/Oracle behavior use existing public DryRun APIs. ExecResult uses
the existing insert pipeline, rejects returning intent, preserves counts/errors
and transaction context, and does not assign generated IDs. Existing APIs and
the public Join struct shape remain compatible.

Role mapping changes are feature-decided sorted disjoint removed/restored/new
sets. Writers use conditional ORM UPDATE or insert with strict affected counts.
Tombstones and unchanged pairs are retained; all permission readers ignore
deleted mappings. No-op/replay/version/credential behavior remains compatible.

The [additive migration](../../../../../docs/sql/role-permission-soft-delete/01_add_deleted_at.sql)
and [activation/rollback guide](../../../../../docs/sql/role-permission-soft-delete/README.md)
are prepared. Initial verification executed the script only in owned test
schemas. It was later applied to the application database under explicit user
authorization; see the application migration record below. No deployment was
performed.

## Verification

Actual commands, UTC timestamps, environment mode, exit codes and test counts
are recorded in [checks.json](checks.json). Counts include named subtests.
Every row below exited zero against the recorded source. PostgreSQL gates were rerun after the original commit publication in both modes.

| Gate | Actual result |
| --- | --- |
| ORM standalone `go test ./...` / `go test -race ./...` | PASS, 205 tests each |
| ORM standalone `go vet ./...` / `go build ./...` | PASS |
| grpc-auth temporary workspace and GOWORK=off `go test ./...` / `go test -race ./...` | PASS, 3265 tests in each run |
| grpc-auth workspace and standalone `go vet ./...`, `make build BIN_DIR=<temporary directory>` | PASS; no repository binary overwritten |
| Full opt-in PostgreSQL workspace normal/race and standalone normal/race | PASS, 536 tests in each of four post-publication runs, PostgreSQL 16.13 |
| gofmt / diff whitespace / strict OpenSpec | PASS |
| Production AST audit | PASS, 78 files, zero manual statements/subqueries/commented SQL/raw execution escapes; [inventory](query-audit.json) |

PostgreSQL configuration is loaded from user-authorized `env-test.yaml` into
explicit test variables without printing credentials. The harness validates the
dedicated database identity and creates/cleans only owned fixture schemas.
Normal and race database suites ran sequentially to avoid cross-process advisory
lock interference. No Docker, application DB, local DBLogger, or application-entrypoint startup
was used for verification.

| Criterion | Current status and evidence |
| --- | --- |
| ORM-01 | PASS local source: AST inventory, conditional writers and composed readers; no StatementWriter/native SQL escape remains |
| ORM-02 | PASS local ORM: all three dialects, named/numbered SET offsets, child/slice immutability, invalid inputs, count/page, driver counts/RowsAffected failures/context/cancellation and existing API regressions |
| ORM-03 | PASS local source/test schemas: migration preservation/compatible repeat/incompatible type-nullability-default rejection, pair PK/schema parity, A/B → B/C → A/B, empty/no-op/replay timestamps and default Member tombstones |
| ORM-04 | PASS local source: original-JWT authority/replay invalidation, overlapping sources, app gates, registered E views including inactive target and consistent snapshots |
| ORM-05 | PASS local source: full ListUsers and catalog sorts/filter/count/null/cursor suites, grouped MIN/COALESCE semantics and two-backend snapshot continuation |
| ORM-06 | PASS local source: soft-delete/restore/new insert failure injection, strict counts, later version/audit/outbox/result/deferred commit rollback, cancellation/panic after all mapping stages, concurrent locks, response-loss replay and pool reuse; full session/last-admin regression |
| ORM-07 | PASS: authorized immutable ORM commit/module/checksums, exact 110-file source/test parity and all 11 release files, standalone and workspace final checks, scope/hashes/OpenSpec/link checks |

Corrected attempts: missing Go module metadata was downloaded; SQL expectations
were updated for bound ORM columns; mapping INSERT failure injection now includes
a genuinely absent pair; old snapshot fixtures restore retained tombstones;
command timestamp assertion accounts for the controlled actor clock; vet's
unkeyed-literal complaint was corrected. None is a remaining runtime failure.

## Published delivery

User authorized a new feat branch, then commit/push. Exactly 11 scoped ORM files
were committed and pushed as `7aeb7624d90fb4a41384f1496ca062aa63ad4806` on
`feat/composable-subqueries-insert-result`. Remote HEAD was verified. That
publication did not create a tag, merge, PR, grpc-auth commit/push or deployment.

grpc-auth now pins the user-published release tag `v1.8.0-support-subquery`,
whose module origin is `489a9e43d79970d2826b1f4322765042b24067b2`.
The remote has no exact `v1.8.0` tag; the existing release tag is used verbatim.
[Module origin/checksums/source comparison](orm-published.json) confirms the
downloaded module matches all 110 verified Go/SQL source/test files and all 11
release files. There is no local replacement or module-cache modification.
[Dependency hashes](delivery-files.json) identify the final go.mod/go.sum.
Only the ORM requirement and two checksum lines changed from the apply baseline.

The release replaces the initial commit-based pseudo-version
`v1.6.10-0.20261015132704-7aeb7624d90f`. All 110 verified Go/SQL files and all
11 release files match the new module byte for byte. Comparison against published
v1.7.0 found no removed production files, only the three planned query-file
modifications and two new implementation files. Standalone and parent-workspace
build lists resolve the release tag without a local replacement.

After switching tags, standalone full tests (3265), vet and build passed again;
the corresponding `grpc-release-tag-*` entries are in checks.json. Earlier
race/PostgreSQL results apply to the byte-identical source and were not rerun
solely for this tag change. Only the ORM requirement and the two current-release
checksums differ from the apply baseline; superseded pseudo-version checksums
were removed.

The earlier standalone compile failure against v1.7.0 is resolved by the published
pin. Tasks 9.2, 9.3 and 9.4 are complete. On 16 October 2026 (Asia/Jakarta),
all four delta specs were synchronized to main specs and the completed change
was archived as `2026-10-16-auth-repository-orm-only`, with all 29 tasks complete.
Key/TLS/caller/consumer and remaining schema readiness checks and deployment
remain Phase 6 DEP-01.

## Application migration

On 2026-10-15 at 14:00:44 UTC, the user-authorized expansion was committed to
database `user`, table `public.role_permissions`, using `env.yaml:Database.user`.
Preflight confirmed the existing composite primary key and absent `deleted_at`.
The reviewed script ran in a transaction with an explicit schema, a five-second
lock timeout and a thirty-second statement timeout. Post-commit verification
confirmed nullable `timestamptz` without a default. All 30 existing mappings
remain active and the primary key is unchanged. Credentials were not printed.
The [migration record](user-database-migration.json) contains the script hash and
results. Service activation, coordinated reader/writer rollout and the remaining
DEP-01 readiness checks are separate.

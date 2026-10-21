# Implementation evidence

Tanggal verifikasi: 11 Oktober 2026. Owner backend grpc-auth. Increment A
diimplementasikan; Phase2 B–E, Phase4/5 dan deployment Phase6 DEP-01 tetap OPEN.
Tidak melakukan migration aplikasi, Docker, bootstrap user, REST/frontend,
broker/relay, commit/push grpc-auth atau sync/archive change ini.

## Source and contract

- Baseline Git HEAD grpc-auth: `869515bdc10ffe6e825ae318dfb2f7bec55cbe11`.
  Working tree berisi lifecycle increment sebelumnya; semua perubahan itu
  dipertahankan. HEAD bukan klaim bahwa management sudah committed.
- [Source manifest](source-manifest.json):326 runtime/test/build input files,
  SHA-256 `d8412ae99c5de50678dcb4801b2d9f2bd9997e93853b483bac65a4c922cc74e2`.
  Manifest mencakup Go source/tests, go.mod/go.sum dan safe env.example.yaml;
  tidak memuat private env/credential, logs atau binary.
- [Contract inventory](contract-inventory.md): api-contracts published pada
  existing branch feat/user-err commit
  `7297589746962a3d994cf850a623502562017db0`, module
  `v0.0.0-20261011113651-729758974696`, checksum
  `h1:MZ1/0dEXvGZIHfWPlGYFOPfDg55lOnxtEYR/HiPr92Q=`.
  Generator, deterministic regeneration, descriptor/wire/error/locale tests,
  full tests/vet, normal push dan standalone download PASS sebelum backend.
- [Schema comparison](schema-comparison.json) membedakan historical preflight,
  fixture dan actual deployment gate. Owned PostgreSQL fixture dilengkapi
  singleton admin_guard; tidak menjalankan SQL ke database aplikasi.
- Go `go1.26.2 darwin/arm64`; dedicated PostgreSQL `16.13`.

## Actual commands and results

Semua Go commands menggunakan `GOCACHE=/tmp/auth-management-go-cache` melalui
RTK proxy. Mode standalone menambahkan `GOWORK=off`, tanpa replace lokal atau
perubahan module cache. Full test count menghitung PASS test/subtest events;
17 packages memiliki tests (package source/mock juga berhasil compile).

| Check | Workspace | Standalone |
| --- | --- | --- |
| go test -json ./... | PASS1786 test/subtest,14.901s | PASS1786,19.337s |
| go vet ./... | PASS0.643s | PASS3.019s |
| go test -race -json ./... | PASS1786,33.728s | PASS1786,29.147s |
| make build | PASS2.086s | PASS1.680s |

Targeted tests `./tests/features/user/... ./tests/handler/...
./tests/integration/auth_reads/... ./tests/configs/...` PASS. Gofmt applied to
changed Go files; final formatting/diff check recorded below.

Dedicated DB uses only private `env-test.yaml` label `Database.GRPC_AUTH_TEST`
to populate explicit `GRPC_AUTH_POSTGRES_TEST_ENABLED=1`, test DSN and exact test
database identity. Private wrapper validates safety before connection, passes
secrets only through process environment and prints no DSN. Harness creates and
drops only owned schema namespaces. Application DB and migration are untouched.
Conceptual commands below omit private DSN values:

```text
go test -tags=postgres ./tests/integration/postgres -count=1
go test -tags=postgres ./tests/integration/postgres -race -count=1
go test -tags=postgres ./tests/integration/postgres -run TestPostgresUserManagement -race -count=1
```

Workspace full PostgreSQL PASS16.035s; full race PASS72.544s. Final additional
management retention/discovery scenarios PASS with race15.472s. Standalone full
PostgreSQL normal PASS16.513s; full race PASS71.447s, executed sequentially.
No broker is running or required; events are asserted pending with
published_at=NULL. No sleep-only concurrency proof: tests use channels/barriers
and separate physical backend IDs.

Initial environment check failed because default Go cache was outside writable
roots; using /tmp fixed it. Initial PostgreSQL run was sandbox-network denied;
the authorized dedicated-test command succeeded with escalation. Test development
corrected an event assertion that counted an earlier ChangePassword outcome,
a duplicated fixture allowlist, and a prepared statement containing two fixture
updates. All affected scenarios were rerun successfully; these were harness
errors, not waived backend requirements. Two simultaneously launched standalone
PostgreSQL processes also contended on database-wide advisory keys despite owned
schemas. They were stopped by failed test outcomes and rerun sequentially; do not
run separate PostgreSQL suites against the same database concurrently.

## Acceptance ledger

All paths below are relative to repository root. Runners import scenario files
from tests/; no test-only behavior or generated protobuf lives in production.

| ID | Status | Actual behavior evidence |
| --- | --- | --- |
| UM-01 | PASS | tests/integration/auth_reads/test_scenarios/lifecycle_registered.go: anonymous19 methods and forbidden service/method/namespace paths including all4 management RPCs; user_detail.go and tests/features/user/test_scenarios/management.go: restricted/stale actor/exact permission; production registered success in postgres/user_management.go |
| UM-02 | PASS | auth_reads/user_detail.go: all4 business statuses, absent profile, missing/deleted, state/permission/detail/collection/commit failures, context/locale/safe serialization; postgres/user_management_race.go postgresManagementSnapshot: RR snapshot authority/profile/access remain consistent across concurrent commit |
| UM-03 | PASS | management_inputs.go + management.go: normalized creation, status/profile/schema/privilege validation, duplicate/hash/required-write failures; postgresUserManagement: one restricted account/default nonadmin/counters1, no session/credential, PENDING activation and restricted password→fresh Login flow |
| UM-04 | PASS | management_inputs.go + tests/handler/test_scenarios/user_management.go: masks/presence/forbidden fields; management.go: stale-before-no-op/zero-row rollback; postgresUserManagement: selected empty/omitted, version only, preserved session/auth_version, committed searchable profile |
| UM-05 | PASS | management_replay.go: original safe fields/collections strict decode, password/presence conflicts, retained/missing key, expiry; management.go: authority before replay before target; postgresManagementRetention + Contention: different-target same key, expired scope24h replacement, response-loss exact replay after mutable target advancement |
| UM-06 | PASS | management.go matrix injects error/panic/cancel at each reachable read/write/event/result stage, verifies shared tx identity and rollback; postgresManagementFailures injects each required account/profile/member/counter/session/refresh/audit/outbox/result write plus zero-row late failure and deferred commit, compares complete business row snapshots, independently persisted failure audit |
| UM-07 | PASS | management.go: begin/lock/commit/dependency/business/cancellation/panic paths and no leaked result; auth_reads/failure_audit.go covers all3 mutation methods plus Logout, rollback before detached bounded audit, audit error/panic preserves original error and telemetry; PostgreSQL asserts pool release; commit uncertainty never reported as certain rollback |
| UM-08 | PASS | postgresManagementQuota: invalid commands across3 methods consume shared admin burst5, RetryInfo2s, no business effects; GetUser burst20 separate read group; full interceptor/limiter capacity/cleanup/regression suites PASS |
| UM-09 | PASS | management_inputs.go full transition matrix; management.go self/last-admin/count-dependency/version/no-op; registered status invalidates active and terminal-old proofs, emits exactly one status event for its version, preserves assignments/generation; Retention verifies revoked role/access remain revoked after reactivation; fresh Login uses current generation |
| UM-10 | PASS | PostgreSQL two-backend Contention, LastAdmin, Recheck, Snapshot, Producers, Discovery: busy then replay/conflict/uniqueness/version, different-admin targets retain one admin, actor invalidation before replay, ordered fresh-tx restart/actor-target dedup, Login/Refresh/password exclusion and postcommit credential denial; full existing lifecycle/ListUsers suites and workspace/standalone gates |
| UM-EXT-01 | PASS | contract-inventory.md: four generated additive RPCs, old signatures/tags/wrappers, new safe fields/presence, semantic catalogs/locales/error mappings, tests/vet/publication/download and backend dependency |

Concrete source: internal/features/user/{detail,create,update,change_status,
management_service,management_transaction}.go, repositories/{reader,writer}/
user_management or user_detail/management_policy, handler/{user_detail,
user_management}.go, configs/{command_fingerprint,foundation,register.service}
and session/rate/security-attempt interceptors. Shared transaction provider,
ordered lock helpers, resolver, Argon2id, admission and safe audit/outbox/idem
persistence are reused. Safe replay stores initial fields, never refetches mutable
detail to patch response. Password digest version is independent config and
retained24h; startup fails before resources on missing/invalid configuration.

Requirement/scenario coverage: administration authority→UM01/07; detail→UM02;
restricted creation→UM03; editable profile→UM04; scoped replay→UM05;
persistence/rollback→UM06/07; admission→UM08; compatibility→UM01/10/EXT01;
status transitions/invalidation/reactivation→UM09; concurrent last-admin→UM10.
All12 requirements and37 scenarios in the two delta specs map to these rows;
foundation tests alone
are not used to claim new management implementation.

## Completion boundary

Backend completion applies only increment A. [Backend usage](../../../../docs/user-management-api.md)
and [Phase2 roadmap](../../../../docs/phase-2-management-plan.md) preserve remaining
B–E and Phase4/5/6. Role-revoke-versus-status concurrency belongs to C after that
RPC exists. Actual schema/admin baseline/key provisioning/caller allowlists/
consumer readiness remain mandatory DEP-01 before deployment. OpenSpec main
spec sync/archive and grpc-auth commit/push require separate user instruction.

Final verification: openspec validate auth-user-management --strict PASS; all
planning artifacts available,36/36 tasks complete.43 relative links/anchors
resolve;326 manifest file hashes match. gofmt -l cmd internal pkg tests empty,
git diff --check PASS. Reviewed working-tree scope retains previous lifecycle
changes; no local generated protobuf, private env/credentials/log/binary added
to source artifacts. api-contracts worktree clean after approved publication.

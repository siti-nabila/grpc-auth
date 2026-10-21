# Implementation evidence

Current status: **producer component-ready PASS**. Required dedicated PostgreSQL evidence now passes; activation-ready remains **BLOCKED**, owned by foundation and external consumers. The earlier pause below is historical.

## Baseline

Apply dimulai 10 Oktober 2026 pada HEAD `b89f37270d019c8d0d7823a3ff5d3526874d2232` dengan working tree foundation 14/36 belum committed. Baseline 144 public source/config hashes disimpan sementara untuk scope review; hash deliverable dicatat setelah verification. Tidak ada sibling/generated/dependency update yang diizinkan oleh change ini.

Reused: `domain.TransactionProvider/Transaction`, feature `TransactionOwner/AcquireLocks`, bound auth/session/audit adapters, `jwt.AccessVerifier`, `AuthenticateSession` dan immutable service/user principal. Credential reader existing belum memuat seluruh authoritative Login fields; signer/refresh writer/limiter belum tersedia pada baseline.

Exact registered methods tetap `/user.v1.UserService/Register`, `/user.v1.UserService/Login`, `/user.v1.UserService/Me`, `/user.v1.UserService/ListUsers`, `/user.v1.UserService/TesRPC`, `/profile.v1.ProfileService/UpdateProfile`.

Module baseline: Go 1.26.2, ORM v1.7.0, api-contracts `v0.0.0-20261010071509-42a9e6fa23ad`. Published additive response fields/catalog tersedia; inspection bukan hasil producer test. Historical schema preflight menunjukkan session/refresh/audit tables, refresh BYTEA/UUID/family/FK/uniqueness dan sessions tanpa version; dedicated test schema tetap harus diverifikasi.

## Acceptance status

| ID | Scope dan prerequisites | Actual evidence / status |
| --- | --- | --- |
| LSP-01 | grpc-auth Login eligibility, authoritative state/lock | PASS: controlled cases and real PostgreSQL locked password/status recheck on distinct backends |
| LSP-02 | grpc-auth atomicity; bound adapters + dedicated PostgreSQL | PASS: required writes/sign/deferred commit rollback, actual row counts, contention, cancellation/panic and pool reuse |
| LSP-03 | grpc-auth issuer + foundation verifier | PASS: key/claim/TTL checks and actual producer credential consumed by foundation on PostgreSQL |
| LSP-04 | grpc-auth initial refresh/published response/response loss | PASS: digest-only BYTEA, initial NULL state, FK/uniqueness, distinct repeated sessions/families; no replay cache |
| LSP-05 | grpc-auth independent safe failure audit | PASS: rollback before independent service-actor denial audit, redaction and preserved denial on audit failure |
| LSP-06 | grpc-auth basic local limiter + verified identities | PASS: exact groups, concurrent layered admission, cap/TTL/cleanup, trusted/fallback identity, locale and published RetryInfo |
| LSP-07 | grpc-auth restricted producer + foundation consumers | PASS: real PostgreSQL normal/restricted Me and ListUsers denial from actual Login; lifecycle RPC follow-up |
| LSP-08 | grpc-auth published contract; workspace + GOWORK=off | PASS: response/catalog/locale/RetryInfo decoder, default/release tests/build and release PostgreSQL producer suite |
| LSP-09 | EXTERNAL_REPOSITORY rest-orc/frontend/deployment | BLOCKED: actual caller/key/client/forwarding and coordinated activation remain foundation 7.3/7.4; not a producer component prerequisite |

## Verification log

Setiap hasil mencatat command, UTC time, versi/working-tree hash, safe environment, expected/actual dan limitation. Source/schema inspection serta historical foundation test tidak menutup producer acceptance. Default suite tanpa real DB/network; opt-in postgres hanya dedicated explicit test DB, bukan `user`/development/production. Tidak menjalankan Docker, migration, seed atau server sebagai test.

## Delivery gates

Component-ready: LSP-01..08 relevan lulus termasuk real PostgreSQL. Activation-ready: foundation seluruh mandatory consumers/Register/Profile dan gates 7.1..7.4 plus LSP-09. Default registered Login/server belum dialihkan. Full RefreshSession rotation/reuse/Logout/ChangePassword/management dan future limiter groups tetap follow-up; tidak mengklaim keseluruhan Phase 1/3 selesai.

### Issuer/config/entropy and bound persistence

Recorded UTC 2026-10-10T10:38:07.308398+00:00; current uncommitted deliverable, baseline module versions unchanged. `rtk proxy env GOCACHE=/tmp/grpc-auth-producer-go-cache go test ./tests/pkg/jwt/... ./tests/configs/... ./tests/features/auth_feature/... ./tests/repositories/transaction/...` PASS. Fixed-key/clock/entropy and SQL fake environment; expected claims/key validation, digest/UUID/NULL/handle/context/error tests passed. Default Go cache returned operation-not-permitted, so verification uses dedicated writable `/tmp` cache. One UUID variant assertion was corrected before passing rerun. No real DB evidence or activation PASS implied. Tasks 1.1–3.3 complete; schema runtime gate remains open.

### Login command

Recorded UTC 2026-10-10T10:43:39.182883+00:00; `rtk proxy env GOCACHE=/tmp/grpc-auth-producer-go-cache go test ./tests/features/auth_feature/...` PASS. Deterministic mocks verify exact proof→Begin→user/session lock→bound recheck→session/refresh/audit/sign→Commit, generic eligibility/stale proof, each failure/panic/cancel/commit ambiguity, safe independent SQL audit and response-loss/repeated sessions. Real PostgreSQL remains unexecuted; runtime registration unchanged.

### Limiter and response adapter

Recorded UTC 2026-10-10T10:51:22.082346+00:00; targeted `go test ./tests/ratelimit/... ./tests/interceptors/...` and `go test -race ./tests/ratelimit/... ./tests/interceptors/...` PASS with writable GOCACHE. Controlled clock covers nanosecond burst/refill, 100 concurrent layered requests, 10,000-entry capacity/TTL/cleanup/shutdown, trusted/spoofed/nil input, shared read quota, locale/catalog/RetryInfo through outer encoder and published decoder. A float boundary failure was fixed with integer credits before passing rerun. Handler/config/feature targeted tests PASS after preserving existing Argon2 (hash,password) argument order. No actual external forwarding or activation evidence.

### Production assembly and broad verification

Recorded UTC 2026-10-10T11:02:19.091608+00:00; Go go1.26.2, uncommitted producer at baseline HEAD. Production `PrepareLoginProducer` integration executes actual password verifier, primary/transaction-bound SQL adapters, signer, handler and session/rate/read consumers; only SQL driver is fake. Normal/restricted paths, each required write/commit failure, generic unknown/password/PENDING denial, independent safe failure audit, absolute DB expiry, unverified direct caller and rate/catalog/RetryInfo are covered. This does not prove real PostgreSQL behavior. Task 8.1 stays open for the required full acceptance evidence with group 7.

All commands below use `GOCACHE=/tmp/grpc-auth-producer-go-cache` because the normal user cache is not writable in the sandbox:

| Command (prefix `rtk proxy env GOCACHE=...`) | Expected / actual |
| --- | --- |
| `go test ./tests/features/auth_feature/... ./tests/integration/auth_reads/...` | PASS / PASS after hash-format guard |
| `go test ./tests/integration/auth_reads/...` | PASS / PASS after added transport/expiry/direct-caller scenarios |
| `go test ./...` | PASS / PASS |
| `go vet ./...` | PASS / PASS |
| `go test -race ./tests/features/auth_feature/... ./tests/integration/auth_reads/... ./tests/ratelimit/... ./tests/interceptors/...` | PASS / PASS |
| `make build` | PASS / PASS; output stays in ignored `bin/` |
| `GOWORK=off go test ./...` | PASS / PASS on published dependencies |
| `GOWORK=off go build ./...` | PASS / exit 0, nonfatal module stat-cache write warning in sandbox |

Workspace `/Users/bleki/Desktop/mini-projects/go.work` selects local api-contracts; release verification explicitly disables it and selects published api-contracts `v0.0.0-20261010071509-42a9e6fa23ad`. Both select ORM v1.7.0. `go.mod`/`go.sum`, default server registration and existing registered Login remain identical to the start-of-apply hash baseline. Existing foundation 14/36 work is preserved.

### Handoff and review

[Concrete assembly/readiness/rollback handoff](../../../../docs/login-session-producer.md#composition-handoff) maps producer components to foundation 7.1/7.2 and owner evidence to 7.3/7.4. This hands over partial component evidence; foundation gates remain unchecked and no external readiness or activation is asserted. Lifecycle ticker creation/cancellation/join belongs to the final coordinated server switch. Full refresh rotation/reuse, Logout, ChangePassword, remaining Register/Profile and management commands retain their owning changes.

`rtk proxy openspec validate auth-login-session-producer --strict` PASS. `rtk proxy openspec validate --specs` PASS (4 main capabilities; only informational long-requirement notices). `rtk git diff --check` PASS. Gofmt check of all 40 Go files changed since the apply baseline PASS. Final acceptance/finalization task 8.4 stays open until real PostgreSQL evidence passes.

### Historical PostgreSQL blocker and pause (resolved below)

Recorded UTC 2026-10-10T11:04:02.861427+00:00. Presence-only check confirms all three `GRPC_AUTH_POSTGRES_TEST_ENABLED`, `GRPC_AUTH_POSTGRES_TEST_DSN`, `GRPC_AUTH_POSTGRES_TEST_DATABASE` are absent from this process. No DSN, credential or server details were printed. `rtk proxy env GOCACHE=/tmp/grpc-auth-producer-go-cache go test -tags=postgres ./tests/integration/postgres -run '^TestPostgresConfiguration$'` PASS (guard tests only). Actual readiness command with the same prefix, `go test -tags=postgres ./tests/integration/postgres -run '^TestPostgresEnvironment$' -count=1`, exits 1: `PostgreSQL test requires explicit opt-in`. This is BLOCKED readiness, not application failure or real-DB PASS. No connection/schema/migration/seed/Docker action occurred.

Tasks 7.1–7.3 remain unchecked: producer-specific PostgreSQL scenarios/fixture session-refresh schema, two-backend barrier tests and their actual evidence must still be completed once the dedicated environment is provided. Existing historical foundation SQL schema lacks the producer session/refresh tables; this is a test-fixture prerequisite, not evidence that the application schema is missing. Neither schema was changed during this apply. Task 8.1 awaits that required full acceptance evidence; task 8.4 is not finalized. Current progress is 24/29, component-ready false, activation-ready false. No archive.

To resume, provide the three variables privately to the test process as described in [PostgreSQL harness README](../../../../tests/integration/postgres/README.md). The DSN must explicitly select the same dedicated database name; application database `user` and system databases are rejected. Then resume this same change, implement/execute group 7 and finish 8.1/8.4. Do not copy credentials to evidence/chat or repurpose development configuration.

The apply workflow pauses on an actual blocker, following the invoked `openspec-apply-change` skill: “Pause on errors, blockers, or unclear requirements - don't guess”. This is not an approval request or a narrowed acceptance criterion. Controlled verification remains valid only for the hashed source snapshot in `component-verification.json`; no database PASS is inferred.

### Resumed with dedicated PostgreSQL — component acceptance PASS

Recorded UTC 2026-10-10T11:35:56.963514+00:00. User supplied private `env-test.yaml`, key `Database.GRPC_AUTH_TEST`. A temporary private bootstrap parsed that file and passed explicit opt-in=1, dedicated DSN and matching expected database only to the test subprocess; it never printed credentials or changed runtime config. The first connection was blocked by sandbox networking; authorized escalation permitted the read-only identity check and test-scoped execution. No automatic approval rejection occurred. `.gitignore` now explicitly excludes `/env-test.yaml` (the existing `/env.*.yaml` pattern did not match this hyphenated filename).

Actual database identity `grpc_auth_test`, PostgreSQL `16.13`; all writes confined to freshly owned disposable `grpc_auth_test_<pid>_<counter>` schemas with synthetic fixtures. Session/refresh test schema matches inspected types/constraints, and actual catalog checks verify 21 columns / 14 validated constraints / 7 indexes. Authorization read dependency tables support actual Login-derived Me/ListUsers checks; this is not evidence of full foundation RBAC management acceptance. A final READ ONLY query confirms **zero remaining fixture schemas**. Application database, production migration/seed, Docker, broker and server activation were not used.

[PostgreSQL report](postgres-producer-verification.json) records safe identity, actual distinct backend IDs/barriers, command/tag/opt-in/results and limitations. New source: `tests/integration/postgres/test_scenarios/login.go`, `login_support.go`, `login_race.go`; thin runner and fixture schema extended. Actual production adapters prove committed session/refresh/SESSION_CREATED atomicity, digest and seven-day expiry, initial NULL state/FK/uniqueness, no user version/outbox/idempotency mutation, and distinct repeated Login sessions/families. Response loss is modeled as a subsequent valid Login after an already committed response; no network fault injection is claimed.

Real after-insert fault triggers for session/refresh/audit and a signer failure produce zero issuance rows. A real deferred FK failure at Commit also returns no credential and persists no issuance. Two physical backends and channel barriers prove committed password/status change before locked recheck causes generic denial and independent safe service audit; separate precommit contention proves RESOURCE_BUSY without credential/visible rows. Commit, signing failure, panic and cancellation each clean up locks/connections and permit a fresh Login. The password/status mutation is controlled fixture SQL following the shared user-lock protocol, not a fabricated production lifecycle RPC.

The first producer DB run found two test harness mistakes (missing SELECT bind argument and stale error in sign-fault cleanup); both were corrected before all passing runs. No production behavior was changed to accommodate the tests. Final source suppresses SQL debug output in its read adapters.

| Verification after resume | Actual |
| --- | --- |
| Production producer PostgreSQL suite, explicit opt-in and `-count=1 -v` | PASS |
| Entire existing PostgreSQL regression suite, `-count=1 -v` | PASS |
| Entire PostgreSQL suite with `-race -count=1 -v` | PASS |
| `GOWORK=off` production producer PostgreSQL suite | PASS on published api-contracts |
| Targeted feature/interceptor/production assembly integration after locale assertions | PASS |
| `go test ./...`, `go vet ./...`, `go test -race ./...` | PASS |
| `go vet -tags=postgres ./tests/integration/postgres/...` | PASS |
| `GOWORK=off go test ./...` | PASS |
| Earlier `make build` and `GOWORK=off go build ./...` | PASS; production sources unchanged during resume, build not repeated |
| Gofmt all 44 changed Go files, strict change/main specs validation, scoped links/whitespace/diff/status | PASS |

Commands use `rtk proxy env GOCACHE=/tmp/grpc-auth-producer-go-cache`; actual DB commands use `go run -tags=postgres /tmp/grpc-auth-postgres-runner.go env-test.yaml` plus the test flags recorded in the JSON report. Standard `go test -tags=postgres ./tests/integration/postgres` commands in the [README](../../../../tests/integration/postgres/README.md) work with the same three privately exported guarded variables. The temporary YAML bridge is not a runtime dependency or a replacement guard.

[Handoff](../../../../docs/login-session-producer.md#composition-handoff) now includes the required producer evidence for foundation 7.1/7.2. User clarification: frontend is a separately owned consuming application/repository, not a required folder in rest-orc; deployment is an operational readiness owner, not a required directory. No sibling/frontend/deployment changes were made. Foundation consumers and external readiness/whole-server cutover remain open; foundation checkboxes are not changed by this producer acceptance. Refresh rotation/reuse/Logout/ChangePassword, remaining Register/Profile commands and management retain their owners. No archive, commit, push or deployment performed.

Final hashed snapshot is `component-verification.json`. Component LSP-01..08 are PASS within this increment; LSP-09 and activation remain BLOCKED without actual external/foundation owner evidence. This does not claim all Phase 1/3 or full MVP completion.

Final review recorded UTC 2026-10-10T11:37:13.299940+00:00: all **29/29 producer tasks complete**, required component tests including dedicated PostgreSQL PASS. Change is ready for explicit archive; no archive performed. Foundation/external activation remains BLOCKED.

## Archive and typed-constant regression verification

Archived 10 October 2026 after explicit user authorization. All planning artifacts are done and all **29/29 tasks complete**. Three main capabilities now contain the twelve ADDED requirements, with original purpose, requirement statements and scenarios preserved. Main-spec acceptance links point to this archived design ledger.

The separately requested typed-constant refactor was verified in the immediately preceding turn on the current uncommitted deliverable: targeted feature/repository/interceptor/read integration tests, `go test ./...`, `go test -race ./...`, `go vet ./...`, `GOWORK=off go test ./...`, and full opt-in PostgreSQL `go test -tags=postgres ./tests/integration/postgres -count=1` all **PASS**. PostgreSQL used the private guarded environment bridge and dedicated `grpc_auth_test` (16.13); a READ ONLY post-test check returned zero fixture schemas. No credentials are recorded here.

`component-verification.json` and `postgres-producer-verification.json` are preserved as historical evidence of the original producer implementation; their original timestamps, source hashes and paths are not rewritten to represent the later refactor. The current source snapshot and archive checks are recorded separately in [archive verification](archive-verification.json). Archive changes only specs/documentation/artifact location; production and test source remain unchanged during archive.

Producer component acceptance remains PASS. Foundation remains **14/36**, and coordinated activation, external client/key/IP/deployment readiness and full Phase 1/3 acceptance remain open. Archive is not server activation, deployment, migration, commit or push.

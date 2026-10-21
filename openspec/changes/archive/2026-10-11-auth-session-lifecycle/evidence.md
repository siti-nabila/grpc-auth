# Apply evidence

Current status: **41/41 tasks complete; SLC-01..10 backend subset PASS**. See [final verification](#final-verification--task8183-complete) and [source manifest](source.sha256). Historical sections below preserve earlier planning/progress, not the current status. Main specs are synchronized and this change is archived as `2026-10-11-auth-session-lifecycle`. Mandatory Phase6 DEP-01 stays open; no application migration/deployment or grpc-auth commit/push.

## Historical status — 10 Oktober 2026

**1/33 tasks complete:** task 1.3 static schema/fixture comparison. Runtime lifecycle implementation remains NOT_STARTED; task 1.1 contract gate and dependent integration are BLOCKED. This report does not claim lifecycle, PostgreSQL tests or deployment readiness PASS.

## Historical contract preflight — 10 Oktober 2026

[lifecycle-preflight.json](lifecycle-preflight.json) records both workspace and pinned api-contracts inventories, required methods, hashes and gaps. The dependency release at that preflight is `v0.0.0-20261010071509-42a9e6fa23ad`; both saved inventories contain only the six existing user/profile RPCs and lack all nine lifecycle RPCs. REFRESH_TOKEN_REUSED is also absent. Existing Login response fields do not supply AuthService/SessionService definitions.

Required separate deliverable: [SLC-EXT-01..05](design.md#external-contract-prerequisites), including published contracts/errors/mappings/locales and generated code, followed by an explicitly scoped grpc-auth dependency update. No sibling, generated file, module cache or dependency was changed. Task 1.1 stays unchecked because its required deliverable is missing; source inventory alone cannot pass it.

## Static schema comparison — task 1.3 complete

Compared root tests/integration/postgres/fixtures/schema.sql against the archived foundation schema-preflight.json captured at 2026-10-10T06:41:26Z. The report includes a six-table column matrix, relevant saved constraints/indexes and current backend source SHA-256. Required auth/password/counters, session terminal state, refresh consumed_at/replaced_by/hash/family, audit/outbox and idempotency fields are present in both sources. No migration gap was identified by this static review.

Important implementation constraints: replaced_by is an immediate self FK, so insert the successor before linking the predecessor within the same transaction; revoked session rows require revoked_at and a trimmed 3–500 character reason. Session terminal revision remains conceptual version 2; no sessions.version column is required. Application validation must preserve the 32-byte digest format even though the DB constraint accepts any nonempty bytea.

This is saved metadata/fixture inspection, not a fresh live-schema verification or a functional test. No database connection, migration, seed, server, Docker or application tests were run. SLC-09 remains NOT_STARTED and actual deployment checks remain mandatory [Phase 6 DEP-01](../../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment).

## Historical verification — 10 Oktober 2026

Module resolution completed in workspace and GOWORK=off GOPROXY=off modes. Static assertions confirmed all nine required RPCs absent in both proto and generated method inventories; required fixture/saved columns and token-hash uniqueness/replaced_by foreign key are present. Exact source hashes and commands are retained in the JSON report. No runtime PASS is inferred from these checks.

## Planning revision — 11 Oktober 2026

Latest request is propose first. Updated proposal/design/delta specs/tasks only; main specs are not synced and runtime, sibling contracts, dependencies, commit/push and publication are not executed in this revision. Current progress is **1/40 tasks complete**, preserving only task1.3. Planning completeness is not implementation completion.

Current contract gate: nine RPC/generated definitions and required catalog additions exist in the local api-contracts checkout from the previous scoped work, but are unpublished. UserListItem.created_at and latest contract documentation still need the explicit task1.1 workstream. grpc-auth still references `v0.0.0-20261010071509-42a9e6fa23ad`; task1.2 remains unchecked. lifecycle-preflight.json retains its original historical inventory and is not rewritten to imply current behavior.

Approved revision adds ListUsers scalar/collection ordered sorting and query-bound composite cursors, ID/numeric compatibility, active collections, statuses/application_code, role OR/categories+search AND and total unique users before cursor/limit in one primary snapshot. Session sorting, consumed-refresh Logout rejection/latest revoked terminal proof, all-error rollback and independent safe failure audit are aligned across artifacts. Immediate refresh self FK requires successor insert before conditional predecessor consume; failure rolls the entire business transaction back. Security reuse remains commit-before-denial.

Added delta files: user-list-sorting (ADDED) and authoritative-authorization (full MODIFIED existing requirement). Existing lifecycle/rate/password capabilities remain in scope. Backend ledger SLC-01..10 stays NOT_STARTED/BLOCKED as applicable; Phase6 DEP-01 is still mandatory outside this backend increment. No lifecycle, ListUsers behavior or PostgreSQL PASS is inferred from documentation.

### Planning verification — executed 2026-10-11 03:53 UTC

Environment: local repository, source HEAD `869515bdc10ffe6e825ae318dfb2f7bec55cbe11`; documentation-only changes, no external services. Commands use RTK.

| Check | Expected | Actual |
| --- | --- | --- |
| `openspec validate auth-session-lifecycle --strict` | Valid change | PASS |
| `openspec validate --specs --strict` | Existing main specs valid without sync | 10 passed, 0 failed; informational long-requirement notices only |
| Local Markdown target/anchor assertions | All relative links resolve | 19 checked, 0 errors |
| Capability/task/modified-requirement assertions | One delta per capability, unique task IDs, original scenarios preserved | Six capabilities match six deltas, 40 unique tasks, only1.3 checked; all three original modified-requirement scenario names retained |
| `git diff --check` and scope/status review | No whitespace errors or changes outside change artifacts | PASS; six existing Markdown files revised, two delta specs added; main specs, historical JSON and implementation unchanged |

Historical preflight SHA-256 remains `97578917fe5c3699e9822156ef81a43e1880149fcecfaea33f5ce7e144bc3fc4`. No Go, generator, PostgreSQL or runtime tests were run for this documentation-only revision. Planning validation does not complete task8.3 or any implementation gate.

## Apply — contract prerequisite task1.1 complete, 11 Oktober 2026

Progress **2/40**: task1.1 publication and original task1.3 complete. Contract publication does not complete runtime lifecycle/ListUsers implementation. The earlier LOCAL_ONLY/BLOCKED sections above are historical; current published prerequisite SLC-EXT-01..06 is available.

Owner api-contracts, branch `feat/user-err`, normal push `cf88b66..668cbd2`. Published source commit `668cbd2cc925326b63d5e59f94ba18923cd5c160`; module `v0.0.0-20261011040220-668cbd2cc925`, checksum `h1:PnoO5de4B9l1UjNDpVFWLJwLQ0BqlTWKmE3GD+omOwg=`. No tag/release/merge/force-push. Contract working tree clean after publication.

All prior local lifecycle work retained: AuthService three + SessionService six unary RPCs, optional Logout credential/expected entity-version presence, safe management outputs and catalog/mapping/locales. Additive UserListItem.created_at timestamp tag10 preserves tags1..9; UserTokenResponse comments now cover Login/Refresh. Contract docs describe latest Logout/consumed rejection, session dynamic sort, ListUsers public-field sorting/filter/search/count/cursor semantics and all-error rollback/separate audit.

Executed checks on published source (local workspace + standalone): `rtk proxy make proto` twice (13 generated files, identical hashes), `rtk go test ./tests/protobuf/... ./tests/grpcerror/...` (70 passed), `rtk go test ./...` (82 passed), `rtk proxy env GOWORK=off go test ./...` (PASS), `rtk go vet ./...` and `rtk proxy env GOWORK=off go vet ./...` (PASS), scoped generated/source review and `git diff --cached --check` (PASS). New compatibility scenario verifies timestamp tag/type and full payload preservation across a legacy dynamic decoder. Initial sandbox test attempt could not write the Go cache; authorized escalation reran successfully, without code workaround.

`GOWORK=off go mod download -json github.com/siti-nabila/api-contracts@668cbd2` resolves the remote commit to the version/checksum above, verifying publication independently of sibling workspace. The 24-file commit contains only scoped proto/generated/catalog/tests/docs; no environment/key artifacts. Task1.2 dependency update is next. Backend SLC-02..10 and DEP-01 remain incomplete.

## Apply — dependency baseline task1.2 complete

Progress **3/40**, tasks1.1/1.2/1.3 complete. grpc-auth/go.mod now references `v0.0.0-20261011040220-668cbd2cc925`; go.sum adds its exact module and go.mod checksums. No other dependency changed. Workspace and downloaded published proto/generated/auth+user catalog/mapping sources are byte-identical; target nine RPCs, optional fields, additive created_at and error symbols resolve from the published module.

Executed `rtk go test ./...` (457 passed), `rtk proxy env GOWORK=off go test ./...` (PASS), `rtk proxy make build` (PASS, no service startup), and `rtk proxy env GOWORK=off go build -o /tmp/auth-session-lifecycle-standalone cmd/main.go` (PASS). No DB connection or deployment check was performed. Remaining tasks implement actual lifecycle/ListUsers behavior; baseline regressions do not complete those tasks.

## Apply — refresh persistence task2.1 complete

Progress **4/40**. Added domain RefreshReader/RefreshRotationWriter and validated consumption request. Primary/tx-bound digest lookup reads full stored rotation state with context and digest-length validation; no pool fallback. Rotation writer reuses initial successor creation, then conditional predecessor consumed_at/replaced_by update while unconsumed/unrevoked/unexpired. It returns affected-row eligibility without beginning/committing/locking; the feature must roll back its successor on false/error. Initial digest format and safe serialization are unchanged.

Strict SQL scenarios cover primary and bound lookup, mapped nullable/terminal fields, missing/dependency/cancellation, malformed digest/UUID/time, absent handles, successor INSERT before UPDATE on the same transaction, zero/one/invalid affected counts and RowsAffected failure. SQL mock now permits explicit driver.Result for affected-row/error cases; existing default remains one. Target `rtk go test ./tests/repositories/transaction/...`:65 passed; full `rtk go test ./...`:476 passed; `rtk go vet ./...` and `git diff --check`:PASS. No real DB or runtime RPC tested by this task. Feature rotation/rollback and PostgreSQL verification remain pending tasks3.x.

## Apply — lifecycle persistence and shared failure outcome tasks2.2–2.5

Progress **8/40**. Verified 2026-10-11T04:56:13.199620+00:00 on source HEAD `869515bdc10ffe6e825ae318dfb2f7bec55cbe11` plus the uncommitted source hashes below, using published api-contracts `v0.0.0-20261011040220-668cbd2cc925`.

Added owner-bound safe session readers/count/page and conditional session/family/user refresh revocation writers. Session sort tests cover all three allowed fields, both directions, UUID ties, null-last boundaries, limit+1, count before cursor, absent handles, context and dependency failures. Credential writer conditionally updates only allowed password/restriction/entity-version/auth-version fields; invalid/overflow versions are rejected. Safe terminal/global event and audit serialization are covered; adapters do not own transaction lifecycle/locks. Binding and invariants are documented in docs/session-lifecycle.md.

Shared attempt recording now stores bounded catalog identity/category rather than raw error text. Integration proves partial write → business/read/zero-row/cancel/panic error → rollback → separate attempt transaction; audit failure/panic retains original error and increments bounded telemetry. Reuse denial bypasses duplicate attempt recording because its feature must commit its own required evidence. This shared boundary test does not claim the not-yet-implemented Refresh RPC PASS.

Provider now retains an explicit connection lease through finalization. database/sql can mark cancellation transactions done before automatic driver rollback finishes; Conn.Close waits for that cleanup. A deterministic driver barrier with a one-slot pool proves the lease remains in use while rollback blocks and is released before independent transaction reuse. No application retry, transaction ownership transfer or ORM dependency change.

Executed: targeted repository/auth-read/interceptor/common tests **244 passed**; full `rtk go test ./...` **555 passed**, `rtk go test -race ./...` **555 passed**; standalone `GOWORK=off go test ./...` PASS; workspace/standalone `go vet ./...` PASS; `make build` PASS; `git diff --check` PASS. Dedicated opt-in PostgreSQL selected configuration/security-attempt/session-foundation/Login-producer tests PASS (3.035s), using private env-test.yaml Database.GRPC_AUTH_TEST to set explicit opt-in DSN/expected database; no secret printed, no application migration/server/Docker. Initial private wrapper had a case-sensitive YAML mapping error before any connection; corrected and rerun successfully. Dedicated full lifecycle/concurrency gates remain pending.

Source SHA-256:

- `pkg/database/transaction.go`: `c1cba4a7d819351bd3e6de8abbb92752f4020da8d2cb60198f2fdc760f39a68d`
- `internal/interceptors/security_attempt.go`: `ed4eab829f2912cf7c495cde4deadc372bf6ba433eeaa090ca1d6c37ff90e189`
- `internal/repositories/domain/session_lifecycle.go`: `894bb76ef8096d73d22601ecd74e6180bd08149544123de2260d888ef61607bd`
- `internal/repositories/domain/credential_invalidation.go`: `327b75ce6091a70245dcb5bb4900b5b351b88a5653904404523dac5c454e0231`
- `internal/repositories/reader/session_lifecycle.go`: `24aacc836eb12bed1f736056a9ca25bc72f63236ba2a8d8d10393dd3a39ab286`
- `internal/repositories/writer/session_revocation.go`: `9e108bc749736d5e547469734aabf628364dd42cd361732741faf360acd38ab1`
- `internal/repositories/writer/refresh_revocation.go`: `f4489272a736795f2537eb316e3d5714c507270834d9cb53b5068ac77c10a578`
- `internal/repositories/writer/credential_invalidation.go`: `c6b9d142a9bf60beabcd6c6d7d46e73416e4ee8cfb0336b90befabf97e21c61f`
- `docs/session-lifecycle.md`: `c50422ea1be319dea82088ef3964b36909f38f884701281768e87c8e621d14c7`

## Apply — refresh tasks3.1–3.5 and Logout proof task4.1

Current progress **15/41**, including user-approved generation prerequisite task1.4. Verification timestamp 2026-10-11T05:49:44.974173+00:00, source HEAD869515b plus uncommitted hashes below, published contract v0.0.0-20261011040220-668cbd2cc925. Default server routing/registration remains task7: passing feature/handler tests does not claim all lifecycle RPCs are registered.

Refresh proves canonical opaque digest and persistent owner/session without access JWT/payload identity, then admission callback, Begin, user/session locks and authoritative bound reread. Unknown/expired/nonactive/deleted/restricted/dependency/cancellation paths, both lock contention stages, owner/family changes and generation guards have exact-order tests. Rotated credentials allocate only two token UUIDs and 32-byte opaque successor entropy, retaining session/family/absolute expiry. Successor INSERT precedes conditional consume/link; audit and EdDSA signing precede Commit; result follows commit. Zero conditional rows and every required operation/error/cancel/panic path rollback. No rotation outbox, counter changes, grace or credential replay.

Consumed reuse writes session/family revocation and REFRESH_REUSE_DETECTED plus effective SESSION_REVOKED/one pending terminal event before Commit, then returns REFRESH_TOKEN_REUSED; no defer rollback after successful security commit and no duplicate boundary attempt. Repeated terminal detection does not duplicate terminal event. PostgreSQL tests prove immediate FK, same family/expiry, exactly one successor, successor denial after reuse, unrelated device still valid, atomic required-write/deferred-commit failures and response-loss semantics. Channel barriers prove distinct physical backends (example holder10677/contender10678 and subsequent distinct pairs), precommit busy, postcommit reuse, cancellation/panic rollback and pool reuse; no sleep proof. Required security write/family/audit/outbox/event-ID/deferred-commit failures preserve the live successor after rollback.

LogoutCredential verifies exactly one access or optional refresh mode and never installs Principal. Feature rechecks owner/account/generation/expiry under canonical user/session locks. Consumed Logout rejects without reuse mutation. Latest revoked unconsumed refresh proves terminal changed=false only; normal ACTIVE-only Principal still rejects terminal sessions. Conditional zero while authoritative state is ACTIVE is an error. Logout writer/family/audit/outbox/cancellation/panic/commit failures rollback before an independent failure audit. Handler maps only published changed response and preserves context/error identity. Single-session Logout is implemented, but task4.2 also includes RevokeMySession and remains unchecked; task4.4 also includes self/global concurrency and remains unchecked.

Executed targeted refresh/auth/handler/common tests332 passed before generation; updated feature/repository/auth-read tests440 passed; Logout credential/feature tests99 passed, handler/repository198 passed. Full workspace tests828 passed, workspace race828 passed, vet PASS; standalone tests PASS; make build PASS; git diff --check PASS. Full dedicated opt-in PostgreSQL suite PASS7.496s; Refresh suite normal PASS4.746s and race PASS6.420s; Logout suite PASS1.810s. Additional generation/Logout/refresh race and standalone checks are recorded in the next result entry. Initial test-only failures (unused import, mapping import name, UUID/text assertion cast, multi-statement parameterized setup) were corrected and rerun; no runtime failure was accepted.

## User-approved generation revision — task1.4

User explicitly approved adding persisted generation and a separate migration prerequisite after the schema gap was surfaced. Original task1.3 historical comparison remains unchanged; its original required-column set missed issuance generation. Existing sessions/refresh had no stored auth_version, so current account state alone could not distinguish a revoked latest refresh before/after later global invalidation. Proposal/design/session-management/refresh deltas/tasks are reconciled; task count becomes41.

Prepared docs/sql/session-lifecycle/01_session_generation.sql for existing public schema: transactional write exclusion on auth/sessions, nullable positive sessions.auth_version, ACTIVE/nonrevoked backfill only, no default1. Revoked legacy generation remains NULL and fails closed instead of guessing from current account/reason/time/audit. Login explicitly persists authoritative current generation; bound state reader preserves NULL legacy state; refresh and Logout compare known generation to current account. No application migration executed. Owned-schema PostgreSQL test remaps public qualification only to its verified grpc_auth_test_* namespace, executes the exact prepared SQL, proves active backfill/revoked NULL/new Login generation2, and cleans its fixture. Real Logout then production-writer controlled global invalidation proves old access/latest revoked refresh denied while terminal session issuance generation remains1 and user becomes2. Full production revoke-all/password evidence remains tasks4.3/5.x. Deployment owner application migration/cutover remains mandatory DEP-01.

Source SHA-256:

- `internal/features/auth_feature/refresh.go`: `561c3e6d1cb878f29976ea2e8ce18b9a07cc1f349ec1923c8344d045f270de62`
- `internal/features/auth_feature/refresh_service.go`: `0297f36e33f7be1f75234b6004881e70efc5842efb18c8342199cae6eb4281c7`
- `internal/features/auth_feature/refresh_reuse.go`: `1ef142e39b5fac76d24a81cba187c1c89ead9bdac5c61cf4da3f403525ecbf29`
- `internal/features/auth_feature/credentials.go`: `0fd275a9cbd8f4d08e16b6e1d646600c07fa382b28477876bbdaf97a8f4532f1`
- `internal/features/auth_feature/logout.go`: `721bd22660b9126d24ef1607e469d689b1bfac46184b6946a5c46c04d98beb18`
- `internal/features/auth_feature/logout_service.go`: `96fc1f5ef9f5a836d4196d79f90528964a1eadfe5a9ea3ae2ef395071405124c`
- `internal/sessions/logout.go`: `24590b3909926a71b16b26cdfa3da60760202b4e6a41a9f3b4190369ecbc27f8`
- `internal/handler/refresh.go`: `1b857154ebd073e357173b16481de752391844d603cc4caf8e6c3cc712ee751b`
- `internal/handler/logout.go`: `3ef537be31b0a02ea4b552656d21f28338c341265ff4f4e5ec3b4029c674d197`
- `internal/repositories/domain/session.go`: `517c122374392387f5fe8dfca4752c2fb678e2219467d35ce068cfd48638be09`
- `internal/repositories/reader/session.go`: `c171b88f98c221e33094c997df82315253a12a3771823dd51864113733eabe5b`
- `docs/sql/session-lifecycle/01_session_generation.sql`: `b34817b08c5878e008e4e13a85c6861011c0be94bb4f127e42f48e8a50b459b8`

## Apply — self session operations tasks4.2–4.4

Timestamp UTC: 2026-10-11T06:12:23.861621+00:00. Base HEAD remains 869515bdc10ffe6e825ae318dfb2f7bec55cbe11; deliverable includes uncommitted implementation below. Published contract remains v0.0.0-20261011040220-668cbd2cc925.

- New SelfService and SelfSessionHandler implement owner-scoped single revoke and global self invalidation; canonical user/session locks, locked authority, required safe audit/outbox and response-after-commit. Single-device counters unchanged; global increments entity/auth counters once and requires Login. No default RPC wiring claim (task7 pending).
- `rtk go test ./tests/features/session ./tests/handler`: PASS,154 tests; self feature81 cases include strict ordering/arguments, both scopes, IDOR, terminal no-op, current-session revoke, restricted/stale principal, busy, conditional failures, each dependency/cancel/panic and commit cleanup.
- `rtk proxy go run /tmp/auth-lifecycle-pg-check.go -run TestPostgresSelfSessionLifecycle -race`: PASS,9.161s; dedicated opt-in test DB only, disposable harness-owned schemas. Exact sessions/refresh/counters/audits/events assertions, old credential denial/new Login generation, required write/deferred commit rollback, zero sessions after counter write and independent service-actor failure audit, distinct backend barriers in both Login/global orders, cancellation/panic cleanup and pool reuse. Test-only initial assertion incorrectly expected user actor for independent failure audit; corrected to verified rest-orc service actor, consistent with boundary contract; all cases rerun passed.
- Previous generation/Logout/refresh selected race PASS11.294s, standalone full tests and make build PASS before this self increment; final broad verification remains task8.
- Progress18/41 tasks. Password, reads/admin/ListUsers, admission/default wiring and final gates remain pending. Deployment migration is not executed; DEP-01 remains open.

Source SHA-256:
- `internal/features/session/impl.go`: `77399ac7b619ab430437082663a3b08426405a699a8c7b0947393d7b89046b6d`
- `internal/features/session/self.go`: `a6227ac555eba113114e1d32066b9b98a1a74b8e34e34dafca0268414a4e3679`
- `internal/features/session/global.go`: `ef802523b4dfeef5af344ce5ccc5776358642a9f87495001a3ff859866a6d4ef`
- `internal/handler/self_session.go`: `64cd055e2c59843deeed389afeb81151f8fbe57c5f193815b578898a58eb67e6`
- `tests/features/session/test_scenarios/self.go`: `0a1989be608c2eb31886d3d0d0e847220a7181231eb7c0ca0cd5b6aa51e85c3d`
- `tests/integration/postgres/test_scenarios/self_session.go`: `c75b44abd8a43f15896d77494c2e76ca98b220a180a1a4cec198440e5fd9fee1`

## Apply — password tasks5.1–5.4

Timestamp UTC: 2026-10-11T07:12:32.380119+00:00. Base HEAD869515bdc10ffe6e825ae318dfb2f7bec55cbe11 plus uncommitted source below; api-contracts v0.0.0-20261011040220-668cbd2cc925.

- Current password proof/hash preparation precedes transaction; under canonical user/all-session locks, principal/account/credential proof is rechecked. Restricted self allowed. Atomic password_changed_at/hash/restriction/counters plus all-session/refresh invalidation, two required audits and one global pending event with PASSWORD_CHANGED reason. No replacement credentials or credential replay.
- `rtk go test ./tests/features/auth_feature -run TestChangePassword`: PASS76 tests; password bounds, safe serialization, wrong password/owner/state/stale proof/dependency, exact writes/context/ordering and all stage failures/cancellation/panic/commit cleanup.
- `rtk go test ./tests/handler ./tests/features/auth_feature -run Test(ChangePassword|ChangePasswordHandler)`: PASS84 tests; published safe response/context/input preserved, nil/malformed result errors and English/Indonesian mismatch catalog.
- `rtk proxy go run /tmp/auth-lifecycle-pg-check.go -run TestPostgresPasswordLifecycle -race`: PASS14.453s; dedicated opt-in harness-owned schemas only. Restricted/ordinary password success, exact hash/timestamp/restriction/counter/session/refresh/two-audit/global-event assertions; old password/access/refresh denial, fresh Login generation2. Every required persistence and deferred commit failure, conditional zero after partial write, cancellation/panic rollback before independent service-actor failure audit; Login/refresh/global contenders on distinct physical backend IDs using channel barriers, postcommit generation and pool reuse.
- Default AuthService wiring/admission remains task7; final broad tests task8 remain pending. Backend progress22/41; DEP-01/fullMVP remain open.

Source SHA-256:
- `internal/features/auth_feature/change_password_service.go`: `6f4083877ccf64ed6df84abd988722e7d76c1a8bdefca994b20f2ee3bdd9cbb5`
- `internal/features/auth_feature/change_password.go`: `ed47d3e0f23ce248fdcc3df4a3100dca3ef5960c331c28a3e8791db53aea42d4`
- `internal/repositories/domain/password.go`: `756c1a1dfa501f437e20873c5782af5a8b5e748afc85fe3de7f1c8f7ff52f12c`
- `internal/handler/change_password.go`: `a06ce9cdb38fe95a9bdc4ab59a21454ba2b800bde73835492379df4f5ff8c03a`
- `tests/features/auth_feature/test_scenarios/change_password.go`: `aa84bea75b729f5bafcd4b7057016ed42b203638f910245a4133430c02d24ff2`
- `tests/integration/postgres/test_scenarios/change_password.go`: `789614de5514b16809c7840acc95871f110a97f58f69f098c2f3a3985ce94f4f`

## Apply — session list task6.1

Timestamp UTC: 2026-10-11T07:39:03.163106+00:00. Base HEAD869515bdc10ffe6e825ae318dfb2f7bec55cbe11 plus uncommitted source; published api-contracts v0.0.0-20261011040220-668cbd2cc925.

- ListMySessions/ListUserSessions implement one primary RR snapshot for current authority/page/count, exact iam.session.read for admin, restricted denial, safe metadata/current flag and owner/query-bound canonical composite cursor. Defaults page1/limit20/max100, created_at DESC/id DESC; created_at/last_seen_at/expires_at ASC/DESC same-direction ID and null-last. Unsupported PageQuery fields/search/multi-primary shapes rejected.
- `rtk go test ./tests/features/session ./tests/handler`: PASS331 tests. Session feature226 tests includes existing self81 plus145 read/cursor cases. Authority/binding/read/commit/cancel/panic cleanup, every supported field/direction/null, malformed/type/owner/sort/direction/limit cursor mismatches before snapshot; safe handler projection/nulls/current/empty metadata/error identity.
- `rtk proxy go run /tmp/auth-lifecycle-pg-check.go -run TestPostgresSessionListLifecycle -race -v`: PASS2.671s; six keyset orders across timestamp ties/nulls, exact once IDs/current flag/total including empty later page; distinct physical backends50725/50726 prove old authority and data stay in one RR snapshot, next request sees permission loss even for empty target. Owned dedicated test schemas only. last_seen_at is NOT NULL in baseline; only this owned boundary fixture drops NOT NULL to verify defensive null ordering. No application migration/schema change required or executed. Initial unused import and fixture constraint setup corrected, entire runner rerun PASS.
- Progress23/41; admin mutations, ListUsers extension, admission/default server and final gates pending. DEP-01 unchanged.

Source SHA-256:
- `internal/features/session/list.go`: `8d5c6128fb24c61fe95c7e80f79892f9597ec1ff4f03c4125eac3a89dfd6578c`
- `internal/features/session/list_cursor.go`: `e31b64548dd907a3c6e515f1598ae4d9a7e649c647eb3b391ce7d573c973aa09`
- `internal/features/session/list_service.go`: `dc89e378a5eb95b1465763156a89f1b987f6b84d024dcf63f5fedb5a8ad55027`
- `internal/handler/list_sessions.go`: `be6584b598843cc1a1d8fffb7dc870487321afdd3575e39a4fe0c2001aa51cc5`
- `tests/features/session/test_scenarios/list.go`: `daa70ddd3d274daaa1c451bf72026a73c525aabc24e6928108974167d597d5a2`
- `tests/integration/postgres/test_scenarios/list_sessions.go`: `cd05e4ea91bb019ebddbe84c81ff0314afc7c5b01123ed009505922839ec307c`

## Apply — admin session tasks6.2–6.5

Timestamp UTC: 2026-10-11T09:15:37.302262+00:00. Base HEAD869515bdc10ffe6e825ae318dfb2f7bec55cbe11 plus uncommitted implementation; api-contracts v0.0.0-20261011040220-668cbd2cc925.

- Admin single/global commands implement current exact iam.session.revoke with mandatory idempotency24h from first command; idem/policy/sorted user/role/application/session locks; current actor check before replay and replay before target expected-version compare. Single terminal new-key no-op audits/result only; global new-key generation invalidation even when zero ACTIVE sessions; atomic required audit/outbox/safe result. Safe target projection excludes credential material; shared terminal event helper now takes actual actor explicitly.
- `rtk go test ./tests/features/session -run TestAdminRevocation`: PASS133 tests, strict calls/context/arguments and all stage errors/cancel/panic, busy, version conflict, lost current permission, IDOR, no-op/replay/expiry/conflict and safe result/event actor.
- `rtk go test ./tests/handler ./tests/features/session ./tests/repositories/transaction`: PASS634 tests; mandatory metadata presence/duplicates, expected entity version bounds/presence, safe mappings; explicit target SQL/invalid ID/missing/dependency/cancellation projection.
- `rtk proxy go run /tmp/auth-lifecycle-pg-check.go -run TestPostgresAdminSessionLifecycle -race`: PASS11.724s; owned dedicated PostgreSQL schemas only. Exact mutation/refresh/counters/audits/events/records; same-key replay and response-loss semantics, new-key no-op, independent target scope, expired record replaced with fresh24h, global stale expected/conflicting payload, new key with empty target, self-target actor denial. Every required write/result and deferred commit fault rollback before independent failure audit; distinct physical backends/channel barriers for idem contention, different-key same-version conflict, policy role disable precommit busy and postcommit replay denial. Initial unsupported test Delete API/safe event assertion corrected to actual ORM Updates and JSON payload actor; full runner rerun passed.
- Existing six-RPC/default composition unchanged until task7. Progress27/41. ListUsers extension, admission/default registration and final checks pending; DEP-01/fullMVP open.

Source SHA-256:
- `internal/features/session/admin_service.go`: `79c2a9641542ba67c25b17e8de330091f15062ef8292832024fbd5cedd83f851`
- `internal/features/session/admin.go`: `0705b2dd802653a049a9ac3549d6cc9886914439ceaab41a96b872bf3a34c986`
- `internal/features/session/admin_revocation.go`: `52e5a58595303907ff24bf33ce8722b4ae6f94012f8a82627c284b0b82c4e076`
- `internal/repositories/reader/session_user.go`: `eec39f87d90f9940447495bcd8e9ffd74198ee61fa4b6503546eb5719e16296d`
- `internal/handler/admin_sessions.go`: `438aafed8928533cab3e7a1d62c192b4578e41eceecfea91183166fa88ab92c2`
- `internal/features/common/session_event.go`: `37dde5b5135246f4df5491587997d855a6b371fbec7a08cd166dddb9e261ddd6`
- `tests/features/session/test_scenarios/admin.go`: `10c5564b3dca8786af12a4eedee87a261ddc9b2dec1f124dcc670bf5ecf2fe8d`
- `tests/integration/postgres/test_scenarios/admin_sessions.go`: `ff3855e3a3b11bc0d694adfeec6b020c218fb99f51433129d56b5c3a07f7b0eb`

## Apply — ListUsers task6.6–6.11 complete

UTC 2026-10-11T09:59:34.385453+00:00; grpc-auth HEAD869515b, uncommitted source below; contract commit668cbd2/module v0.0.0-20261011040220-668cbd2cc925.

- `rtk proxy go test ./tests/features/user ./tests/handler ./tests/repositories/reader/user ./tests/integration/auth_reads ./internal/features/user`: PASS. Strict query/filter SQL, 44 query/cursor scenarios, published safe response mapping, session optional matching ID tie, dependency count/page/role/app rollback, six-RPC existing snapshot regression.
- `rtk proxy go test ./...`: PASS after ListUsers initial implementation; full final verification remains task8.
- `rtk proxy go run /tmp/auth-lifecycle-pg-check.go -run TestPostgresUserListLifecycle -race -v`: PASS dedicated private test DB, exclusively owned harness schemas. Every10 public field ASC/DESC, three mixed multi-sorts, defaults and auth_id/numeric compatibility, ties/null/empty/missing profile/index, 6 unique users despite two-role × two-app collections, total before cursor including empty last page, role OR + search/category AND, status/application/created range, independent full active collections, email/name keyword and projection preserving cursor. Two physical primary backend IDs + channel barrier prove authority/total/page/collections one RR snapshot; next request rejects committed permission loss. No application migration, default real-network suite, Docker, REST or sibling change.

Implementation count/page builders are separate because ORM Select appends columns. Public nullable strings use COALESCE only for response, while separate safe nullable keys retain actual nulls for ordering/cursor. Auth is the base with LEFT JOIN unique profile/search records: missing index does not hide users in unfiltered totals. Unknown/duplicate/internal/expression sort rejected through explicit registry. Scalar safe fields and collection projections carry no credentials or auth_version. Runtime wiring/final capability verification remain pending.

|Source|SHA-256|
|---|---|
|`internal/repositories/domain/user_list.go`|`540c6ad57280e530ee88dcb77e6daae8aeeaf50aa4ff60bbc17b0213b99868e8`|
|`internal/repositories/reader/user_list_query.go`|`e9c6a7429e60ce9fbc7833faa3f870f1f0113b0d15217c1d2c26dd3789fd1090`|
|`internal/repositories/reader/user_list_collections.go`|`42755d768570429681925f7ae506ac0beae2979ce99c0dfa203eb3481163fc10`|
|`internal/features/user/user.go`|`d9f8fd772048c2f8fa66142f8f4281f31b56a1cabcb207d3979125c276842ef3`|
|`internal/handler/user.handler.go`|`a36c900a9f2ec622d414c9468bc73262f0bf0b91315ec12c02140216ecd1a2c2`|
|`tests/features/user/test_scenarios/user_list.go`|`6413452699324a04ae48851db96bf7cf18b0183b450074075025fe24ff44961d`|
|`tests/integration/postgres/test_scenarios/user_list.go`|`965e5a767b515f472b3802b94d54a77cfd4c230b4aac42791e16334017f6eb02`|

## Apply — admission and default server task7.1–7.5 complete

UTC 2026-10-11T10:15:36.144577+00:00; uncommitted grpc-auth source, baseline HEAD869515b, published contract668cbd2.

- `rtk proxy go test -race ./tests/interceptors ./tests/ratelimit ./tests/configs ./tests/integration/auth_reads`: PASS. Nine RPC × eight credential/restriction/dependency/namespace modes; separate actor read/admin/password quotas and no invented self/Logout quotas; controlled refill/concurrent atomic refresh admission, TTL/cap/cleanup regression. Dependency lookup remains UNAVAILABLE; early refresh failure charges only IP, proven session callback consumes both applicable buckets once; failed session admission preserves eight IP tokens. No payload identity picks a session bucket.
- `rtk proxy go test -race ./tests/integration/auth_reads -run TestRegisteredLifecycle`: PASS. Default production assembly inventory4 services/15 methods, actual in-memory TLS/locale/caller allowlist boundary for all15; strict controlled SQL verifies independent safe audits for nine credential failures, no unverified-service persistence or bucket allocation. Existing registered mock Login/Me/Profile and mapping tests remain PASS.
- `rtk proxy go run /tmp/auth-lifecycle-pg-check.go -run TestPostgresRegisteredLifecycle -race`: PASS (3.857s), dedicated owned PostgreSQL schema and same production assembly over bufconn TLS. Executed real Login → rotation → protected Me/session reads → admin single/replay/terminal-noop → self single/global → admin global/replay → wrong-current-password legacy INVALID_ARGUMENT → password invalidation/fresh Login → Logout access/repeat terminal access/refresh. Consumed refresh cannot Logout; old generation terminal refresh fails. Registered ListUsers verifies public collection multi-sort, role OR/search/app/status AND, safe fields and full collections. Restricted methods deny while Me/Logout/password work; fresh Login unrestricted. Required outbox trigger failure rolls back session/refresh/all effects before safe independent failure audit; retry succeeds. Refresh session/IP quotas, shared Me/session read quota and localized RetryInfo verified with exact persistence counts.
- `rtk proxy go run /tmp/auth-lifecycle-pg-check.go -run TestPostgresRegisteredServer`: PASS (1.040s), six-RPC regression, PENDING/token-empty Register and unique ListUsers now includes pending user without index. Full public field pagination/snapshot checks are additionally task6 executed production ReadUsersSnapshot plus handler request/response mapping on actual PostgreSQL; they are not fabricated network coverage.

Composition uses reusable primary adapter/provider/issuer/limiter and synchronized injected entropy. Existing fail-fast configuration, reverse startup cleanup and worker join/repeated stop tests PASS. env.example.yaml lists all15 exact method examples without secrets; actual local/private caller configuration is unchanged and remains deployment-owned. README and service docs updated. Actual REST cookie/refresh single-flight, TLS/key rotation/trusted-IP/login-again readiness and application migration remain mandatory Phase6 DEP-01. No deployment/server/app migration, RPC sibling change, grpc-auth commit/push or archive.

|Source|SHA-256|
|---|---|
|`internal/configs/lifecycle.go`|`b72be00bc8313dcd1494a4411efe3b66bc9e3d6d08bef6246a9e686cb7847ce9`|
|`internal/configs/lifecycle_services.go`|`c4c4f4448e628cb927bcf8c32f26b399d7ae935699a92d305b289f25e9684736`|
|`internal/configs/foundation.go`|`2313419518af674ac0159979e9568a168a074f48103929a6f47fa9e430cfa0c2`|
|`internal/interceptors/refresh_admission.go`|`75f1afbac14c6ab45f8502c4d74ca6555824256f8be45147745d07362f5fbdbc`|
|`internal/interceptors/session_interceptor.go`|`7b4670717306cbcf82d0b883b2eccee4e1ab263c0c108e7904e7480e06876c4f`|
|`internal/interceptors/rate_interceptor.go`|`8e7919c29c2ba4d8059255a8e8e7337981609930c5060a854f2e175931beb9cf`|
|`internal/ratelimit/store.go`|`144c79a4cd2fd38e207bebe638bea76a8ce0a49473f0b11632ed59229f2aad95`|
|`tests/integration/postgres/test_scenarios/lifecycle_registered.go`|`9c41dadb919aac08d29fb70df50b5d8135b6b9ee357e2ba331b6952822077ae9`|

## Final verification — task8.1–8.3 complete

UTC 2026-10-11T10:18:01.897814+00:00; local macOS Go toolchain, grpc-auth baseline HEAD `869515bdc10ffe6e825ae318dfb2f7bec55cbe11` plus uncommitted deliverable. [source.sha256](source.sha256) captures 134 changed production/test/SQL/doc/module/example files; manifest SHA-256 `1dd532c1c0c6aaa935a290a6f78b2aac06260577425e30be21f6673b630e89e9`. Module api-contracts `v0.0.0-20261011040220-668cbd2cc925`, published commit `668cbd2cc925326b63d5e59f94ba18923cd5c160`. Historical milestone hashes remain point-in-time records; this manifest identifies final source.

|Executed command/check|Expected|Actual|
|---|---|---|
|gofmt -l on all127 changed Go files|No unformatted files|PASS|
|`rtk proxy go test ./...`|Full default deterministic suite|PASS, all packages|
|`rtk go test -race ./...`|Full workspace race suite|PASS, RTK reports1477 passed in65 packages|
|`rtk go vet ./...`|Workspace static analysis|PASS, exit0|
|`rtk proxy make build`|Workspace binary compile only|PASS, ignored bin artifact, not executed|
|`rtk proxy env GOWORK=off rtk go test ./...`|Published standalone contract test|PASS,1477 passed in65 packages|
|`rtk proxy env GOWORK=off rtk go test -race ./...`|Standalone race suite|PASS,1477 passed in65 packages|
|`rtk proxy env GOWORK=off rtk go vet ./...`|Standalone static analysis|PASS, exit0|
|`rtk proxy env GOWORK=off make build`|Standalone compile only|PASS, binary not executed|
|`rtk proxy go run /tmp/auth-lifecycle-pg-check.go`|Full opt-in dedicated PostgreSQL suite|PASS17.340s, actual owned schemas/fixture cleanup|
|`rtk proxy go run /tmp/auth-lifecycle-pg-check.go -race`|Full opt-in PostgreSQL race/concurrency/foundation/last-admin regression|PASS57.260s; distinct physical backend/barrier assertions and pool cleanup|
|`openspec validate auth-session-lifecycle --strict`|Six deltas/artifacts valid|PASS|
|`openspec validate --specs --strict`|Main specs valid without sync|10 passed,0 failed; informational long-requirement notices only|
|Changed Markdown link/anchor check|Every relative target/anchor exists|12 Markdown files checked,0 unresolved|
|`git diff --check` and status/scope/redaction review|No whitespace errors or secret/runtime/generated scope leaks|PASS; no credentials/PEM/private config/log/binary tracked or newly exposed|

Full PostgreSQL suite includes transaction ownership/locking/persistence/last-admin harness, generation migration in owned fixture only, rotation/reuse/self/admin/password interleavings, required/deferred-commit errors, cancellation/panic/independent audit, dynamic user/session lists and registered production assembly. Assertions use actual rows/counters/audits/events/idempotency, not inferred status. Detailed earlier evidence identifies two backend IDs/barriers and each targeted scope. In-memory TLS boundary tests use controlled SQL mocks in the default suite; full successful lifecycle flows over the same TLS transport with actual adapters run only in the explicit opt-in PostgreSQL suite.

Completion applies to this increment only: nine lifecycle RPCs, approved ListUsers extension and required backend guarantees. No status lifecycle/CreateUser/role/application CRUD, actual rest-orc/frontend client, RabbitMQ relay, application migration or deployment cutover implemented. Session generation SQL remains a prepared prerequisite file, not executed against application DB. Actual caller allowlists/rotation, TLS/client-IP, secure token storage, single-flight refresh/RetryInfo and login-again behavior remain mandatory [Phase6 DEP-01](../../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment). Main specs not synced; change not archived; grpc-auth work remains uncommitted as requested.

## Archive and spec synchronization — 11 Oktober 2026

Executed `openspec archive auth-session-lifecycle --yes --json` with validation enabled. CLI synchronized all six capabilities (17 ADDED and one MODIFIED requirement) and moved the complete change, including metadata/evidence, to this directory. All 41 tasks remain checked. `openspec validate --specs --strict` passed all 14 main specs; `openspec list --json` reports no active changes.

Archive relocation required relative documentation/evidence link repairs only. The source manifest retains its 134 entries; the `docs/session-lifecycle.md` digest was refreshed for its archive links. Previous manifest digest `1dd532c1c0c6aaa935a290a6f78b2aac06260577425e30be21f6673b630e89e9` is historical final-verification evidence; current manifest SHA-256 is `8bb2e879db7997a717636176bdc14e526454a5c578e6b48735b56177df17c9cb`. No production code or test behavior changed during archive. Migration execution and deployment readiness remain Phase 6 DEP-01. No grpc-auth commit/push was performed.

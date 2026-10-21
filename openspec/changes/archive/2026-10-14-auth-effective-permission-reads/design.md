# Design

## Context

Motivasi ada pada [proposal](proposal.md); behavior normatif ada pada [delta spec](specs/effective-permission-reads/spec.md). Target adalah increment E grpc-auth, sesuai keputusan user 14 Oktober 2026, termasuk filter opsional dan tampilan union ACTIVE untuk akun nonaktif.

Baseline inspected 14 Oktober 2026: HEAD `2462cb09d0f8e7eef3b17a70456451b8bd2dd6ec`, Go 1.26.2, ORM 1.7.0, api-contracts pin `v0.0.0-20261013075041-21082d7a243d`. Banyak perubahan B/C/D/refactor belum committed; source working tree beserta untracked files menjadi baseline apply. Parent go.work memakai `../api-contracts`, berbeda dari nama direktori grpc-contracts pada workspace chat. api-contracts hanya dibaca dalam proposal ini.

Graph commit `bf6be877296b8f4191f2e2db5f744b23ab636ed7` lebih lama dari working tree. Degree hub auth/idempotency/outbox/user-role/transaction/snapshot adalah 236/228/222/217/214/213. AST EXTRACTED menunjukkan path ReadUserDetailSnapshot ke snapshot owner/state/permission reader/ResolvePermissions; source sekarang sudah memakai AuthorizeCurrentPermission. Graph menjadi routing, bukan bukti runtime terbaru. Tidak ditemukan graph-instruction.md atau AGENTS lebih dekat.

| Source inspected | Temuan yang membatasi desain |
| --- | --- |
| `features/common/authorization.go`, `repositories/domain/authorization.go`, `reader/authorization.go` | Resolver sudah menghasilkan sorted unique code dan role ID/code, mengecek assignment/role/application ACTIVE, same-app/namespace dan business access. Reader mengambil seluruh contribution rows termasuk yang nonactive; resolver yang menyaring |
| `features/auth_feature/read_permission.go`, `sessions/principal.go` | AuthorizeCurrentPermission re-authenticate current state dan exact permission. Self read perlu AuthenticateSession + AuthorizeMethod langsung; helper permission menolak empty required set |
| `features/user/detail.go`, `list_assignments.go`, `assignment_service.go` | ReadUserDetailSnapshot menjadi pola fungsi read dengan provider/now eksplisit dan explicit owner/commit/finish. Safe target checks existing dipakai ulang; service/binder assignment tidak dibawa ke E |
| `reader/session_user.go`, `domain/session_user.go` | Safe identity/status/version/deleted reader reusable; tidak perlu credential/profile/session target |
| `pkg/database/transaction.go`, `features/common/transaction.go` | BeginSnapshot memakai primary RepeatableRead ReadOnly dan leased connection; owner di feature, cleanup termasuk canceled/panic/failed commit |
| `handler/session_reads.go`, `user_role_assignment.go`, `configs/foundation*.go`, `register.service.go` | Thin adapters dan explicit construction; server baseline 33 methods, belum ada E |
| `interceptors/session_interceptor.go`, `rate_interceptor.go`, `security_attempt.go` | Exact classification wajib ditambah; unknown methods denied. Shared read quota dan independent safe failure audit reusable |
| `api-contracts/proto/user/v1/user.{service,payload}.proto`, dictionary/mapping auth/common | Dua RPC/messages E belum ada. Error validation/not-found/forbidden/session/dependency/rate sudah tersedia |
| `tests/features/common`, `features/user`, `integration/auth_reads`, `integration/postgres` | Resolver tests, strict SQL/tx harness, registered server dan two-physical-connection snapshot barriers reusable |

Historical AGENTS/config context menyebut baseline token-only/HS256 dan enam method. Keputusan user serta main specs yang lebih baru dan source current mengatur EdDSA/session-aware/33 methods. Tidak ada rollback behavior ke baseline lama.

## Goals / Non-Goals

**Goals:** explicit self/admin read functions dengan satu domain result dan dependency provider/now melalui parameter; reuse resolver dan tx-bound readers tanpa mengubah fungsi otorisasi; contract kecil dan hasil lengkap/deterministik; registered-path dan real PostgreSQL evidence.

**Non-Goals:** generic read framework/policy registry, boolean-mode orchestration, role history engine, target-session impersonation, paginator/filter DSL, mutation/lock/idempotency baru, schema/config update, refactor unrelated atau implementasi REST/UI/EvaluateAccess. KISS/DRY/YAGNI dan complexity rendah mengikuti [AGENTS.md](../../../../AGENTS.md).

## Decisions

### 1. Shared additive response, separate self/admin inputs

Rencana contract di api-contracts, seluruh message baru dalam user.v1:

| New message | Planned fields and tags |
| --- | --- |
| GetMyEffectivePermissionsRequest | string application_code=1 |
| GetUserEffectivePermissionsRequest | uint64 user_id=1; string application_code=2 |
| EffectivePermissionRole | uint64 role_id=1; string code=2 |
| EffectivePermission | string code=1; repeated EffectivePermissionRole roles=2 |
| GetEffectivePermissionsResponse | uint64 user_id=1; string user_status=2; uint64 version=3; repeated EffectivePermission permissions=4 |

Both new UserService methods return GetEffectivePermissionsResponse. Existing tags/types/methods remain untouched; tags above belong only to new messages. No user/password/profile DTO reuse, credential/session ID, expected version, mutation metadata or pagination. Names/tags are concrete planning choices; contract apply must inventory descriptors first and preserve these semantics if the sibling has changed meanwhile.

Domain inputs/result live near existing authorization contracts, with protobuf mapping only in handler. Response `version` is current target entity version, informational only: role-permission changes can alter this view without incrementing target user version, so it is not a cache validator or lasting access proof. Full results are returned without silent cap; existing catalog size and admission bound the initial MVP. Pagination would require a separately approved contract.

Alternative: reuse User/UserRoleAssignment messages. Rejected because they expose unrelated fields and conflate assignment history with unique permission sources. Separate response types per identical payload would duplicate mapping without a consumer need.

### 2. Two explicit snapshot functions, one local result assembly

Tambahkan ReadMyEffectivePermissionsSnapshot dan ReadUserEffectivePermissionsSnapshot di `internal/features/user`, mengikuti ReadUserDetailSnapshot existing. Masing-masing menerima request ctx, verified claims, domain input, ReadSnapshotProvider dan now time.Time melalui parameter. Provider berasal dari shared startup wiring; nilai now berasal dari injected clock handler. Tidak ada read-service struct/constructor, interface service, Dependencies/Bind bundle atau writer/event/entropy dependency baru.

Setelah BeginSnapshot, fungsi membuat State/Permissions/Users readers existing pada tx yang sama. Guard nil provider mengikuti catalog dependency error existing. Helper lokal hanya untuk safe target loading dan result assembly yang benar-benar dipakai kedua flow. Test memakai controlled snapshot provider/transaction dan strict SQL harness existing untuk membuktikan query/context/order/handle, tanpa binder atau exported test-only hook.

Self: validate filter/caller scope; get principal claims; BeginSnapshot(requestCtx); AuthenticateSession with tx reader; AuthorizeMethod exact self method; read target safe identity for that verified ID; read contributions; apply filter/caller visibility; ResolvePermissions; assemble result; Commit. No fabricated empty permission requirement and no input target override.

Admin: validate ID/filter/caller IAM and explicit filter scope; obtain actor claims; BeginSnapshot; AuthorizeCurrentPermission exact admin method/iam.user.read with tx-bound actor state/contributions; then read target via SessionUserReader; reject absent/soft-deleted; read target contributions; apply filter/caller visibility; ResolvePermissions; assemble/Commit. Even self-target uses the admin permission path. Jangan menambah run-read boolean flag atau callback framework untuk menyatukan kedua flow.

The result-bearing feature function defers TransactionOwner.Finish directly and clears result on any error/panic. Owner Commit remains explicit at feature call site. Perluas SessionReadHandler existing: validate nil input, ambil principal claims, map input, panggil fungsi snapshot sekali dengan ctx/provider/h.now(), lalu map successful result dan reject inconsistent output. Constructor NewSessionReadHandler tetap memakai dependency provider/clock existing. Repositories receive tx, never start/finalize lifecycle or fallback to pool. No concurrent queries on the handle.

Alternative: read service dengan constructor, narrow interface dan binder baru. Tidak dipilih karena E hanya membutuhkan dua read flow dengan dependency yang sudah tersedia pada handler; pola fungsi existing lebih sederhana dan tetap menyediakan snapshot ownership serta controlled testing.

### 3. Namespace filtering uses verified caller capabilities

Normalize application code by TrimSpace and reuse package-local application-code validation: lowercase grammar,1..50 for nonempty; blank means all permitted namespaces. Do not lowercase invalid input or treat malformed filter as empty.

Self requires service method authority and whichever namespace is explicitly requested; it does not introduce a blanket IAM namespace check for empty/business-only self reads. Admin additionally requires IAM service namespace independent of filter. For a nonempty code, call verified Caller.AuthorizeNamespace before any application/target lookup. Unknown code is empty only if caller was allowed that namespace; otherwise deny, concealing existence.

For blank filter, inspect each contribution's application namespace and include only rows for which Caller.AuthorizeNamespace succeeds. Caller need not expose/enumerate its private allowlist. Apply namespace filtering to returned target contributions only; admin authority is always resolved from full current actor rows with exact iam.user.read. Never accidentally filter actor authority to requested business namespace. After filtering, the existing resolver still validates application IDs/code prefix and access gates. No application catalog lookup is needed: allowed unknown/inactive namespaces naturally yield no eligible rows.

Alternative: return all applications based only on IAM read or reject blank filters. Rejected by the user's explicit namespace-limited optional-filter decision.

### 4. Inactive-target display preserves authentication semantics

Use the same resolver for target data regardless of target status. It already filters assignment/role/application/access but does not authenticate target account. Always return target `user_status`; for PENDING/SUSPENDED/DEACTIVATED the list describes retained ACTIVE-role configuration. This is the user's clarified meaning of historical contribution, not previous mappings, revoked history or pre-suspension snapshots. Soft-deleted targets remain NOT_FOUND. Do not require a session for the target or manufacture a principal for it.

This separates descriptive target data from current actor authorization: actor must remain ACTIVE/unrestricted; no changes to PrincipalForState, AuthorizeCurrentPermission, ResolvePermissions or protected-action gates. Target must_change_password is not an entitlement check in this admin display. Clients must not infer a usable access grant even from ACTIVE status, since credentials/restriction/resource policy remain independent.

Alternative: empty list for every nonactive target. Rejected by user decision. Including REVOKED/DISABLED sources or audit reconstruction was explicitly not selected.

### 5. Read-only snapshot lifecycle, errors and effects

BeginSnapshot uses existing primary PostgreSQL RepeatableRead ReadOnly provider; all actor/target/contribution reads share its handle and request ctx. Snapshot boundary is first consistent query after BeginSnapshot, not merely the interceptor or method invocation. Snapshot created after a revoke commit sees it; in-flight consistent reads may finish. Two physical connections/barriers test this precisely.

Neither E RPC calls TryLock, writes domain state or uses persistent idempotency. Existing mutation lock order/last-admin/discovery and ORM pg_try_advisory_xact_lock remain unchanged; they are not read prerequisites. Existing B/C/D mutation gates and last-admin regression remain mandatory without new E locks. Read commit/rollback ownership, cancellation/panic cleanup and leased pool return still require tests. Failed commit never returns a result or retries automatically.

| Outcome | Business/version/session effects | Audit/outbox/replay |
| --- | --- | --- |
| Success, including empty/inactive-target view | None | No success audit/outbox/saved result |
| Retry or response loss | Fresh authorized snapshot | No saved response or key requirement |
| Auth/validation/dependency/panic failure | No partial response; finish snapshot | Existing independent security attempt after cleanup; original error retained |
| Rate rejection | No business work | Bounded limiter telemetry; no per-rejection DB audit |

Reuse common.ErrBadRequest→INVALID_ARGUMENT; common.ErrForbidden→PERMISSION_DENIED; auth.ErrNotFound→NOT_FOUND; auth.ErrInvalidSession/ErrServiceAuthenticationFailed→UNAUTHENTICATED; common.ErrDependencyUnavailable→UNAVAILABLE; common.ErrRateLimited→RESOURCE_EXHAUSTED; common.ErrInternalServerError→INTERNAL. Preserve cancellation/deadline as CANCELED/DEADLINE_EXCEEDED and existing localized mapping/cause/redaction. No new error dictionary is presently necessary. No-op/replay/stale-version conflict/mutation rollback matrices are inapplicable to E reads; baseline mutation behavior is regression, not redesigned here.

### 6. Secure composition and numeric admission

Gunakan SessionReadHandler yang sudah dibuat oleh PrepareFoundation dengan primary provider dan injected clock existing; tambahkan explicit sessionUserServer delegates ke kedua method handler. Tidak ada prepare-service helper atau service/handler instance baru untuk E. Register UserService once; method count becomes 35 across existing seven services, baseline 33 exact names preserved. Add both full methods to protected SessionInterceptor classification and protected-read RateInterceptor group. Admin IAM check is required; self filter scope is enforced in feature. Preserve service exact-method allowlists and documentation/fixtures with only intended E additions, never wildcard enabling.

Retain quota 120/min burst 20 keyed by verified user/shared group, capacity 10,000, idle TTL 15m, cleanup 1m and bounded RetryInfo. Invalid handler inputs consume valid actor admission; unverified principals cannot allocate read keys. Available quota is not authorization. Existing security-attempt path covers failures without a new write in the read snapshot.

EdDSA-only, ApplicationName issuer, ignored audience, access<=15m capped by absolute 7d session, skew 30s without session extension, strict refresh rotation/reuse and Me/ChangePassword/Logout restricted allowlist are unchanged. Register remains PENDING/token-empty, TesRPC/pagination/cursors retained. No runtime key/config or consumer edit is needed for backend E; external IP sanitization and consumer readiness remain DEP-01.

## Risks / Trade-offs

- Nonactive target union mistaken for access → required user_status, documented descriptive semantics, tests that suspended credentials still fail and revoked sources stay excluded.
- Namespace filtering accidentally removes actor IAM authority or leaks other namespace → separate authority evaluation from output scope; explicit cross-namespace/missing-permission negatives and blank-filter tests.
- Fresh interceptor combined with stale/mixed target reads → recheck in one RR snapshot and two-backend controlled interleavings; next-read visibility tests.
- Full union grows → no silent truncation, shared admission and current MVP catalog limits; larger-scale pagination needs a future explicit contract.
- Contract checkout differs from release → pin published E version and GOWORK=off gates; no module-cache or local generated workaround.
- Uncommitted baseline overwritten → capture status/file digests before apply, preserve all existing/untracked hunks, review scope against that capture.

## Migration Plan

This invocation creates only artifacts. A new apply request is required. Contract work is an explicit EXTERNAL_REPOSITORY dependency: prepare source/generator/tests in api-contracts only when that repository scope is authorized; publication/commit/push only with explicit Git authorization. Record immutable release commit/module/checksum and standalone retrieval before transport completion. No dummy contract or partial unsecured registration if blocked.

Implementasikan dua fungsi snapshot, perluasan handler existing, registration dan guards bersama, lalu jalankan targeted/default/full workspace dan standalone test/vet/race/build gates. Run separate normal/race PostgreSQL suites through existing dedicated opt-in test configuration only; never Docker, application DB, automatic migration/seed or server startup as verification. Unavailable required gate is BLOCKED. Update API docs/Phase 2 E status from actual evidence; do not rewrite historical baseline claims as current PASS.

Rollout follows mandatory Phase 6 DEP-01 for live schema/admin/TLS/key/caller/consumer readiness; it is not an unfinished backend E task. Rollback removes only E hunks from recorded working-tree baseline, preserving B/C/D and EdDSA/session security; retain data/audit/outbox, never restore token-only bypass. No schema rollback is required. Sync/archive needs its own request after verified implementation.

## Acceptance Ledger

Planning baseline was NOT_STARTED. All backend ledger rows now PASS from actual implementation/gates; see [final evidence](evidence/progress.md). Owner grpc-auth except EPR-EXT-01. Each apply evidence entry records source commit or working-tree digest, published contract commit/module/checksum, command/mode, safe environment, timestamp, expected/actual, exit code and status. Prerequisites are B/C/D + foundation, E contract for transport, dedicated opt-in PostgreSQL configuration for DB gates. Unavailable prerequisites/gates are BLOCKED, not waived.

| ID | Scope / traceability | Required tests / evidence |
| --- | --- | --- |
| EPR-01 | MVP_REQUIRED; M-02/03/05, AC-31 subset | Self no-admin, admin exact read including self-target, caller TLS/method/IAM/namespace, invalid/expired/revoked/mismatched/restricted principal; feature and registered paths |
| EPR-02 | MVP_REQUIRED; AC-09/10/13/23 subset | Multi-role union/dedup/sort; all active gates/cross-app invalid rows; optional/unknown/inactive/malformed/disallowed filters; B/C/D revoke/replace/disable/regrant visibility with original JWT |
| EPR-03 | MVP_REQUIRED; user 14 Oct decision, M-05/06 | All target statuses, active-only retained sources, missing/deleted/ID limits, descriptive status/version and credential redaction; no target principal |
| EPR-04 | MVP_REQUIRED; M-02/09 | Strict begin→tx-bound actor→target→contributions→commit ordering/handle/ctx; begin/read/commit/rollback faults, cancel/deadline/panic; PG two-backend authority/target snapshot interleavings and pool cleanup |
| EPR-05 | MVP_REQUIRED; M-05/06/10/11 | Exact35 inventory and prior 33, shared read bucket/invalid-input/RetryInfo, locale/error cause/redaction, no business side effects and independent failed-attempt preservation |
| EPR-06 | MVP_REQUIRED; M-09/10/11 | Targeted tests; full workspace + GOWORK=off test/vet/race/build; dedicated PostgreSQL normal/race both modes; scoped diff/readability/local links/strict OpenSpec |
| EPR-EXT-01 | EXTERNAL_REPOSITORY; EXT-01; owner api-contracts | New descriptor/tags/source/generation/additive compatibility, error/locale tests, clean regeneration, full/static checks, authorized publication and standalone module resolution |

Mandatory backend commands in workspace and GOWORK=off: `go test ./...`, `go vet ./...`, `go test -race ./...`, `make build`. PostgreSQL commands in both modes: `go test -tags postgres -count=1 ./tests/integration/postgres/...` and the same with `-race`, using only existing private dedicated-test configuration. Add E-focused tests to root tests/ runner/test_scenarios/mocks/fixtures via testutils.Run. Two connections prove distinct backend IDs; barriers establish snapshots/commit ordering; no sleep-only evidence. Existing last-admin/refresh/idempotency mutation tests remain regression, not new E functionality.

# Design

## Context

Motivasi dan scope ada pada [proposal](proposal.md); contract behavior ada pada [delta spec](specs/user-application-access/spec.md). User menetapkan query lengkap, absent-access revoke tanpa insert, dan additive grant saat access ACTIVE. KISS/DRY/YAGNI, readability untuk developer junior dan complexity rendah berlaku sejak implementasi, mengikuti [AGENTS.md](../../../../AGENTS.md).

Baseline inspeksi 13 Oktober 2026: HEAD `2462cb09d0f8e7eef3b17a70456451b8bd2dd6ec`, branch `feat/role-management`, working tree A/B/C/refactor belum committed; pin api-contracts `v0.0.0-20261012102712-bfb073390ea9`, ORM1.7.0 dan Go1.26.2. Baseline apply wajib working tree, termasuk untracked files, bukan HEAD saja.

Graph dibangun pada `bf6be877296b8f4191f2e2db5f744b23ab636ed7`. AST EXTRACTED menempatkan foundation pada composition dan domain sebagai shared hub; degree auth/idempotency/outbox/user-role/transaction adalah236/228/222/217/214. Graph belum mencakup C dan refactor terbaru, sehingga routing diverifikasi melalui source. Tidak ada AGENTS lebih dekat atau graph-instruction.md.

| Source yang diperiksa | Temuan / pemakaian |
| --- | --- |
| features/user/assignment_transaction.go dan assignment_mutation.go | Canonical locks, feature-owned Finish, replay sebelum target version, direct commit; pakai pattern lokal, jangan memanggil RPC assignment sebagai nested command |
| features/common/discovery.go dan auth_feature/read_permission.go | Algoritma bounded discovery tersedia; helper permission masih menerima satu permission |
| repositories/domain/idempotency.go, audit.go, snapshot.go, outbox.go | ApplicationID dan changed IDs sudah dapat direpresentasikan; validator C menganggap changed iff roles berubah dan max20 sehingga tidak dapat dipakai D |
| repositories/reader/writer/user_role_assignment.go | Pair activation/revocation/version writer reusable; selected-role reader dibatasi20, tidak sesuai untuk seluruh role revoke aplikasi |
| repositories/domain/catalog.go, catalog_cursor.go dan reader/catalog.go | Query engine reusable tetapi identity/tie-breaker masih id; D memakai application_id dalam satu target user |
| configs/foundation_features.go, roles.go, register.service.go dan interceptors | Satu provider, thin delegation dan explicit admission; ApplicationService kini hanya ListApplications, total30 methods |
| api-contracts proto application/user/paginator dan catalog errors | Tiga RPC D belum ada; PageQuery/PageMeta dan application/user/role errors relevan sudah tersedia |
| SQL backfill dan PostgreSQL fixture/schema.sql | Access PK(user_id,application_id), provenance, ACTIVE/REVOKED. Owned fixture belum mempunyai revocation check lengkap seperti DDL; B metadata mencakup lima tabel lain, tidak access |
| tests/features/user, tests/repositories/assignment, tests/integration/auth_reads, tests/integration/postgres | Strict sequence/context mocks, registered production harness, owned schema dan two-connection barriers dapat diperluas tanpa framework baru |

Beberapa paragraf AGENTS/context/heading Phase2 masih menggambarkan baseline token-only atau C belum dimulai. Normative main specs dan keputusan user yang lebih baru mengatur PENDING/token-empty, EdDSA, runtime30 methods dan C selesai. Tidak ada perubahan runtime kembali ke legacy; historical docs/specs tidak ditulis ulang oleh proposal ini.

## Goals / Non-Goals

**Goals:** satu outcome untuk access + roles + entity version + evidence; current AND authorization; query history yang aman; backend33 methods dengan baseline kompatibel; orchestration lokal mudah dibaca dan regression gates yang terukur.

**Non-Goals:** transaksi generik baru, policy registry, universal command runner, option bags/reflection, import generated protobuf ke business layer, perubahan konfigurasi/dependency selain published contract D, migration aplikasi otomatis, bulk API tambahan atau refactor area yang sudah fokus. E/Phase4/REST/UI/DEP-01 dan POST_MVP tetap terpisah.

## Decisions

### 1. Contract additive dengan source ownership yang jelas

api-contracts menambah tepat satu method UserService dan dua method ApplicationService sesuai delta. Existing protobuf tags/types/services dan default response semantics tetap. Paginator tidak diubah; optional expected_user_version membedakan absent dari zero. Baru generate setelah source proto diperbarui; backend menunggu published dependency yang dapat diambil dengan GOWORK=off. Tidak ada local proto/pb, manual generated edit atau module-cache patch.

Rancangan messages baru berikut mengunci field names/types dan tag baru; semuanya berada di message baru, tanpa reuse tag existing:

| Message / owner proto | New fields dan tags |
| --- | --- |
| UserApplicationAccessItem / user.payload.proto | application_id uint64=1; application_code string=2; name string=3; application_status string=4; is_system bool=5; status string=6; optional granted_by uint64=7; granted_at Timestamp=8; optional revoked_by uint64=9; revoked_at Timestamp=10; optional revoke_reason string=11 |
| UserApplicationAccessFilter / user.payload.proto | application_code string=1; statuses repeated string=2 |
| ListUserApplicationAccessRequest / user.payload.proto | user_id uint64=1; query PageQuery=2; filter UserApplicationAccessFilter=3 |
| ListUserApplicationAccessResponse / user.payload.proto | user_id uint64=1; version uint64=2; items repeated UserApplicationAccessItem=3; meta PageMeta=4; next_cursor string=5 |
| GrantApplicationAccessRequest / application.payload.proto | user_id uint64=1; application_code string=2; role_ids repeated uint64=3; reason string=4; optional expected_user_version uint64=5 |
| RevokeApplicationAccessRequest / application.payload.proto | user_id uint64=1; application_code string=2; reason string=3; optional expected_user_version uint64=4 |
| GrantApplicationAccessResponse and RevokeApplicationAccessResponse / application.payload.proto | user_id uint64=1; application_id uint64=2; optional access_status string=3; changed_role_ids repeated uint64=4; version uint64=5; changed bool=6 |

granted_at is nonnull in inspected DDL; source IDs and revoked metadata retain nullable presence. An absent access pair in revoke maps to absent access_status, including changed=true role cleanup. No synthetic REVOKED history, ID alias or role count is introduced. Safe scalar response avoids importing User (which contains a password field) or creating a cross-proto dependency cycle.

Reuse errors: common bad request/forbidden/dependency; auth not-found/invalid-session; user grant-target-inactive/version-conflict/idempotency-conflict/resource-busy; role role-not-found/disabled; application application-not-found/inactive/system-application-protected. Wrong-application roles are INVALID_ARGUMENT, not invalid-permission-set or a new error. Verify existing catalogs, EN/ID messages and gRPC mappings; add catalog only if an actual uncovered semantic is established during contract review. No global legacy remapping.

Alternative embedding an existing User or inventing access surrogate ID is rejected because it adds unsafe fields or mismatches schema. Full application/permission CRUD remains out of scope.

### 2. One application feature and concrete adapter responsibilities

Implement a focused service in `internal/features/application/`, with impl.go limited to interface/type/constructor/binding. Separate list, grant, revoke and transaction orchestration by intent. UserService.ListUserApplicationAccess and ApplicationService mutations delegate to thin handlers for the same feature; composition can retain catalogServer and add the access handler without another service registry or connection.

New domain access contracts/models live in `repositories/domain/application_access.go`, adapters in reader/writer/application_access.go. Reuse SessionStateReader, PermissionReader, SessionUserReader, the existing application catalog lookup, UserRoleAssignmentWriter, IdempotencyStore, AuditWriter and OutboxWriter through small consumer-facing interfaces. Add only concrete reads for stored access, all target-application assignments and history page. Grant can reuse bounded AssignmentRoles; revoke uses a separate unbounded-by20 application-assignment query ordered by role ID. Validate row target/application/role identity and state; malformed or inconsistent repository results fail closed before writes.

The access writer handles pair insert/reactivation/revoke with affected-row checks. It never begins/commits/rolls back/locks. Existing role writer handles role pair transitions and the conditional version update; no bulk status flip without verifying changed IDs. No call to GrantUserRoles/RevokeUserRoles feature methods: those have different auth, key scope, access preconditions and evidence and would create nested transactions or duplicate events.

Functions follow preparation → locked proof/validation → explicit mutations → evidence/result. Extract only a clear business/query stage or real reuse; avoid wrappers that obscure a single statement, deep callback chains and boolean mode flags. Grant/revoke loops stay separate. A local transaction/replay helper follows existing C pattern; it is not a reusable framework for unrelated commands.

### 3. Current AND permission evaluation and shared trust boundary

| Exact new method | User authority | Admission |
| --- | --- | --- |
| /user.v1.UserService/ListUserApplicationAccess | iam.user.read | ProtectedRead120/min burst20 |
| /application.v1.ApplicationService/GrantApplicationAccess | iam.user.application.grant AND iam.user.role.grant | AdminMutation30/min burst5 |
| /application.v1.ApplicationService/RevokeApplicationAccess | iam.user.application.revoke AND iam.user.role.revoke | AdminMutation30/min burst5 |

All require TLS and verified x-service-token with exact method + IAM namespace allowlists, ACTIVE nonrestricted access principal and current primary state. Target application namespace is not a replacement for IAM administration namespace. Browser actor IDs/role codes/service identity are never authority. Missing either permission denies. Extend the existing permission helper with a small plural form that resolves state/permission rows once per evaluation and tests every requested exact permission; keep the existing singular API delegating to it so baseline callers keep semantics. No configurable policy engine or second permission read per individual permission.

List uses one BeginSnapshot(requestCtx) for authority + target + count/page. Mutations verify current authority again after locks, before retained result or target validation. Pre-lock routing reads described below are not final authorization. Dependency errors preserve cause/mapping, and cancellation/deadline never masquerade as forbidden/busy/empty data. Preserve Language → Error → Service → Attempt → Session → Rate. Add explicit namespace/protected-method/mutation/failure-audit classification for all three RPCs.

Limiter groups remain shared across baseline/new methods, including applicable invalid payload admission. Keep cap10000, idleTTL15m, cleanup1m, atomic admission and structured RetryInfo; no new buckets or quota values. Trusted rest-orc x-client-ip sanitation/fallback service identity remains existing external trust contract, without new reliance by protected per-user quota.

### 4. Canonical numeric scope before lock acquisition

Application code must resolve to numeric ID before building the canonical idempotency lock. Start a feature-owned transaction on the injected primary provider with request context. Bind all readers/writers immediately. Within that same handle, verify current actor/AND permissions and resolve normalized application code as **routing only**. No mutation, result replay or target-user disclosure occurs here. This early authority check prevents missing-application disclosure to an unauthorized caller. Then build canonical scope/hash and acquire locks; after discovery stabilizes, re-read authoritative actor/app/target state under locks before replay/fresh execution. The application ID/code must still match the resolved identity; fail closed if not, without changing scope mid-command.

This keeps routing, all current checks and writes on one bound handle, and avoids an extra pool lookup/snapshot or code-based duplicate idempotency scope. The inspected TransactionProvider.Begin uses READ COMMITTED, allowing locked reads to see commits after preliminary routing; BeginSnapshot uses READ ONLY REPEATABLE READ for lists. Neither isolation mode is changed. It adds one deliberate preliminary authority evaluation; the locked evaluation is mandatory, not cached. Application identity/code is not editable via D and uniqueness/FKs remain enforcement against bypass writers. A changed application identity on restart aborts rather than switching retained scope. No retry of a mutation or uncertain commit is added.

Feature retains direct `defer owner.Finish(&err)` at the transaction owner; nested wrapper defer would break recover semantics. Begin/Commit/Rollback/TryLock orchestration call sites remain features/common or feature; technical Db.BeginTx/ORM implementation remains infrastructure. Commit succeeds before response. Commit failure is not a reason to rerun writes automatically.

### 5. Ordered discovery reuses the existing bounded algorithm

Scope order remains idem → last-admin/policy → account/role-code → sorted numeric users → roles → applications → sessions. D needs policy global guard, actor/target user locks, selected/all target application role locks, actor contributing role/application locks, target application and actor session. Canonical key builders deduplicate actor=target and input order. Revoke discovery covers **every ACTIVE stored target-application assignment**, not a paginated first20. Selected grant role mapping and actor contributions are re-read for stable identity/application lock coverage.

Use common.LockDiscoveredAuthority on this bound transaction. Preserve max3 transaction attempts and max3 inner discovery/rechecks. New predecessor returns enlarged keys, direct Finish rolls back, then a new bounded transaction resolves routing again before acquisition. No target writes, replay or evidence precede stable locks/current authority. Nonpredecessor keys may be acquired in canonical order on the same owner. acquired=false maps user RESOURCE_BUSY/ABORTED; DB failures remain dependency/cancellation with cause.

Actual ORM1.7.0 PostgreSQL dialect uses `SELECT pg_try_advisory_xact_lock($1)`; the transaction adapter uses its current handle, retained until commit/rollback. SQL uniqueness/constraints remain necessary. Shared policy/user/role/application keys coordinate with C, role management, status/profile/admin-session commands. Since IAM and all system applications are protected, D cannot remove platform-admin assignment and does not need a new last-admin guard/count. Verify system app and cross-app role rejection plus baseline last-admin regression rather than weakening C/status invariants or adding an unrelated global guard.

Alternative copying C discovery loops is rejected; a generic command runner is also rejected because lifecycle, routing and compound behavior should remain visible.

### 6. Explicit compound mutation and persistence matrix

Fresh request order after current authority is retained replay → target exists/version → application protection/status and role eligibility → no-op/effective execution → required evidence/result → commit. Structural input validation occurs before Begin. Replays do not rerun fresh target/version/eligibility or mutation; canonical app resolution and current authority remain required.

| Command / case | Access and assignment state | User counters | Required evidence |
| --- | --- | --- | --- |
| Grant effective | Activate/create access if needed; activate/create selected eligible pairs only; unselected ACTIVE/REVOKED remain | version+1 once, auth unchanged | APPLICATION_ACCESS_GRANTED + exactly one application_access_granted event + safe result |
| Revoke effective | Revoke ACTIVE access if present and all ACTIVE pairs of application; absent access never inserted | version+1 once, auth unchanged | APPLICATION_ACCESS_REVOKED + exactly one application_access_revoked event + safe result |
| Fresh full no-op | Existing eligible selected pairs/access already active or no ACTIVE revoke state; provenance untouched | unchanged | one NO_OP audit + changed=false safe result; no event |
| Identical unexpired replay | No writes | unchanged | original safe result only |

Grant validates the entire selected set before access write. Active pair provenance remains unchanged; reactivated pair retains PK and clears revoked_by/at/reason. Revoke retains original grant provenance, changes only ACTIVE rows and has no20-row truncate/chunked commits. A full target application can have more20 roles from multiple grants. Version overflow/zero-row or unexpected multi-row conditional results fail safely without partial commit. Access-only changed=true with empty changed_role_ids is valid; role-only cleanup with absent access is also valid.

D-specific safe-result validation checks exact target/application IDs, version, sorted unique actual changed role IDs, optional access state and command shape, rather than C's changed iff roles/nonempty/max20 rule. Reuse SafeCommandResult.ApplicationID/Status/ChangedRoleIDs; Status empty means absent access for D. Reject malformed/cross-target retained results, unrelated safe fields and invalid ID/status/version. Extend only this validation/typed projection, leaving C and existing stored result formats intact.

Audit target remains user/{id} through existing target fields, safe before/after includes application ID/access state, version and actual role transitions, with bounded free-form reason in audit only. Add typed APPLICATION_ACCESS_GRANTED/REVOKED audit actions and exact permissions where absent; event types/envelope/aggregate mapping already exist. One event uses aggregate_type=user, aggregate_id target decimal string, committed user version, application_id/code, status ACTIVE/REVOKED, actual changed_role_ids and controlled reason code when used. In role-only absent-access cleanup, event status REVOKED describes the access operation's denied outcome, not a fabricated storage row; response access_status remains absent. Preserve schema_version1, UTC, producer, actor, correlation/causation/traceparent and pending published_at=NULL. No separate roles_granted/revoked events for D, broker call or credential material.

Any required read, row transition, version/audit/outbox/idempotency write failure, cancellation or panic before commit rolls back business state together. Existing Attempt interceptor writes independent safe failure audit after owner cleanup; its failure preserves original error and bounded telemetry. Unknown commit outcome does not claim success or certain rollback. Existing refresh reuse remains explicit commit-before-denial and is not routed through D.

### 7. Replay scope and immutable safe result

Use actor + full method + `user/{id}/application/{application_id}` + exact metadata key. Normalize code/reason, validate raw role count/IDs then canonicalize role set; hash numeric application ID, reason, expected version/presence and selected set. Different target app/user/method is independent; correlation/deadline/service ID do not alter scope. Key is exactly one nonempty value <=200 characters. Existing Replay/IdempotencyStore retain24h and atomic expired replacement.

Save only user/application IDs, optional access state, actual changed IDs, version and changed; return the original stored result on replay even after later target changes. Replay is checked before fresh business state/no-op but after locked current actor and two permissions. No password/hash/token needed; general credential replay is not added. No-op with absent access stores empty status and returns absent optional access_status without inserting a row. Actor privilege change or stale session denies replay. Busy and idempotency conflict remain separate.

### 8. Query identity is application_id inside a fixed target

Add UserApplicationAccessCatalog with its explicit eleven-field public registry. A small catalog identity selector returns application_id for D and id for existing catalogs; use it in identity projection, count expression, default/tie sort and numeric cursor handling. Extend target validation to D, and select application_code/name as its default search fields without changing old catalog defaults. Preserve old registry fields, existing query hashes/cursor encoding and validation behavior; old serialized B/C/user cursors become mandatory regression fixtures. Do not add a surrogate ID column, expose a fake id alias or duplicate the normalization/keyset engine.

Access page is anchored on user_application_access filtered by target, joined once to applications. Count unique application_id within that target, before cursor/limit; no role/permission join is needed. Search application_code/name with literal ILIKE escaping and C collation/typed SQL ordering consistent with existing catalog behavior. Request fields map only through approved SQL expressions. All eleven scalars support ASC/DESC; granted_by/revoked_by/revoked_at/revoke_reason retain nulls-last and typed cursor values. granted_at follows its nonnull DDL. Status filters are access ACTIVE/REVOKED, application_status is separate metadata.

Projection always includes application_id; hidden sort fields remain available before mapping. Opaque cursor binds target/method/filter/search/sort; numeric application-id-only mode and offset fallback are explicit. Keep generic reader's existing count/page stages rather than adding a query builder abstraction. Changes to shared normalization must not accidentally change old query identity, clone inputs or interpret D missing access as catalog entries.

### 9. Registration and deployment preserve the secure baseline

Add application service/handler construction to Foundation following existing prepare* functions, using the same provider, injected Now/EventID and synchronized entropy. User server delegates list; application server delegates grant/revoke and keeps catalog ListApplications. Update exact inventories, service allowlist examples, namespace/protected/rate/attempt classification and controlled fixture permissions together; no admin method is enabled with incomplete guards. Production must register33 methods across the same seven services.

Retain EdDSA-only issuer/verifier, issuer ApplicationName, ignored audience, access<=15m capped by session expiry, absolute7d session/refresh, skew30s without session extension, strict refresh single-use/no grace and restricted Me/ChangePassword/Logout allowlist. D changes no key or runtime config. Register remains PENDING/token-empty with external client readiness tracked in DEP-01; only Login/Refresh return credentials. Me/GetUser/ListUsers defaults and session lifecycle remain compatible, including generic Login denial and TesRPC response.

## Risks / Trade-offs

- **Partial access or omitted roles on revoke** → validate full set before write, no20 bound on discovered revoke set, same-handle conditional transitions, one commit and real PostgreSQL fault injection including more20 roles.
- **AND check passes with one permission or stale interceptor** → plural exact check reads one contribution set per evaluation, locked recheck before replay, each single-permission negative and privilege-race tests.
- **Canonical app routing races or wrong replay target** → preliminary authorized bound routing, numeric scope, identity recheck, strict D safe result and cross-user/app/method tests.
- **Shared catalog identity breaks old cursors** → one identity distinction without encoding changes, pre-refactor serialized fixtures plus old B/C full query regressions and every D scalar/tie/null pagination test.
- **Helpers obscure flow** → local business stages, explicit owners/grant/revoke, no framework/options registry; readability review must justify every helper/dependency without arbitrary LOC thresholds.
- **Owned fixture differs from application schema** → inspect agreed DDL; extend only owned fixture missing revocation constraints/indexes necessary for D tests. Update affected synthetic status-only fixture statements to supply valid revoke metadata, preserving their business assertions. Verify matching types/PK/FKs/checks on dedicated DB and preserve baseline table invariants. B live metadata excludes access; no claim of live access parity. Actual schema compatibility remains DEP-01 and any newly discovered business/schema mismatch must be resolved before affected implementation.
- **Uncommitted baseline overwritten** → capture nonignored file hashes/status before apply, compare own diff with that baseline including C/refactor, never reset/stash/commit unrelated work.

## Migration Plan

Apply requires a new explicit request. Prepare contract source/generated/tests in api-contracts within the approved D scope; publish only under valid Git authorization, record immutable commit/module/checksum and confirm standalone retrieval. Then update grpc-auth's contract dependency, implement secured feature/adapters/handlers and wire all guards together. No fake method/dependency fallback. Contract publication unavailable blocks dependent tasks; source/mocks alone cannot mark release verification PASS.

No application migration/backfill/seed, environment change, new key, consumer edit or deployment is performed by proposal/apply automatically. Dedicated tests use existing private env-test.yaml and owned schemas only. Actual rollout follows DEP-01 for live schema/constraints/indexes/admin/key/TLS/caller/consumer and rollback/cutover; it is not an unfinished backend D task.

Source rollback removes only D hunks or returns to the recorded session-aware/EdDSA baseline while preserving C/refactor. If access changes have already committed, rollback must retain stored histories/audit/outbox/version and deny unsafe fallback; no reset to token-only auth or deletion of production data. Removal of a method does not undo an access grant. Publication/deployment rollback requires its own authorized procedure.

## Acceptance ledger

Planning baseline was **NOT_STARTED**. Actual implementation is COMPLETE with all backend ledger rows PASS; see [final evidence](evidence/progress.md). Backend owner grpc-auth unless contract row says otherwise. Prerequisites: baseline C/refactor, contract D for transport work and dedicated test configuration for PostgreSQL. Evidence during apply must include final source identity, contract commit/module, command/mode/time/exit, expected/actual scenario result, strict order/handle assertions and real DB results. No planning/source inspection PASS.

| ID | Scope / main traceability | Required tests and evidence |
| --- | --- | --- |
| AAP-01 | Current AND authority; AC-08/24–26/31 | All service/principal/restricted failures, each missing permission, no disclosure and locked stale actor/session/permission; feature + registered gRPC + PG races |
| AAP-02 | Safe history/snapshot; AC-11/12/31, M-05 | ACTIVE/REVOKED/disabled/system rows, all account statuses, missing/deleted/empty, nullable sources, count/page/dependency and concurrent snapshot |
| AAP-03 | Query identity/compatibility; M-05/06/10/11 | All11 scalars ASC/DESC, mixed ties/nulls, literal search/projection/filters, target-bound/numeric/opaque/offset/terminal cursors; SQL + PG + old serialized cursor fixtures |
| AAP-04 | Initial/additive grant; AC-08/12/24–26 | Access+selected roles atomic, active provenance/identity, regrant subset, access-only changes, raw20 bound, cross-app/disabled/unknown/inactive input and version |
| AAP-05 | Complete revoke; AC-11/12/24–26 | Access+all roles including >20, absent/revoked cleanup and no fabricated row, inactive targets/roles/apps, other apps untouched |
| AAP-06 | System protection; AC-19/20 regression | iam/is_system and cross-app role attacks, no platform-admin loss; baseline last-admin cross-command races remain PASS |
| AAP-07 | Version/no-op/replay; AC-24–26, M-07/08 | Fresh stale no-op conflict, one version per compound, optional access status, role-only/access-only result, safe original replay/expiry24h/conflict/different target/response loss |
| AAP-08 | Atomic evidence; AC-08/11/24–26, M-07/09 | Every access/assignment/version/audit/outbox/result fault, controlled affected rows, one catalog event/no role duplicates, safe envelope/audit and rollback assertions |
| AAP-09 | Concurrency/lifecycle; AC-24–26, M-08/10 | Same pair uniqueness, D-D/C/status/role-management overlap, predecessor restart and3x3 bounds, two physical connections/barriers, panic/cancel/commit/independent-audit failure, lock/pool cleanup |
| AAP-10 | Credentials/current contribution; AC-13 subset, M-02/05 | Original JWT remains valid, primary resolver changes after revoke/subset regrant, overlapping/other-app contributions remain, password/auth/session/refresh unchanged; public EvaluateAccess/E and CLM excluded |
| AAP-11 | Admission/registration/compatibility; AC-31, M-01..04/10/11 | Exact33/all30 preserved, shared quotas120/20 and30/5 including invalid input, RetryInfo/locale/redaction, startup/entropy/provider and full session regression |
| AAP-12 | Gates/readability/evidence; M-10/11 | gofmt/targeted/full/vet/race/build workspace and GOWORK=off; full PG normal/race both modes; scoped diff, helper justification, strict OpenSpec/local links and source identity |
| AAP-EXT-01 | EXTERNAL_REPOSITORY api-contracts | Additive descriptor/tag/presence/error/locale/generator tests, publication commit/module/checksum and standalone resolution; unavailability blocks backend transport/publication tasks |

Mandatory gates on both workspace and GOWORK=off: `go test ./...`, `go vet ./...`, `go test -race ./...`, `make build`. Separate `go test -tags postgres -count=1 ./tests/integration/postgres/...` and `-race` use only dedicated DB through privately loaded env-test.yaml, never Docker/application DB/migration. Two physical connections and barriers prove exclusion/visibility; sleeps alone are insufficient. Unavailable gates are BLOCKED, not skip/PASS. Contract uses its generator and targeted/full tests/static checks with clean regeneration review. DEP-01 stays open separately and no completion claim includes actual deployment readiness.

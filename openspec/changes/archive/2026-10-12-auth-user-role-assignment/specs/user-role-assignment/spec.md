# Spec Delta

## Purpose

Menyediakan pembacaan dan perubahan assignment role user yang terotorisasi,
atomik, dapat diulang dengan aman, serta menjaga admin aktif terakhir tanpa
membatalkan sesi ketika kontribusi permission berubah.

## ADDED Requirements

### Requirement: Assignment methods require exact current verified authority

`/user.v1.UserService/ListUserRoles`, `/user.v1.UserService/GrantUserRoles` and
`/user.v1.UserService/RevokeUserRoles` SHALL respectively require
`iam.user.read`, `iam.user.role.grant` and `iam.user.role.revoke`. Every method
SHALL require TLS, a verified method/namespace-allowed service and an ACTIVE
nonrestricted verified user/session. Actor SHALL come only from verified
principal; target user_id SHALL be separately validated as positive
PostgreSQL INTEGER-compatible. Current authority SHALL precede target-existence
disclosure and retained-result replay. Numeric role codes, role names, supplied
actor identities and earlier authorization SHALL NOT bypass exact permission.
Dependency failure SHALL remain UNAVAILABLE; cancellation/deadline SHALL retain
their mapping and SHALL NOT produce partial successful data or writes.

#### Scenario: A valid actor lacks the exact mutation permission
- **WHEN** a caller with iam.user.update or iam.user.role.grant invokes RevokeUserRoles without iam.user.role.revoke
- **THEN** PERMISSION_DENIED returns without target detail, retained result or business mutation

#### Scenario: Service or principal is invalid or restricted
- **WHEN** a method/namespace-disallowed service, invalid session or restricted actor calls any assignment method
- **THEN** the applicable authentication/permission error occurs before successful assignment disclosure or effects

#### Scenario: Actor authority changes before mutation recheck
- **WHEN** actor status, session, assignment or contributing role permission changes after initial verification and before locked recheck
- **THEN** the command uses current authority and denies stale mutation or replay without success effects

### Requirement: Assignment listing exposes safe historical state from one primary snapshot

ListUserRoles SHALL read authority, target existence/version, assignments,
role/application metadata, application-access state, total and page on one
consistent primary snapshot. Existing nondeleted targets in any account status
SHALL be readable; authorized absent/deleted targets SHALL return NOT_FOUND.
Default items SHALL include ACTIVE and REVOKED assignments, including assignments
to DISABLED or system/legacy IAM roles and disabled applications. A never-assigned
role SHALL not appear. Safe items SHALL expose assignment id, role_id, string
code, numeric role_code, name, assignment status, role_status, application_id,
application_code, application_status, is_system, nullable application_access_status,
granted_by/granted_at/revoked_by/revoked_at/revoke_reason. Nullable legacy metadata
SHALL remain absent. Actor source IDs SHALL not expand into personal profiles or
credentials. The response SHALL include target user_id and current entity version.
Listing SHALL NOT assert that a listed assignment gives effective permission,
mutate a version, emit a domain event, or change existing Me/GetUser/ListUsers
collection defaults.

#### Scenario: A suspended target retains revoked and disabled role assignments
- **WHEN** an authorized administrator lists a nondeleted SUSPENDED target with ACTIVE and REVOKED assignments, including disabled/system roles
- **THEN** both assignment states and safe source metadata are returned independently of effective permission, with nullable legacy fields preserved

#### Scenario: Target exists with no assignments or is absent
- **WHEN** an authorized caller lists an existing empty target or a nonexistent/deleted target
- **THEN** only the existing target returns an empty page with total/total_pages zero and current version; the absent/deleted target returns NOT_FOUND

#### Scenario: A concurrent commit changes authority and assignment data
- **WHEN** another connection commits changes between authority, target, total and page reads
- **THEN** all returned state describes one primary snapshot and a new evaluation observes committed current policy

#### Scenario: Required read fails after some data was loaded
- **WHEN** an authority, target, total, page or source-metadata query fails or the request is canceled
- **THEN** the appropriate dependency/cancellation error returns without a partial list or empty-success substitution

### Requirement: Assignment queries support safe filters search projection and scalar multi sort

ListUserRoles SHALL accept PageQuery/PageMeta with page default1, limit default20
and max100, next_cursor, application_code and repeated assignment statuses
ACTIVE/REVOKED. Status values SHALL combine OR; categories and keyword SHALL
combine AND. Unknown statuses/fields/modes SHALL return INVALID_ARGUMENT. Search
SHALL use case-insensitive literal CONTAINS or PREFIX, default CONTAINS, trimmed
keyword at most200 characters, with code/name/application_code fields and default
code/name. Selected search fields SHALL combine OR. Percent and underscore SHALL
remain literal. Each public item scalar
SHALL support ASC/DESC multi-sort through an explicit allowlist; request order
SHALL be retained, duplicate/unknown sort fields rejected, and id ASC appended
when absent. Default sort SHALL be id ASC and nullable values SHALL sort last
in either direction. Projection SHALL allow only public item fields, default
to all, always include id/role_id, and retain hidden sort values for cursors.
Total SHALL count unique matching assignments before cursor/limit, unaffected
by permission mapping multiplicity. Cursor SHALL carry typed sort values/nulls
and bind to full method, target user and normalized filters/search/sort.
Malformed, cross-user, cross-method and mismatched cursors SHALL be rejected.
ID-only mode SHALL also accept positive numeric last_id. Keyset continuation
SHALL be deterministic on unchanged state; absent cursor with page>1 SHALL use
the deterministic requested page. No snapshot across separate requests is promised.

#### Scenario: Application and status filters combine with literal search
- **WHEN** a caller selects application clm, ACTIVE/REVOKED statuses and a name keyword containing percent or underscore
- **THEN** matching assignments satisfy the combined filters with literal search and unique pre-cursor total

#### Scenario: All public scalars and mixed nullable ties are sorted
- **WHEN** either direction of each public scalar or a mixed multi-sort with tied/null source fields is requested
- **THEN** ordering uses the approved public expression, nulls last and ID tie-breaking, with deterministic keyset continuation

#### Scenario: Cursor is reused for another target or query
- **WHEN** an opaque cursor is malformed or reused with another user, method, filter, search or sort
- **THEN** INVALID_ARGUMENT returns without silently restarting or accepting caller SQL

#### Scenario: Projection hides ordering values or the final page is empty
- **WHEN** projection omits sort fields, numeric ID-only pagination is used or the page reaches the end
- **THEN** id/role_id remain, hidden typed values sustain correct cursors, total remains pre-cursor and terminal next_cursor is empty

### Requirement: Grant activates an eligible role set without creating application access

GrantUserRoles SHALL require a nonempty role_ids input of at most20 entries,
deduplicate and sort valid positive INTEGER-compatible IDs as a set, trimmed
reason 3–500 characters, present positive int64-compatible expected_user_version,
and exactly one valid nonempty idempotency-key metadata value at most200
characters. A new command SHALL compare the current target entity version before
business/no-op decisions. Target SHALL be nondeleted ACTIVE, each role and its
application ACTIVE, and business application access ACTIVE for the target.
IAM role assignment SHALL not require IAM application access; system-role metadata
protection SHALL not forbid otherwise authorized assignment. Unknown roles or any
invalid entry SHALL fail the whole batch. Grant SHALL create one pair or reactivate
its existing REVOKED row, preserving pair identity, recording current grant source
and clearing obsolete revocation metadata. Existing eligible ACTIVE pairs SHALL
remain unchanged. Grant SHALL NOT create application-access records or assign
roles omitted from the request. changed_role_ids SHALL be sorted unique actual
transitions; any effective batch SHALL increment user version once.

#### Scenario: Several roles are granted including a revoked pair
- **WHEN** an authorized request with current version contains eligible new, revoked and already-active role pairs
- **THEN** exactly the new/revoked pairs become ACTIVE, revoked pair IDs are reused, unchanged pair provenance remains and user version advances once

#### Scenario: One role fails eligibility
- **WHEN** one requested role is unknown/disabled, its application is disabled, business app access is absent/revoked or the target is not ACTIVE
- **THEN** the documented NOT_FOUND/FAILED_PRECONDITION error rejects the entire batch without partial assignment or successful persistence

#### Scenario: Administrator assigns a system IAM role
- **WHEN** an actor with exact grant permission assigns an ACTIVE registered IAM role to an ACTIVE target without IAM application access
- **THEN** assignment is allowed under ordinary batch/version rules without editing the protected role or creating app access

#### Scenario: Batch input or expected version is invalid
- **WHEN** role_ids is empty, has more than20 submitted entries, contains invalid IDs, reason/key/presence is invalid or a new request has stale version
- **THEN** validation or VERSION_CONFLICT/ABORTED returns before successful effects, including stale requests that otherwise appear to be no-ops

#### Scenario: Concurrent requests grant the same pair
- **WHEN** two physical connections grant the same target role using identical or different scoped keys
- **THEN** ordered exclusion and unique-pair enforcement prevent duplicate assignments or batch events, and retries follow original replay or current version/no-op rules

### Requirement: Revoke changes only requested existing active assignments

RevokeUserRoles SHALL use the same batch/reason/version/key input contract as
GrantUserRoles and SHALL allow nondeleted targets in any account status, regardless
of target role/application/access status. Every requested role SHALL be registered;
an unknown role SHALL fail the whole batch. ACTIVE requested pairs SHALL become
REVOKED with current actor/time/reason while keeping their grant provenance.
Already REVOKED or never-assigned registered pairs SHALL be no-ops; absent pairs
SHALL NOT be inserted and prior revoked metadata SHALL NOT be rewritten by no-op.
Other assignments and all application-access records SHALL remain unchanged.
changed_role_ids SHALL contain only sorted unique actual transitions; user version
SHALL advance once for any effective batch and remain unchanged for a valid full
no-op. A new stale request SHALL conflict before no-op.

#### Scenario: Active revoked and absent pairs are requested together
- **WHEN** a valid batch references registered roles with ACTIVE, REVOKED and never-created pairs
- **THEN** only ACTIVE pairs are revoked, absent rows are not inserted, prior revoked metadata stays and the batch has one version increment

#### Scenario: Revoke cleans up an inactive target and disabled role
- **WHEN** an authorized caller revokes an assignment from a PENDING/SUSPENDED/DEACTIVATED target or to a disabled role/application
- **THEN** ordinary revoke/version rules apply without needing business application access or reactivating anything

#### Scenario: Unknown role is mixed with valid assignments
- **WHEN** one requested role ID does not exist even though other requested roles have ACTIVE assignments
- **THEN** NOT_FOUND rejects the entire batch without partial revoke or successful audit/event/result

### Requirement: Shared protection retains an active administrator under concurrent removal

RevokeUserRoles SHALL reject self revoke of iam.platform_admin, including an
otherwise nominal no-op. A command that would remove the last ACTIVE nondeleted
user with ACTIVE iam.platform_admin assignment SHALL return
LAST_ADMIN_REQUIRED/FAILED_PRECONDITION without successful effects. Revocation
and ChangeUserStatus SHALL share current invariant protection; concurrent
different-target removal SHALL retain at least one such administrator.
Contention SHALL return RESOURCE_BUSY/ABORTED, while dependency failure SHALL
remain UNAVAILABLE and SHALL NOT be treated as a safe count or busy outcome.
No endpoint SHALL silently create an admin, bootstrap identity or replacement role.

#### Scenario: Administrator attempts to revoke their own platform role
- **WHEN** a valid actor includes iam.platform_admin on their own target in a revoke batch
- **THEN** the dedicated self-revoke PERMISSION_DENIED error rejects the entire command before no-op or assignment write

#### Scenario: Only one active administrator would remain unassigned
- **WHEN** an otherwise authorized actor would revoke the sole ACTIVE-assigned administrator
- **THEN** LAST_ADMIN_REQUIRED preserves assignments, versions and successful persistence

#### Scenario: Revoke and status changes remove different administrators in parallel
- **WHEN** two physical connections run admin revoke/revoke or revoke/suspend-deactivate against different current administrators
- **THEN** contention and locked current recount permit only outcomes retaining an eligible active administrator, without deadlock or partial writes

### Requirement: Assignment replay and required persistence have one atomic safe outcome

Grant/revoke scope SHALL be verified actor + exact full method + user/{id} + key;
fingerprint SHALL include sorted unique role_ids, trimmed reason, expected version
and presence while excluding correlation/deadline. Safe original results SHALL
be retained for24 hours. Current authority SHALL precede replay; identical replay
SHALL precede target expected-version comparison and return the original result
without mutation/audit/event/version. Different scoped payload SHALL return
IDEMPOTENCY_CONFLICT/ABORTED; another target SHALL have independent scope and
expired scope SHALL be processed atomically as a new request.
Effective grant/revoke SHALL commit respectively USER_ROLES_GRANTED/
USER_ROLES_REVOKED audit and exactly one pending iam.user.roles_granted.v1/
iam.user.roles_revoked.v1 batch event at the committed user version with actual
changed_role_ids. Assignment, version, audit, event and safe result SHALL share
one outcome. A new full no-op SHALL commit one NO_OP audit and changed=false
result without event/version increment. Every precommit business/read/write/
conditional-result/cancellation/panic error SHALL roll back all business writes.
After cleanup a separate safe failure audit SHALL follow existing policy; failure
of that audit SHALL preserve the original error with bounded telemetry. Commit
uncertainty SHALL NOT be claimed success or certain rollback. No broker is required.

#### Scenario: Response is lost and role input order changes on retry
- **WHEN** a committed batch is retried within retention with the same normalized role set and payload after the target version has advanced
- **THEN** the original result is returned without duplicate assignments, audit, event or counter changes

#### Scenario: Conflicting scoped payload different target or expired result is used
- **WHEN** role/reason/version differs under the same scope, the key is reused for another target or retention expires
- **THEN** conflict returns ABORTED, another target remains independent and expiry follows new-request checks under atomic exclusion

#### Scenario: Valid full no-op is recorded and replayed
- **WHEN** a correct-version grant changes no eligible pair or revoke finds only revoked/absent registered pairs, then its identical key is retried
- **THEN** one NO_OP audit and safe result are committed initially and replay adds no further persistence or version

#### Scenario: Required persistence or later guard fails after tentative writes
- **WHEN** any assignment/version/audit/event/result operation, conditional row count or subsequent guard fails before commit
- **THEN** all business writes roll back and independent failure auditing runs only after cleanup without fabricated success

#### Scenario: Cancellation panic or uncertain commit interrupts a command
- **WHEN** cancellation/panic aborts a batch, independent audit also fails or the commit outcome cannot be established
- **THEN** resources are cleaned, the original safe outcome is preserved and no certain success/rollback or persisted audit is fabricated

### Requirement: Assignment changes preserve credentials and compatible protected admission

Grant/revoke SHALL preserve auth_version, password, session/refresh state,
application-access records and other roles. Evaluation beginning after commit
SHALL use current primary assignment/mapping/gates; overlapping eligible roles
SHALL retain their independent contributions. Existing in-flight snapshots SHALL
not promise retroactive cancellation. ListUserRoles SHALL share protected-read
120/minute burst20; grant/revoke SHALL share admin-mutation30/minute burst5,
including applicable invalid input, bounded capacity/cleanup and structured
RATE_LIMITED/RESOURCE_EXHAUSTED RetryInfo without business effects. api-contracts
SHALL own and generate the three additive RPCs/messages, errors/locales/mappings
before backend wiring, preserving existing tags/types and all27 baseline methods.
Existing Me/GetUser/ListUsers, TesRPC and session lifecycle SHALL remain
compatible. Register SHALL retain intentional PENDING/token-empty behavior and
external client adjustment; only Login/Refresh return credentials. EdDSA-only,
exact issuer, ignored audience, bounded15-minute access, absolute7-day sessions,
30-second JWT skew and restricted allowlist SHALL remain. Actual deployment
readiness SHALL remain mandatory Phase6 DEP-01.

#### Scenario: One overlapping role is revoked with a valid token
- **WHEN** a role assignment is revoked while another eligible role supplies the same permission
- **THEN** the original credential remains valid and subsequent primary evaluation retains only actual remaining contributions without resetting auth_version

#### Scenario: Shared quota is exhausted across baseline and assignment methods
- **WHEN** verified actor/read requests exhaust their existing shared group using old and new methods
- **THEN** the applicable assignment method receives structured rejection without target business effects or permission bypass

#### Scenario: Additive contract is generated and registered
- **WHEN** the published C dependency is consumed and the production server is inspected
- **THEN** exactly three new full methods bring registration to30, existing27 descriptors/tags/types remain and localized catalog errors preserve their mappings

Traceability: [URA-01..11 and URA-EXT-01](../../design.md#acceptance-ledger),
AC-04..07/09/10/19/20/24..26/31/35 and M-05/07..11. Status implementasi C PASS menurut [evidence aktual](../../evidence.md);
backend owner grpc-auth, contract owner api-contracts; publication and dedicated
PostgreSQL required gates telah dijalankan PASS. [Tasks](../../tasks.md) define verification;
planning/source inspection is not evidence of implementation PASS.

# User Application Access Spec Delta

## Purpose

Menyediakan histori akses aplikasi user dan compound grant/revoke yang terotorisasi serta atomik, dengan safe replay dan permission terbaru tanpa membatalkan sesi pengguna.

## ADDED Requirements

### Requirement: Application access methods require current exact authority

`/user.v1.UserService/ListUserApplicationAccess` SHALL require `iam.user.read`.
`/application.v1.ApplicationService/GrantApplicationAccess` SHALL require both
`iam.user.application.grant` AND `iam.user.role.grant`; RevokeApplicationAccess
under the same ApplicationService SHALL require both
`iam.user.application.revoke` AND `iam.user.role.revoke`. All three methods SHALL
require TLS, verified service allowed for the exact method and IAM namespace,
and an ACTIVE nonrestricted verified user/session. Actor SHALL come only from
the verified principal; payload user_id is a separately validated target.
Current authority SHALL precede target-existence disclosure and replay. Mutation
SHALL recheck both permissions after shared ordered exclusion. Dependency failure
SHALL return UNAVAILABLE, never false permission denial, empty success or allow;
cancellation/deadline SHALL retain their transport mappings.

#### Scenario: Actor has only one required mutation permission

- **WHEN** a valid actor has only application grant/revoke permission or only its corresponding role permission
- **THEN** that compound method returns PERMISSION_DENIED without target disclosure, replay or business effects

#### Scenario: Service or user credential is invalid or restricted

- **WHEN** a service is method/namespace-disallowed, a user/session is invalid or the actor must change password
- **THEN** authentication or permission denial prevents all three application access operations without successful disclosure or effects

#### Scenario: Actor privilege changes before locked recheck

- **WHEN** actor session/status or either required permission changes committed after interceptor verification and before command recheck
- **THEN** current state denies the mutation and any retained replay without successful business persistence

### Requirement: Application access listing returns safe stored history on one snapshot

ListUserApplicationAccess SHALL read actor authority, target existence/entity
version, access rows, application metadata, total and page on one consistent
primary snapshot. A nondeleted target in any account status SHALL be readable;
authorized absent/deleted targets SHALL return NOT_FOUND. Default rows SHALL
include ACTIVE and REVOKED access, including disabled/system applications with
existing rows; a never-granted application SHALL NOT be fabricated. Items SHALL
expose only application_id, application_code, name, application_status, is_system,
access status named status, granted_by/granted_at/revoked_by/revoked_at/revoke_reason.
Nullable source fields SHALL preserve absence and SHALL NOT expand actor IDs into
personal profiles. The response SHALL include target user_id/current version,
PageMeta and next_cursor. Listing SHALL NOT assert effective permission, change
business state, expose credentials, or replace the ListApplications catalog.

#### Scenario: A suspended target has historical and disabled application rows

- **WHEN** an authorized caller lists a SUSPENDED target with ACTIVE/REVOKED rows and disabled or system applications
- **THEN** stored rows and safe metadata are returned with current target version and faithful nullable provenance

#### Scenario: Target is empty missing or deleted

- **WHEN** an authorized caller lists a nondeleted target without access rows or a missing/deleted target
- **THEN** only the existing target returns empty items with total/total_pages zero; absent/deleted target returns NOT_FOUND

#### Scenario: Policy or data changes between component reads

- **WHEN** a concurrent commit changes authority, access or application metadata between reads
- **THEN** authority, target version, total and page describe one primary snapshot without promising retroactive cancellation

#### Scenario: A required read fails after loading partial state

- **WHEN** authority, target, count or page fails or the request is canceled
- **THEN** no partial response or empty-success replacement is returned

### Requirement: Access queries support public scalar sorting projection and bound cursors

ListUserApplicationAccess SHALL accept PageQuery/PageMeta with page default1,
limit default20/max100, application_code and repeated access statuses ACTIVE/REVOKED.
Statuses combine OR; filter categories and keyword combine AND. Search SHALL be
case-insensitive literal CONTAINS or PREFIX, default CONTAINS, trimmed keyword
at most200 characters, with application_code/name fields combined OR and both
as default. Percent/underscore SHALL remain literal. Every public item scalar
SHALL support ASC/DESC multi-sort through an allowlist, preserving request order
and rejecting unknown/duplicate fields. Default and appended tie-breaker SHALL
be application_id ASC; nullable fields sort nulls last in both directions.
Projection SHALL default to all public item fields, always include application_id
and retain hidden sort values for cursors. Total SHALL count unique matching
access pairs before cursor/limit, unaffected by role/permission multiplicity.
Opaque cursors SHALL bind to full method, target user and normalized filters,
search and sort, carrying typed values/nulls. Cross-target/method/query or malformed
cursors SHALL return INVALID_ARGUMENT. Positive INTEGER-compatible numeric last_id
SHALL be accepted only for application_id-only sort. Without cursor, page>1 SHALL
use deterministic offset pagination. No cross-request retained snapshot is promised.

#### Scenario: Filters combine with a literal keyword

- **WHEN** a query selects an application, both access statuses and a keyword containing percent or underscore
- **THEN** only matching access pairs appear once and total is pre-cursor with literal search semantics

#### Scenario: Every public scalar and mixed nullable ties are sorted

- **WHEN** either sort direction of each public scalar or a mixed multi-sort with ties/nulls is requested
- **THEN** approved expressions, nulls-last and application_id tie-breaking give deterministic continuation on unchanged state

#### Scenario: Projection excludes cursor values

- **WHEN** requested projection omits the fields used for ordering
- **THEN** application_id remains visible and hidden typed values still produce the correct next cursor

#### Scenario: Invalid query or foreign cursor is supplied

- **WHEN** status/field/mode/limit is invalid or a cursor is malformed or belongs to another user, method, filter, search or sort
- **THEN** INVALID_ARGUMENT returns without silent restart or accepting SQL expressions

#### Scenario: Numeric cursor offset page or terminal page is requested

- **WHEN** application_id-only sort uses numeric last_id, a page>1 without a cursor is requested or traversal reaches the end
- **THEN** numeric compatibility and deterministic offset work, total remains pre-cursor and terminal next_cursor is empty

### Requirement: Application mutations validate canonical targets and explicit concurrency inputs

Both mutations SHALL require positive PostgreSQL INTEGER-compatible user_id,
canonical application_code, trimmed reason3–500 characters, present positive
int64-compatible expected_user_version and exactly one nonempty idempotency-key
metadata value at most200 characters. Application code SHALL be normalized by
trimming/lowercasing then validated against `^[a-z][a-z0-9_]*$` and length50.
Grant SHALL require1–20 submitted role IDs, validate each positive INTEGER-compatible
ID before deduplicating/sorting as a set. Revoke SHALL accept no role selection and
SHALL cover all stored assignments of the target application, including more than20.
Registered application code SHALL resolve to its authoritative numeric ID.
Unknown application SHALL return APPLICATION_NOT_FOUND/NOT_FOUND after authority.
IAM or is_system application mutations SHALL return
APPLICATION_ACCESS_SYSTEM_PROTECTED/FAILED_PRECONDITION, including nominal no-ops.

#### Scenario: Invalid input reaches either command

- **WHEN** target/code/reason/key/version presence or range is invalid, or grant has an empty/over20/invalid role input
- **THEN** INVALID_ARGUMENT returns without successful compound persistence

#### Scenario: Application is unknown or protected

- **WHEN** an authorized valid command targets an unregistered code, iam or another is_system application
- **THEN** the matching catalog error returns without changing access or any role, including platform-admin assignment

#### Scenario: Role input contains duplicates or reordered values

- **WHEN** grant contains valid duplicate IDs within the submitted-entry bound or the same set in another order
- **THEN** normalization yields one canonical role set without duplicate mutation and the same replay fingerprint

### Requirement: Grant activates business access and selected eligible roles atomically

GrantApplicationAccess SHALL require a nondeleted ACTIVE target, ACTIVE nonsystem
application and every selected role registered, ACTIVE and belonging to that
application. Unknown roles SHALL return ROLE_NOT_FOUND/NOT_FOUND; disabled roles
ROLE_DISABLED/FAILED_PRECONDITION; inactive application APPLICATION_INACTIVE/
FAILED_PRECONDITION; inactive target GRANT_TARGET_INACTIVE/FAILED_PRECONDITION;
cross-application role selection SHALL return INVALID_ARGUMENT. All eligibility
SHALL be validated before the first write, including otherwise active no-ops.
Grant SHALL create or reactivate the single access pair and create/reactivate only
selected role pairs. Reactivation SHALL preserve pair identity, update grant
actor/time and clear old revocation metadata. Eligible ACTIVE pairs SHALL preserve
their provenance. Grant is additive: unselected existing ACTIVE roles SHALL remain,
and unselected REVOKED roles SHALL NOT be reactivated. Other applications SHALL
remain unchanged. changed_role_ids SHALL contain sorted unique actual role
transitions; changed SHALL be true if access or any selected role changes.

#### Scenario: Initial grant includes new revoked and active selected pairs

- **WHEN** an eligible command opens access with a mix of new, REVOKED and ACTIVE selected roles
- **THEN** access and actual selected role transitions commit together, pair identity is retained and unchanged ACTIVE provenance is preserved

#### Scenario: Access is already active and grant adds one role

- **WHEN** eligible ACTIVE access has other ACTIVE roles and the command selects a new or revoked role
- **THEN** only the selected role transitions, existing unselected ACTIVE roles remain and the command is effective

#### Scenario: Access was revoked and only a subset is selected on regrant

- **WHEN** revoked access and revoked historical roles are regranted with a valid subset
- **THEN** only selected roles are reactivated; old unselected REVOKED assignments stay revoked

#### Scenario: One selected role fails eligibility

- **WHEN** any selected role is unknown, disabled or belongs to another application, or target/application is ineligible
- **THEN** the matching catalog/validation error rejects the complete grant without partial access, roles or success evidence

#### Scenario: Only the application access row changes

- **WHEN** access is absent/revoked and all selected eligible role pairs are already ACTIVE
- **THEN** changed is true with empty changed_role_ids, one user version increment and one application event

### Requirement: Revoke closes access and every active role of that application

RevokeApplicationAccess SHALL permit a nondeleted target in any account status,
regardless of nonsystem application/role status. It SHALL revoke an ACTIVE access
pair and every ACTIVE stored role assignment belonging to that application,
preserving grant provenance and recording current revoke actor/time/reason.
Already REVOKED rows SHALL preserve their provenance; absent access SHALL NOT be
inserted. Even when access is absent/already REVOKED, remaining ACTIVE assignments
SHALL be revoked atomically. Other applications and roles SHALL remain unchanged.
changed_role_ids SHALL contain all sorted unique actual revoked role IDs without
the submitted grant batch bound. changed SHALL mean that access or an assignment
transitioned; absence with no ACTIVE assignments is a valid full no-op.

#### Scenario: Revoke affects one application with many roles

- **WHEN** a target has ACTIVE access and more than20 ACTIVE assignments for that application plus roles for another application
- **THEN** all target-application roles and access become REVOKED in one command and other applications remain unchanged

#### Scenario: Access is absent or revoked but active assignments remain

- **WHEN** an authorized valid revoke finds no ACTIVE access pair but finds ACTIVE roles for the application
- **THEN** roles are revoked, changed is true and no missing access row is inserted or old revoked metadata rewritten

#### Scenario: Target and application are inactive

- **WHEN** the target is PENDING/SUSPENDED/DEACTIVATED or the nonsystem application/roles are DISABLED
- **THEN** revoke follows ordinary version and compound persistence rules without requiring reactivation

#### Scenario: All affected state is absent or already revoked

- **WHEN** a valid current-version revoke finds no ACTIVE access or assignments
- **THEN** changed is false, changed_role_ids is empty, old provenance remains and no history row is fabricated

### Requirement: Compound persistence and entity version have one outcome

A new command SHALL compare expected_user_version with current target entity
version before eligibility/no-op. Any effective access/assignment change SHALL
increment user version once, regardless of role count; auth_version SHALL remain.
Effective grant/revoke SHALL commit respectively APPLICATION_ACCESS_GRANTED/
APPLICATION_ACCESS_REVOKED audit, exactly one pending
iam.user.application_access_granted.v1/iam.user.application_access_revoked.v1 event
and safe result with the mutation. Event SHALL use user aggregate ID/committed
version, safe application identity/status and actual changed_role_ids, without
per-role events or free-form reason. Valid full no-op SHALL commit one NO_OP audit
and changed=false safe result without event/version increment. Every precommit
read/write/conditional-result/cancellation/panic failure SHALL roll back all
business writes. Independent safe failure audit SHALL run only after cleanup
under existing policy and SHALL preserve original failure if it also fails.
Commit uncertainty SHALL NOT be claimed success or certain rollback.

#### Scenario: Several role transitions have one compound event

- **WHEN** grant/revoke changes access or multiple roles and all required persistence succeeds
- **THEN** version advances once and one user aggregate application event carries all actual changed_role_ids without role event duplication

#### Scenario: Stale new request would otherwise be a no-op

- **WHEN** a fresh scoped command has an outdated expected_user_version even though requested state already holds
- **THEN** VERSION_CONFLICT/ABORTED precedes no-op and no successful evidence is persisted

#### Scenario: A required write or conditional update fails

- **WHEN** access/assignment/version/audit/outbox/result persistence fails after tentative writes
- **THEN** all business changes roll back together and no successful replay result remains

#### Scenario: Cancellation panic or commit uncertainty interrupts mutation

- **WHEN** cancellation/panic occurs before commit or the commit outcome is unknown, including an independently failing failure audit
- **THEN** cleanup preserves the original safe error without fabricated success, certainty or leaked locks/connections

### Requirement: Replay uses canonical application scope and original safe result

Scope SHALL be verified actor + exact full method +
user/{user_id}/application/{authoritative_application_id} + key. Fingerprint SHALL
include canonical application ID, normalized reason, expected version/presence and
grant role set, excluding trace/deadline. Safe original results SHALL last24 hours.
After current authority and canonical target resolution, identical replay SHALL
precede current target-version and fresh eligibility/no-op checks, return the
original result and perform no business/audit/event/version writes. Different
payload under the same scope SHALL return IDEMPOTENCY_CONFLICT/ABORTED; another
application/user/method SHALL have independent scope. Expiry SHALL follow fresh
request checks and replace the record atomically. Safe response SHALL contain
user/application IDs, optional access_status, changed_role_ids, version and changed.
Absent access SHALL remain absent in response; it SHALL NOT be represented as a
new persisted REVOKED row. Credentials SHALL NOT enter retained results.

#### Scenario: Response is lost after commit and target version advances

- **WHEN** an identical canonical request is retried during retention after its response was lost
- **THEN** the original safe response returns before current target-version comparison with no repeated compound effects

#### Scenario: Scope payload or retention differs

- **WHEN** the same scoped key changes roles/reason/version, another application/user/method reuses the key or retention expires
- **THEN** conflicting payload returns ABORTED, different targets remain independent and expiry is processed as a new command under exclusion

#### Scenario: A no-op with absent access is retried

- **WHEN** a current-version absent-access full no-op commits then is retried identically
- **THEN** absent access_status and changed=false replay exactly, with one initial NO_OP audit and no inserted access row or repeated evidence

#### Scenario: Current authority is lost before replay

- **WHEN** a retained success exists but the actor no longer satisfies session or either exact mutation permission
- **THEN** the saved result is not disclosed and no replay bypass occurs

### Requirement: Access changes preserve sessions and compatible bounded admission

Grant/revoke SHALL preserve password/auth_version, session/refresh state, other
applications, profile and role/catalog definitions. Evaluation beginning after
commit with a still-valid JWT SHALL use current primary access and assignment
state; no in-flight cancellation is promised. IAM self-service SHALL NOT gain an
application-access gate. List SHALL share protected-read120/minute burst20;
mutations SHALL share admin-mutation30/minute burst5 with existing methods,
bounded cleanup/capacity and structured RATE_LIMITED/RESOURCE_EXHAUSTED RetryInfo.
api-contracts SHALL own/generate three additive methods/messages before backend
wiring, preserve existing tags/types and all30 baseline methods, and allow exact33
production registered methods. Existing ListUsers/role catalogs/assignment C,
TesRPC, generic Login denial, profile compatibility and credential lifecycle SHALL
remain. Register SHALL retain intentional PENDING/token-empty behavior and external
client adjustment. EdDSA-only/exact issuer/ignored audience, access15m capped by
session expiry, session/refresh absolute7d and JWT skew30s SHALL remain unchanged.
Actual deployment readiness SHALL remain mandatory Phase6 DEP-01 separately.

#### Scenario: Old JWT is used after revoke and selected regrant

- **WHEN** application revoke commits and a subset is later regranted while the original session remains valid
- **THEN** primary evaluation denies revoked contributions and restores only eligible active contributions, without credential invalidation or effects on other applications

#### Scenario: Shared read or mutation quota is exhausted

- **WHEN** old and new methods exhaust the verified actor's shared group, including applicable invalid mutation input
- **THEN** structured rate rejection prevents business effects without substituting for exact authorization

#### Scenario: Published additive contract reaches production registration

- **WHEN** generated contract D is published and consumed by production composition
- **THEN** exactly33 methods are registered, all30 baseline methods remain compatible and catalog/localized error mappings remain consistent

Traceability: [AAP-01..12 / AAP-EXT-01](../../design.md#acceptance-ledger),
[AC-08/11/12/13/24–26/31](../../../../../../docs/grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence).
Backend owner grpc-auth, contract owner api-contracts. Implementation **PASS**:
33/33 tasks and12 gates passed on the published D dependency; see
[actual evidence](../../evidence/progress.md). Baseline C/refactor and dedicated
PostgreSQL prerequisites were verified. Public effective permission reads,
EvaluateAccess, CLM acceptance and mandatory DEP-01 remain separate.

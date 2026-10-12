# role-administration Specification

## Purpose

Menyediakan administrasi custom business role dan permission mapping yang
terotorisasi, terlindung dari perubahan role sistem, serta atomik dan dapat
diulang tanpa menggandakan perubahan atau membatalkan sesi user.

## Requirements

### Requirement: Role mutations require exact current authority and protected system roles

`/role.v1.RoleService/CreateRole` SHALL require `iam.role.create`;
`/role.v1.RoleService/UpdateRole` and `/role.v1.RoleService/ChangeRoleStatus`
SHALL require `iam.role.update`; `/role.v1.RoleService/ReplaceRolePermissions`
SHALL require `iam.role.permission.update`. Every method SHALL require TLS,
verified method/namespace-allowed service and ACTIVE nonrestricted user/session.
Actor SHALL come only from verified principal. Current authority SHALL be
rechecked under shared ordered locks before target disclosure, replay or write.
System roles, IAM roles and legacy IAM roles SHALL reject metadata/status/
permission mutations, including nominal no-ops; caller role names or numeric
codes SHALL NOT bypass permission or protection. Dependency failure SHALL remain
UNAVAILABLE, with deadline/cancellation mapping preserved.

#### Scenario: Caller lacks one exact permission
- **WHEN** a valid caller attempts ReplaceRolePermissions with iam.role.update but without iam.role.permission.update
- **THEN** PERMISSION_DENIED occurs without target detail, replay or successful mutation

#### Scenario: Restricted service or user attempts management
- **WHEN** the service method/namespace is disallowed, credentials/session are invalid or the user is restricted
- **THEN** applicable authentication/permission denial occurs before business effects

#### Scenario: Administrator attempts an IAM legacy or system no-op
- **WHEN** an otherwise authorized actor edits metadata, changes status or replaces permissions of a protected role, even with identical values
- **THEN** SYSTEM_ROLE_PROTECTED/FAILED_PRECONDITION leaves role, mappings, users and credentials unchanged

#### Scenario: Actor privilege changes before lock recheck
- **WHEN** actor status/session/contributing permission changes after initial verification and before locked recheck
- **THEN** stale authority cannot mutate the target or disclose a retained result

### Requirement: CreateRole atomically creates a uniquely coded custom business role

CreateRole SHALL accept an existing ACTIVE nonsystem business application_code,
canonical lower-case code matching that application's namespace, a trimmed
nonempty name at most 50 characters, optional description at most 100 characters,
and a final set of registered permission_codes from that application. Code SHALL
be at most 100 characters and match the stored two-segment role-code grammar;
malformed or wrong-prefix codes SHALL be rejected, not silently rewritten.
Empty permission sets SHALL be allowed; duplicate codes SHALL be deduplicated.
Creation SHALL produce ACTIVE, is_system=false, version=1, actual timestamps,
and a database-allocated unique positive INTEGER-compatible numeric role_code.
It SHALL NOT assign the role, grant application access or create credentials.
Global string-code uniqueness SHALL survive concurrency; safe response SHALL
preserve string code and numeric role_code as distinct fields.

#### Scenario: Administrator creates an empty custom role
- **WHEN** an authorized actor submits a unique valid business role with an empty permission set and required key
- **THEN** one ACTIVE version-one role with no mappings is returned without user assignment or credential changes

#### Scenario: Permission set contains duplicates or another namespace
- **WHEN** permission codes repeat, are unknown or belong to IAM/another application
- **THEN** duplicates represent one mapping each, while unknown or cross-application entries reject the entire command with INVALID_PERMISSION_SET/INVALID_ARGUMENT

#### Scenario: Application or code is unsuitable
- **WHEN** application is missing, disabled, system/IAM, code prefix is wrong, or code/name/description violates bounds
- **THEN** the documented catalog/validation failure commits no role, mapping or saved success result

#### Scenario: Two requests compete for the same code
- **WHEN** independent authorized creation commands target the same canonical string code
- **THEN** at most one role is created; contention returns RESOURCE_BUSY/ABORTED or the later duplicate returns ALREADY_EXISTS, without partial persistence

### Requirement: Role metadata status and permission updates use explicit versioned outcomes

UpdateRole SHALL accept only name/description with explicit presence and matching
nonempty field_mask; selected empty description SHALL clear it and omitted fields
SHALL remain. Code/application/status/ownership/system/numeric-code updates SHALL
be rejected. ChangeRoleStatus SHALL accept explicit ACTIVE or DISABLED and reason
3–500 trimmed characters; ReplaceRolePermissions SHALL require explicit final-set
intent, reason 3–500 and registered same-application permissions, including empty
set to remove all mappings. These methods SHALL require positive INTEGER-compatible
role_id and present positive int64-compatible expected_role_version. Stale new
requests SHALL return VERSION_CONFLICT/ABORTED before no-op. Effective change
SHALL increment role version once; a valid new no-op SHALL return changed=false
with one NO_OP audit and no event/version increment. Updates to an existing custom
role in a disabled application SHALL remain manageable, but SHALL NOT bypass
application gates during permission evaluation.

#### Scenario: Selected metadata is cleared while other fields are omitted
- **WHEN** authorized UpdateRole selects only present empty description with current version
- **THEN** description is cleared, name/code/application remain, and an effective update advances role version once

#### Scenario: Mask presence or immutable fields are invalid
- **WHEN** selected fields are absent, present fields are unselected, mask is empty/unknown or input attempts an immutable field
- **THEN** INVALID_ARGUMENT occurs with no business writes or saved success

#### Scenario: Same status set or metadata has stale version
- **WHEN** a new valid request repeats current values with a stale expected version
- **THEN** VERSION_CONFLICT/ABORTED occurs rather than a successful no-op

#### Scenario: Final permission set replaces rather than appends
- **WHEN** an authorized actor replaces A/B with B/C or explicitly supplies an empty set
- **THEN** the resulting mappings are exactly B/C or empty respectively, with one effective role-version increment and no changes to other roles

#### Scenario: Valid same values produce a new no-op
- **WHEN** an authorized new request has current version and canonical values/set identical to current state
- **THEN** changed=false and one NO_OP audit commit without another domain event, including unkeyed UpdateRole

### Requirement: Role policy changes preserve credentials and take effect on subsequent evaluation

Disable, enable and permission replacement SHALL preserve user entity versions,
auth_version, session/refresh state, role assignments and application-access
records. An evaluation starting after committed change SHALL resolve current
primary state: DISABLED contributes nothing, ACTIVE contributes current mappings
subject to assignment/account/application/access gates. Enable SHALL not regrant
REVOKED assignments or restore revoked sessions. Overlapping roles SHALL retain
their independent permission contributions. In-flight reads SHALL only promise
one consistent snapshot, not retroactive cancellation.

#### Scenario: A role is disabled for users with valid tokens
- **WHEN** a custom role is disabled and a new protected evaluation uses existing still-valid credentials
- **THEN** identity remains valid while that role contributes no permission, without session revocation or user counter changes

#### Scenario: A disabled role is enabled again
- **WHEN** an administrator enables a custom role that has ACTIVE and REVOKED assignments
- **THEN** only eligible ACTIVE assignments can contribute; revoked assignments/sessions and disabled application gates remain unchanged

#### Scenario: Another role still supplies a removed permission
- **WHEN** replacement removes a permission also supplied by another eligible ACTIVE role
- **THEN** the permission remains effective from the other role and source contributions use current mappings

### Requirement: Role mutation replay and required persistence have one atomic outcome

CreateRole, ChangeRoleStatus and ReplaceRolePermissions SHALL require exactly one
nonempty idempotency-key of at most 200 characters; UpdateRole SHALL accept an
optional key under the same protocol. Scope SHALL be verified actor + exact full
method + role-code/{canonical_code} for creation or role/{id} + key. Fingerprint
SHALL include normalized payload, presence, expected version and sorted unique
permission set, excluding correlation/deadline. Safe original response SHALL
persist for 24 hours; current authorization SHALL precede replay and replay
SHALL precede target version comparison. Different payload in the same scope
SHALL return IDEMPOTENCY_CONFLICT/ABORTED. Effective creation SHALL commit
ROLE_CREATED and `iam.role.created.v1` at version one; metadata/status SHALL
commit ROLE_UPDATED and `iam.role.updated.v1`; replacement SHALL commit
ROLE_PERMISSIONS_CHANGED and `iam.role.permissions_changed.v1`. Role/mappings,
version, audit, pending event and keyed safe result SHALL commit together.
Every precommit error, including business/read/conditional-write/cancellation/
panic, SHALL roll back all business writes. After cleanup, safe failure audit
SHALL use an independent transaction; audit failure SHALL preserve the original
error and bounded telemetry. Commit uncertainty SHALL NOT be claimed success
or certain rollback. Broker availability SHALL NOT be a prerequisite.

#### Scenario: Response is lost and permission input order changes
- **WHEN** a committed keyed request is retried within retention with the same canonical permission set in a different order
- **THEN** the original safe response is returned with no extra mappings, audit, event or version, even if target version subsequently advanced

#### Scenario: Conflicting payload different target or expired retention
- **WHEN** the same scoped key carries different input, the key is reused for another target, or retention expires
- **THEN** conflicting input returns ABORTED, another target uses independent scope, and expired scope is processed as a new atomically excluded command with current version/business checks

#### Scenario: A later mapping guard or required write fails
- **WHEN** a delete/insert/version/audit/event/result step or subsequent business guard fails after tentative writes
- **THEN** the whole business transaction rolls back and independent failure auditing can record the safe reason without success effects

#### Scenario: Failure audit also fails after cancellation or panic
- **WHEN** cancellation/panic aborts a command and the separate audit transaction fails
- **THEN** cleanup releases resources, the original safe error remains and bounded telemetry does not claim persisted audit

#### Scenario: Commit outcome is unknown or broker is absent
- **WHEN** commit cannot establish its outcome or an otherwise valid command commits without a relay
- **THEN** uncertainty is reported without fabricated rollback certainty, while confirmed committed events remain pending without requiring RabbitMQ

### Requirement: Additive role contracts and numeric mutation admission preserve existing behavior

api-contracts SHALL own and generate all new role messages/full methods and
feature errors/locales/mappings before backend registration. Existing field
numbers/types, nineteen methods, numeric user role_codes, TesRPC, dynamic
ListUsers and session lifecycle SHALL remain compatible. All four mutations
SHALL share verified-actor admin-mutation admission 30/minute burst5, including
applicable invalid input; RATE_LIMITED/RESOURCE_EXHAUSTED SHALL carry bounded
RetryInfo and permit no business effect. Register SHALL retain intentional
PENDING/token-empty behavior requiring consuming-client adjustment; only
Login/Refresh SHALL return credentials. EdDSA-only/exact issuer/ignored audience,
access at most15 minutes, absolute7-day session and30-second JWT skew SHALL
remain. External deployment readiness SHALL remain mandatory Phase6 DEP-01.

#### Scenario: New methods are generated and registered
- **WHEN** the published additive contract is consumed by grpc-auth
- **THEN** all eight planned methods have exact descriptors and existing nineteen method signatures/field tags/types remain unchanged, without local proto substitutes

#### Scenario: Mutation quota is exhausted across user and role commands
- **WHEN** an actor exhausts the shared admin-mutation group using existing and new methods
- **THEN** the next applicable role mutation receives structured rate rejection with no target writes or successful persistence

#### Scenario: Role catalog errors cross the localized gRPC boundary
- **WHEN** not-found, duplicate, system protection, permission-set, stale-version, idempotency or busy failures cross the boundary
- **THEN** catalog identity, safe locale and documented gRPC mapping agree without remapping legacy errors or exposing credentials

Traceability: [RM-01..06 and RM-EXT-01](../../changes/archive/2026-10-12-auth-role-management/design.md#acceptance-ledger),
AC-21/22/23/24/25/26/28/31 and M-05/07/08/10/11. Backend status PASS;
backend owner grpc-auth, contract owner api-contracts. Published contract and
dedicated PostgreSQL verification prerequisites are satisfied by actual
[evidence](../../changes/archive/2026-10-12-auth-role-management/evidence.md) and [scenario mappings](../../changes/archive/2026-10-12-auth-role-management/scenario-traceability.md).
[Tasks](../../changes/archive/2026-10-12-auth-role-management/tasks.md) track completion; actual deployment remains Phase6 DEP-01.

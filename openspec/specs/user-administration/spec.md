# user-administration Specification

## Purpose

Menyediakan administrasi detail, creation dan profil user melalui permission
authoritative, safe response, optimistic version dan persistence atomik tanpa
membocorkan credential atau mengubah ownership autentikasi caller.

## Requirements

### Requirement: User administration authenticates exact methods and permissions

`/user.v1.UserService/GetUser`, `/user.v1.UserService/CreateUser`,
`/user.v1.UserService/UpdateUser` dan `/user.v1.UserService/ChangeUserStatus`
SHALL require TLS, verified method/namespace-allowed service caller and an ACTIVE
nonrestricted verified user/session. Exact permissions SHALL respectively be
`iam.user.read`, `iam.user.create`, `iam.user.update`, and
`iam.user.status.update`. Actor SHALL come only from verified principal; target
ID SHALL be a separate positive PostgreSQL INTEGER-compatible ID. Numeric role
codes, role names, supplied actor IDs and earlier authorization SHALL NOT bypass
current authority. Dependency failures SHALL remain UNAVAILABLE and request
cancellation/deadline SHALL retain their mapping.

#### Scenario: Caller or actor lacks authority
- **WHEN** any new management RPC is called with invalid service credential, disallowed method/namespace, invalid session, restricted user or missing exact permission
- **THEN** the applicable authentication/permission denial is returned before target disclosure, replay or business mutation

#### Scenario: Actor privilege changes after initial verification
- **WHEN** actor state or contributing permission changes before locked mutation recheck
- **THEN** stale authorization does not permit mutation or saved-result disclosure

#### Scenario: Target and actor are different
- **WHEN** an authorized administrator supplies a valid target ID different from their own
- **THEN** the operation applies only its permitted target behavior while audit and authorization retain the verified actor identity

### Requirement: GetUser exposes safe detail from one authoritative snapshot

`/user.v1.UserService/GetUser` SHALL read current caller authority, target scalar
data and eligible role/application collections from one consistent primary
snapshot. Authorized detail SHALL expose safe ID/email/profile/status/version/
created_at and must_change_password metadata; collection defaults SHALL use
ACTIVE assignments/access with ACTIVE roles/applications. Nondeleted targets
in any business status SHALL be readable by authorized administrators. A missing
or deleted target SHALL return the agreed not-found catalog mapping after
authority checks. Password/plaintext/hash/access/refresh/internal auth state
SHALL NOT appear. Read SHALL NOT mutate version or create a domain event.

#### Scenario: Authorized administrator reads a pending user
- **WHEN** an unrestricted actor with iam.user.read requests an existing PENDING target
- **THEN** safe detail and current entity version are returned without credential material

#### Scenario: Target is absent or caller permission is revoked
- **WHEN** the target is missing or caller permission was revoked before the snapshot
- **THEN** an authorized caller receives not-found and an unauthorized caller receives permission denial without target-existence disclosure

#### Scenario: Collection or permission changes during a read
- **WHEN** a concurrent commit changes role/access/permission between reads within GetUser
- **THEN** authority and response describe one primary snapshot; a required dependency failure returns UNAVAILABLE without partial detail

### Requirement: CreateUser produces a restricted account without credentials or privilege injection

`/user.v1.UserService/CreateUser` SHALL normalize email with trim/lowercase and
validate existing email/password bounds 6–50, supplied profile fields and status
PENDING or ACTIVE. Initial password SHALL be securely hashed using the existing
password policy; user version/auth_version SHALL start at one and
must_change_password SHALL be true. Account/profile and any default nonadmin
member creation SHALL be atomic with required persistence. Payload SHALL NOT
grant admin roles, permission or application access. No session or access/refresh
credential SHALL be created or returned. Login for PENDING SHALL remain denied;
ACTIVE created users SHALL follow the existing restricted allowlist until
ChangePassword commits global credential invalidation and requires login again.

#### Scenario: Administrator creates an active account
- **WHEN** an authorized actor submits valid unique normalized email, initial password, profile and ACTIVE status
- **THEN** one restricted account is created with initial counters and safe management response, without a session or echoed password

#### Scenario: Pending creation is activated later
- **WHEN** an administrator creates PENDING and later validly changes its status to ACTIVE
- **THEN** login was denied while PENDING and subsequent login remains restricted until the existing ChangePassword flow succeeds

#### Scenario: Duplicate email or privilege-bearing input is attempted
- **WHEN** normalized email already exists or input attempts forbidden privilege/status values
- **THEN** agreed catalog/validation failure is returned without partial account/profile/assignment or success persistence

### Requirement: UpdateUser uses explicit safe field presence and target version

`/user.v1.UserService/UpdateUser` SHALL update only target name/address/phone
using explicit field presence/field_mask and a positive expected_user_version.
Selected name SHALL follow 3–100 validation; selected address/phone SHALL respect
verified schema bounds; omitted fields SHALL remain unchanged and selected
empty values SHALL clear only fields that permit empty. Unknown fields, owner,
email, password, role, status and privilege updates SHALL be rejected.
Current target version SHALL be checked before deciding a new no-op; stale
version SHALL return VERSION_CONFLICT/ABORTED. Effective change SHALL increment
target user version once without changing auth_version, password or sessions.

#### Scenario: Only address is selected
- **WHEN** an authorized actor sends address presence/mask and the current expected target version
- **THEN** only address changes, omitted name/phone remain, user version increments once and credential state remains valid

#### Scenario: Stale version identical values or forbidden fields
- **WHEN** a new request has a stale expected version even with identical values, or selects a forbidden field
- **THEN** version conflict or validation is returned with no mutation, success audit/event or saved success result

#### Scenario: Valid update has no effective change
- **WHEN** a new request has the correct version and valid selected values identical to current state
- **THEN** changed=false with unchanged version is returned, one NO_OP audit commits and no state-change event is created

### Requirement: User management replay is scoped safe and currently authorized

CreateUser and ChangeUserStatus SHALL require nonempty metadata idempotency-key
at most 200 characters; UpdateUser SHALL accept an optional key and apply the
same protocol when provided. Scope SHALL be verified actor + full method +
canonical account/{normalized_email} for creation or user/{id} for target
mutation + key. Fingerprint SHALL include normalized payload, target, presence
and expected version; CreateUser password comparison SHALL use a versioned
keyed digest separate from authentication hashes and limiter secrets. Unexpired
safe results SHALL be retained for 24 hours. Current actor/session/permission
SHALL be checked before replay; replay SHALL precede target expected-version
comparison and return the original safe result without another effect/audit/
event/version. Different payload in the same scope SHALL yield
IDEMPOTENCY_CONFLICT/ABORTED. Unknown retained digest key SHALL fail closed.

#### Scenario: Creation response is lost and the same request is retried
- **WHEN** CreateUser committed but its response was lost and the same authorized actor retries identical scoped input during retention
- **THEN** the original safe creation result is returned without another account, hash write, audit or event, and no credential is cached

#### Scenario: Password changes on a creation retry
- **WHEN** the same actor/method/account/key is reused with a different initial password or other normalized payload
- **THEN** idempotency conflict is returned without disclosing or replacing the first user's credential

#### Scenario: Update replay has an old target version
- **WHEN** an identical authorized keyed UpdateUser retry reaches a committed result while the target version has advanced
- **THEN** the original safe result is returned before target version comparison without additional audit/event

#### Scenario: Different target expired result or rotated digest key
- **WHEN** a key is reused for another target, retention expires, or an unexpired creation result refers to a retained digest key version
- **THEN** another target has independent scope, expired scope is processed atomically as a new command, and retained digest keys verify replay; missing required keys return a dependency error rather than new creation

### Requirement: User mutations commit required persistence and rollback every precommit error

Effective CreateUser SHALL commit USER_CREATED audit and pending
`iam.user.created.v1` at version one; effective UpdateUser SHALL commit
USER_PROFILE_UPDATED audit and pending `iam.user.profile_updated.v1` at the
committed target version. Mutation, counters, required audit/outbox and keyed
safe result SHALL share one business transaction. New valid no-op SHALL commit
one NO_OP audit and keyed safe result where applicable, without event/version
increment. Every precommit business/read/conditional-write/write/cancellation/
panic error SHALL roll back all business writes. After cleanup an independent
transaction SHALL record the safe failure reason; audit failure SHALL preserve
the original error and produce bounded telemetry. Commit uncertainty SHALL NOT
be reported as success or certain rollback. Broker availability SHALL NOT be a
runtime prerequisite; events remain pending without a relay.

#### Scenario: Required result or event fails after target write
- **WHEN** any required audit/outbox/idempotency write fails after a tentative account/profile/version update
- **THEN** all business writes roll back and a separate safe failure audit can persist after cleanup

#### Scenario: Guard or conditional update fails after a write
- **WHEN** a business guard/read/affected-row check returns an error after earlier writes
- **THEN** the entire business transaction rolls back, including tentative success audit/event/result

#### Scenario: Cancellation panic or failure-audit error occurs
- **WHEN** request cancellation or panic aborts the operation and independent failure auditing also fails
- **THEN** resources are cleaned up, the original safe operation error is preserved and bounded telemetry does not claim successful audit persistence

#### Scenario: Concurrent creation or missing broker
- **WHEN** two requests compete for one normalized email or a valid command commits with no RabbitMQ relay
- **THEN** uniqueness/ordered serialization prevents duplicate creation and committed events remain pending without requiring publication

### Requirement: User administration participates in shared numeric admission groups

GetUser SHALL share the existing protected-read verified-user/group quota
120/minute burst20. CreateUser, UpdateUser and ChangeUserStatus SHALL share the
admin-mutation verified-actor/group quota30/minute burst5. Invalid input SHALL
count through applicable verified admission. Existing bounded capacity, cleanup
and structured RATE_LIMITED/RESOURCE_EXHAUSTED RetryInfo SHALL remain. Quota
SHALL NOT replace authoritative authorization or permit a business mutation on
rejection.

#### Scenario: Different management mutations exhaust their shared group
- **WHEN** a verified actor exhausts admin-mutation quota across the new methods
- **THEN** another applicable mutation receives bounded RetryInfo without target writes, success audit/event or saved result

#### Scenario: Read quota is available but target permission is absent
- **WHEN** a caller with available protected-read quota lacks iam.user.read
- **THEN** GetUser remains permission denied without target detail

### Requirement: Additive management contracts preserve existing credentials and list behavior

New management RPC/messages/errors SHALL be owned and generated by api-contracts
and available in a published dependency before backend wiring. api-contracts
SHALL provide GetUser/CreateUser methods using additive compatible existing
messages, and UpdateUser/ChangeUserStatus methods/messages with explicit safe
field presence, target/version/status/reason and safe responses. Semantic error
catalogs, locale messages and gRPC mappings SHALL remain consistent, reusing
existing meanings without global legacy remapping. Existing fields
SHALL retain tags/types; legacy User.password SHALL always remain empty in
management responses. Existing Register PENDING/token-empty, Login/Refresh-only
credential responses, TesRPC, restricted allowlist and all lifecycle RPCs SHALL
remain. Dynamic ListUsers sort/filter/search/unique total/composite cursor,
default ID ASC/auth_id alias and numeric ID cursor compatibility SHALL remain.
Access SHALL retain EdDSA-only, exact issuer, ignored audience, 15-minute bounded
lifetime, absolute seven-day sessions and existing 30-second JWT skew.

#### Scenario: Legacy and lifecycle clients call after management registration
- **WHEN** existing clients call the prior fifteen methods or use numeric ID-only ListUsers cursor
- **THEN** prior approved behavior and field types remain, without exposing credential fields through new management responses

#### Scenario: Restricted newly created user calls administrative or refresh methods
- **WHEN** an ACTIVE created user logs in and attempts management, RefreshSession or another method outside restricted Me/ChangePassword/Logout
- **THEN** permission denial preserves the restricted allowlist until password change succeeds

#### Scenario: Generated management contract is inspected
- **WHEN** the four methods/messages are generated from api-contracts proto sources
- **THEN** exact user.v1.UserService full methods and additive tags/types are available, UpdateUser preserves omitted versus selected-empty semantics, and existing fifteen methods retain their descriptors

#### Scenario: Management catalog errors cross the gRPC boundary
- **WHEN** a declared not-found, version/conflict/busy or status/last-admin error is encoded and decoded with supported locale
- **THEN** its catalog identity and gRPC mapping agree with the published contract without credential leakage or changed legacy error semantics

Evidence/traceability: [UM-01..08, UM-10 and UM-EXT-01](../../changes/archive/2026-10-11-auth-user-management/design.md#acceptance-ledger), M-05..08/10/11 and restricted subset AC-33/34. Status PASS; actual evidence pada [implementation ledger](../../changes/archive/2026-10-11-auth-user-management/evidence.md); backend owner grpc-auth, contract owner api-contracts. Contract creation/generation/tests/publication are scoped tasks 1.1–1.6; published dependency remains prerequisite EXT-01. Source inspection is not implementation evidence; [tasks](../../changes/archive/2026-10-11-auth-user-management/tasks.md) define required tests.

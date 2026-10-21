# Spec Delta

## Purpose

Menyediakan pembacaan permission berbasis role untuk diri sendiri dan target administrasi, beserta sumber role dan status akun dari satu snapshot authoritative tanpa menjadikan response sebagai tiket akses.

## ADDED Requirements

### Requirement: Self reads use verified identity and current unrestricted authority

`/user.v1.UserService/GetMyEffectivePermissions` SHALL require TLS, a verified service allowed for that exact method, and a current ACTIVE unrestricted user/session. The target SHALL come exclusively from verified principal identity; the request SHALL contain only optional `application_code`, never a target user ID. No administrative permission or IAM application-access grant SHALL be required. Current session-user binding, expiry, auth_version and restricted policy SHALL be rechecked in the response snapshot. JWT roles, role names or numeric role codes MUST NOT grant authority.

#### Scenario: Self reader has no administrative role
- **WHEN** an allowed service and active unrestricted user with no IAM admin role request their permissions
- **THEN** the RPC succeeds with that user's permitted role-derived union, including an empty union, without requiring iam.user.read

#### Scenario: Caller tries to redirect a self read
- **WHEN** metadata or untrusted request data claims a different user ID
- **THEN** the response target remains the verified principal and no other user's data is returned

#### Scenario: Caller becomes invalid after interception
- **WHEN** session revocation, expiry, account inactivation, auth_version mismatch or restriction is visible in the feature snapshot
- **THEN** the RPC returns UNAUTHENTICATED for invalid user/session or PERMISSION_DENIED for restriction, with no permission response

### Requirement: Administrative reads require exact permission before target disclosure

`/user.v1.UserService/GetUserEffectivePermissions` SHALL require TLS, verified exact-method service authorization, IAM namespace authority, current ACTIVE unrestricted actor/session and exact `iam.user.read` from that same snapshot. This permission SHALL also be required when target equals actor. Request `user_id` SHALL be uint64 in 1..2147483647; nil request or invalid ID SHALL produce INVALID_ARGUMENT. Only after actor authorization SHALL target existence/status and permission data be read. Missing or soft-deleted targets SHALL produce NOT_FOUND using the existing auth not-found catalog.

#### Scenario: Authorized cross-user read
- **WHEN** actor has exact iam.user.read and an existing nondeleted target is requested
- **THEN** the response contains that target's identity/status/version and permitted permission sources, rather than actor data

#### Scenario: Administrative permission is missing
- **WHEN** caller lacks iam.user.read, has only an admin role name/numeric code, or lacks required IAM service namespace
- **THEN** the RPC returns PERMISSION_DENIED without disclosing whether the target exists, including for a self target

#### Scenario: Missing or invalid target
- **WHEN** an authorized actor requests zero/out-of-range ID, or a valid missing/soft-deleted target
- **THEN** invalid ID returns INVALID_ARGUMENT and missing/soft-deleted target returns NOT_FOUND; neither is an empty success

### Requirement: Application filters enforce caller namespace boundaries

Both RPCs SHALL trim `application_code`; absent, empty and whitespace-only values SHALL mean all namespaces allowed for the verified service caller. Nonempty codes SHALL be lowercase application codes matching `^[a-z][a-z0-9_]*$`, at most 50 characters; malformed values and nil requests SHALL produce INVALID_ARGUMENT. An explicit namespace outside the service allowlist SHALL produce PERMISSION_DENIED before application/target lookup. Empty-filter responses SHALL omit disallowed namespaces. Valid allowed unknown/inactive applications SHALL yield an empty permission collection for an otherwise authorized existing target, without revealing catalog detail. IAM namespace authority required by the admin method SHALL remain independent of response filtering.

#### Scenario: Empty filter with partial caller authority
- **WHEN** a target has IAM and business contributions but caller is allowed only a subset of namespaces
- **THEN** only permission codes and sources within that subset are returned; omitted namespaces do not cause a partial-response error

#### Scenario: Explicit disallowed namespace
- **WHEN** the requested application namespace is not allowed for the verified service
- **THEN** PERMISSION_DENIED is returned before existence lookup, whether the code is registered or unknown

#### Scenario: Valid unknown or inactive application
- **WHEN** caller is allowed the requested namespace, target exists and application is unknown or DISABLED
- **THEN** the RPC succeeds with an empty permissions collection and safe target metadata

#### Scenario: Malformed application filter
- **WHEN** application_code contains wildcard, uppercase, dots, invalid characters or exceeds 50 characters after trimming
- **THEN** INVALID_ARGUMENT is returned without treating it as an unfiltered query

### Requirement: Role-derived unions preserve every currently contributing source

Both RPCs SHALL return each permission code once, with unique source role ID/string-code pairs. Only ACTIVE assignments and ACTIVE roles on ACTIVE applications SHALL contribute; business applications SHALL additionally require target application access ACTIVE. Role and permission application IDs and code namespace SHALL agree. IAM SHALL not require an IAM application grant. REVOKED assignments, DISABLED roles, wildcard, inheritance, direct grants, JWT claims and cached/event projections MUST NOT contribute. Permission codes SHALL sort ascending lexicographically and each source-role collection SHALL sort ascending by numeric role ID. Results SHALL be complete for the requested namespace scope, without pagination, silent truncation, search or caller-controlled projection/sort.

#### Scenario: Overlapping role contributions
- **WHEN** two active roles provide the same permission, including duplicate underlying join rows
- **THEN** one permission appears with both unique role sources in deterministic order

#### Scenario: One source is revoked
- **WHEN** one of two contributing assignments is revoked before the next snapshot
- **THEN** the shared permission remains with only the surviving role; permissions exclusive to the revoked role disappear

#### Scenario: Business gate or mapping is invalid
- **WHEN** business access is absent/REVOKED, application or role is DISABLED, assignment is REVOKED, or mapping crosses application boundaries
- **THEN** the affected rows contribute nothing even if old JWT claims contain those roles

### Requirement: Inactive targets expose descriptive active-role contributions only

An authorized admin SHALL be able to read nondeleted PENDING, SUSPENDED and DEACTIVATED targets. Response SHALL always include target `user_status`, identity and entity version. For those nonactive targets, the collection SHALL describe the union of still-ACTIVE assignments/roles under the same application/access gates, ignoring only target account status. It MUST NOT include REVOKED/DISABLED history or reconstruct earlier permission mappings from audit records. Such data SHALL NOT authorize login, refresh, a protected action or a target session; caller authentication and authoritative authorization rules remain unchanged. A response for any target is descriptive and MUST NOT be reused as an access grant.

#### Scenario: Suspended account retains active roles
- **WHEN** an authorized admin reads a SUSPENDED target with active assignments/roles and active business access
- **THEN** the response includes user_status SUSPENDED and those role-derived contributions, while that account's protected requests remain rejected

#### Scenario: Inactive target has revoked history
- **WHEN** PENDING/SUSPENDED/DEACTIVATED target has a mixture of ACTIVE and REVOKED assignments or DISABLED roles
- **THEN** only still-contributing ACTIVE assignments/roles appear; no revoked sources are reintroduced

#### Scenario: Target status changes between reads
- **WHEN** target status changes committed before the next admin read
- **THEN** status and role-derived view reflect the new snapshot without reactivating any session or changing stored assignments

### Requirement: Authority and response share one primary snapshot with safe failures

Actor state, admin permission when required, target identity/status/version and role contributions SHALL come from one consistent primary read snapshot. A new snapshot established after committed assignment/role/permission/application/access changes SHALL observe that state with existing valid credentials. An in-flight snapshot MAY finish using its previously consistent state; no cancellation-after-revoke guarantee is introduced. Dependency failure SHALL return UNAVAILABLE, never an empty success, stale-cache result or permission denial. Request cancellation/deadline SHALL preserve CANCELED/DEADLINE_EXCEEDED. Snapshot finalization failure or panic SHALL suppress the response, preserve safe error mapping and release resources.

#### Scenario: Policy changes during a read
- **WHEN** a concurrent transaction commits actor authority or target role/application changes between candidate reads
- **THEN** response and authorization represent one state that existed together; the next snapshot observes the commit, including denial if actor lost authority

#### Scenario: Empty result hides a failed dependency
- **WHEN** snapshot begin, actor read, target read, contribution read or finalization fails
- **THEN** no successful permission collection is returned; dependency failures map to UNAVAILABLE with localized/redacted catalog details

#### Scenario: Cancellation or panic
- **WHEN** request is canceled, its deadline expires or a dependency panics after snapshot creation
- **THEN** no partial response escapes, resources are cleaned up and the existing cancellation/deadline/internal error boundary is preserved

### Requirement: Additive contracts and protected-read admission preserve existing behavior

api-contracts SHALL own new protobuf source/generated output and compatible catalog mappings. The two new methods SHALL be added to UserService without changing existing tags/types/RPCs. The safe response SHALL contain target ID/status/entity version and permission/source pairs only; password/hash/access/refresh credential, session identifiers and private database details MUST NOT be exposed. Both methods SHALL use the existing shared verified-user protected-read group at 120/minute burst 20, with existing bounded capacity/TTL/cleanup and RESOURCE_EXHAUSTED/RATE_LIMITED RetryInfo. Admission SHALL count invalid input after verified identity and SHALL NOT replace current authorization. Default server inventory SHALL be exactly35 methods with all prior 33 preserved.

#### Scenario: Quota is exhausted across existing and new reads
- **WHEN** verified actor exhausts the shared protected-read quota through old/new methods
- **THEN** the new RPC returns RATE_LIMITED/RESOURCE_EXHAUSTED with bounded RetryInfo and performs no business read work after admission rejection

#### Scenario: Existing clients and credential modes
- **WHEN** existing clients invoke Register/Login/Me/ListUsers/TesRPC or lifecycle/B/C/D methods after E is registered
- **THEN** existing contracts, pagination/cursors, Register PENDING/token-empty, generic Login denial, restricted allowlist and EdDSA session rules remain compatible

### Requirement: Permission reads have no business mutation or persistent replay

Successful reads SHALL change no user/role/entity/auth_version counters, assignments/access, session/refresh state, audit-success record, outbox or idempotency result. No expected-version or idempotency key SHALL be required. Repeated calls, including after response loss or with incidental idempotency metadata, SHALL execute current reads rather than replay a saved permission view. Failed attempts SHALL retain existing independent safe failure audit after snapshot cleanup, without masking the original error; rate rejection SHALL retain bounded limiter telemetry rather than per-request database audit.

#### Scenario: Repeated read after response loss
- **WHEN** a successful response is lost and role policy changes before the caller retries
- **THEN** the retry returns the current permitted view, without replaying stale data or creating mutation/version/event effects

#### Scenario: Failure audit also fails
- **WHEN** the permission read fails and independent attempt persistence fails
- **THEN** the original read error is preserved, bounded telemetry records audit failure and no sensitive details or permission response are returned

## Traceability

Owner: grpc-auth except contract E (api-contracts). Planning baseline was NOT_STARTED; backend E criteria now PASS with [actual evidence](../../evidence/progress.md). Source inspection and artifact validation alone are not functional evidence. Prerequisites are implemented B/C/D and session/authorization/snapshot foundation; public transport additionally requires EXT-01 publication. During apply each criterion SHALL have actual command/mode/time/source and expected/actual evidence in the design ledger, with unavailable dependencies marked BLOCKED.

| Local criterion | Main requirement scope | Required evidence |
| --- | --- | --- |
| EPR-01 | M-02/03/05, AC-31 subset | Self/admin service/principal/restricted/ownership negatives and registered paths |
| EPR-02 | AC-09/10/13/23 subset | Union/source/gates, namespace filters and post-B/C/D committed changes |
| EPR-03 | User decision 14 October 2026; M-05/06 | Inactive target view/status, missing/deleted/invalid targets, credential redaction |
| EPR-04 | M-02/09 | Single primary snapshot, two-connection barriers, stale-actor/target visibility and pool cleanup |
| EPR-05 | M-05/06/10/11 | Exact35 inventory, baseline 33 regression, shared admission, RetryInfo/locale/failure audit |
| EPR-06 | M-09/10/11 | Required workspace/standalone and PostgreSQL normal/race gates with actual evidence |
| EPR-EXT-01 | EXT-01 | Additive tags/descriptors, generator/tests/static checks, publication/module/checksum and GOWORK=off resolution |

AC-13 here covers permission/application-gate visibility only; public EvaluateAccess and CLM resource decisions remain separate. AC-09/10/23 cover role-change visibility, not reimplementation of their mutations. Existing access 15m/session 7d absolute/skew 30s/audience-ignored/EdDSA-only, strict refresh and restricted Me/ChangePassword/Logout are explicit preserved decisions. Actual consuming-client/key/TLS/schema deployment readiness stays mandatory Phase 6 DEP-01, outside backend E completion.

# access-evaluation Specification

## Purpose

Menyediakan keputusan akses untuk satu permission milik user pemanggil berdasarkan identitas, sesi dan policy primary yang konsisten, dengan batas namespace caller serta hasil deny yang dibedakan dari kegagalan autentikasi atau dependency.

## Requirements

### Requirement: EvaluateAccess requires a verified service and the current unrestricted user

`/authorization.v1.AuthorizationService/EvaluateAccess` SHALL require TLS, a verified opaque service credential allowed for that exact method, and an ACTIVE unrestricted user with a currently valid bound session. The target SHALL come exclusively from the verified user identity; the request SHALL NOT contain `user_id`, `sid` or another target identity. Server-owned caller policy SHALL authorize the requested application namespace before application/access/permission data lookup. Missing namespace authority SHALL produce PERMISSION_DENIED, never a boolean deny. No separate IAM namespace, `iam.user.read`, administrative role or numeric role code SHALL be required to evaluate an allowed business namespace. Current account/session status, deletion, expiry, auth_version and restriction SHALL be checked again in the decision snapshot.

#### Scenario: Business caller has no IAM administration authority

- **WHEN** a service allowed for the exact method and `clm` namespace presents valid unrestricted user credentials and requests a `clm` permission
- **THEN** evaluation proceeds for that user without requiring IAM service authority, IAM application access or an administrative permission

#### Scenario: Service method or namespace is disallowed

- **WHEN** an otherwise authenticated service lacks the exact method or requested namespace in its configured allowlist
- **THEN** PERMISSION_DENIED is returned without application/access/permission disclosure, regardless of whether the application exists or the user has that permission

#### Scenario: User identity is invalid or restricted

- **WHEN** credentials are absent/ambiguous/invalid, account or session is inactive/deleted/revoked/expired, session-user binding or auth_version differs, or the user becomes restricted before the decision snapshot
- **THEN** identity/session failures return UNAUTHENTICATED and restriction returns PERMISSION_DENIED, with no decision response

#### Scenario: Untrusted metadata claims another identity

- **WHEN** payload metadata supplies a user ID, role name, numeric admin role code or service identity different from verified credentials
- **THEN** it cannot redirect the evaluation or grant authority; any response identity remains the user and session validated in the decision snapshot

### Requirement: Evaluation inputs identify one exact permission in one namespace

The request SHALL require string `application_code` and `permission_code`, trimming surrounding whitespace without changing case. Application code SHALL match `^[a-z][a-z0-9_]*$` and be 1–50 characters. Permission code SHALL match `^[a-z][a-z0-9_]*([.][a-z][a-z0-9_]*){2,}$` and be 1–150 characters; its first segment SHALL equal application code. Nil request, blank required value, uppercase/invalid character, wildcard, empty segment, overlength or namespace mismatch SHALL produce INVALID_ARGUMENT through existing shared validation catalogs. Absent application SHALL NOT mean all namespaces. Syntactically valid unknown codes SHALL remain eligible for a boolean denial under the decision rules.

#### Scenario: Multi-segment resource is valid

- **WHEN** an authorized caller requests application ` iam ` and permission ` iam.user.status.update `
- **THEN** the exact normalized permission `iam.user.status.update` is evaluated without limiting codes to three segments

#### Scenario: Input is missing malformed or inconsistent

- **WHEN** the request is nil, either required value is blank, codes contain wildcard/uppercase/invalid segments, limits are exceeded, or application `clm` is paired with `iam.user.read`
- **THEN** INVALID_ARGUMENT is returned without a fabricated decision or interpretation as an unfiltered request

### Requirement: Decision reasons follow current application access and exact role-derived authority

For a valid active unrestricted identity and authorized namespace, the RPC SHALL return a decision using this precedence: unknown application → `allowed=false/PERMISSION_MISSING`; registered DISABLED application → `false/APPLICATION_INACTIVE`; ACTIVE business application with absent or REVOKED user application access → `false/APPLICATION_ACCESS_REQUIRED`; otherwise absent or unknown exact effective permission → `false/PERMISSION_MISSING`; exact effective permission present → `true/ALLOWED`. Unknown application behavior follows the user's 18 October 2026 decision. IAM SHALL bypass only the user application-access grant gate, not ACTIVE application or exact permission checks. Invalid persisted state or failed dependency SHALL NOT be interpreted as a domain absence or denial.

Only ACTIVE assignments, ACTIVE roles and ACTIVE applications with matching role/permission application IDs and namespace SHALL contribute. Business contributions SHALL additionally require ACTIVE user application access. Role-permission mappings with non-NULL `deleted_at` SHALL NOT contribute. Union semantics SHALL preserve access from any remaining valid source. Wildcards, inheritance, direct grants, JWT roles, numeric admin codes, cached decisions and events SHALL NOT grant access.

#### Scenario: Application is unknown or disabled

- **WHEN** an allowed namespace is syntactically valid but its application is unregistered, or the registered application is DISABLED
- **THEN** the unknown application returns false/PERMISSION_MISSING and the DISABLED application returns false/APPLICATION_INACTIVE after identity validation, without NOT_FOUND or dependency-error substitution

#### Scenario: Application access fails before permission membership

- **WHEN** an ACTIVE business application has absent or REVOKED access for the verified user, including when its requested permission is unknown or an old role remains assigned
- **THEN** the result is false/APPLICATION_ACCESS_REQUIRED, and another application or session is not changed

#### Scenario: Unknown permission and ordinary missing privilege

- **WHEN** the application/access gates pass but the requested exact permission is unknown or supplied by no eligible source
- **THEN** the result is false/PERMISSION_MISSING without a transport permission error, wildcard match or role-name bypass

#### Scenario: Overlapping source survives revocation

- **WHEN** two valid roles supply the same permission and one assignment or mapping is revoked or soft-deleted before the next evaluation
- **THEN** the permission stays allowed through the remaining eligible role; it becomes PERMISSION_MISSING when no eligible source remains

#### Scenario: IAM evaluation has no application grant

- **WHEN** the verified user has the requested exact IAM permission on an ACTIVE IAM application but no IAM application-access record
- **THEN** the result is true/ALLOWED; the same user without that exact permission receives false/PERMISSION_MISSING

#### Scenario: Soft-deleted or mismatched mapping is the only source

- **WHEN** the only candidate mapping is soft-deleted, has a DISABLED role, REVOKED assignment, inconsistent application IDs or a namespace inconsistent with its application
- **THEN** it grants no permission even when the access JWT remains valid or a disabled role is enabled again without restoring the mapping

### Requirement: Decision and current identity come from one primary snapshot

Account/session identity, status, entity version, application/access gates and permission contributions SHALL describe one consistent primary state. A snapshot established after a committed policy/session/account change SHALL observe that change. An in-flight snapshot SHALL remain internally consistent and is not a guarantee of cancellation after later revocation. Current valid credentials SHALL follow role/application/permission changes without requiring auth_version increment or session revocation for those privilege changes. Suspended/deactivated accounts and globally revoked/password-changed credentials SHALL remain rejected; reactivation SHALL NOT restore old sessions.

#### Scenario: Original token is used after policy mutations

- **WHEN** registered role revoke, role disable/enable, permission replacement/soft-delete, application revoke or subset regrant commits before a new evaluation with the original still-valid JWT
- **THEN** the decision reflects the latest eligible sources/gates, including overlap preservation and subset restoration, without evaluator changes to session, refresh credentials or auth_version

#### Scenario: Commit occurs between candidate state reads

- **WHEN** a concurrent transaction commits an assignment, application/access, permission mapping or actor session/status change during an evaluation
- **THEN** identity/version and decision correspond to one state that existed together; a later snapshot observes the committed policy or authentication failure, with no mixed-state allow

#### Scenario: Administrative display does not authorize an inactive user

- **WHEN** an inactive target still has descriptive active-role contributions visible through the existing administrative permission-read API
- **THEN** those contributions do not make EvaluateAccess succeed for that account or restore revoked credentials

### Requirement: Infrastructure failures return errors and suppress decisions

Dependency failures during snapshot start, identity/application/access/permission read or finalization SHALL produce UNAVAILABLE with safe localized shared catalog details, never false deny, empty success, stale-cache allow or a partial decision. Request cancellation and deadline SHALL preserve CANCELED and DEADLINE_EXCEEDED. Unexpected panic SHALL produce redacted INTERNAL. All acquired read resources SHALL be released before returning or recording an independent attempt. Error identity/cause SHALL remain available to the existing error boundary without exposing database errors, credentials or stack traces.

#### Scenario: Any decision dependency fails

- **WHEN** a required lookup or snapshot finalization fails because its dependency is unavailable, including after an allowed or denied candidate was computed
- **THEN** the RPC returns UNAVAILABLE with no decision response and does not misclassify the error as application/permission absence

#### Scenario: Cancellation deadline or panic interrupts evaluation

- **WHEN** evaluation is canceled, exceeds its deadline or encounters a panic after opening a snapshot
- **THEN** it returns the matching cancellation/deadline/internal error, releases the snapshot and permits reuse of the primary pool without partial response or business writes

### Requirement: Additive safe responses are descriptive and never reusable access tickets

api-contracts SHALL own the new protobuf source/generated output, reason enum and shared error compatibility. New request tags SHALL be `application_code=1 string`, `permission_code=2 string`; response tags SHALL be `allowed=1 bool`, `reason=2 enum`, `user_id=3 uint64`, `sid=4 string`, `user_version=5 uint64`, `evaluated_at=6 google.protobuf.Timestamp`. Reason enum values SHALL be UNSPECIFIED=0, ALLOWED=1, PERMISSION_MISSING=2, APPLICATION_INACTIVE=3 and APPLICATION_ACCESS_REQUIRED=4 using the `ACCESS_DECISION_REASON_` prefix. The server SHALL NOT return UNSPECIFIED; allowed=true SHALL pair only with ALLOWED and each false response only with one denial reason.

Both true and false responses SHALL contain the snapshot-verified user ID, canonical session UUID, positive current user entity version and injected evaluation instant in UTC. Responses SHALL NOT contain password/hash/access/refresh credentials, a full role/permission list or private database details. User version SHALL NOT be advertised as a complete authorization-policy revision. Each call SHALL evaluate anew; response loss, retries and incidental idempotency metadata SHALL NOT replay a saved decision. Consumers SHALL still authorize the resource/action in their own domain and SHALL NOT treat this response as a cacheable access ticket.

#### Scenario: Allowed and denied results expose only verified safe fields

- **WHEN** evaluation succeeds with either a true or false decision
- **THEN** reason and identity/version/time match that evaluation, with no credential or unverified user identity, and user_version is not used as a freshness guarantee for future role changes

#### Scenario: Response loss is followed by a policy change

- **WHEN** a prior evaluation response is lost, policy then changes and the caller retries with the same incidental idempotency metadata
- **THEN** the retry returns a fresh current decision without retrieving or writing a saved result, extending a session or issuing credentials

### Requirement: Evaluation shares bounded protected-read admission

The new method SHALL share the existing verified-user protected-read group at 120/minute, burst 20, with the existing capacity of 10,000 entries, idle TTL fifteen minutes and one-minute cleanup. Admission SHALL count invalid input after verified identity, preserve bounded RESOURCE_EXHAUSTED/RATE_LIMITED RetryInfo and locale, and SHALL NOT replace current feature authorization. Unverified users SHALL NOT allocate protected-read entries. There SHALL be no independent bucket keyed by arbitrary application/permission strings or a new runtime service dependency.

#### Scenario: Existing and new reads consume one quota

- **WHEN** a verified actor exhausts the shared read burst using old methods and EvaluateAccess, including invalid EvaluateAccess inputs
- **THEN** subsequent EvaluateAccess receives RESOURCE_EXHAUSTED with RetryInfo before decision work, and existing bucket capacity/cleanup behavior remains intact

#### Scenario: Quota is available but authority is invalid

- **WHEN** a caller has quota but is restricted or its current session or service namespace is invalid
- **THEN** admission cannot turn that failure into an allowed or boolean-denied authorization result

### Requirement: Denial auditing preserves decisions without business mutation

Allowed reads SHALL NOT change accounts, assignments, application access, versions/auth_version, session/refresh state, success audit, outbox or idempotency results. False decisions SHALL change none of that business state and SHALL attempt exactly one independent safe denied-attempt audit after read cleanup, using the verified service actor, RPC target, existing SECURITY_ATTEMPT_FAILED action and REQUEST_DENIED reason. No raw request, requested permission, session identifier or credential SHALL be copied into that audit. gRPC errors SHALL retain the existing independent safe failure audit once, without a duplicate false-result audit. Audit persistence failure/panic SHALL preserve the original false decision or gRPC error and update only the existing bounded failure observation. Rate rejection SHALL retain bounded limiter telemetry without per-rejection database audit.

#### Scenario: Transport succeeds with a false access decision

- **WHEN** EvaluateAccess returns allowed=false with any valid denial reason
- **THEN** the read snapshot is already finished before one independent safe denied-attempt audit is attempted, and no business mutation/outbox/idempotency result is created

#### Scenario: Independent audit fails or panics

- **WHEN** the denied/error attempt audit cannot be persisted or its dependency panics
- **THEN** the original false decision or error remains unchanged, access is never granted, bounded audit-failure observation increases and all audit resources are cleaned up

#### Scenario: Allowed rate-rejected and failed requests have distinct effects

- **WHEN** calls respectively produce allowed=true, quota rejection or a feature error
- **THEN** allowed creates no audit-success write, quota rejection creates no database attempt, and the error path records at most its one existing safe independent attempt after cleanup

### Requirement: Registration preserves all existing secure RPC behavior

The default server SHALL register exactly 36 methods across eight services by adding AuthorizationService/EvaluateAccess to the prior 35-method/seven-service inventory. Existing field tags/types/RPCs, TesRPC, pagination/cursors, Register PENDING/token-empty behavior, generic Login denial, credential-only Login/Refresh responses, EdDSA-only issuer/session rules, access lifetime fifteen minutes bounded by absolute seven-day session and thirty-second skew with ignored audience SHALL remain unchanged. Restricted operations SHALL remain limited to Me, ChangePassword and Logout. No runtime allowlist wildcard, legacy credential fallback or deployment/migration action SHALL be introduced by this increment.

#### Scenario: Existing callers invoke prior operations

- **WHEN** the new service is registered and clients invoke any of the prior 35 methods
- **THEN** descriptors, credential modes, protected permissions, restricted policy, response shapes and pagination/cursor behavior remain compatible

#### Scenario: Deployment readiness is incomplete

- **WHEN** backend evaluator tests pass but consuming-client or deployment readiness has not passed Phase 6 DEP-01
- **THEN** backend evidence does not claim deployment readiness or authorize migration, credential invalidation or release

## Traceability

Acceptance IDs, scope/repository owners, prerequisites, required tests/evidence and NOT_STARTED/BLOCKED proposal statuses are defined in the [EVA ledger](../../changes/archive/2026-10-18-auth-evaluate-access/design.md#8-acceptance-dan-evidence-ledger). EVA-01..07 cover this RPC's portions of AC-09/10/11/12/13/23/27/31 and M-01/02/03/05/06/07/09/10/11; EVA-EXT-01 tracks the external additive contract. These retain the meanings of the [MVP criteria](../../../docs/grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence). Default controlled tests and separate dedicated opt-in PostgreSQL snapshot/cleanup evidence are both required where identified. Planning artifacts, source inspection, skipped database suites and prior unrelated tests SHALL NOT be reported as this evaluator's acceptance PASS. Full Phase 4 reconciliation, ListAuditLogs, Phase 5 hardening and Phase 6 DEP-01 remain separate.

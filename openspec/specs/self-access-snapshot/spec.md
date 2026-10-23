# self-access-snapshot Specification

## Purpose

Menyediakan profil dan akses pengguna login melalui satu Me yang konsisten dengan state primary dan namespace service caller, tanpa menjadikan response sebagai credential atau bukti izin yang dapat dikirim ulang oleh browser.

## Requirements

### Requirement: Me reads only the currently authenticated user

`/user.v1.UserService/Me` SHALL retain request `google.protobuf.Empty` and require TLS, verified `x-service-token` authorized for that exact method, and a valid user JWT/session. The response target MUST come only from verified principal identity. Account ACTIVE, nondeleted user, session ACTIVE, session ownership, expiry and auth_version SHALL be rechecked in the response snapshot. Me SHALL require neither `iam.user.read`, an administrative role, nor an IAM application-access grant. The request SHALL add no target user ID or application filter.

#### Scenario: Self read without administrative permission

- **WHEN** an allowed service calls Me with an active user's valid session and no administrative role or IAM application grant
- **THEN** Me returns that user's permitted profile/access view without requiring `iam.user.read`

#### Scenario: Untrusted metadata claims another identity

- **WHEN** browser-supplied metadata includes a different user ID, roles, permissions or service identity
- **THEN** those values do not select the target or grant authority; only the verified user and service determine the response

#### Scenario: User or session invalidates after interception

- **WHEN** the response snapshot observes inactive/deleted user, missing/revoked/expired or cross-user session, or mismatched auth_version
- **THEN** Me returns generic UNAUTHENTICATED using existing catalog mapping, without profile or permission response

#### Scenario: Caller authentication or method admission fails

- **WHEN** TLS/service credential validation or exact-method service authorization fails
- **THEN** existing service-authentication denial applies and Me does not disclose user data

### Requirement: Me exposes a compatible safe user and permission contract

api-contracts SHALL own `user.v1.UserData` source and generated output. Me SHALL preserve existing field numbers and types 1 through 14 and add exactly `map<string, bool> permissions = 15`. Successful unrestricted Me SHALL populate safe identity/profile, account `status`, positive user entity `version`, `must_change_password`, `created_at` from user creation time, `country_code`, role/application collections, and effective permissions. Each present permission entry SHALL be `true`; absent keys SHALL mean not granted. An empty result after successful authoritative reads SHALL contain no granted keys. No password/hash, access/refresh credential, session ID, private caller credential or auth_version SHALL be added. Entity version MUST NOT be interpreted as a permission-policy version.

#### Scenario: Complete unrestricted response

- **WHEN** an unrestricted valid user with profile, active scoped roles/application access and effective permissions calls Me
- **THEN** all safe fields come from the current snapshot, `created_at` is user creation time, and every granted permission code maps to `true`

#### Scenario: No effective permissions

- **WHEN** all required reads succeed but no contribution passes the current gates
- **THEN** Me succeeds with safe user metadata and no granted permission keys, without inventing `false` entries or default permissions

#### Scenario: Old and new protobuf consumers

- **WHEN** an old descriptor reads an enriched response or a new descriptor performs a binary/proto-JSON roundtrip
- **THEN** fields 1 through 14 keep their tags/types, old consumers can ignore field 15, and new consumers preserve the permission map without credential exposure

### Requirement: Privilege collections honor caller namespaces and active-source gates

Unrestricted Me SHALL include every effective permission in namespaces allowed by the verified service caller, without client application selection, pagination or silent truncation. Permission union SHALL include only ACTIVE assignments, ACTIVE roles, ACTIVE applications and nondeleted role-permission mappings with matching role/permission application IDs and exact permission namespace. Business permissions SHALL additionally require ACTIVE target application access; IAM SHALL not require an IAM application grant. Wildcards, role names/numeric codes, JWT role snapshots, direct grants, hierarchy, stale cache or events MUST NOT contribute.

New `roles` metadata SHALL include unique ACTIVE assignments on ACTIVE roles/applications in allowed namespaces, with ACTIVE business application access where applicable, including eligible roles that have no permission mapping. `application_access` SHALL include unique ACTIVE grants on ACTIVE allowed applications even when no permission is granted. Collections SHALL have deterministic ordering by numeric role/application ID. Legacy `role_codes` and `role_names` SHALL preserve their existing numeric/name metadata mapping and restricted suppression; they MUST NOT grant authority or replace the scoped collections.

#### Scenario: Caller allows only one application namespace

- **WHEN** a user has valid IAM and CLM contributions but the service caller allows only CLM
- **THEN** permission map, new roles and application-access collections expose only allowed CLM data; no IAM entry is inferred from legacy role metadata

#### Scenario: Overlapping permission contributions

- **WHEN** two eligible roles provide the same permission and one source is revoked or its mapping is soft-deleted
- **THEN** the permission appears once with `true` while a valid source remains and disappears after its last valid source is removed

#### Scenario: An application or mapping gate fails

- **WHEN** assignment is REVOKED, role/application is DISABLED, business access is absent/REVOKED, mapping is soft-deleted or crosses namespace/application boundaries
- **THEN** that contribution grants no permission, even if old JWT claims or role metadata contain it

#### Scenario: Safe collections exist without permissions

- **WHEN** an allowed ACTIVE business application grant and eligible ACTIVE role have no contributing permission mappings
- **THEN** their safe metadata remains visible in the relevant new collections while the permission map remains empty

### Requirement: Restricted Me returns profile minimum without privilege collections

When current primary state has `must_change_password=true`, Me SHALL remain allowed for a valid ACTIVE user/session and return only safe self identity/profile/status/version/creation time plus the restriction marker. It SHALL return no legacy or new role collection, application-access collection or granted permission key. Me SHALL not require privilege-data reads to produce this minimum. The restricted allowlist SHALL remain exactly Me, `/auth.v1.AuthService/ChangePassword`, and `/auth.v1.AuthService/Logout`.

#### Scenario: Restricted user still has stored assignments

- **WHEN** a restricted valid user calls Me while roles and app grants exist in the database
- **THEN** `must_change_password` is true, safe profile fields are returned and all privilege collections remain empty

#### Scenario: Privilege dependencies are unavailable for a restricted user

- **WHEN** profile/session reads succeed for a restricted user but an optional privilege reader would fail if invoked
- **THEN** Me returns the minimum without invoking that privilege reader or exposing stored privilege data

### Requirement: Me combines profile and privileges in one failure-safe snapshot

Me SHALL recheck user/session and obtain identity, profile, legacy metadata, new collections and permission contributions from one consistent primary read-only snapshot. A snapshot established after committed access changes SHALL reflect those changes with existing still-valid credentials; an earlier in-flight snapshot can finish with its consistent earlier state. Me SHALL make no separate public permission RPC to assemble the response.

Begin/read/finalization dependency failures SHALL return UNAVAILABLE via existing localized/redacted catalogs, never partial success, stale-cache success or an empty permission map as fallback. Request cancellation/deadline SHALL preserve CANCELED/DEADLINE_EXCEEDED. Panic SHALL suppress any response, preserve the safe INTERNAL boundary and release snapshot resources. Cleanup failure SHALL not turn a failed read into success.

#### Scenario: Privilege changes during an in-flight read

- **WHEN** another database connection commits role/application/mapping changes between candidate Me reads
- **THEN** one Me response contains values from a single state that existed together, and the next snapshot observes the committed change

#### Scenario: Enrichment or finalization fails

- **WHEN** identity, profile, role, application or permission read fails, or snapshot finalization fails
- **THEN** no UserData response escapes; dependency errors map to UNAVAILABLE rather than a successful empty map

#### Scenario: Request cancellation deadline or panic

- **WHEN** cancellation, deadline expiry or panic occurs after snapshot creation
- **THEN** Me exposes no partial response, retains the existing context/internal error category and releases transaction/connection resources

### Requirement: Me phone is a literal calling-code concatenation

For Me only, field `phone` SHALL equal stored `country_code` plus stored `phone`, treating NULL country code as empty. `country_code` SHALL remain separately populated. This explicitly approved semantic change SHALL apply to legacy rows without trimming, prefix removal, inserted separators, country-code defaults, deduplication or database writes. Other profile/detail/list response mappings SHALL keep their existing phone behavior.

#### Scenario: Separate country code and national digits

- **WHEN** stored country code is `+62` and phone is `81234567890`
- **THEN** Me returns phone `+6281234567890` and country code `+62`

#### Scenario: Legacy phone starts with zero or already contains calling code

- **WHEN** stored country code is `+62` and phone is `081234567890` or `+6281234567890`
- **THEN** Me returns `+62081234567890` or `+62+6281234567890` respectively, leaving stored data unchanged

#### Scenario: Country code is null or empty

- **WHEN** stored country code is NULL or empty and phone is `+6281234567890`
- **THEN** Me returns phone `+6281234567890` and an empty country code without adding a default

#### Scenario: Another RPC reads the same phone

- **WHEN** GetUser, ListUsers or profile response mapping reads that stored record
- **THEN** its existing phone semantics remain unchanged; the Me concat rule is not applied globally

### Requirement: Enriched Me remains a current read without mutation or cached grant

Successful Me SHALL create no mutation, version/auth_version increment, session/refresh change, success audit, outbox or idempotency result. It SHALL require neither expected-version nor idempotency key. Response loss or repeated invocation SHALL perform a fresh read, not replay saved UserData. Existing independent failed-attempt audit, limiter admission/RetryInfo, locale, context and registered method inventory SHALL remain unchanged. Existing permission/evaluation RPCs SHALL remain available with their current request/response behavior. Browser-supplied permission maps MUST NOT authorize protected backend RPCs.

#### Scenario: Response is lost and access changes before retry

- **WHEN** a successful Me response is lost, a role is revoked and caller retries with the same still-valid credential and incidental idempotency metadata
- **THEN** the retry reads current state without replay, mutation or new version/event effects

#### Scenario: Frontend replays an old permission map

- **WHEN** a caller sends a formerly granted permission map after access is revoked
- **THEN** protected grpc-auth operations still apply their current authoritative checks; map contents and role names cannot bypass denial

#### Scenario: Existing methods are called after enrichment

- **WHEN** clients call GetMyEffectivePermissions, GetUserEffectivePermissions, EvaluateAccess, Register/Login/TesRPC, profile/detail/list or lifecycle methods
- **THEN** their contracts, credential modes and permissions remain unchanged; Register stays PENDING/token-empty and existing pagination is preserved

## Traceability

MAS-EXT-01 and MAS-01..06 have PASS implementation evidence for the local workspace in [progress](../../changes/archive/2026-10-23-enrich-me-access-snapshot/evidence/progress.md); publication/standalone remains separate. Proposal/spec validation is not functional evidence. Each implementation evidence entry MUST record source/module versions, command, environment, time, expected/actual result and failures; unavailable required dependencies are BLOCKED.

| Criterion | Scope / repository | Main AC / prerequisite | Required test/evidence | Status |
| --- | --- | --- | --- | --- |
| MAS-01 | Self identity/session/restricted, grpc-auth | M-02/03/05/06; AC-14/31/33 subsets; existing session/service authentication | Registered self-only reads, no-admin case, caller/session negatives, minimum restricted response | PASS |
| MAS-02 | Scoped union/collections, grpc-auth | AC-09/10/11/13/23 subsets; existing resolver and active/soft-delete gates | Namespace subset, overlap, role/app/mapping exclusion and metadata without permissions | PASS |
| MAS-03 | Snapshot/cleanup, grpc-auth | M-07/09; AC-27 subset; primary snapshot provider | Default failure tests plus two-connection PostgreSQL barrier tests, cancellation/panic/finalization and pool cleanup | PASS |
| MAS-04 | Phone concat, grpc-auth | Explicit user plan; existing nullable country code | All literal concat examples and unchanged profile/detail/list/database values | PASS |
| MAS-05 | Read-only/compatibility, grpc-auth | M-06/10/11; protected-read admission and independent failed audit | No side effects/replay, existing method regression, redaction/locale/rate behavior | PASS |
| MAS-EXT-01 | Field 15 and generated contract, api-contracts | EXT-01; contract owner implementation | Descriptor tags/types, binary/proto-JSON map roundtrip, generator and contract suite | PASS |
| MAS-06 | Workspace verification, grpc-auth + api-contracts; rest-orc regression only | MAS-EXT-01; compatible local workspace | Targeted/full tests, vet, targeted race and consumer regression with exact version evidence | PASS |
| MAS-EXT-02 | Published version and standalone, contract/service owners | Separate publication/pin request | Published module/checksum and grpc-auth `GOWORK=off` verification after version exists | DEFERRED_TO_SEPARATE_REQUEST |

AC references are subsets only; this change does not reimplement mutations or certify all MVP acceptance criteria. Existing EdDSA-only/access 15m/session 7d absolute/skew 30s/audience-ignored rules remain explicit preserved policy. Role/app/permission changes do not revoke sessions or increase auth_version; global invalidation operations keep their existing behavior. Actual consumer/deployment readiness remains mandatory [Phase 6 DEP-01](../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment), outside backend increment completion.

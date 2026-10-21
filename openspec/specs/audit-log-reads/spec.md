# audit-log-reads Specification

## Purpose

Menyediakan pembacaan audit administratif yang aman bagi pemegang permission saat ini, dengan filter exact, pagination terbaru yang deterministik, dan satu snapshot primary untuk authority serta hasil riwayat.

## Requirements

### Requirement: Audit history requires current verified administrative authority

`/audit.v1.AuditService/ListAuditLogs` SHALL require TLS, an opaque verified service credential allowed for the exact method and IAM namespace, and an ACTIVE unrestricted verified user with a valid bound current session and exact effective `iam.audit.read` permission. Account/session status, deletion, expiry, binding, auth_version, restriction and effective permission SHALL be rechecked in the same primary snapshot as the result. Permission SHALL use ACTIVE assignment/role/application and non-soft-deleted mappings; numeric role codes, role names, JWT roles, cached decisions or supplied metadata SHALL NOT grant authority. IAM application-access grant, `iam.user.read` and `iam.session.read` SHALL NOT be additional prerequisites. Historical actor filters SHALL NOT replace the verified caller identity or constrain history to the caller's own records.

#### Scenario: Auditor reads another user's historical events

- **WHEN** a valid IAM caller and unrestricted actor have `iam.audit.read` without user-read or session-read permission and filter another user's audit history
- **THEN** safe matching audit records can be returned without granting access to user/session administrative APIs

#### Scenario: Caller or current actor lacks authority

- **WHEN** service credentials or user/session are invalid, service method/IAM namespace is disallowed, the user is restricted, or current exact permission is absent
- **THEN** credential/session failure returns UNAUTHENTICATED and restriction/authority failure returns PERMISSION_DENIED without audit records, totals or target-existence disclosure

#### Scenario: Numeric administrator or spoofed actor is presented

- **WHEN** payload/metadata asserts another caller/user or a privileged role name/numeric code without current `iam.audit.read`
- **THEN** the request remains denied and neither actor filters nor cursor can establish caller authority

### Requirement: Audit filters are exact optional historical selectors

The request SHALL support optional actor_user_id presence, actor_service, action, target_type, target_id, occurred_from and occurred_before filters. Present actor_user_id SHALL be positive and at most2147483647; present zero SHALL be INVALID_ARGUMENT. String filters SHALL trim surrounding whitespace, treat empty as absent, preserve case, reject invalid UTF-8/control characters and enforce maximum100 characters for actor_service/action/target_id and50 for target_type. target_id SHALL require target_type; target_type alone SHALL be valid. All supplied filters, including both actor selectors, SHALL combine with AND and SHALL NOT imply live actor/target lookup. Well-formed unknown values SHALL return no matches, not NOT_FOUND; percent/underscore/SQL-shaped text SHALL be literal parameter values.

Time bounds SHALL be optional valid protobuf timestamps normalized to UTC, selecting `occurred_at >= occurred_from` and `occurred_at < occurred_before` with equivalent comparison at database timestamp precision. When both bounds are present, from SHALL precede before. No absent bound SHALL imply a new rolling window. Target identifiers SHALL remain bounded historical strings, supporting RPC method names and session UUIDs as well as user/role identifiers.

#### Scenario: Exact selectors combine across actor action target and time

- **WHEN** actor user/service, action, target pair and both timestamps are supplied
- **THEN** each record must satisfy every selector, lower-bound equality is included and upper-bound equality is excluded without joining current user or session state

#### Scenario: Filters are malformed or target type is missing

- **WHEN** actor_user_id is present zero/out of INTEGER range, strings exceed limits/contain invalid text, target_id lacks target_type, timestamp is invalid or from is not before before
- **THEN** INVALID_ARGUMENT returns with no fabricated result

#### Scenario: Filter matches no historical record

- **WHEN** an authorized valid query uses absent filters or well-formed unknown exact values, including literal wildcard characters
- **THEN** absent filters impose no constraint and unknown exact values produce safe empty items with total/total_pages zero

### Requirement: Minimal fixed pagination preserves BIGINT audit identity

The RPC SHALL reuse PageQuery/PageMeta only for page, limit and last_id. Nil whole request SHALL be INVALID_ARGUMENT; nil query/filter SHALL select defaults. Default page SHALL be1 and limit20; negative page/limit, limit above100 and offset overflow SHALL be INVALID_ARGUMENT. Nonempty fields or sort and any present search SHALL be INVALID_ARGUMENT, including a custom sort identical to the fixed order. Results SHALL always sort occurred_at DESC then id DESC; audit IDs SHALL support positive signed-BIGINT range1..9223372036854775807 independently of INTEGER user-ID bounds.

Without a cursor, page>1 SHALL apply deterministic offset. last_id SHALL accept only a canonical versioned opaque cursor of at most4096 bytes containing the fixed-order timestamp/ID tuple and bound to the full method, ordering and normalized filters. Malformed/noncanonical/unknown-field/trailing-data/type-overflow/numeric legacy/cross-method or filter-mismatched cursors SHALL be INVALID_ARGUMENT. With a cursor, lexicographic keyset pagination SHALL replace offset; page remains requested/default metadata. Changing page or limit alone SHALL be allowed with the same cursor. Cursors SHALL NOT establish authority or preserve a snapshot across calls.

Total SHALL count all matching audit records before cursor/offset/limit in that call's snapshot. Total_pages SHALL be ceiling(total/limit); has_next SHALL reflect an additional matching row; has_prev SHALL reflect a supplied cursor or page>1. next_cursor SHALL be nonempty only when another page exists. Unrepresentable metadata SHALL suppress the whole response with safe UNAVAILABLE rather than numeric wraparound.

#### Scenario: Equal timestamps span pages with large IDs

- **WHEN** multiple events share occurred_at and IDs exceed2147483647, including the largest supported BIGINT
- **THEN** descending ID breaks ties and successive cursor pages on unchanged data neither duplicate nor omit rows or truncate identity

#### Scenario: Unsupported query capability or cursor is sent

- **WHEN** fields/sort/search is supplied or cursor is malformed, numeric, from another method or bound to different filters
- **THEN** INVALID_ARGUMENT returns instead of silently ignoring the option or restarting pagination

#### Scenario: Offset cursor total and terminal metadata are observed

- **WHEN** an authorized client requests offset pages, follows a cursor with a different limit, or reaches a terminal/empty page
- **THEN** each mode follows the fixed order, total remains pre-cursor, page/limit metadata is preserved, and terminal next_cursor is empty

### Requirement: Authority total and history describe one primary state

Current caller account/session/permission, matching total, item data and cursor values SHALL describe one consistent primary snapshot. Historical actors/targets SHALL remain readable without requiring current ACTIVE status or current ownership. A snapshot established after committed actor permission/session/account revocation SHALL observe it. An already established snapshot SHALL be internally consistent without promising interruption after a later revocation. Newly committed audit entries SHALL appear only according to the state visible to each call; no multi-page frozen export SHALL be promised. Reads SHALL NOT acquire mutation locks or mutate business state.

#### Scenario: Audit insert and permission revoke commit during listing

- **WHEN** a concurrent transaction inserts audit records and revokes the actor's permission between candidate authority/count/page operations
- **THEN** the in-flight result and authority match one state that existed together, and the next fresh call observes the committed permission failure rather than a mixed-state page

#### Scenario: Historical user or session is inactive

- **WHEN** the authorized auditor selects audit records whose actor/target is suspended, deactivated, revoked or otherwise no longer eligible to authenticate
- **THEN** safe historical records remain selectable without restoring any session or privilege

#### Scenario: Privilege changes keep the original JWT valid

- **WHEN** the auditor's role/permission changes commit without session revocation or auth_version increment
- **THEN** a new read uses the changed effective permission; password/global revoke/suspend/deactivate still invalidate credentials and reactivation does not restore old sessions

### Requirement: Additive contract returns only typed safe audit history

api-contracts SHALL own the new audit v1 source/generated contract. ListAuditLogsRequest SHALL use query=1 PageQuery and filter=2 AuditLogFilter. Filter SHALL use actor_user_id=1 optional uint64, actor_service=2 string, action=3 string, target_type=4 string, target_id=5 string, occurred_from=6 Timestamp and occurred_before=7 Timestamp. Response SHALL use items=1 repeated AuditLogItem, meta=2 PageMeta and next_cursor=3 string.

AuditLogItem SHALL use id=1 uint64, actor_user_id=2 optional uint64, actor_service=3 optional string, action=4 string, target_type=5 string, target_id=6 string, safe_before=7 AuditSnapshot, safe_after=8 AuditSnapshot, reason=9 optional string, request_id=10 optional string, trace_id=11 optional string and occurred_at=12 Timestamp. Both historical actor fields SHALL be preserved when stored. Nullable actor/metadata fields, absent snapshots and present empty snapshots SHALL remain distinguishable, and timestamps SHALL represent UTC instants.

AuditSnapshot SHALL expose only version=1 int64, status=2 string, name=3 optional string, description=4 optional string, address=5 optional string, phone=6 optional string, role_ids=7 repeated uint64, permission_codes=8 repeated string, application_id=9 uint64, session_id=10 string, reason_code=11 string and no_op=12 bool. No raw JSON/map, credential/password/hash/token/header, arbitrary request payload, private database error or stack trace SHALL appear. Session identifiers SHALL remain descriptive historical IDs and SHALL NOT authorize session administration. Invalid stored data, malformed/nonobject/null JSON in a non-NULL snapshot or unknown snapshot keys SHALL suppress the entire page with safe UNAVAILABLE, never raw fallback or silent row removal.

#### Scenario: Nullable actors and snapshots are returned

- **WHEN** stored records contain service-only/user-only/both actors, optional metadata, SQL NULL snapshots or valid empty safe objects
- **THEN** the response preserves those distinctions and maps only the bounded typed allowlist

#### Scenario: Historical snapshot contains an unexpected secret field

- **WHEN** a candidate page contains malformed snapshot JSON, an unknown field such as access_token or an invalid allowed-field value
- **THEN** the whole call returns redacted UNAVAILABLE without the field, partial rows or raw JSON in response/log/error

### Requirement: Read failures suppress results and release resources

Snapshot start, state/permission/count/page/mapping and finalization dependency failures SHALL return UNAVAILABLE through existing safe localized shared catalogs, preserving error identity/cause internally. No dependency error SHALL become empty success, NOT_FOUND, authorization denial or partial page. Cancellation/deadline SHALL preserve CANCELED/DEADLINE_EXCEEDED and unexpected panic SHALL return redacted INTERNAL. All acquired read resources SHALL be released before returning or recording an independent failure attempt; no success response SHALL escape a failed finalization.

#### Scenario: Count page or finalization fails

- **WHEN** dependency failure occurs before or after candidate items and metadata have been calculated
- **THEN** UNAVAILABLE returns with no response payload and the read connection is cleaned up for reuse

#### Scenario: Cancellation deadline or panic interrupts a read

- **WHEN** the request is canceled, times out or panics after opening its snapshot
- **THEN** the matching safe gRPC error is returned after cleanup without business writes or leaked pool resources

### Requirement: Listing preserves append-only history and independent error attempts

Successful pages and empty pages SHALL create no audit-success/read record, mutation, version/auth_version change, session/refresh change, outbox event or idempotency result. Audit edit/delete SHALL NOT be exposed. Response loss, retry and incidental idempotency metadata SHALL cause a fresh authorized read, never stored replay. Errors with a verified service SHALL retain at most the one existing independent safe SECURITY_ATTEMPT_FAILED audit after read cleanup using RPC target and safe reason, without request/filter/cursor/result or credentials. Attempt persistence failure/panic SHALL preserve the original error and only update bounded failure observation. It SHALL NOT recursively invoke listing or add duplicate audits. Rate rejection SHALL retain bounded limiter telemetry without per-request database audit.

#### Scenario: Listing succeeds or a response is lost

- **WHEN** a valid page/empty page succeeds or the caller retries after response loss with the same incidental key
- **THEN** there is no success audit/outbox/idempotency/business write, and retry rechecks current authority and snapshot

#### Scenario: Failed list needs an attempt audit

- **WHEN** listing errors after a snapshot was acquired and independent attempt persistence succeeds, fails or panics
- **THEN** read cleanup precedes at most one safe attempt, original error remains, and no audit-of-success or recursion occurs

### Requirement: Audit listing shares existing bounded protected-read admission

The method SHALL share verified-user protected-read quota120/minute burst20, maximum10000 entries/process, idle TTL15minutes and cleanup1minute. Invalid input after verified identity SHALL count; unverified identities SHALL NOT allocate protected-read entries. There SHALL be no new bucket keyed by arbitrary actor/filter/cursor. RESOURCE_EXHAUSTED/RATE_LIMITED SHALL preserve bounded RetryInfo and locale. Available quota SHALL never replace exact current permission checks.

#### Scenario: Audit and existing reads share the same exhausted group

- **WHEN** a verified actor consumes the read burst across existing reads and ListAuditLogs, including malformed audit requests
- **THEN** subsequent listing is rate-rejected with structured RetryInfo before audit query work and without per-rejection DB attempt writes

#### Scenario: Quota is available without audit permission

- **WHEN** the actor has quota but lacks current `iam.audit.read` or is restricted
- **THEN** listing is denied without audit contents or a fabricated successful empty page

### Requirement: Registration preserves every existing secure RPC contract

The default server SHALL add exactly AuditService/ListAuditLogs to the prior36-method/eight-service inventory, producing37 methods/nine services. Every existing descriptor/tag/type and method including TesRPC, pagination/cursor behavior, caller/permission/restricted policy, Register PENDING/token-empty, generic Login denial and credential-only Login/Refresh SHALL remain compatible. EdDSA-only, issuer ApplicationName, ignored audience, access15minutes bounded by absolute7day session and30second skew, strict refresh rotation and reuse commit-before-denial SHALL remain. Restricted operations SHALL remain only Me/ChangePassword/Logout. No wildcard caller authority, legacy authentication fallback, schema migration or deployment activation SHALL follow from backend test completion.

#### Scenario: Prior clients invoke existing methods

- **WHEN** the new audit service is registered and prior clients use any of the36 existing methods
- **THEN** their contracts and secure behavior remain unchanged while ListAuditLogs requires its own exact caller and permission authority

#### Scenario: Backend evidence passes before deployment readiness

- **WHEN** audit-read backend gates pass while consuming-client/deployment DEP-01 has not passed
- **THEN** the increment can report backend completion but cannot claim deployment readiness, full Phase4 reconciliation or Phase5 hardening completion

## Traceability

[ALR acceptance ledger](../../changes/archive/2026-10-18-auth-audit-log-reads/design.md#acceptance-ledger) defines owner/scope/prerequisites/tests/evidence and baseline NOT_STARTED/BLOCKED for ALR-01..06/ALR-EXT-01, mapped to unchanged AC-31, M-01/02/03/05/06/07/09/10/11 and EXT-01 from the [MVP criteria](../../../docs/grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence). Only actual contract, controlled backend and dedicated PostgreSQL normal/race results can satisfy identified gates. Source/schema inspection, skipped DB suites and unrelated prior evidence SHALL NOT establish this RPC's PASS. REST/UI/retention readiness remains Phase6 DEP-01; full mutation reconciliation and broader hardening remain separate.

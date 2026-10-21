# Audit log API

`/audit.v1.AuditService/ListAuditLogs` requires TLS, exact-method/IAM-allowed
service authentication and an ACTIVE unrestricted user/session with current
`iam.audit.read`. Authority, total and items use one primary read-only snapshot.
Actor filters select history, including other users and inactive historical
actors/targets. They never establish caller identity. User-read/session-read
permissions and an IAM application grant are not additional prerequisites.

## Query

Query is minimal, following the user's 18 October 2026 decision. PageQuery accepts
only page/limit/last_id. Defaults: page 1, limit 20; max 100. Negative values, unknown
cursor and any nonempty fields/sort or present search return INVALID_ARGUMENT.
Nil request is invalid; nil query/filter selects defaults. Order is always
`occurred_at DESC, id DESC`.

| Filter | Semantics |
| --- | --- |
| actor_user_id | Optional presence; if present 1..2147483647, zero invalid |
| actor_service | Trimmed exact case-sensitive string, max 100 |
| action | Trimmed exact case-sensitive string, max 100; unknown matches nothing |
| target_type | Trimmed exact string, max 50; may be used alone |
| target_id | Trimmed exact string, max 100; requires target_type; may identify user/role/session/RPC |
| occurred_from | Optional valid Timestamp, inclusive lower bound |
| occurred_before | Optional valid Timestamp, exclusive upper bound; must exceed from if both present |

Strings require valid UTF-8 without control characters. Empty trimmed strings
mean absent. Every supplied filter, including both actor selectors, combines
with AND. No live target lookup, implicit OR, default rolling window or wildcard
interpretation occurs. Time bounds normalize to UTC; database comparisons retain
the correct [from,before) set even for nanosecond bounds against stored
microsecond timestamps.

Without last_id, page>1 uses fixed-order offset. With last_id, keyset comparison
uses occurred_at/id and replaces offset. Audit IDs span positive BIGINT through
9223372036854775807, independently of INTEGER user IDs. Cursor is canonical,
versioned, max 4096 bytes and method/filter/order-bound; it is neither numeric
legacy last_id nor an authorization ticket. Changing page/limit alone is allowed;
changing filters requires a new first page. Each request rechecks authority.

Total counts all filtered rows before cursor/offset/limit. Total_pages is
ceil(total/limit), empty total/pages0; has_next uses a matching lookahead row,
has_prev follows cursor or page>1. next_cursor is present only with a next page.
Metadata overflow returns safe UNAVAILABLE. Pagination holds no database snapshot
between calls; new/backdated entries can change later pages. It is not an export.

## Safe contract

Source/generated messages belong to api-contracts, published/pinned at
`v0.0.0-20261018073250-b4ad807ad5fa`. Field numbers/types are frozen in the
[design contract table](../openspec/changes/archive/2026-10-18-auth-audit-log-reads/design.md#2-freeze-an-additive-typed-contract-before-backend-wiring).
ListAuditLogsRequest has query1/filter2; response has items1/meta2/next_cursor3.

Item fields: id, nullable actor_user_id/actor_service, action, target_type/id,
nullable typed safe_before/safe_after, nullable reason/request_id/trace_id,
occurred_at. Both stored actors are preserved. SQL NULL snapshot is absent;
stored empty object is a present empty message. UTC timestamps represent the
stored instant. No raw JSON, credentials, hashes, auth headers or internal error
details are returned.

Snapshot allowlist: version, status, nullable name/description/address/phone,
role_ids, permission_codes, application_id, session_id, reason_code and no_op.
Historical session IDs are identifiers; they do not grant session-admin access.
Malformed/nonobject/null JSON, unknown keys or invalid stored allowed fields
fail the whole page with redacted UNAVAILABLE. No silent row dropping or raw
fallback is available; this API does not repair historical data automatically.

## Errors and effects

Invalid credentials/session: UNAUTHENTICATED. Method/IAM namespace/restriction/
missing exact permission: PERMISSION_DENIED. Invalid request: INVALID_ARGUMENT.
Dependency/finalization: UNAVAILABLE; cancellation/deadline retain CANCELED and
DEADLINE_EXCEEDED; panic is redacted INTERNAL. Existing localized catalogs and
error cause are retained. Only an authorized successful lookup returns empty.

| Outcome | Effects |
| --- | --- |
| Success/empty | No mutation, version/auth_version/session change, success/read audit, outbox or idempotency result |
| Error with verified service | At most one existing independent safe SECURITY_ATTEMPT_FAILED attempt after snapshot cleanup; RPC target/safe reason, no filters/rows/credentials |
| Attempt failure/panic | Original error preserved, bounded failure observation |
| Rate rejection | Bounded limiter telemetry; no per-rejection DB audit |
| Response-loss retry/incidental idempotency key | Fresh current authorized read, no persistent replay |

Shared protected-read admission remains120/minute burst20, cap10000 entries,
idle 15 minutes/cleanup 1 minute. Invalid input after verified identity counts;
unverified identities allocate no user bucket. No bucket is keyed by arbitrary
filters/cursors. RESOURCE_EXHAUSTED/RATE_LIMITED retains RetryInfo and locale.

The default registration has 37 methods/nine services. Prior 36 methods,
TesRPC, Register PENDING/token-empty, credential-only Login/Refresh, EdDSA-only,
restricted allowlist and existing ListUsers/catalog pagination remain unchanged.
Privilege-only role/app changes keep sessions but affect the next snapshot;
password/global revoke/suspend/deactivate still invalidate credentials.

Implementation and test status are recorded in
[evidence](../openspec/changes/archive/2026-10-18-auth-audit-log-reads/evidence/progress.md).
Phase6 DEP-01, retention/archival readiness, full Phase4 mutation reconciliation
and Phase5 hardening remain separately tracked.

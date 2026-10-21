# iam-catalog-reads Specification

## Purpose

Menyediakan pembacaan catalog role, permission dan aplikasi yang aman serta
konsisten dengan otorisasi, dengan detail dampak assignment dan pagination
cursor yang mendukung seluruh scalar publik tanpa menerima expression SQL.

## Requirements

### Requirement: Catalog reads require exact current permissions on one primary snapshot

`/role.v1.RoleService/ListRoles` and `/role.v1.RoleService/GetRole` SHALL require
`iam.role.read`; `/permission.v1.PermissionService/ListPermissions` and
`/application.v1.ApplicationService/ListApplications` SHALL require
`iam.permission.read`. Every read SHALL require TLS, method/namespace-allowed
verified service and ACTIVE nonrestricted verified user/session. Authority,
target/page, total, collections and derived count SHALL use one consistent
primary snapshot. Target role_id SHALL be positive INTEGER-compatible, distinct
from actor identity. Authorization SHALL precede target-existence disclosure.
Dependency failure SHALL return UNAVAILABLE, never empty success or permission
denial; cancellation/deadline SHALL retain their mappings. Reads SHALL NOT
change business versions or emit domain events. All four methods SHALL share
verified-user protected-read quota120/minute burst20 with existing bounded
capacity/cleanup and structured RetryInfo/RESOURCE_EXHAUSTED.

#### Scenario: Caller can read users but lacks role permission
- **WHEN** a verified actor has iam.user.read without iam.role.read and requests ListRoles or GetRole
- **THEN** permission denial returns without catalog or target-existence disclosure

#### Scenario: Catalog permission is revoked before the next snapshot
- **WHEN** role/permission state changes before a new read begins
- **THEN** that read uses current primary authority even with a still-valid JWT

#### Scenario: Empty catalog restricted caller or failing dependency
- **WHEN** a catalog has no matches, the actor is restricted or snapshot/query fails
- **THEN** only an authorized successful read returns empty metadata; restricted or failed reads return denial/dependency error without partial data

#### Scenario: A concurrent commit changes catalog mappings
- **WHEN** mappings/assignments change between authority, page and detail reads
- **THEN** response, total and derived count come from the same primary snapshot

#### Scenario: Protected read quota is exhausted
- **WHEN** an actor exhausts the read group across existing and catalog methods
- **THEN** structured rate rejection discloses no catalog data and quota never replaces exact permission checks

### Requirement: Safe catalog detail exposes assignment impact independently of effective permission

ListRoles SHALL return safe scalar role metadata including separate id/string
code/numeric role_code, name, nullable description, application_id/application_code,
status, is_system, version, nullable created_at/updated_at and affected_user_count.
GetRole SHALL additionally return the registered permission collection, including
for DISABLED/system roles and disabled applications. affected_user_count SHALL
count distinct existing user IDs with ACTIVE assignments to the role, regardless
of account status, role/application status or application access; REVOKED
assignments SHALL not count. Counts SHALL not multiply with permission joins.
ListPermissions SHALL expose registered id/code/description/application metadata;
ListApplications SHALL expose id/code/name/status/is_system. Credentials, hashes,
user identities behind counts and internal policy state SHALL NOT appear.
Authorized missing role SHALL return NOT_FOUND. Catalog SHALL include unassigned
roles and SHALL NOT substitute for user filter-options or grant endpoints.

#### Scenario: Pending suspended and deactivated accounts hold active assignments
- **WHEN** a role has ACTIVE assignments across all account statuses plus REVOKED assignments and multiple permissions
- **THEN** each existing ACTIVE-assigned user counts once, REVOKED does not count, and the count remains the same after disabling the role

#### Scenario: Administrator inspects a disabled system role
- **WHEN** authorized GetRole targets a DISABLED system role in a disabled application
- **THEN** safe metadata/version/registered permissions and assignment count are readable without making the role editable or its permissions effective

#### Scenario: An unused role exists in the catalog
- **WHEN** ListRoles matches a role held by no user
- **THEN** that role appears with count zero; user filter-options remain separately scoped

### Requirement: Catalog filters search and total operate on unique matching entities

ListRoles SHALL support application_code and repeated ACTIVE/DISABLED statuses;
ListPermissions SHALL support application_code; ListApplications SHALL support
repeated ACTIVE/DISABLED statuses. Absent filters SHALL include all catalog states.
Values within statuses SHALL use OR; categories and keyword SHALL use AND.
Search SHALL use case-insensitive literal CONTAINS or PREFIX, default CONTAINS,
with trimmed keyword at most200 characters. Allowed search fields SHALL be
role code/name/description/application_code; permission code/description/
application_code; application code/name, with default code/name for roles/apps
and code/description for permissions. Selected search fields SHALL use OR.
Unsupported fields/modes/statuses SHALL return INVALID_ARGUMENT; wildcard
characters SHALL be treated literally. Totals SHALL count unique matching roles,
permissions or applications before cursor/limit, not mapping/assignment join rows.
No matches SHALL return total/total_pages0 with safe empty items.

#### Scenario: Status alternatives combine with application and name search
- **WHEN** ListRoles selects ACTIVE and DISABLED, application clm and a name keyword
- **THEN** results satisfy either selected status AND clm AND the search, including unassigned matching roles

#### Scenario: Role has multiple assignments and permission mappings
- **WHEN** joins could produce repeated rows for one matching role
- **THEN** it appears once and total counts one regardless of cursor/limit

#### Scenario: Search contains SQL wildcard or an unsupported mode
- **WHEN** a keyword contains percent/underscore or FULL_TEXT/TRIGRAM is requested
- **THEN** supported search treats characters literally and unsupported modes fail validation without accepting SQL expressions

### Requirement: Catalog cursors support deterministic public scalar multi sort

All public scalar fields in each list item, including timestamps, boolean fields
and role affected_user_count, SHALL support ASC/DESC through an explicit public
field registry. Multi-sort SHALL honor request order, reject duplicate/unknown
fields, and append id ASC when id is absent; default SHALL be id ASC. Nullable
description/timestamps SHALL sort nulls last in both directions. Internal column
names and SQL expressions SHALL be rejected. Query SHALL use PageQuery/PageMeta,
page default1, limit default20/max100, and response next_cursor. Cursor SHALL
carry all sort values/null markers and ID and bind to catalog method, normalized
filter/search/sort; malformed, cross-catalog or mismatched cursors SHALL fail
validation. Changing query SHALL start without a cursor. ID-only mode SHALL
also accept positive numeric last_id. Cursor pages SHALL use keyset comparison;
without cursor, page>1 SHALL use the deterministic requested page. Projection
fields SHALL be validated against the public list item; absent fields SHALL mean
all, with id always returned and internal sort values retained for cursors even
when not projected. New public scalar fields SHALL enter the registry before
exposure. Pagination SHALL not promise a snapshot retained across requests.

#### Scenario: Every public scalar is selected as sort
- **WHEN** either direction of a list item's public field is requested, including role count or nullable timestamps
- **THEN** the matching allowed expression and typed cursor values produce deterministic ordering with nulls last and ID tie-breaking

#### Scenario: Multiple sort values tie across a page boundary
- **WHEN** rows share primary/secondary values under mixed ASC/DESC multi-sort
- **THEN** lexicographic keyset comparison returns the next rows without duplicates on unchanged fixture state

#### Scenario: Cursor belongs to another catalog or query
- **WHEN** a cursor is malformed, typed incorrectly or used with changed filter/search/sort or another method
- **THEN** INVALID_ARGUMENT returns without silently restarting or executing an injected expression

#### Scenario: Numeric cursor projection and empty next page
- **WHEN** ID-only sort uses numeric last_id, requested projection omits a sort field, or pagination reaches the end
- **THEN** ID compatibility remains, hidden sort values still build a valid cursor, total remains pre-cursor and terminal next_cursor is empty

Traceability: [RM-07..10 and RM-EXT-01](../../changes/archive/2026-10-12-auth-role-management/design.md#acceptance-ledger),
AC-21/23/31 and M-05/06/10/11. Backend status PASS; owner grpc-auth with
published api-contracts prerequisite satisfied. User decisions on sort/count
remain normative; actual query/snapshot/PostgreSQL results are in
[evidence](../../changes/archive/2026-10-12-auth-role-management/evidence.md) and [scenario mappings](../../changes/archive/2026-10-12-auth-role-management/scenario-traceability.md).

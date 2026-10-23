# Spec Delta

## MODIFIED Requirements

### Requirement: Catalog filters search and total operate on unique matching entities

ListRoles SHALL support application_code and repeated ACTIVE/DISABLED statuses;
ListPermissions SHALL support application_code; ListApplications SHALL support
repeated ACTIVE/DISABLED statuses. Absent filters SHALL include all catalog states.
Values within statuses SHALL use OR; categories and keyword SHALL use AND.
Compatibility search SHALL use case-insensitive literal CONTAINS or PREFIX, default CONTAINS,
with trimmed keyword at most200 characters. Allowed search fields SHALL be
role code/name/description/application_code; permission code/description/
application_code; application code/name, with default code/name for roles/apps
and code/description for permissions. Selected search fields SHALL use OR.
Unsupported fields/modes/statuses SHALL return INVALID_ARGUMENT; wildcard
characters SHALL be treated literally. Totals SHALL count unique matching roles,
permissions or applications before cursor/limit, not mapping/assignment join rows.
No matches SHALL return total/total_pages0 with safe empty items.


In addition, `/role.v1.RoleService/ListRoles`,
`/permission.v1.PermissionService/ListPermissions` and
`/application.v1.ApplicationService/ListApplications` SHALL accept exactly
`search.fields=["keyword"]` with explicit `SEARCH_MODE_FULL_TEXT` or
`SEARCH_MODE_FULL_TEXT_TRIGRAM` using database-owned documents. FULL_TEXT SHALL
match all plain-text tokens with AND in the vector only. FULL_TEXT_TRIGRAM SHALL
match either all full-text token prefixes with AND or a literal substring in
lexeme text. Trigram SHALL NOT promise typo similarity. Document and query
separator rules SHALL follow catalog-search-projections. Keyword SHALL NOT be
accepted as projection or sort, mixed with physical fields, used with a missing
or other mode, or inferred from omitted fields. Raw and normalized new-path
keywords SHALL each be bounded to200 Unicode characters; raw overflow SHALL
fail before shortening by normalization. Missing/empty/whitespace-only keyword
SHALL mean no search, while nonblank zero-lexeme input SHALL NOT remove the
filter. Legacy trim, fields and default mode SHALL remain unchanged. Authorization,
application/status filters and keyword SHALL combine with AND around the grouped
search alternatives. Existing exact permissions, service/user admission, primary
snapshot, rate limits, safe response types and error mappings SHALL remain.
No RPC, field tag/type or api-contracts ownership SHALL change.

#### Scenario: Status alternatives combine with application and name search
- **WHEN** ListRoles selects ACTIVE and DISABLED, application clm and a name keyword
- **THEN** results satisfy either selected status AND clm AND the search, including unassigned matching roles

#### Scenario: Role has multiple assignments and permission mappings
- **WHEN** joins could produce repeated rows for one matching role
- **THEN** it appears once and total counts one regardless of cursor/limit

#### Scenario: Search contains SQL wildcard or an unsupported mode
- **WHEN** compatibility input contains percent/underscore or a full-text mode is requested against physical search fields
- **THEN** compatible search treats characters literally and unsupported field/mode combinations fail validation without accepting SQL expressions

#### Scenario: Explicit keyword mode finds a stored document
- **WHEN** an authorized actor calls one of the three catalog methods with keyword field and explicit FULL_TEXT or FULL_TEXT_TRIGRAM
- **THEN** only that catalog's stored document is searched, vector-only mode requires all tokens, and combined mode returns items matching either branch under the existing filters

#### Scenario: Invalid logical field or raw keyword overflow
- **WHEN** keyword is mixed with physical fields, selected for projection/sort, requested with unspecified/contains/prefix/trigram mode, or raw input exceeds200 Unicode characters
- **THEN** INVALID_ARGUMENT returns through the existing bad-request mapping without executing a broader search

#### Scenario: A substring matches outside the authorized filter
- **WHEN** only the text branch matches a document in a different application or status
- **THEN** that item remains excluded because the OR branches are grouped within the required AND filters

#### Scenario: Missing permission or failing search dependency
- **WHEN** an actor lacks the exact current read permission, is restricted, or the search query/snapshot fails
- **THEN** existing authentication/permission or UNAVAILABLE errors return without partial data or empty-success substitution; deadline and cancellation preserve their mappings


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
validation. Changing query SHALL start without a cursor. Compatibility ID-only mode SHALL
also accept positive numeric last_id. The new keyword path SHALL require a
query-bound opaque cursor and reject numeric last_id. Its binding SHALL include
canonical keyword input, search mode/field, method, filters, sort and a search
semantics revision, without changing legacy cursor serialization or hashes. Cursor pages SHALL use keyset comparison;
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
- **WHEN** compatibility ID-only sort uses numeric last_id, requested projection omits a sort field, or pagination reaches the end
- **THEN** ID compatibility remains, hidden sort values still build a valid cursor, total remains pre-cursor and terminal next_cursor is empty

#### Scenario: Keyword cursor changes query or semantics revision
- **WHEN** a keyword cursor is reused with changed keyword, mode, field, method, filter, sort or search revision, or numeric last_id is supplied for keyword search
- **THEN** INVALID_ARGUMENT returns without restarting; unchanged keyword queries continue deterministically with unique pre-cursor totals


Traceability: CKS-01/03/04/05 in [acceptance ledger](../../design.md#acceptance-ledger); owner grpc-auth, status NOT RUN for this increment. Existing AC-21/23/31 and M-05/06 remain in force.

# user-list-sorting Specification

## Purpose
Menyediakan ListUsers dengan sort field publik yang dinamis, cursor konsisten terhadap query, filter/search terpadu dan total user unik tanpa menduplikasi user akibat collection role atau aplikasi.

## Requirements

### Requirement: ListUsers sorts only supported public fields with stable ordered keys

`/user.v1.UserService/ListUsers` SHALL require a verified active nonrestricted user/session and exact iam.user.read permission before disclosure. It SHALL support ASC/DESC for id, name, email, address, phone, status, version and created_at, plus roles and application_access. The collection key SHALL be the alphabetically smallest name from the complete displayed collection, not a count or only filter-matching items. Empty collections and null sort values SHALL sort last in both directions. Ordered multi-sort SHALL preserve request field order; ID ASC SHALL be appended when absent, and explicitly requested ID direction SHALL be retained. Default SHALL remain ID ASC; auth_id SHALL be a compatibility alias for id. Duplicate canonical sort fields, unknown/internal fields and SQL expressions SHALL be rejected with validation errors. Every newly exposed sortable public field SHALL have an explicit allowed mapping before acceptance.

#### Scenario: Every scalar sort and multi-sort is requested
- **WHEN** an authorized caller requests any supported scalar ASC/DESC or an ordered combination with equal primary values
- **THEN** rows follow the requested keys/directions and deterministic ID tie-breaker, without treating request strings as database expressions

#### Scenario: Roles and applications contain several names or no items
- **WHEN** users have multiple displayed roles/applications or empty collections and collection sorting is requested
- **THEN** the smallest displayed name determines each collection key, empty collections are last in either direction and tied keys use subsequent sort keys and ID

#### Scenario: Legacy ID sort or unsupported expression is sent
- **WHEN** the caller uses auth_id or sends an internal column, SQL expression or duplicate canonical field
- **THEN** auth_id behaves as id and unsupported or duplicate input is rejected without data disclosure or mutation

### Requirement: ListUsers cursor binds all key values to the normalized query

Dynamic ListUsers cursors SHALL carry all typed ordered sort values including null markers and ID, bound to normalized sort/filter/search. Pagination SHALL compare keys consistently with the full requested order and null placement. Malformed, wrong-type or query-mismatched cursors SHALL return validation error. Existing numeric last_id SHALL remain accepted only for ID-only sorting in either direction, including default ID ASC. Clients SHALL restart with no cursor after changing sort/filter/search. An opaque cursor SHALL NOT bypass current authorization or promise one snapshot across different requests. Existing PageQuery/last_id field numbers and types SHALL remain unchanged.

#### Scenario: Equal or nullable keys span pages
- **WHEN** an unchanged query pages through users with equal, null or empty-collection keys
- **THEN** the stable keyset order yields each matching user once for an unchanged dataset, with ID disambiguating ties

#### Scenario: Query changes or cursor is malformed
- **WHEN** a cursor belongs to different sort/filter/search, contains wrong typed values or is malformed
- **THEN** validation fails; the caller can start the changed query using an empty cursor

#### Scenario: Existing client supplies numeric ID cursor
- **WHEN** a numeric last_id is supplied with default or explicit ID-only ASC/DESC
- **THEN** existing ID cursor behavior remains supported, while the same numeric cursor is invalid for non-ID or multi-sort queries

### Requirement: ListUsers combines filters and search and counts unique users before pagination

ListUsers SHALL apply existing role_codes, statuses, application_code, created range and supported keyword search semantics. Multiple selected roles SHALL combine with OR; role category, status category, application category, created range and search SHALL combine with AND. Role/application matching SHALL use ACTIVE assignments/access and ACTIVE roles/applications. Total SHALL count unique visible users matching the complete query before cursor and limit; without filters/search it SHALL count all users visible to the caller. Total SHALL NOT count collection items or join rows. A user with multiple matching roles/applications SHALL appear only once. A matching user's displayed collection SHALL retain its full eligible collection rather than only selected filter matches.

#### Scenario: Role alternatives and search are active
- **WHEN** selected role codes identify Member or IAM Editor and a name/email keyword is supplied using supported search fields
- **THEN** only users matching the keyword AND at least one selected role appear, and total counts those unique users

#### Scenario: Status and application narrow role results
- **WHEN** statuses and application_code accompany role/search filters
- **THEN** every returned user satisfies each active category and inactive assignment/role/access/application does not contribute a match

#### Scenario: Several collections would multiply joined rows
- **WHEN** a user has multiple matching roles and applications and a later cursor page is requested
- **THEN** that user is counted once, appears at most once and total remains the full matching-user count before cursor/limit, including an empty last page

### Requirement: ListUsers safe fields collections total and authority share a primary snapshot

ListUsers SHALL populate id, name, email, address, phone, status, version, created_at, roles and application_access according to the public response projection without exposing credentials/internal state. UserListItem.created_at SHALL be an additive contract field preserving existing field numbers/types. Default collections SHALL include only ACTIVE assignments/access with ACTIVE roles/applications. Authority, page, total and collection reads SHALL use one consistent primary snapshot; query filters and sorting SHALL remain effective regardless of projected fields. Dependency failure SHALL return UNAVAILABLE rather than partial or empty success.

#### Scenario: Collection or permission changes between reads
- **WHEN** a concurrent commit changes authority or collections while a single ListUsers request reads its page and total
- **THEN** authorization, scalar data, displayed collections, sort keys and total describe one primary snapshot, not mixed states

#### Scenario: Response fields are populated and projection is limited
- **WHEN** an authorized caller requests a full response or a supported field projection
- **THEN** eligible requested safe fields including created_at and active collections are populated and projection does not disable active sort/filter predicates

#### Scenario: Primary dependency fails
- **WHEN** any required snapshot/page/count/collection read fails
- **THEN** the request returns the mapped dependency error without partial response or a fabricated zero total

Evidence: [SLC-10](../../changes/archive/2026-10-11-auth-session-lifecycle/design.md#acceptance-ledger). Approved extension implemented and verified PASS at the referenced ledger/final evidence; no filter-options endpoint added. REST/frontend and deployment remain outside this increment.

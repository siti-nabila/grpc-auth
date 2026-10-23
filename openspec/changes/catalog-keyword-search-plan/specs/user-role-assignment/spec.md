# Spec Delta

## MODIFIED Requirements

### Requirement: Assignment queries support safe filters search projection and scalar multi sort

ListUserRoles SHALL accept PageQuery/PageMeta with page default1, limit default20
and max100, next_cursor, application_code and repeated assignment statuses
ACTIVE/REVOKED. Status values SHALL combine OR; categories and keyword SHALL
combine AND. Unknown statuses/fields/modes SHALL return INVALID_ARGUMENT. Compatibility search
SHALL use case-insensitive literal CONTAINS or PREFIX, default CONTAINS, trimmed
keyword at most200 characters, with code/name/application_code fields and default
code/name. Selected search fields SHALL combine OR. Percent and underscore SHALL
remain literal. Each public item scalar
SHALL support ASC/DESC multi-sort through an explicit allowlist; request order
SHALL be retained, duplicate/unknown sort fields rejected, and id ASC appended
when absent. Default sort SHALL be id ASC and nullable values SHALL sort last
in either direction. Projection SHALL allow only public item fields, default
to all, always include id/role_id, and retain hidden sort values for cursors.
Total SHALL count unique matching assignments before cursor/limit, unaffected
by permission mapping multiplicity. Cursor SHALL carry typed sort values/nulls
and bind to full method, target user and normalized filters/search/sort.
Malformed, cross-user, cross-method and mismatched cursors SHALL be rejected.
Compatibility ID-only mode SHALL also accept positive numeric last_id. Keyset continuation
SHALL be deterministic on unchanged state; absent cursor with page>1 SHALL use
the deterministic requested page. No snapshot across separate requests is promised.


`/user.v1.UserService/ListUserRoles` SHALL additionally accept exactly
`search.fields=["keyword"]` with explicit FULL_TEXT or FULL_TEXT_TRIGRAM, and
SHALL apply the same validation, literal parser, empty-input and raw/normalized
200-Unicode-character bounds as the keyword catalog path. Its initial document
SHALL contain only the assigned role name, independently of the broader legacy
search field set. FULL_TEXT SHALL require all plain-text tokens in the vector;
FULL_TEXT_TRIGRAM SHALL match all token prefixes with AND OR a literal lexeme-text
substring. Keyword SHALL NOT become a projection/sort field or combine with
physical search fields. Query-bound opaque keyword cursors SHALL include target
user, canonical keyword input, mode/field, full method, filters/sort and search
semantics revision; numeric cursors SHALL be rejected on the new path. Existing
legacy cursor encodings SHALL remain compatible. Keyword alternatives SHALL be
grouped inside AND with the fixed target user_id, application and assignment
status; another user's assignment SHALL never match through the text branch.
The verified actor SHALL retain iam.user.read and existing service/TLS,
ACTIVE nonrestricted session, snapshot, quota and error requirements. Search
SHALL add no mutation, version, audit, outbox or idempotency side effect and
SHALL NOT alter grant/revoke, effective-permission or application-access behavior.

#### Scenario: Application and status filters combine with literal search
- **WHEN** a caller selects application clm, ACTIVE/REVOKED statuses and a name keyword containing percent or underscore
- **THEN** matching assignments satisfy the combined filters with literal search and unique pre-cursor total

#### Scenario: All public scalars and mixed nullable ties are sorted
- **WHEN** either direction of each public scalar or a mixed multi-sort with tied/null source fields is requested
- **THEN** ordering uses the approved public expression, nulls last and ID tie-breaking, with deterministic keyset continuation

#### Scenario: Cursor is reused for another target or query
- **WHEN** an opaque cursor is malformed or reused with another user, method, filter, search or sort
- **THEN** INVALID_ARGUMENT returns without silently restarting or accepting caller SQL

#### Scenario: Projection hides ordering values or the final page is empty
- **WHEN** projection omits sort fields, compatibility numeric ID-only pagination is used or the page reaches the end
- **THEN** id/role_id remain, hidden typed values sustain correct cursors, total remains pre-cursor and terminal next_cursor is empty

#### Scenario: Role name search is isolated to the selected user
- **WHEN** reviewer matches an assignment belonging only to user456 while the authorized request targets user123
- **THEN** neither vector nor text matching returns user456 and the response retains user123 identity/version with an empty matching page

#### Scenario: Assignment document excludes initial role code
- **WHEN** a role name is Pemeriksa and its code is clm.reviewer
- **THEN** keyword reviewer does not match the initial assignment document while legacy explicit code search remains supported

#### Scenario: Active and revoked assignment names change together
- **WHEN** a role rename commits and a new snapshot lists ACTIVE and REVOKED assignments using its new name
- **THEN** both states match their refreshed documents while old-name-only documents do not persist and assignment status filters remain authoritative

#### Scenario: Keyword assignment cursor has another target or numeric value
- **WHEN** the new-path cursor belongs to another target or query/revision, or last_id is numeric
- **THEN** INVALID_ARGUMENT returns and no cross-user continuation or unbound fallback occurs


Traceability: CKS-01/03/04/05 in [acceptance ledger](../../design.md#acceptance-ledger); owner grpc-auth, status NOT RUN for this increment. Preserve AC-04/10/31/35 and M-05 without asserting new evidence for prior increments.

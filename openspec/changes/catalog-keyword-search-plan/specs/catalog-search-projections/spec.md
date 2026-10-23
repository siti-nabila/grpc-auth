# Spec Delta

## Purpose

Menyediakan dokumen keyword catalog yang komposisi dan sinkronisasinya dimiliki database agar pencarian aman, konsisten, dan dapat diperluas tanpa perubahan binary grpc-auth.

## ADDED Requirements

### Requirement: Catalog keyword search uses database-owned documents

The system SHALL provide one derived search document per role, permission,
application and user-role assignment. Initial sources SHALL be role name/code,
permission code, application code/name, and assignment role name respectively.
The assignment document SHALL NOT initially include role code. Permission
description, role application code and user address SHALL NOT be included in
the initial formulas. The runtime SHALL search the stored document without a
business-source whitelist or reconstruction of its contents. Adding a permitted
source through database formula, relevant triggers and rebuild SHALL require no
grpc-auth binary or protobuf change when the search contract is unchanged.
Projection state SHALL NOT determine authorization or expose credentials.

#### Scenario: Initial assignment and permission scope is narrower than legacy search
- **WHEN** only a role code contains reviewer and only a permission description contains dokumen
- **THEN** initial keyword documents do not match those terms, while the unchanged explicit-field compatibility search can still match its allowed sources

#### Scenario: A database formula gains a permitted source
- **WHEN** a permission description is added to its document formula, relevant update triggers are adjusted and existing documents rebuilt
- **THEN** the same request and grpc-auth binary find a description-only term through either the vector-only or text-only matching branch without a source whitelist change

### Requirement: Catalog documents and queries share literal separator semantics

Catalog document and query processing SHALL treat dot, hyphen and repeated
whitespace as word separators and use case-insensitive simple tokenization.
`iam.role.create` SHALL yield create at position3, iam at1 and role at2;
`iam auditor` SHALL yield iam at1 and auditor at2; `application-clm` SHALL
yield application at1 and clm at2, without joined dot/hyphen tokens.
Substring text SHALL be derived from the vector's lexeme array joined by a
single space, not raw source concatenation; its order SHALL NOT be treated as
source phrase order. Other punctuation SHALL NOT introduce query operators,
wildcards, parser failures or SQL injection. Raw REST keyword SHALL remain a
caller input without requiring frontend normalization; source values and
response values SHALL remain unchanged.

#### Scenario: Equivalent catalog separators
- **WHEN** the source or query contains clm.document-read, clm document read or repeated mixed separators
- **THEN** the same document/query word boundaries apply and no combined dotted or hyphenated lexeme is retained

#### Scenario: Literal metacharacters and empty query vectors
- **WHEN** nonblank input contains percent, underscore, backslash, quotes, ampersand, exclamation or parentheses and produces no query lexemes
- **THEN** vector matching is false without parser errors, a combined-mode text branch can only match its literal substring, and failure of both branches returns no matches rather than removing the filter

#### Scenario: Lexeme text has a different order from the source
- **WHEN** a multi-token source is tokenized into lexeme text whose ordering differs from the source phrase
- **THEN** full-text matching tests all required tokens independently of the substring branch, and combined mode succeeds if either branch matches

### Requirement: Projection synchronization is atomic with source changes

Source insert or relevant source-field update SHALL synchronously refresh its
derived document in the same transaction. Role rename SHALL update all affected
assignment documents including REVOKED assignments; assignment insert and role
reassignment SHALL refresh from the current role name. Status/version/reason-only
changes and same-role grant/revoke SHALL NOT require document rebuild. Source
deletion SHALL remove its derived document, without adding hard-delete APIs.
Projection failure SHALL abort the source transaction. Concurrent role rename,
assignment insert/regrant/revoke and administrative reassignment SHALL NOT
commit a stale document or introduce an unanalysed locking cycle. Projection
maintenance SHALL add no independent business version, auth_version, audit,
outbox event or idempotency result.

#### Scenario: Rename a role held by active and revoked assignments
- **WHEN** a role name changes successfully
- **THEN** role and all assignment documents reflect the committed name in the same transaction while existing mutation counters and persistence retain their original semantics

#### Scenario: Trigger failure or transaction rollback
- **WHEN** document refresh fails or the source transaction rolls back after tentative refresh
- **THEN** neither the source change nor the refreshed projection commits and no partial successful mutation is returned

#### Scenario: Concurrent rename and grant
- **WHEN** distinct physical connections interleave role rename and assignment insert or regrant
- **THEN** every committed assignment document reflects the current committed role name after both operations finish, with transaction cleanup and existing command ordering preserved

### Requirement: Keyword projection rollout requires complete verified data

Deployment SHALL prepare a separate reviewed migration with preflight,
schema/functions/triggers, resumable backfill, indexes, verification and rollback.
Each projection SHALL have exactly one row per source, source-linked deletion,
nonnull vector/text and full-text/trigram indexes. Backfill SHALL include inactive
and revoked records and verify missing/orphan/duplicate rows and document content
under concurrent writes. Active keyword queries SHALL use the projection without
silent legacy fallback; queries without keyword SHALL NOT require projection rows.
Consumer cutover SHALL wait for complete data, transactional/concurrency checks
and representative performance evidence. Rollback SHALL restore old caller modes
first and retain compatible projections until separate cleanup is safe.

#### Scenario: Incomplete backfill before rollout
- **WHEN** missing or stale projection rows remain
- **THEN** consumer keyword cutover is blocked, unfiltered lists remain complete, and runtime does not silently fall back to broader legacy search

#### Scenario: Backfill overlaps writes
- **WHEN** the resumable backfill overlaps source updates with triggers installed
- **THEN** final verification proves complete current documents rather than accepting an older backfill value that overwrote a concurrent update

### Requirement: Users search baseline remains isolated from catalog normalization

`/user.v1.UserService/ListUsers` SHALL retain its existing search and pagination
contract and database baseline containing email, name, phone and digit-only
phone. Catalog separator rules SHALL NOT replace the users formula. Test
fixtures SHALL represent that baseline and verify email/phone/profile updates,
including profile ownership reassignment, through the actual wrapper triggers.
An unverified live definition or unavailable PostgreSQL environment SHALL be
reported as an unresolved verification gate rather than implementation PASS.

#### Scenario: Catalog search is added alongside users
- **WHEN** users are searched by full email, email fragment, name, formatted phone or digit-only phone, including at-sign, plus and underscore inputs
- **THEN** the existing supported results and pagination remain compatible and catalog normalization does not change the users document formula

#### Scenario: Missing database evidence
- **WHEN** wrapper definitions, dedicated test database or required performance results are unavailable
- **THEN** the corresponding verification gate remains BLOCKED and source inspection or mock success does not mark it PASS

Traceability: CKS-01..08 in [acceptance ledger](../../design.md#acceptance-ledger).
Owner grpc-auth; prerequisites and evidence/status are recorded per criterion there.
This increment extends M-05 and preserves AC-04/10/21/23/31/35 in
[source acceptance criteria](../../../../../docs/user-management-spec.md#15-acceptance-criteria-dan-pengujian).

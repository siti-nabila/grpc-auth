# Spec Delta

## Purpose

Memastikan repository grpc-auth membangun persistence melalui ORM dengan nested query, hasil write dan transaksi yang dapat diverifikasi, tanpa SQL statement manual atau penghapusan mapping secara fisik.

## ADDED Requirements

### Requirement: Repository persistence uses ORM construction and execution

Production reader/writer SHALL construct and execute SELECT, INSERT and UPDATE through the project ORM, including nested SELECT. They MUST NOT embed complete SQL statements/subqueries or call raw database execution to bypass ORM construction. Allowed projections, aggregates, predicates, joins and sort expressions SHALL remain validated, with request values bound as arguments. Soft deletion SHALL be a conditional ORM UPDATE, never a hard DELETE. Direct SQL for reviewed schema scripts and isolated test-fixture setup SHALL remain outside this production repository rule.

#### Scenario: Repository query inventory is reviewed
- **WHEN** the final production reader/writer and transaction adapter source is inspected
- **THEN** no manual complete SELECT/INSERT/UPDATE/DELETE statement or raw execution escape remains, including nested queries and dead commented SQL examples

#### Scenario: Untrusted filters or sorting reach persistence
- **WHEN** a caller submits unknown sort/projection fields or SQL-like input values
- **THEN** unsupported fields are rejected and valid values remain bound data, never SQL structure

### Requirement: ORM query composition preserves nested semantics and argument ordering

ORM consumers SHALL be able to compose EXISTS, NOT EXISTS, IN-subquery and grouped derived joins from ORM-built child queries. The composed query SHALL preserve correlation, grouping, null semantics and complete result sets. Child arguments SHALL be ordered by their actual position in the parent SQL and rebound using the parent's dialect and placeholder mode, including nested conditions and UPDATE predicates. Invalid or absent required child queries SHALL return an error before execution, never silently drop a filter. Composition/count/DryRun SHALL not mutate the source child or parent query. PostgreSQL, MySQL and Oracle SHALL retain their placeholder, quoting, alias and pagination rules.

#### Scenario: Nested query has arguments before parent conditions
- **WHEN** a derived join, nested existence predicate and parent filter each contain arguments
- **THEN** SQL and argument order agree for each supported dialect, without duplicate numbered placeholders or loss of an argument

#### Scenario: Nested filter cannot be built
- **WHEN** an existence or IN query has an absent child or a child that fails construction
- **THEN** DryRun and execution fail without emitting an unfiltered query or database call

#### Scenario: Count and pagination reuse composed queries
- **WHEN** a consumer produces count and page queries with grouped derived joins and child arguments
- **THEN** the count remains unique and the page respects its limit/cursor while neither query changes the reusable child

### Requirement: ORM inserts expose affected-row outcomes without weakening existing APIs

ORM SHALL offer insert execution that returns the driver's write result without requiring RETURNING or assigning a generated ID. Existing Create, returning/scanning APIs and DryRun APIs SHALL preserve their existing behavior. Repository conditional write validation SHALL distinguish zero rows, exactly one row, invalid counts, missing result, driver RowsAffected failure and execution failure according to the existing consumer contract. Dependency causes and request cancellation SHALL remain available through error wrapping; no failure SHALL be fabricated as an effective mutation. Building SQL for execution SHALL use the same mapper/dialect machinery exercised by DryRun.

#### Scenario: Assignment or application write produces unexpected count
- **WHEN** an insert/update reports zero, multiple or negative rows, returns no result, or RowsAffected fails
- **THEN** the consumer preserves its conditional/no-change or dependency-error contract and the feature cannot commit a fabricated effective change

#### Scenario: Existing creation needs a generated primary ID
- **WHEN** an existing consumer calls Create or the returning/scanning APIs
- **THEN** generated-ID and returning semantics remain compatible with prior callers

### Requirement: ORM integration preserves request and transaction ownership

Repositories SHALL use the injected primary transaction or read snapshot and request context. Features SHALL retain Begin/Commit/Rollback/TryLock orchestration, current actor/session recheck, ordered locks, entity-version checks and required atomic persistence. ORM composition and write results MUST NOT create another pool, own transaction finalization or introduce parallel writes on a shared transaction. Failed build/read/write/commit, cancellation and panic SHALL retain safe fail-closed mapping and cleanup. Read snapshots SHALL remain consistent across actor authority and response data.

#### Scenario: Required persistence fails after mapping change
- **WHEN** soft-delete, restore or insertion succeeds tentatively but a later required version/audit/outbox/result write fails
- **THEN** confirmed rollback leaves the previous mapping set and timestamps intact; no success response or retained replay escapes

#### Scenario: Concurrent revocation races a read
- **WHEN** a controlled independent connection commits mapping revocation during an in-flight read snapshot
- **THEN** authorization and response use one consistent state, and the next snapshot observes revocation with the existing credentials

### Requirement: Refactor preserves public RPC behavior and admission

The 35 registered gRPC methods and their names, field numbers/types, error catalogs/locales, safe payloads, filters, sort/null/tie rules, counts and cursor formats SHALL remain compatible. `/user.v1.UserService/ListUsers` SHALL keep unique-user totals, numeric role filters, ACTIVE collection gates and numeric/composite cursor behavior; role catalog counts SHALL preserve the current affected-user definition. `/user.v1.UserService/Register` SHALL retain PENDING/token-empty, nonprivileged default membership and no credentials. TesRPC SHALL remain. Service-token TLS/method/namespace checks, verified ownership, restricted policy, EdDSA-only issuer validation with audience ignored, access at most 15 minutes, absolute 7-day sessions and 30-second JWT skew SHALL remain. Existing limiter numbers and failure-audit policy SHALL remain. This change SHALL create no new RPC or api-contracts protobuf/error update.

#### Scenario: Collection sorting and filters include duplicate joins and absent collections
- **WHEN** authorized clients list users or roles with overlapping assignments, null collection sort keys, filters and page continuation
- **THEN** counts, ordering, keys, cursor validation and rows match the existing contract without duplicate entities, omissions or skipped matches

#### Scenario: Existing authentication and mutation paths run after refactor
- **WHEN** existing clients exercise self/admin B/C/D/E, authentication, session and profile operations
- **THEN** existing permissions, quotas, credential modes, error details and side-effect rules remain, with no new role-name/JWT bypass

### Requirement: Deliverable requires compatible ORM and additive soft-delete schema

The service deliverable SHALL identify the tested ORM source/release version and compatible role_permissions schema. Existing pairs SHALL gain nullable deleted_at with NULL preserving their prior active meaning, while composite pair uniqueness remains. Missing schema or an unavailable published dependency SHALL be reported as an unsatisfied delivery prerequisite, not a successful fallback. Workspace tests alone SHALL not prove release readiness. A release delivery SHALL resolve and pass required standalone checks against a real published ORM version. Application-schema migration and deployment execution SHALL remain separate explicitly authorized operations; an old binary that ignores tombstones MUST NOT be used as a safe rollback after soft deletion becomes active.

#### Scenario: Schema expansion preserves existing rows
- **WHEN** the reviewed additive migration is applied in an isolated controlled test
- **THEN** existing pairs remain unique, stored and active with deleted_at NULL, without deleting/recreating mappings or changing role/user counters

#### Scenario: Published ORM dependency is unavailable
- **WHEN** local workspace passes but standalone service cannot resolve the required ORM APIs from a published version
- **THEN** release verification is BLOCKED and no module-cache edit, invented version or implicit local replacement is claimed as published delivery

## Traceability

Owner grpc-auth for adapters/business/schema; sibling ORM for generic composition and insert-result APIs. All ORM-01..07 criteria are PASS with actual evidence in the [design ledger](../../design.md#acceptance-ledger). Prerequisites are implemented auth/session/transaction/B/C/D/E, supported ORM and additive schema. AC-09/10/13/21–26/31 and M-02/05/07/08/09/10/11 retain their existing meaning; this change tests their affected subset. EvaluateAccess/ListAuditLogs and actual consuming-client/deployment Phase 6 DEP-01 remain separate.

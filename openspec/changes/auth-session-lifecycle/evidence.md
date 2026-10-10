# Apply evidence

## Status — 10 Oktober 2026

**1/33 tasks complete:** task 1.3 static schema/fixture comparison. Runtime lifecycle implementation remains NOT_STARTED; task 1.1 contract gate and dependent integration are BLOCKED. This report does not claim lifecycle, PostgreSQL tests or deployment readiness PASS.

## Contract preflight — task 1.1 remains blocked

[lifecycle-preflight.json](lifecycle-preflight.json) records both workspace and pinned api-contracts inventories, required methods, hashes and gaps. The currently pinned release is `v0.0.0-20261010071509-42a9e6fa23ad`; both inventories contain only the six existing user/profile RPCs and lack all nine lifecycle RPCs. REFRESH_TOKEN_REUSED is also absent. Existing Login response fields do not supply AuthService/SessionService definitions.

Required separate deliverable: [SLC-EXT-01..05](design.md#external-contract-prerequisites), including published contracts/errors/mappings/locales and generated code, followed by an explicitly scoped grpc-auth dependency update. No sibling, generated file, module cache or dependency was changed. Task 1.1 stays unchecked because its required deliverable is missing; source inventory alone cannot pass it.

## Static schema comparison — task 1.3 complete

Compared root tests/integration/postgres/fixtures/schema.sql against the archived foundation schema-preflight.json captured at 2026-10-10T06:41:26Z. The report includes a six-table column matrix, relevant saved constraints/indexes and current backend source SHA-256. Required auth/password/counters, session terminal state, refresh consumed_at/replaced_by/hash/family, audit/outbox and idempotency fields are present in both sources. No migration gap was identified by this static review.

Important implementation constraints: replaced_by is an immediate self FK, so insert the successor before linking the predecessor within the same transaction; revoked session rows require revoked_at and a trimmed 3–500 character reason. Session terminal revision remains conceptual version 2; no sessions.version column is required. Application validation must preserve the 32-byte digest format even though the DB constraint accepts any nonempty bytea.

This is saved metadata/fixture inspection, not a fresh live-schema verification or a functional test. No database connection, migration, seed, server, Docker or application tests were run. SLC-09 remains NOT_STARTED and actual deployment checks remain mandatory [Phase 6 DEP-01](../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment).

## Verification

Module resolution completed in workspace and GOWORK=off GOPROXY=off modes. Static assertions confirmed all nine required RPCs absent in both proto and generated method inventories; required fixture/saved columns and token-hash uniqueness/replaced_by foreign key are present. Exact source hashes and commands are retained in the JSON report. No runtime PASS is inferred from these checks.

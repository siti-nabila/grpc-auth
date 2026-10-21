# Audit-log reads implementation evidence

Date:18 October2026 (Asia/Jakarta). Status:**BACKEND_COMPLETE,22/22 tasks complete**. All required contract, backend and dedicated PostgreSQL gates pass. Phase6 DEP-01 remains separate.

## Deliverable and identity

- RPC:/audit.v1.AuditService/ListAuditLogs. Current unrestricted actor requires exact iam.audit.read; actor filters select safe history. Authority/total/page use one primary read-only snapshot, fixed occurred_at DESC/id DESC and typed safe snapshots.
- Contract:b4ad807ad5faafe35c3c5081932597f43f8d9e7f, published with explicit user authorization on existing feat/role-management; exactly11 scoped files committed. Unrelated proto/response.json remains untouched. Published/pinned api-contracts v0.0.0-20261018073250-b4ad807ad5fa; module checksum h1:Ib/KQ53uqsoWXZ1JbPi1ILjKyLqUdbVjb+s2Q8C5uOs=; go.mod checksum h1:Jb5bNxnX/aW6YWghT/pXorXlmIDRRxXltY8gL7Udzg4=.
- Prior18 complete project descriptors and36 methods/eight services were frozen in [baseline.json](baseline.json). Exactly37 methods/nine services now register; prior descriptors are unchanged as protobuf messages in workspace and standalone inventories. ORM remains v1.8.0-support-subquery.
- [Implementation source](implementation-source.json) hashes33 scoped source/docs/config files, including27 gofmt-clean Go files. All721 other captured baseline files and the schema_fixture.go rename remain unchanged. Private configs, credentials, runtime artifacts and graph outputs are excluded.
- [Contract source](contract-source.json) records11 prepared/published file digests, commit/version/checksums and authorization. grpc-auth source remains in its working tree; it was not committed/pushed by this change.

## Verification results

| Gate | Actual result | Status |
| --- | --- | --- |
| Contract targeted/full tests and vet |140 tests pass,0fail/skip in workspace and GOWORK=off; all prior descriptors/tags/types preserved | PASS |
| Contract generator roundtrip |All generated Go files byte-identical on regeneration; no manual generated edits | PASS |
| Domain/reader/feature/handler/interceptor/registered controlled suites |Exact query/UTC bounds/BIGINT/presence/strict snapshot/context/order/cleanup/current authority/attempt/rate/retry cases pass | PASS |
| Full default tests |3513 pass,0fail/skip in workspace and GOWORK=off | PASS |
| Full default race detector |3513 pass,0fail/skip in workspace and GOWORK=off | PASS |
| Vet/build |Workspace and GOWORK=off pass; temporary BIN_DIR keeps repository binaries untouched | PASS |
| Target PostgreSQL normal/race |26 pass each,0fail/skip; six count/page barriers each use two distinct physical backends | PASS |
| Full PostgreSQL workspace normal/race |583 pass each,0fail/skip | PASS |
| Full PostgreSQL standalone normal/race |583 pass each,0fail/skip | PASS |
| Shape/scope/docs |Example and metadata YAML objects valid;37 unique exact example methods; strict OpenSpec, local links/anchors, gofmt and diff checks pass | PASS |

[Backend receipts](backend-checks.json), [contract receipts](contract-checks.json) and [PostgreSQL receipts](postgres-checks.json) preserve exact commands, environment mode, UTC times, exit codes, test counts and log SHA256. PostgreSQL receipts also retain actual audit case names and distinct physical backend IDs. Raw logs remain in temporary storage; durable evidence contains no DSN/password/credential. GOCACHE uses writable temporary storage because the regular cache is sandbox-restricted.

## Acceptance and traceability

All22 spec scenarios map to actual test groups in [scenario traceability](scenario-traceability.md). ALR IDs below retain the scope and prerequisite meanings of the planning ledger and original AC/M IDs.

| Criterion | Scope / owner / prerequisite | Actual evidence / status |
| --- | --- | --- |
| ALR-EXT-01 / EXT-01 |api-contracts; separately authorized source and publication |Typed tags/presence/BIGINT/wire/redaction compatibility, generator,140-test workspace/standalone and published resolution:PASS |
| ALR-01 / AC-31,M-01/02/03/05 |grpc-auth; published contract and auth/resolver foundation |All37 exact caller boundaries; auditor-only permission, spoofed metadata, namespace/JWT/session/restricted/stale authority and historical selectors:PASS |
| ALR-02 / M-05/06 |grpc-auth; typed domain/reader |Default/presence/unknown/AND/range/control/forbidden query options; canonical method/filter cursor, BIGINT through MaxInt64, fixed tie/order/offset/keyset/meta; PostgreSQL submicrosecond bounds:PASS |
| ALR-03 / M-06/07 |grpc-auth; strict typed allowlist |NULL/empty/both actors, inactive historical actor/target, typed snapshots; invalid/unknown secret/nonobject/null snapshots suppress whole page; no raw fallback/partial response:PASS |
| ALR-04 / M-02/07/09 |grpc-auth; primary snapshot and dedicated DB |Strict handle/context/order failures; six two-backend barriers insert/mapping/assignment/role/session/account between count/page, next-state visibility and old JWT recheck; pool reuse/cancel/deadline/panic/driver/finalization normal/race:PASS |
| ALR-05 / M-10/11 |grpc-auth; existing admission/attempt recorder |Mixed old/new invalid-input quota, one bounded user bucket, RetryInfo/locale; zero success effects, fresh incidental-key retry, one error attempt after cleanup, recorder failure/panic and real failing-audit trigger preserve original error:PASS |
| ALR-06 / AC-31,M-05/06/07 |grpc-auth; pinned published contract and complete evaluator baseline |37/nine inventory,18 prior descriptors, prior36 secure behavior and full backend/PostgreSQL normal/race in both modes;33 scoped file hashes and721 unrelated files preserved:PASS |
| DEP-01 |External consumers/deployment owner |Remains separate,not claimed PASS; no deployment/caller activation/private key/config/migration or retention job |

## Scope and execution notes

All new tests/scenarios/mocks/fixtures live under root tests/. Default suites use controlled dependencies/in-memory TLS; the postgres build-tag suite uses only the guarded dedicated test database and owned fixture namespaces. Full PostgreSQL processes ran sequentially. No Docker, application database, baseline migration/seed or application server startup was used.

Initial inventory-helper filtering and new descriptor-presence expectations were corrected before the final contract gates. Unit checks exposed an invalid stored zero instant and reader-error expectation mismatch; these were corrected before final backend gates. The sandbox initially prevented the test DB connection; the authorized opt-in run outside the sandbox then exposed a multi-statement parameterized fixture setup, which was split into supported statements before every final PostgreSQL run. Earlier failed attempts are not counted as acceptance PASS.

Full Phase4 mutation/outbox/idempotency reconciliation, Phase5 broader metrics/hardening and Phase6 consumer/deployment/retention readiness remain separate. Main-spec sync/archive remains a subsequent workflow.

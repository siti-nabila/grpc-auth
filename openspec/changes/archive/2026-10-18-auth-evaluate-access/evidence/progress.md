# EvaluateAccess implementation evidence

Date: 18 October 2026 (Asia/Jakarta). Status: **BACKEND_COMPLETE, 24/24 tasks complete**. All required contract, backend and dedicated PostgreSQL gates pass.

## Deliverable and identity

- RPC: /authorization.v1.AuthorizationService/EvaluateAccess; default registration is 36 methods/eight services with prior 35 preserved.
- Contract: bfc60a40a858c5a2b154d7049e2a97fd2eb80ffe, published on existing feat/role-management with explicit user commit/push authorization. The 10 scoped contract files were committed; unrelated proto/response.json is preserved.
- Published dependency: api-contracts v0.0.0-20261018052225-bfc60a40a858, resolved standalone with module checksum h1:uHJAzZd2N0LpmZSDvjF2RvqmSzoaJcqvEuk+mZJVP0g= and go.mod checksum h1:Jb5bNxnX/aW6YWghT/pXorXlmIDRRxXltY8gL7Udzg4=.
- ORM remains v1.8.0-support-subquery. Base HEAD, 562 safe baseline file digests and frozen 35-method/seven-service generated descriptor inventory: [baseline.json](baseline.json).
- Scoped implementation hashes: [implementation-source.json](implementation-source.json). Published contract hashes: [contract-source.json](contract-source.json). Private config, credentials, runtime files and graph artifacts are excluded. All unrelated baseline files and the schema_fixture.go rename are preserved.

## Verification completed

| Check | Expected / actual | Status |
| --- | --- | --- |
| Contract targeted/full tests + vet, workspace and GOWORK=off | All tests pass; existing 16 full descriptors unchanged; 36 methods/eight services | PASS |
| Contract make proto clean round-trip | All generated files byte-identical on second generation | PASS |
| Domain input tests | Five valid and nineteen invalid cases; required/trim/case/length/segments/wildcard/namespace rules | PASS |
| Feature tests | Primary read-only handle/order/context, all gate outcomes, reauthentication, begin/read/finalization/cancel/panic cleanup | PASS |
| Handler/interceptor/registered controlled suites | Safe mapping, exact identity/namespace, denial/error audit once, quota and localized failure behavior | PASS |
| Default go test ./... / go test -race ./... | Workspace and standalone normal/race pass; JSON runs report 3384 passing tests, zero failures/skips | PASS |
| go vet ./... and make build with temporary BIN_DIR | Workspace and standalone pass; repository binary untouched | PASS |
| Targeted PostgreSQL normal/race | 21 passing tests each, zero failures/skips | PASS |
| Full PostgreSQL workspace normal/race | 557 passing tests each, zero failures/skips | PASS |
| Full PostgreSQL standalone normal | 557 passing tests, zero failures/skips | PASS |
| Full PostgreSQL standalone race | 557 passing tests, zero failures/skips | PASS |

[Backend receipts](backend-checks.json) retain commands, mode, UTC start/end when captured, exit, counts and log hashes. [PostgreSQL receipts](postgres-checks.json) retain times, modes, counts, evaluation case names and physical-backend IDs. Raw logs stay in temporary storage; durable summaries contain no credentials. GOCACHE uses a writable temporary directory because the regular user cache is sandbox-restricted.

## Requirement-to-test traceability

| Criterion | Scope / owner and prerequisite | Actual evidence / status |
| --- | --- | --- |
| EVA-EXT-01 / EXT-01 | api-contracts; explicit source and later publication authorization | Published compatible module, full descriptor/enum/wire tests, clean generator, workspace/standalone tests/vet: PASS |
| EVA-01 / M-01/02/03/05/06, AC-31 | grpc-auth; published contract + service/session foundation | Feature/handler/registered cases cover business-only namespace, missing/expired/mismatched/revoked/restricted identity, stale interceptor recheck and spoofed metadata; prior/new exact caller boundaries: PASS |
| EVA-02 / AC-09/10/11/12/13/23 | grpc-auth; concrete application/access readers + active-role resolver | Decision table, selected unknown-app PERMISSION_MISSING, IAM exception, overlap, disabled role, soft-delete/restore, cross-application rows and subset regrant; controlled and PostgreSQL commands: PASS |
| EVA-03 / AC-13/23/27, M-02/07/09 | grpc-auth; RR READ ONLY provider and leased cleanup | Six two-physical-backend barrier cases for assignment/mapping/application/access/session/account; next snapshots see commit; failure injection suppresses partial responses and pool reusable: PASS workspace/standalone normal/race |
| EVA-04 / M-05/10 | grpc-auth; existing ProtectedRead limiter | Mixed old/new invalid requests exhaust one verified-user bucket; RetryInfo, exact admission and existing refill/cap/TTL/cleanup regression pass: PASS |
| EVA-05 / M-07/11 | grpc-auth; existing independent safe recorder | False decisions audit REQUEST_DENIED once after read cleanup; errors audited once, allowed has no success audit, rate has no DB audit, audit failure/panic preserves outcome; registered SQL ordering and PostgreSQL trigger fault: PASS |
| EVA-06 / AC-09/11/12/13/23/27 | grpc-auth; B/C/D and dedicated PostgreSQL | Original JWT survives privilege changes; mapping remove/restore, overlap and app subset regrant, other-app/session preservation; inactive descriptive views, password/global revoke, cleanup and pool reuse: PASS workspace/standalone normal/race |
| EVA-07 | grpc-auth; published contract + complete evaluator | Exactly36/eight registration, prior 35 retained; workspace/standalone default/vet/race/build PASS; full PostgreSQL normal/race PASS in workspace and standalone; 557 passing tests in each run |
| Phase 6 DEP-01 | External consuming-client/deployment owner | Remains separate and not claimed PASS; no deployment or private runtime caller/key changes |

## Scope and remaining verification

All tests and fixture code live under root tests/. Default suites use controlled SQL, in-memory TLS and injected time; PostgreSQL suites are explicit opt-in with a dedicated-test guard and owned fixture namespaces. Full database normal/race processes run sequentially. No Docker, application database, baseline migration/seed or application server startup was used.

A temporary standalone test launcher initially imported an undeclared YAML path. The launcher was corrected to use the existing pinned go.yaml.in/yaml/v3 package; go.mod was not tidied or extended for that helper. The incomplete launcher attempt is not counted as a test PASS.

Contract/source and module publication are complete. All full standalone PostgreSQL gates, final source manifest, strict OpenSpec/link/YAML/scope review and checklist closure passed. ListAuditLogs, full Phase 4 mutation reconciliation, full Phase 5 hardening and deployment readiness are separate increments.

## Final review

All 22 scoped Go files are gofmt-clean. Example config and change metadata parse as YAML objects; the caller example contains 36 unique exact methods. Change/API local links and anchors, strict OpenSpec validation and git diff --check pass. Scoped source hashes match the delivered source; all unrelated captured user files and the fixture rename remain unchanged. grpc-auth changes remain in its working tree; only the explicitly authorized api-contracts commit was pushed. Main-spec sync and archive were completed on 18 October 2026; see the archive record below.

## Archive record

Archived on 18 October 2026 using schema spec-driven after all 24 tasks and artifacts were complete. The access-evaluation main spec contains all 9 requirements and 26 scenarios unchanged. Relative documentation links were rebased for the archive location. Historical source manifests and verification receipts remain unchanged; their digests describe the implementation delivery before this documentation-only archive. [archive-source.json](archive-source.json) records the documentation changes and retained evidence digests. Phase 6 DEP-01 remains separate.

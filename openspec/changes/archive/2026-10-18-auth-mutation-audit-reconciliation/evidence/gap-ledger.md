# Reconciliation gap ledger

Owner: grpc-auth. Status: CLOSED by actual current-deliverable normal/race verification; no production defect found.

| ID | Expected / observed | Action and verification | Status |
| --- | --- | --- | --- |
| MAR-G01 | Commit failure may mean committed state with lost acknowledgement; existing commands.go commitFailureTransaction returns before underlying commit, while common TransactionOwner explicitly documents unknown outcome | Add controlled ownership and real production UpdateUser/refresh-reuse post-commit acknowledgement-loss cases; prove no success/automatic rerun, actual atomic rows, safe management retry and credential suppression | CLOSED: controlled and PostgreSQL normal/race PASS |
| MAR-G02 | Producer correlation must preserve bounded request/trace metadata and argument-owned actor; UserEvent/RoleEvent share eventCorrelation, SessionRevokedEvent delegates UserEvent; persistence tests already cover all 12 event catalog types | Add focused common producer-builder correlation assertions for valid, absent, duplicate and invalid metadata and sentinel auth inputs; reuse existing safe persistence/producer exact-count suites | CLOSED: controlled and PostgreSQL normal/race PASS |
| MAR-G03 | Reuse must keep busy/error namespaces and business ordering; existing AcquireLocks already provides shared lock operation, while managementLocks/roleLocks translate their consumer catalog | Compare actual callers and retain consumer wrappers if further abstraction has no concrete benefit; existing strict failure matrices plus final reruns prove preserved behavior | CLOSED: keep separate; all caller regression PASS |

Optional nullable audit request_id/trace_id fields retain their existing producer semantics. This change verifies the bounded typed persistence contract and event request-id/correlation/trace propagation; it does not introduce a new audit metadata transport convention. No production defect is claimed from nullable historical metadata or source sampling alone.

No schema, dependency, external contract, authentication policy, or business behavior change is planned. New evidence must identify deliverable/source and actual commands; historical receipts are routing only.

Final evidence: [checks](backend-checks.json), [PG checks](postgres-checks.json), [scenario mapping](scenario-traceability.json), [reuse callers](reuse-callers.json). MAR-G01 is per-consumer coverage: existing role/assignment/app suites already had actual uncertain-commit proof; new cases extend it to UpdateUser and refresh reuse. No mandatory reconciliation gap remains unresolved.

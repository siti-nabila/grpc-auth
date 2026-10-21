# Implementation evidence — auth-user-role-assignment

Baseline grpc-auth `2462cb0`, branch `feat/role-management`, Go1.26.2 on
Darwin/arm64. Backend files remain uncommitted; no deployment was performed.
**Backend C COMPLETE:36/36 tasks,30 delta scenarios, twelve required full gates PASS.**

Contract C was generated, committed and pushed on the authorized api-contracts
branch; [contract evidence](evidence/contract.md) records publication and sums.
The backend explicitly pins `v0.0.0-20261012102712-bfb073390ea9`.

## Verification environment and gates

Default suites use controlled SQL, clocks, entropy, and in-memory TLS transport.
`GOCACHE=/tmp/auth-role-build-cache` is a disposable compilation cache.
Workspace mode uses the existing parent go.work; standalone uses `GOWORK=off`
and the published C module, without local replace. Actual commands, durations,
exit status and log digests are recorded in [verification](evidence/verification.json).

Opt-in PostgreSQL runs load ignored root env-test.yaml privately through a local
wrapper. `GRPC_AUTH_POSTGRES_TEST_ENABLED=1`, dedicated DSN/database and owned
namespace fixture checks are enforced; secrets are absent from this evidence.
All four full suites execute sequentially, workspace normal/race then standalone
normal/race, with no skipped tests. Existing refresh/reuse/password/logout/status/
role/user scenarios execute along with C. No Docker, application database writes,
application migration or business-row reads were performed.
[Schema comparison](evidence/schema.json) uses archived B metadata and the owned
fixture, recording the one equivalent status-check cast rendering difference.

## Acceptance ledger and source mapping

| Criterion / original coverage | Actual implementation and exercised proof |
| --- | --- |
| URA-01 / AC-31, M-05 | exact handler/session/IAM/service routing; lifecycle_registered.go inventory, TLS/service/principal matrix; assignment_race.go locked actor/session/generation/assignment/role/app/permission invalidation |
| URA-02 / AC-04/10 C subset | historical safe domain/handler/query; feature List snapshot failure matrix; assignment_list.go two physical backend RR barrier changes authority, target, source, role/app/access, total; old response consistent, fresh denial/current read |
| URA-03 | domain assignment query/cursor scenarios; SQL allowlist34 directions; assignment_list.go PostgreSQL all17 fields both directions, mixed ties/nulls, literal search, filtered unique totals, target cursor and hidden projection |
| URA-04 / AC-04..07/35 | grant_roles.go; strict feature matrix and assignment_management.go registered new/reactivated/already-active batch, same ID/source, one version/event, raw21 invalid, whole invalid batch and IAM exception |
| URA-05 / AC-09/35 C subset | revoke_roles.go; registered ACTIVE/REVOKED/absent cases, no absent insert, unchanged old source, inactive target/disabled policy cleanup, unknown mixed role rejection |
| URA-06 / AC-19/20 | shared last-admin key and ManagementPolicyReader.AdminCount; assignment_race.go self nominal no-op, singleton failure, six two-backend revoke/revoke and revoke/status orders, current recount retains one admin |
| URA-07 / AC-24..26 | canonical key/fingerprint, authority before replay/version; assignment_management.go scope/expiry/counts and assignment_race.go actual committed response-loss error followed by original replay |
| URA-08 / M-07/08/11 | strict all-read/write/error/panic/cancel tests; actual assignment/reactivation/revoke/later write/version/zero-row/audit/outbox/result/deferred-commit failures, after-write panic/cancel rollback, independent audit failure, preserved safe error and lock/pool cleanup |
| URA-09 / AC-09/10 C subset | original registered Login JWT/Me plus primary resolver, overlapping role contribution before/after each transition; byte-equal password/auth_version/session/refresh/app-access/profile/other role state |
| URA-10 / M-10 | handler validation/projection, registered30 inventory, localized error decoding; shared old/new concurrent read/mutation quota, invalid-input counts, RetryInfo/no effects; C security-attempt matrix cause/cancel/deadline/panic and bounded failure telemetry |
| URA-11 / M-11 | full default/vet/race/build workspace/off plus four full opt-in PostgreSQL suites, recorded commands/durations below |
| URA-EXT-01 | api-contracts baseline27 descriptors/tags, generated C30 inventory, two deterministic generations, dictionaries/locales/mappings, full/vet both modes, published commit and standalone download |

Every C scenario is mapped individually in [scenario traceability](evidence/scenarios.md).
Original AC-09/10 are only satisfied for C assignment contribution. D application
access RPCs, E effective-permission RPCs/EvaluateAccess and CLM actions are not
claimed implemented. Phase6 DEP-01 remains OPEN and mandatory before deployment.

## Ownership, scope and review

Assignment feature owns transaction lifecycle; all persistence is bound to one
handle. Ordered exclusion and bounded missing-predecessor restart reuse existing
foundation utilities. A commit error never triggers automatic mutation retry.
Audit/outbox/result are mandatory in the same outcome. New no-op emits one NO_OP
and result only; replay emits nothing. Pending outbox requires no running broker.
List uses a primary read-only REPEATABLE READ snapshot. Domain item/query contains
no credential field and nullable legacy provenance is preserved.

All new tests reside under tests/, with thin runners and testutils.Run. No new
connection-per-request, pool fallback, migration, runtime config key or dependency
other than the published C contract was introduced. Source/generation/module
checksums are in [manifest](evidence/manifest.json). The untracked api-contracts
proto/response.json belonging to the user is preserved and excluded from commit.
Strict OpenSpec validation, changed-document links/anchors, diff whitespace,
test layout, source registration and secret/scope review results are recorded
in verification.json. Existing B documentation remains historical inventory27;
[current C API/inventory30](../../../../docs/user-role-assignment-api.md) and
[Phase2 C](../../../../docs/phase-2-management-plan.md#6-increment-c--assignment-role)
provide the current status. No OpenSpec sync/archive is implied by implementation.

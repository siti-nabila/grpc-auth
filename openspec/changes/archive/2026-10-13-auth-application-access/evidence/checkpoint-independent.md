# Historical independent-stage checkpoint (superseded)

# Apply progress

Implementation in progress. This checkpoint is not completion, publication or deployment evidence.

Baseline HEAD/branch/status/nonignored hashes and backup inventory for grpc-auth and api-contracts are in baseline.json. Recovery copies under /tmp/aap-baseline preserve C/refactor and the preexisting untracked api-contracts proto/response.json. No ignored configuration/credential was copied. No reset/stash/commit/push or application DB operation was performed.

## Verified work

- Baseline default workspace and standalone: 2,690 passed test events each, no failures/skips; exact30 registered inventory covered by the baseline suite.
- Contract D: three additive methods, eight new messages, generator output and complete pre-D descriptor fixture. Existing30 descriptors/tags/types remain compatible. New wire/presence/query/docs scenarios and baseline B/C inventory assertions pass.
- Contract targeted/full workspace suites: 124 passed test events each; go vet PASS. These are checkpoint results before the pending system-application error decision and are not final publication gates.
- Domain inputs/result: raw grant bound20 before canonicalization, revoke without role selection, absent/access-only/role-only/>20 results, safe target/application binding and no unrelated retained fields.
- Catalog D: eleven public scalars, application_id identity/tie/count/numeric cursor, target-bound cursor and application_code/name search defaults. B/C fields/cursor encoding remain unchanged and serialized legacy fixtures pass.
- AND authority: plural exact checks use one state/contribution read per evaluation; singular caller semantics preserved; missing either permission, empty required set, restricted/dependency cases return no verified principal.
- Bound access/assignment reads, pair writes and history pages: optional absence, owner/application/provenance/row checks, >20 assignments, scalar query/projection/filter/cursor tests, conditional/unique/FK/dependency/cancellation failures and no-op write rejection. Latest repository/authority/common targeted suite: 839 passed test events, no failures/skips.

Counts include Go parent tests/subtests. JSON files record exact command, mode, time, exit, source identity and log location. Logs remain in /tmp. This checkpoint has no real PostgreSQL results; owned schema/fixture work and required PostgreSQL gates remain incomplete.

Full checkpoint standalone: 2,806 passed events with no failures/skips. Full checkpoint workspace: NOT PASS, with 2,804 passed events and one failing registered lifecycle scenario (plus its parent/package failure events). Generated local D descriptors now contain33 methods while the old production lifecycle inventory still expects30; feature D and group5 admission/wiring/inventory are pending. This is an expected incomplete-stage failure, not a baseline/environment failure and not a final gate waiver. Both runs use source identity `758193b041bdd801dbbe51ea6c8c6aedd5f56a7b4e092c45a4d5ce57d7163307`. Contract still has no publication/pin update. Required full workspace and standalone final gates must be rerun after D is completely wired.

## Pending contract decision

Existing application.system_application_protected (SAP) maps to FAILED_PRECONDITION but says “System applications cannot contain custom roles.” Reusing it for D access mutations follows the current artifact name but gives the wrong action-specific message. User input is pending: add a dedicated APPLICATION_ACCESS_SYSTEM_PROTECTED error for D while retaining the old key/code/messages, or retain existing SYSTEM_APPLICATION_PROTECTED unchanged. Neither choice has been implemented or published before the answer.

Task1.2 final error reconciliation, task1.4 final catalogs/documentation and task1.5 publication await that decision. Contract publication, dependency pin, application feature/handlers/admission/wiring and final backend gates are not complete. Generated workspace descriptors already include the additions; registered backend inventory/wiring assertions must be completed in group5. Standalone still uses the existing published C pin.

Skill openspec-apply-change requires pausing for a design issue: “Implementation reveals a design issue → suggest updating artifacts.” See .agents/skills/openspec-apply-change/SKILL.md. Current source work on independent domain/authority/repositories does not choose the pending error or weaken system-application protection. DEP-01 remains separate and open.

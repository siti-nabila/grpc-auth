# Reuse review

Decision: retain current production helpers and consumer wrappers. No production refactor is needed to close the observed coverage gaps. Owner: grpc-auth; review obeys root AGENTS.md and plan section2.1.

| Candidate / callers | Shared capability and invariant | Decision / evidence |
| --- | --- | --- |
| managementLocks: managementTransaction and assignment transaction in user/assignment_transaction.go; roleLocks: commandTransaction | AcquireLocks already handles canonical shared transaction locks; wrappers preserve user versus role RESOURCE_BUSY catalog and dependency/cause mapping | Keep consumer wrappers. Replacing them with a broader framework adds coupling; current feature matrices assert exact order/context/namespace. Actual targeted receipt mar-targeted:3387 pass,0fail/skip |
| UserEvent: register/profile/user management/assignment/app/global-password; RoleEvent: role outcome; SessionRevokedEvent: logout/self/admin/reuse | User/role aggregate payload differs; session aggregate uses UUID/revision2. All already reuse bounded eventCorrelation directly or through UserEvent | Keep named builders; new TestMutationEventCorrelation tests all three builders with absent/valid/boundary/duplicate/invalid metadata, actor spoofing and sentinel authentication inputs |
| Fingerprinter and Replay: user/role/assignment/app/admin session | Shared canonical hash/expiry/result logic already exists; consumer mappings and safe-result shapes differ | Keep existing consumer-specific checks. Targeted matrices cover current authority before replay, stale version, no-op, conflict, expiry and role/app differences |
| Existing PostgreSQL roleUncertainProvider versus new acknowledgement mock | Role-specific test wrapper exposes InsertWriter/CreateWith for generated IDs and returns its injected failure on every commit. New mock loses exactly one acknowledgement for UpdateUser/reuse then allows retry on the same service | Keep narrow test-only mocks; new one is not used as a general CreateRole transaction adapter. No production interface was widened |

Before/after: all internal/,pkg/,cmd/ source bytes match baseline. Six test files change/add assertions; consumer arguments/return value, exact business side effects, current authority, transaction/lock boundaries and error/locale contracts are unchanged. Targeted normal and final full/race plus dedicated PG receipts are tracked in progress; existing independent failure suites remain mandatory.

No mode flags, reflection, global registry, command framework, dependency update, copied event builder or new business abstraction was introduced. Keeping separate functions is the outcome of the candidate review, not a skipped refactor task.

The complete source-verified caller inventory (including assignment_transaction.go using managementLocks) is recorded in [reuse callers](reuse-callers.json); it preserves path, line and call expression for each reviewed helper.

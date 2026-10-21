# Spec Delta

## MODIFIED Requirements

### Requirement: Role metadata status and permission updates use explicit versioned outcomes

UpdateRole SHALL accept only name/description with explicit presence and matching
nonempty field_mask; selected empty description SHALL clear it and omitted fields
SHALL remain. Code/application/status/ownership/system/numeric-code updates SHALL
be rejected. ChangeRoleStatus SHALL accept explicit ACTIVE or DISABLED and reason
3–500 trimmed characters; ReplaceRolePermissions SHALL require explicit final-set
intent, reason 3–500 and registered same-application permissions, including empty
set to remove all active mappings. These methods SHALL require positive INTEGER-compatible
role_id and present positive int64-compatible expected_role_version. Stale new
requests SHALL return VERSION_CONFLICT/ABORTED before no-op. Effective change
SHALL increment role version once; a valid new no-op SHALL return changed=false
with one NO_OP audit and no event/version increment. Updates to an existing custom
role in a disabled application SHALL remain manageable, but SHALL NOT bypass
application gates during permission evaluation.

Role-permission pairs SHALL be retained physically. Removing a mapping SHALL set nullable deleted_at to the feature command timestamp; a re-added retained pair SHALL clear deleted_at rather than create a duplicate. Pairs remaining in the final set SHALL keep their state, and repeated removal SHALL not overwrite an existing deletion timestamp. Active mapping reads SHALL include only deleted_at NULL. The existing composite role_id/permission_id uniqueness SHALL cover active and deleted pairs. No hard DELETE or general DeleteRole RPC SHALL be introduced.

#### Scenario: Selected metadata is cleared while other fields are omitted
- **WHEN** authorized UpdateRole selects only present empty description with current version
- **THEN** description is cleared, name/code/application remain, and an effective update advances role version once

#### Scenario: Mask presence or immutable fields are invalid
- **WHEN** selected fields are absent, present fields are unselected, mask is empty/unknown or input attempts an immutable field
- **THEN** INVALID_ARGUMENT occurs with no business writes or saved success

#### Scenario: Same status set or metadata has stale version
- **WHEN** a new valid request repeats current values with a stale expected version
- **THEN** VERSION_CONFLICT/ABORTED occurs rather than a successful no-op

#### Scenario: Final permission set replaces rather than appends
- **WHEN** an authorized actor replaces A/B with B/C or explicitly supplies an empty set
- **THEN** the resulting mappings are exactly B/C or empty respectively, with one effective role-version increment and no changes to other roles

#### Scenario: Valid same values produce a new no-op
- **WHEN** an authorized new request has current version and canonical values/set identical to current state
- **THEN** changed=false and one NO_OP audit commit without another domain event, including unkeyed UpdateRole

#### Scenario: Removed mapping is retained and later restored
- **WHEN** an authorized versioned replacement removes permission A and a later valid replacement re-adds A
- **THEN** the same role/permission pair remains stored, first with deleted_at set and then NULL, without duplicate rows or changes to unrelated pairs

#### Scenario: Empty and identical sets preserve soft-delete history
- **WHEN** an empty final set removes all active mappings and a subsequent new request with current version repeats that empty set
- **THEN** the first command soft-deletes only active pairs and advances role version once; the second returns a valid no-op without changing retained deletion timestamps

### Requirement: Role mutation replay and required persistence have one atomic outcome

CreateRole, ChangeRoleStatus and ReplaceRolePermissions SHALL require exactly one
nonempty idempotency-key of at most 200 characters; UpdateRole SHALL accept an
optional key under the same protocol. Scope SHALL be verified actor + exact full
method + role-code/{canonical_code} for creation or role/{id} + key. Fingerprint
SHALL include normalized payload, presence, expected version and sorted unique
permission set, excluding correlation/deadline. Safe original response SHALL
persist for 24 hours; current authorization SHALL precede replay and replay
SHALL precede target version comparison. Different payload in the same scope
SHALL return IDEMPOTENCY_CONFLICT/ABORTED. Effective creation SHALL commit
ROLE_CREATED and `iam.role.created.v1` at version one; metadata/status SHALL
commit ROLE_UPDATED and `iam.role.updated.v1`; replacement SHALL commit
ROLE_PERMISSIONS_CHANGED and `iam.role.permissions_changed.v1`. Role/mappings,
version, audit, pending event and keyed safe result SHALL commit together.
Every precommit error, including business/read/conditional-write/cancellation/
panic, SHALL roll back all business writes. After cleanup, safe failure audit
SHALL use an independent transaction; audit failure SHALL preserve the original
error and bounded telemetry. Commit uncertainty SHALL NOT be claimed success
or certain rollback. Broker availability SHALL NOT be a prerequisite.

Soft deletion, restoration and new mapping insertion SHALL share that same command transaction. A failure at any of those stages SHALL restore the previous active set and deletion timestamps on confirmed rollback; replay SHALL perform no mapping write, including no timestamp refresh.

#### Scenario: Response is lost and permission input order changes
- **WHEN** a committed keyed request is retried within retention with the same canonical permission set in a different order
- **THEN** the original safe response is returned with no extra mappings, audit, event or version, even if target version subsequently advanced

#### Scenario: Conflicting payload different target or expired retention
- **WHEN** the same scoped key carries different input, the key is reused for another target, or retention expires
- **THEN** conflicting input returns ABORTED, another target uses independent scope, and expired scope is processed as a new atomically excluded command with current version/business checks

#### Scenario: A later mapping guard or required write fails
- **WHEN** a soft-delete/restore/insert/version/audit/event/result step or subsequent business guard fails after tentative writes
- **THEN** the whole business transaction rolls back and independent failure auditing can record the safe reason without success effects

#### Scenario: Failure audit also fails after cancellation or panic
- **WHEN** cancellation/panic aborts a command and the separate audit transaction fails
- **THEN** cleanup releases resources, the original safe error remains and bounded telemetry does not claim persisted audit

#### Scenario: Commit outcome is unknown or broker is absent
- **WHEN** commit cannot establish its outcome or an otherwise valid command commits without a relay
- **THEN** uncertainty is reported without fabricated rollback certainty, while confirmed committed events remain pending without requiring RabbitMQ

#### Scenario: Restoration or soft deletion fails during replacement
- **WHEN** a command fails after a tentative mapping soft-delete or restoration and rollback is confirmed
- **THEN** all mapping states/timestamps and role version remain as before, with no committed success audit, outbox or replay result

Traceability: [ORM acceptance ledger](../../design.md#acceptance-ledger). Owner grpc-auth; prerequisite additive ORM and nullable schema; status PASS; actual evidence: [apply ledger](../../evidence/progress.md). Existing AC IDs and other requirement/scenarios remain unchanged.

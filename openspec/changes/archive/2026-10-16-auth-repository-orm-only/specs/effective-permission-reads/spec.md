# Spec Delta

## MODIFIED Requirements

### Requirement: Role-derived unions preserve every currently contributing source

Both RPCs SHALL return each permission code once, with unique source role ID/string-code pairs. Only ACTIVE assignments and ACTIVE roles on ACTIVE applications SHALL contribute; business applications SHALL additionally require target application access ACTIVE. Role and permission application IDs and code namespace SHALL agree. IAM SHALL not require an IAM application grant. REVOKED assignments, DISABLED roles, wildcard, inheritance, direct grants, JWT claims and cached/event projections MUST NOT contribute. Permission codes SHALL sort ascending lexicographically and each source-role collection SHALL sort ascending by numeric role ID. Results SHALL be complete for the requested namespace scope, without pagination, silent truncation, search or caller-controlled projection/sort.

Both /user.v1.UserService/GetMyEffectivePermissions and /user.v1.UserService/GetUserEffectivePermissions SHALL omit any role-permission mapping with deleted_at non-NULL from both permission codes and source roles. This SHALL apply to descriptive PENDING/SUSPENDED/DEACTIVATED target views too; those views SHALL not reconstruct deleted mapping history. Restored pairs SHALL contribute under the existing namespace and application/access gates in a subsequent snapshot.

#### Scenario: Overlapping role contributions
- **WHEN** two active roles provide the same permission, including duplicate underlying join rows
- **THEN** one permission appears with both unique role sources in deterministic order

#### Scenario: One source is revoked
- **WHEN** one of two contributing assignments is revoked before the next snapshot
- **THEN** the shared permission remains with only the surviving role; permissions exclusive to the revoked role disappear

#### Scenario: Business gate or mapping is invalid
- **WHEN** business access is absent/REVOKED, application or role is DISABLED, assignment is REVOKED, or mapping crosses application boundaries
- **THEN** the affected rows contribute nothing even if old JWT claims contain those roles

#### Scenario: Only one overlapping source mapping is soft-deleted
- **WHEN** an authorized permission read observes two otherwise eligible source roles but one role/permission mapping has deleted_at set
- **THEN** the permission is returned only from the nondeleted source, in the existing deterministic order

#### Scenario: Inactive target retains a soft-deleted mapping
- **WHEN** an authorized admin reads a nonactive target whose only source for a permission is soft-deleted
- **THEN** the response still includes target status and safe metadata but omits that permission and source

Traceability: [ORM acceptance ledger](../../design.md#acceptance-ledger). Owner grpc-auth; prerequisite additive ORM and nullable schema; status PASS; actual evidence: [apply ledger](../../evidence/progress.md). Existing AC IDs and other requirement/scenarios remain unchanged.

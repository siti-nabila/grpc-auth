# Spec Delta

## MODIFIED Requirements

### Requirement: Effective permissions are an authoritative active-role union

Permission SHALL merupakan union mapping dari assignment ACTIVE dan role ACTIVE pada primary database, dengan contributing roles dipertahankan. Business application permission SHALL juga memerlukan application ACTIVE dan application access ACTIVE milik user. Role code numerik tertentu, wildcard, inheritance, direct grant, JWT role snapshot, event atau cache MUST tidak menjadi bypass. IAM self-service MUST tidak memerlukan admin role atau IAM application grant.

Only role-permission mappings with deleted_at NULL SHALL contribute. A retained soft-deleted mapping SHALL not grant authority even when its role, assignment and application remain ACTIVE or the role is disabled and later enabled. Restoration SHALL contribute only after a committed authorized replacement and only under the existing account/session/application/access gates.

#### Scenario: Two active roles contribute the same permission

- **WHEN** dua role aktif memberi permission yang sama dan satu assignment dicabut sebelum evaluasi berikutnya
- **THEN** permission tetap diberikan dari role aktif yang tersisa dan source contribution mengikuti snapshot terbaru

#### Scenario: A role or application gate is inactive

- **WHEN** assignment/role nonactive atau business application/access nonactive
- **THEN** kontribusi role tersebut tidak memberi permission walaupun JWT masih mencantumkan role lama

#### Scenario: A self-service user has no admin role

- **WHEN** active verified user/session tanpa IAM admin role atau IAM app grant memanggil Me atau UpdateProfile yang memenuhi restricted/ownership policy
- **THEN** self-service tidak ditolak hanya karena tidak mempunyai privilege admin

#### Scenario: Soft-deleted actor permission cannot authorize a new request
- **WHEN** the sole mapping supplying an actor permission is soft-deleted and committed before the next protected read, mutation or replay recheck
- **THEN** the operation lacks that permission even with still-valid credentials, without revoking the session or increasing auth_version

#### Scenario: Restored mapping remains subject to business access
- **WHEN** a mapping is restored but its business application access is REVOKED
- **THEN** restoration alone grants no effective permission

Traceability: [ORM acceptance ledger](../../design.md#acceptance-ledger). Owner grpc-auth; prerequisite additive ORM and nullable schema; status PASS; actual evidence: [apply ledger](../../evidence/progress.md). Existing AC IDs and other requirement/scenarios remain unchanged.

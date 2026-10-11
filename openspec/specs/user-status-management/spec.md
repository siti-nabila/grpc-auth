# user-status-management Specification

## Purpose

Mengelola status akun melalui transisi yang diaudit dan diberi version, menjaga
minimal satu admin aktif, serta membatalkan credential secara atomik saat akun
dinonaktifkan tanpa memulihkan sesi lama pada aktivasi ulang.

## Requirements

### Requirement: ChangeUserStatus validates explicit transitions and prevents self inactivation

`/user.v1.UserService/ChangeUserStatus` SHALL require the management caller and
exact `iam.user.status.update` policy, positive target ID/expected_user_version,
explicit target status and trimmed reason 3–500 characters. Allowed transitions
SHALL be PENDING to ACTIVE/DEACTIVATED, ACTIVE to SUSPENDED/DEACTIVATED,
SUSPENDED to ACTIVE/DEACTIVATED and DEACTIVATED to ACTIVE; no transition back to
PENDING SHALL be accepted. The actor SHALL NOT suspend/deactivate their own
account. A new same-status request with a correct version and otherwise valid
input SHALL follow management no-op policy; stale version SHALL conflict before
no-op. Required idempotency/replay SHALL follow user-administration.

#### Scenario: Pending registration is activated
- **WHEN** an authorized administrator selects ACTIVE for a PENDING target with current version and valid reason/key
- **THEN** status becomes ACTIVE with user version incremented once and login becomes eligible under existing password/restricted rules

#### Scenario: Illegal transition invalid reason or self suspension
- **WHEN** input requests an illegal transition, reason outside bounds or suspension/deactivation of the verified actor
- **THEN** the agreed validation/business catalog error is returned without changing any user/session/assignment or success persistence

#### Scenario: Same status is requested with current or stale version
- **WHEN** a new valid keyed request selects the current target status
- **THEN** a matching version produces changed=false/one NO_OP audit/safe result without event or counter increment, while stale version returns VERSION_CONFLICT/ABORTED

### Requirement: Suspend and deactivate invalidate every credential in one status outcome

Effective suspension/deactivation SHALL increment target user version and
auth_version once, revoke all target sessions and refresh families, and persist
USER_STATUS_CHANGED audit plus exactly one pending `iam.user.status_changed.v1`
with sessions_revoked=true at the committed user version. The command SHALL NOT
emit duplicate per-session or separate global-revocation events. Historical
role/application assignments SHALL be retained. Access/Refresh/Login SHALL deny
the inactive account; latest revoked refresh SHALL NOT prove repeat Logout when
its immutable session generation no longer matches account auth_version.
Authority, locks, target/counter changes, revocation and keyed safe result SHALL
share one transaction, including rollback/independent failure audit policy.

#### Scenario: Target has several devices and an already logged-out session
- **WHEN** suspension/deactivation commits for a target with active and revoked sessions
- **THEN** all old access/refresh proofs fail, counters advance once, assignments remain and one status event records global invalidation without duplicate session events

#### Scenario: Login refresh or password mutation races status change
- **WHEN** a valid credential producer or mutation overlaps a status transition
- **THEN** shared user/session serialization and recheck prevent credentials from remaining usable after a later committed inactive status; failed competing requests do not commit partial writes

#### Scenario: Revocation or required persistence fails
- **WHEN** status/counter/session/family/audit/outbox/result persistence or a later business guard fails before commit
- **THEN** all tentative changes roll back; the independent failure audit preserves safe reason without converting the operation into success

### Requirement: Reactivation preserves historical access but never restores sessions

Effective activation/reactivation SHALL increment user version once without
decreasing or resetting auth_version. Existing revoked sessions/refresh tokens
SHALL stay revoked and no replacement credential SHALL be returned. Historical
role/access records SHALL not be silently reset; currently active eligible
assignments can contribute after status becomes ACTIVE. Safe detail SHALL let
the authorized administrator inspect applicable role/access metadata. An
eligible new Login SHALL create a new session bound to the current auth_version.

#### Scenario: Suspended or deactivated account becomes active
- **WHEN** authorized reactivation commits
- **THEN** old credentials remain invalid, eligible existing role/access contributions can apply again and a fresh login follows current password/restricted requirements

#### Scenario: Previously revoked access exists on reactivation
- **WHEN** the target had a revoked assignment or application access before becoming ACTIVE
- **THEN** reactivation does not regrant that record or recover an old session

### Requirement: Last active administrator survives concurrent management commands

Any status change that can remove an ACTIVE user with ACTIVE
iam.platform_admin assignment SHALL participate in the shared global last-admin
guard and current primary recheck. The service SHALL retain at least one such
administrator; otherwise LAST_ADMIN_REQUIRED SHALL return the agreed business
mapping with no successful effects. Guard/order SHALL be shared with subsequent
admin assignment revocation. Contention SHALL return RESOURCE_BUSY/ABORTED;
dependency failures SHALL NOT become safe-count assumptions or busy results.
Existing admin_guard singleton/constraints SHALL be respected without assigning
admin privilege to the first registrant or silently creating bootstrap users.

#### Scenario: Only one active administrator remains
- **WHEN** an authorized command would suspend/deactivate the sole eligible platform administrator
- **THEN** last-admin denial leaves status, counters, sessions, audit success/event/result unchanged

#### Scenario: Two requests remove different administrators
- **WHEN** controlled concurrent commands target different current administrators
- **THEN** global guard plus locked recount permits only outcomes that retain at least one ACTIVE assigned administrator, including busy/business denial where applicable

#### Scenario: Actor is invalidated before a delayed command rechecks
- **WHEN** one committed status command invalidates an actor waiting to mutate another target
- **THEN** the delayed command fails current actor recheck rather than relying on earlier authority or saved results

Evidence/traceability: [UM-09/10](../../changes/archive/2026-10-11-auth-user-management/design.md#acceptance-ledger), AC-02/14/15/19/20/24..26/31 and M-07..09/11. Status PASS; actual evidence pada [implementation ledger](../../changes/archive/2026-10-11-auth-user-management/evidence.md); owner grpc-auth; contract EXT-01 and dedicated opt-in PostgreSQL verification are required. Actual migration/bootstrap/deployment remain Phase 6 DEP-01.

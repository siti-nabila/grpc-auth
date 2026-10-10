# Spec Delta

## Purpose

Menyediakan perubahan password mandiri dengan bukti password saat ini, termasuk restricted user, serta invalidasi atomik seluruh credential lama sebelum pengguna login kembali.

## ADDED Requirements

### Requirement: Password change requires current self identity and current password proof

`/auth.v1.AuthService/ChangePassword` SHALL require a verified TLS service caller, valid active self access/session and current_password/new_password validated with existing password bounds 6–50. Restricted users SHALL be permitted without an IAM admin role. Identity SHALL come from the principal; profile/admin payloads SHALL NOT substitute ownership or modify passwords. Current password proof and credential/account state SHALL be rechecked under shared user/session serialization. Wrong current password SHALL use the semantic catalog mapping agreed in api-contracts, without globally remapping legacy password errors. Unknown/internal errors SHALL remain redacted and dependency errors SHALL NOT become password mismatch.

#### Scenario: Restricted owner changes password
- **WHEN** an ACTIVE restricted user proves their current password and submits a valid new password using a valid access/session
- **THEN** ChangePassword is permitted without administrative privilege

#### Scenario: Wrong password invalid input or cross-user substitution
- **WHEN** current-password proof fails, required password input is missing/out of bounds, or owner substitution is attempted
- **THEN** the corresponding safe catalog/validation/ownership error is returned without changing password, counters or sessions

#### Scenario: Password proof becomes stale
- **WHEN** another password/global invalidation commits before locked recheck
- **THEN** the stale proof/session cannot authorize the change and no password success effects commit

### Requirement: Password and global credential invalidation commit as one outcome

Successful ChangePassword SHALL atomically store a new Argon2id password hash, set password_changed_at, clear must_change_password, increment user version/auth_version once, revoke all user's sessions/refresh families, persist PASSWORD_CHANGED and USER_SESSIONS_REVOKED audits and one pending iam.user.sessions_revoked.v1 with reason_code PASSWORD_CHANGED at the committed user version. No per-session duplicate events, raw password/hash audit data, credential result cache or replacement credentials SHALL be created. Response SHALL report changed/login_required after commit; the user SHALL log in again. Session reactivation and refresh expiry extension SHALL remain forbidden.

#### Scenario: Password change commits
- **WHEN** current self proof and all required writes succeed
- **THEN** the new password is usable for a new Login, old access/refresh credentials on every device fail, restrictions are cleared and exact required audits plus one global pending event commit

#### Scenario: Required persistence fails
- **WHEN** password/counter/session/family/audit/outbox write or signing-independent transaction commit fails
- **THEN** no success response is returned and known precommit changes roll back together, with ambiguous commit outcome reported safely

#### Scenario: Password response is lost
- **WHEN** password change commits but the response is lost
- **THEN** no credential response is replayed and the old session cannot authorize a repeat change; the client must log in with the current password

Evidence/traceability: [SLC-05/08/09](../../design.md#acceptance-ledger), restricted subset AC-33, AC-34 and M-03/04/07/11. Backend status NOT_STARTED; published contract prerequisite BLOCKED. Admin CreateUser remains separate, so this increment does not close all AC-33.

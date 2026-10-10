# Spec Delta

## ADDED Requirements

### Requirement: Lifecycle RPCs use the agreed numeric admission groups

`/auth.v1.AuthService/RefreshSession` SHALL use trusted-IP 60/minute burst 10 and verified-session 10/minute burst 2 quotas with atomic consumption of applicable buckets. A supplied sid or unknown token SHALL NOT allocate a claimed session bucket. `/auth.v1.AuthService/ChangePassword` SHALL use verified actor/group 5/minute burst 2. `/session.v1.SessionService/ListMySessions` and `/session.v1.SessionService/ListUserSessions` SHALL use protected-read 120/minute burst 20, and RevokeUserSession/RevokeAllUserSessions SHALL use admin-mutation 30/minute burst 5 keyed by verified actor/group. Invalid input SHALL count through applicable trusted admission without account disclosure. Existing capacity 10,000, idle TTL 15 minutes, cleanup 1 minute, trusted-IP fallback and bounded RetryInfo/RESOURCE_EXHAUSTED behavior SHALL remain. Logout and self revoke operations SHALL not be assigned an invented numeric quota absent the agreed matrix.

#### Scenario: Refresh has valid identity but session quota is exhausted
- **WHEN** a proven refresh credential maps to an exhausted verified-session bucket while its trusted-IP bucket has quota
- **THEN** the request is rate rejected without consuming the IP bucket or performing rotation/reuse mutation

#### Scenario: Unproven refresh identity
- **WHEN** a method-allowed service submits a malformed or unknown refresh token with a claimed session ID
- **THEN** applicable trusted-IP admission counts the attempt but no claimed session bucket or business mutation is created

#### Scenario: Protected and admin session requests
- **WHEN** a verified actor exhausts the applicable read, admin mutation or password group
- **THEN** the request receives bounded RATE_LIMITED/RESOURCE_EXHAUSTED with RetryInfo and no business effects; available quota never bypasses current permission or restricted checks

Evidence/traceability: [SLC-08/09](../../design.md#acceptance-ledger), M-10/11. Status NOT_STARTED; lifecycle message prerequisite BLOCKED. Actual consumer IP sanitization/RetryInfo handling remains mandatory Phase 6 DEP-01, outside backend completion.

package types

type AuditAction string

const (
	AuditSessionCreated           AuditAction = "SESSION_CREATED"
	AuditSessionRefreshRotated    AuditAction = "SESSION_REFRESH_ROTATED"
	AuditRefreshReuseDetected     AuditAction = "REFRESH_REUSE_DETECTED"
	AuditSessionRevoked           AuditAction = "SESSION_REVOKED"
	AuditUserSessionsRevoked      AuditAction = "USER_SESSIONS_REVOKED"
	AuditPasswordChanged          AuditAction = "PASSWORD_CHANGED"
	AuditLoginFailed              AuditAction = "LOGIN_FAILED"
	AuditUserRegistered           AuditAction = "USER_REGISTERED"
	AuditUserCreated              AuditAction = "USER_CREATED"
	AuditUserRolesGranted         AuditAction = "USER_ROLES_GRANTED"
	AuditUserRolesRevoked         AuditAction = "USER_ROLES_REVOKED"
	AuditApplicationAccessGranted AuditAction = "APPLICATION_ACCESS_GRANTED"
	AuditApplicationAccessRevoked AuditAction = "APPLICATION_ACCESS_REVOKED"
	AuditUserStatusChanged        AuditAction = "USER_STATUS_CHANGED"
	AuditUserProfileUpdated       AuditAction = "USER_PROFILE_UPDATED"
	AuditRoleCreated              AuditAction = "ROLE_CREATED"
	AuditRoleUpdated              AuditAction = "ROLE_UPDATED"
	AuditRolePermissionsChanged   AuditAction = "ROLE_PERMISSIONS_CHANGED"
	AuditNoOp                     AuditAction = "NO_OP"
	AuditSecurityAttemptFailed    AuditAction = "SECURITY_ATTEMPT_FAILED"
)

type AuditTarget string

const (
	AuditTargetSession AuditTarget = "session"
	AuditTargetRPC     AuditTarget = "rpc"
	AuditTargetUser    AuditTarget = "user"
	AuditTargetRole    AuditTarget = "role"
)

type AuditReason string

const AuditCredentialDenied AuditReason = "CREDENTIAL_DENIED"
const AuditRequestDenied AuditReason = "REQUEST_DENIED"

const (
	AuditLogout                AuditReason = "LOGOUT"
	AuditSelfSessionRevoked    AuditReason = "SELF_SESSION_REVOKED"
	AuditAdminSessionRevoked   AuditReason = "ADMIN_SESSION_REVOKED"
	AuditSelfSessionsRevoked   AuditReason = "SELF_SESSIONS_REVOKED"
	AuditAdminSessionsRevoked  AuditReason = "ADMIN_SESSIONS_REVOKED"
	AuditRefreshReuse          AuditReason = "REFRESH_REUSE_DETECTED"
	AuditPasswordChange        AuditReason = "PASSWORD_CHANGED"
	AuditUserStatusChange      AuditReason = "USER_STATUS_CHANGED"
	AuditRoleStatusChange      AuditReason = "ROLE_STATUS_CHANGED"
	AuditRolePermissionsChange AuditReason = "ROLE_PERMISSIONS_CHANGED"
)

const (
	AuditRequestCanceled  AuditReason = "REQUEST_CANCELED"
	AuditDeadlineExceeded AuditReason = "DEADLINE_EXCEEDED"
	AuditValidationFailed AuditReason = "VALIDATION_FAILED"
	AuditInternalFailure  AuditReason = "INTERNAL_ERROR"
)

package types

type AuditAction string

const (
	AuditSessionCreated        AuditAction = "SESSION_CREATED"
	AuditLoginFailed           AuditAction = "LOGIN_FAILED"
	AuditUserRegistered        AuditAction = "USER_REGISTERED"
	AuditUserProfileUpdated    AuditAction = "USER_PROFILE_UPDATED"
	AuditNoOp                  AuditAction = "NO_OP"
	AuditSecurityAttemptFailed AuditAction = "SECURITY_ATTEMPT_FAILED"
)

type AuditTarget string

const (
	AuditTargetSession AuditTarget = "session"
	AuditTargetRPC     AuditTarget = "rpc"
	AuditTargetUser    AuditTarget = "user"
)

type AuditReason string

const AuditCredentialDenied AuditReason = "CREDENTIAL_DENIED"
const AuditRequestDenied AuditReason = "REQUEST_DENIED"

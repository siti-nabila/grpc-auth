package types

type UserStatus string

const (
	UserPending     UserStatus = "PENDING"
	UserActive      UserStatus = "ACTIVE"
	UserSuspended   UserStatus = "SUSPENDED"
	UserDeactivated UserStatus = "DEACTIVATED"
)

type SessionStatus string

const (
	SessionActive  SessionStatus = "ACTIVE"
	SessionRevoked SessionStatus = "REVOKED"
)

type RoleStatus string

const (
	RoleActive   RoleStatus = "ACTIVE"
	RoleDisabled RoleStatus = "DISABLED"
)

type ApplicationStatus string

const (
	ApplicationActive   ApplicationStatus = "ACTIVE"
	ApplicationDisabled ApplicationStatus = "DISABLED"
)

type AssignmentStatus string

const (
	AssignmentActive  AssignmentStatus = "ACTIVE"
	AssignmentRevoked AssignmentStatus = "REVOKED"
)

type ApplicationAccessStatus string

const (
	ApplicationAccessActive  ApplicationAccessStatus = "ACTIVE"
	ApplicationAccessRevoked ApplicationAccessStatus = "REVOKED"
)

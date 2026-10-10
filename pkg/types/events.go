package types

type EventType string

const (
	EventUserCreated                  EventType = "iam.user.created.v1"
	EventUserProfileUpdated           EventType = "iam.user.profile_updated.v1"
	EventUserStatusChanged            EventType = "iam.user.status_changed.v1"
	EventUserRolesGranted             EventType = "iam.user.roles_granted.v1"
	EventUserRolesRevoked             EventType = "iam.user.roles_revoked.v1"
	EventUserApplicationAccessGranted EventType = "iam.user.application_access_granted.v1"
	EventUserApplicationAccessRevoked EventType = "iam.user.application_access_revoked.v1"
	EventRoleCreated                  EventType = "iam.role.created.v1"
	EventRoleUpdated                  EventType = "iam.role.updated.v1"
	EventRolePermissionsChanged       EventType = "iam.role.permissions_changed.v1"
	EventSessionRevoked               EventType = "iam.session.revoked.v1"
	EventUserSessionsRevoked          EventType = "iam.user.sessions_revoked.v1"
)

type AggregateType string

const (
	AggregateUser    AggregateType = "user"
	AggregateRole    AggregateType = "role"
	AggregateSession AggregateType = "session"
)

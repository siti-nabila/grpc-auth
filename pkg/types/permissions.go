package types

type PermissionCode string

const PermissionIAMAuditRead PermissionCode = "iam.audit.read"

const PermissionIAMUserRead PermissionCode = "iam.user.read"

const (
	PermissionIAMUserApplicationGrant  PermissionCode = "iam.user.application.grant"
	PermissionIAMUserApplicationRevoke PermissionCode = "iam.user.application.revoke"
)

const (
	PermissionIAMRoleRead             PermissionCode = "iam.role.read"
	PermissionIAMRoleCreate           PermissionCode = "iam.role.create"
	PermissionIAMRoleUpdate           PermissionCode = "iam.role.update"
	PermissionIAMRolePermissionUpdate PermissionCode = "iam.role.permission.update"
	PermissionIAMPermissionRead       PermissionCode = "iam.permission.read"
)

const (
	PermissionIAMUserRoleGrant    PermissionCode = "iam.user.role.grant"
	PermissionIAMUserRoleRevoke   PermissionCode = "iam.user.role.revoke"
	PermissionIAMUserCreate       PermissionCode = "iam.user.create"
	PermissionIAMUserUpdate       PermissionCode = "iam.user.update"
	PermissionIAMUserStatusUpdate PermissionCode = "iam.user.status.update"
)

const (
	PermissionIAMSessionRead   PermissionCode = "iam.session.read"
	PermissionIAMSessionRevoke PermissionCode = "iam.session.revoke"
)

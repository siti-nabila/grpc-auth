package types

type PermissionCode string

const PermissionIAMUserRead PermissionCode = "iam.user.read"

const (
	PermissionIAMUserCreate       PermissionCode = "iam.user.create"
	PermissionIAMUserUpdate       PermissionCode = "iam.user.update"
	PermissionIAMUserStatusUpdate PermissionCode = "iam.user.status.update"
)

const (
	PermissionIAMSessionRead   PermissionCode = "iam.session.read"
	PermissionIAMSessionRevoke PermissionCode = "iam.session.revoke"
)
